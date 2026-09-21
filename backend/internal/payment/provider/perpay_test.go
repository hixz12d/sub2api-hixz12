package provider

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func perPayTestConfig() map[string]string {
	secret := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("s", 32)))
	return map[string]string{"apiBase": "https://pay.example.com", "notifyUrl": "https://shop.example.com/api/v1/payment/webhook/perpay", "returnUrl": "https://shop.example.com/payment/result", "apiSecret": secret, "webhookSecret": secret}
}
func perPayTestOrder() perPayOrder {
	o := perPayOrder{ID: "order-1", MerchantOrderNo: "sub2_test", Requested: 1000, Payable: 1001, Currency: "CNY", Version: 2}
	o.Checkout.Status = "OPEN"
	o.Checkout.ExpiresAt = time.Now().Add(5 * time.Minute)
	o.Payment.Status = "UNPAID"
	o.Refund.Status = "NONE"
	return o
}
func perPayTestServer(t *testing.T, o *perPayOrder, check func(*http.Request, []byte)) *PerPay {
	t.Helper()
	p, err := NewPerPay("1", perPayTestConfig())
	require.NoError(t, err)
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		digest := sha256.Sum256(body)
		text := strings.Join([]string{"PERPAY-HMAC-SHA256", "v1", r.Method, r.URL.RequestURI(), r.Header.Get("X-PerPay-Timestamp"), r.Header.Get("X-PerPay-Nonce"), "default", hex.EncodeToString(digest[:])}, "\n")
		mac := hmac.New(sha256.New, p.apiSecret)
		mac.Write([]byte(text))
		require.Equal(t, hex.EncodeToString(mac.Sum(nil)), r.Header.Get("X-PerPay-Signature"))
		nonce, err := base64.RawURLEncoding.DecodeString(r.Header.Get("X-PerPay-Nonce"))
		require.NoError(t, err)
		require.Len(t, nonce, 32)
		if check != nil {
			check(r, body)
		}
		copyOrder := *o
		copyOrder.Checkout.URL = server.URL + "/checkout/token"
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"data": copyOrder}))
	}))
	t.Cleanup(server.Close)
	p.apiBase = server.URL
	p.client = server.Client()
	return p
}
func perPaySignedWebhook(p *PerPay, raw string, at time.Time) map[string]string {
	h := map[string]string{"x-perpay-webhook-version": "1", "x-perpay-webhook-key-id": "key-1", "x-perpay-webhook-timestamp": strconv.FormatInt(at.UnixMilli(), 10), "x-perpay-webhook-delivery-id": "delivery-1", "x-perpay-webhook-event-id": "event-1", "x-perpay-webhook-attempt": "1"}
	digest := sha256.Sum256([]byte(raw))
	text := strings.Join([]string{"perpay:webhook:v1", "key-1", h["x-perpay-webhook-timestamp"], "delivery-1", "event-1", "1", hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, p.webhookSecret)
	mac.Write([]byte(text))
	h["x-perpay-webhook-signature"] = "v1=" + hex.EncodeToString(mac.Sum(nil))
	return h
}

const perPayEvent = `{"schema":"perpay:outbox-event:v2","event_id":"event-1","event_type":"PAYMENT_CONFIRMED","order_id":"order-1","merchant_order_no":"sub2_test","order_version":2}`

func TestPerPayCreateAndQuery(t *testing.T) {
	o := perPayTestOrder()
	var nonces []string
	p := perPayTestServer(t, &o, func(r *http.Request, b []byte) {
		nonces = append(nonces, r.Header.Get("X-PerPay-Nonce"))
		if r.Method == "POST" {
			var req map[string]any
			require.NoError(t, json.Unmarshal(b, &req))
			require.Equal(t, float64(1000), req["amount_cents"])
			require.Equal(t, "sub2api-sub2_test", req["idempotency_key"])
			require.Equal(t, "https://shop.example.com/payment/result?resume_token=abc", req["return_url"])
		} else {
			require.Equal(t, "/api/v1/orders/by-merchant-no/sub2_test", r.URL.Path)
		}
	})
	req := payment.CreatePaymentRequest{OrderID: o.MerchantOrderNo, Amount: "10.00", PaymentType: "alipay", Subject: "Recharge", ReturnURL: "https://shop.example.com/payment/result?resume_token=abc"}
	for i := 0; i < 2; i++ {
		result, err := p.CreatePayment(context.Background(), req)
		require.NoError(t, err)
		require.Contains(t, result.PayURL, "/checkout/")
		require.Empty(t, result.QRCode)
		require.Equal(t, o.Checkout.ExpiresAt.Unix(), result.ExpiresAt.Unix())
	}
	require.NotEqual(t, nonces[0], nonces[1])
	o.Payment.Status = "CONFIRMED"
	o.Received = &o.Payable
	o.Payment.Received = &o.Payable
	result, err := p.QueryOrder(context.Background(), o.MerchantOrderNo)
	require.NoError(t, err)
	require.Equal(t, payment.ProviderStatusPaid, result.Status)
	require.Equal(t, 10.0, result.Amount)
	require.Equal(t, "1001", result.Metadata["received_amount_cents"])
}
func TestPerPayWebhookRejectsTamperReplayAndMismatch(t *testing.T) {
	for _, tc := range []string{"valid", "duplicate", "tampered", "expired", "header_event", "wrong_order", "future_version", "underpaid", "refunded", "stale_confirmation"} {
		t.Run(tc, func(t *testing.T) {
			o := perPayTestOrder()
			o.Payment.Status = "CONFIRMED"
			o.Received = &o.Payable
			o.Payment.Received = &o.Payable
			calls := 0
			p := perPayTestServer(t, &o, func(_ *http.Request, _ []byte) { calls++ })
			raw := perPayEvent
			h := perPaySignedWebhook(p, raw, time.Now())
			switch tc {
			case "tampered":
				raw += " "
			case "expired":
				h = perPaySignedWebhook(p, raw, time.Now().Add(-6*time.Minute))
			case "header_event":
				h["x-perpay-webhook-event-id"] = "other"
			case "wrong_order":
				o.ID = "other"
			case "future_version":
				o.Version = 1
			case "underpaid":
				v := int64(1000)
				o.Received = &v
			case "refunded":
				o.Refund.Status = "FULL"
			case "stale_confirmation":
				o.Version = 3
				o.Payment.Status = "DISPUTED"
			}
			result, err := p.VerifyNotification(context.Background(), raw, h)
			switch tc {
			case "valid", "duplicate":
				require.NoError(t, err)
				require.Equal(t, "success", result.Status)
				if tc == "duplicate" {
					again, e := p.VerifyNotification(context.Background(), raw, h)
					require.NoError(t, e)
					require.Equal(t, result, again)
				}
			case "stale_confirmation":
				require.NoError(t, err)
				require.Equal(t, "disputed", result.Status)
			default:
				require.Error(t, err)
			}
			if tc == "tampered" || tc == "expired" || tc == "header_event" {
				require.Zero(t, calls)
			}
		})
	}
}
func TestPerPayConfigurationAndRefund(t *testing.T) {
	for _, field := range []string{"apiBase", "apiSecret", "webhookSecret", "notifyUrl", "returnUrl"} {
		cfg := perPayTestConfig()
		cfg[field] = "invalid"
		_, err := NewPerPay("1", cfg)
		require.Error(t, err)
	}
	p, err := NewPerPay("1", perPayTestConfig())
	require.NoError(t, err)
	_, err = p.Refund(context.Background(), payment.RefundRequest{})
	require.Error(t, err)
}
