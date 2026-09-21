package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// PerPay uses a static collection code. The independently verified payable
// amount includes the matching offset; balance orders credit that offset too.
// Never expose the raw collection code: the hosted checkout explains that offset.
type PerPay struct {
	apiBase, notifyURL, returnURL string
	apiSecret, webhookSecret      []byte
	client                        *http.Client
}

func NewPerPay(_ string, cfg map[string]string) (*PerPay, error) {
	base, err := perPayHTTPSURL(cfg["apiBase"])
	if err != nil || (base != nil && base.Path != "" && base.Path != "/") {
		return nil, fmt.Errorf("perpay apiBase must be an HTTPS origin")
	}
	notify, err := perPayHTTPSURL(cfg["notifyUrl"])
	if err != nil {
		return nil, fmt.Errorf("perpay notifyUrl must be an HTTPS URL")
	}
	ret, err := perPayHTTPSURL(cfg["returnUrl"])
	if err != nil || !strings.EqualFold(ret.Host, notify.Host) {
		return nil, fmt.Errorf("perpay returnUrl must use the notification origin")
	}
	apiSecret, err := perPaySecret(cfg["apiSecret"])
	if err != nil {
		return nil, fmt.Errorf("perpay apiSecret must be a 32-byte base64url secret")
	}
	webhookSecret, err := perPaySecret(cfg["webhookSecret"])
	if err != nil {
		return nil, fmt.Errorf("perpay webhookSecret must be a 32-byte base64url secret")
	}
	return &PerPay{apiBase: strings.TrimRight(base.String(), "/"), notifyURL: notify.String(), returnURL: ret.String(), apiSecret: apiSecret, webhookSecret: webhookSecret,
		client: &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func perPayHTTPSURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, fmt.Errorf("invalid HTTPS URL")
	}
	return u, nil
}
func perPaySecret(raw string) ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil || len(b) != 32 {
		return nil, fmt.Errorf("invalid secret")
	}
	return b, nil
}
func (p *PerPay) Name() string        { return "PerPay" }
func (p *PerPay) ProviderKey() string { return payment.TypePerPay }
func (p *PerPay) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeAlipay}
}
func (p *PerPay) MerchantIdentityMetadata() map[string]string {
	return map[string]string{"perpay_origin": p.apiBase, "currency": "CNY"}
}

type perPayOrder struct {
	ID              string `json:"order_id"`
	MerchantOrderNo string `json:"merchant_order_no"`
	Requested       int64  `json:"requested_amount_cents"`
	Payable         int64  `json:"payable_amount_cents"`
	Received        *int64 `json:"received_amount_cents"`
	Currency        string `json:"currency"`
	Version         int64  `json:"version"`
	Checkout        struct {
		Status    string    `json:"status"`
		URL       string    `json:"checkout_url"`
		ExpiresAt time.Time `json:"expires_at"`
	} `json:"checkout"`
	Payment struct {
		Status   string `json:"status"`
		Received *int64 `json:"received_amount_cents"`
	} `json:"payment"`
	Refund struct {
		Status string `json:"status"`
	} `json:"refund"`
}

func (p *PerPay) request(ctx context.Context, method, target string, data any) (*perPayOrder, error) {
	var body []byte
	var err error
	if data != nil {
		body, err = json.Marshal(data)
		if err != nil {
			return nil, err
		}
	}
	nonceBytes := make([]byte, 32)
	if _, err = rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := base64.RawURLEncoding.EncodeToString(nonceBytes)
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	digest := sha256.Sum256(body)
	text := strings.Join([]string{"PERPAY-HMAC-SHA256", "v1", method, target, ts, nonce, "default", hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, p.apiSecret)
	mac.Write([]byte(text))
	req, err := http.NewRequestWithContext(ctx, method, p.apiBase+target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range map[string]string{"Client-Id": "default", "Timestamp": ts, "Nonce": nonce, "Signature-Version": "v1", "Signature": hex.EncodeToString(mac.Sum(nil))} {
		req.Header.Set("X-PerPay-"+k, v)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("perpay request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("perpay request returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Data *perPayOrder `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil || envelope.Data == nil {
		return nil, fmt.Errorf("perpay invalid order response")
	}
	o := envelope.Data
	if o.ID == "" || o.MerchantOrderNo == "" || o.Currency != "CNY" || o.Requested < 1 || o.Requested > 9999999998 || o.Payable <= o.Requested || o.Payable-o.Requested > 99 || o.Payable > 9999999999 || o.Version < 1 {
		return nil, fmt.Errorf("perpay invalid order identity or amounts")
	}
	return o, nil
}

func (p *PerPay) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	if req.PaymentType != payment.TypeAlipay {
		return nil, fmt.Errorf("perpay only supports alipay")
	}
	cents, err := payment.YuanToFen(req.Amount)
	if err != nil || cents < 1 || cents > 9999999998 {
		return nil, fmt.Errorf("perpay invalid amount")
	}
	returnURL := p.returnURL
	// Keep Sub2API's signed recovery token, but use only the configured origin.
	if candidate, e := url.Parse(req.ReturnURL); e == nil && candidate.Scheme == "https" && candidate.User == nil && candidate.Fragment == "" {
		configured, _ := url.Parse(p.returnURL)
		if strings.EqualFold(candidate.Host, configured.Host) {
			returnURL = candidate.String()
		}
	}
	data := map[string]any{"idempotency_key": "sub2api-" + req.OrderID, "merchant_order_no": req.OrderID, "amount_cents": cents, "product_name": req.Subject, "notify_url": p.notifyURL, "return_url": returnURL}
	o, err := p.request(ctx, http.MethodPost, "/api/v1/orders", data)
	if err != nil {
		return nil, err
	}
	if o.MerchantOrderNo != req.OrderID || o.Requested != cents {
		return nil, fmt.Errorf("perpay created order does not match request")
	}
	checkout, err := url.Parse(o.Checkout.URL)
	base, _ := url.Parse(p.apiBase)
	if err != nil || checkout.Scheme != "https" || !strings.EqualFold(checkout.Host, base.Host) || checkout.User != nil || checkout.Fragment != "" || !o.Checkout.ExpiresAt.After(time.Now()) {
		return nil, fmt.Errorf("perpay invalid checkout")
	}
	return &payment.CreatePaymentResponse{TradeNo: o.ID, PayURL: o.Checkout.URL, Currency: "CNY", ExpiresAt: o.Checkout.ExpiresAt, PayableAmountCents: o.Payable}, nil
}

// Query by merchant number, including when an initial create response was lost.
func (p *PerPay) order(ctx context.Context, merchantNo string) (*perPayOrder, error) {
	if merchantNo == "" {
		return nil, fmt.Errorf("perpay merchant order number is required")
	}
	o, err := p.request(ctx, http.MethodGet, "/api/v1/orders/by-merchant-no/"+url.PathEscape(merchantNo), nil)
	if err == nil && o.MerchantOrderNo != merchantNo {
		return nil, fmt.Errorf("perpay merchant order mismatch")
	}
	return o, err
}
func (p *PerPay) result(o *perPayOrder) (*payment.QueryOrderResponse, error) {
	status := payment.ProviderStatusPending
	switch o.Payment.Status {
	case "CONFIRMED":
		if o.Received == nil || o.Payment.Received == nil || *o.Received != o.Payable || *o.Payment.Received != o.Payable || o.Refund.Status != "NONE" {
			return nil, fmt.Errorf("perpay confirmed amount or refund mismatch")
		}
		status = payment.ProviderStatusPaid
	case "DISPUTED":
		status = "disputed"
	case "UNPAID":
		if o.Checkout.Status == "CLOSED" || o.Checkout.Status == "EXPIRED" {
			status = payment.ProviderStatusFailed
		}
	default:
		return nil, fmt.Errorf("perpay unknown payment state")
	}
	metadata := p.MerchantIdentityMetadata()
	metadata["merchant_order_no"] = o.MerchantOrderNo
	metadata["perpay_order_id"] = o.ID
	metadata["order_version"] = strconv.FormatInt(o.Version, 10)
	metadata["requested_amount_cents"] = strconv.FormatInt(o.Requested, 10)
	metadata["payable_amount_cents"] = strconv.FormatInt(o.Payable, 10)
	if o.Received != nil {
		metadata["received_amount_cents"] = strconv.FormatInt(*o.Received, 10)
	}
	// Keep the requested amount for binding to legacy orders. The service checks
	// all three amounts and credits the verified offset using stored order terms.
	return &payment.QueryOrderResponse{TradeNo: o.ID, Status: status, Amount: payment.FenToYuan(o.Requested), Metadata: metadata}, nil
}
func (p *PerPay) QueryOrder(ctx context.Context, merchantNo string) (*payment.QueryOrderResponse, error) {
	o, err := p.order(ctx, merchantNo)
	if err != nil {
		return nil, err
	}
	return p.result(o)
}
func (p *PerPay) CancelPayment(ctx context.Context, merchantNo string) error {
	o, err := p.order(ctx, merchantNo)
	if err != nil {
		return err
	}
	if o.Checkout.Status != "OPEN" || o.Payment.Status != "UNPAID" {
		return nil
	}
	_, err = p.request(ctx, http.MethodPost, "/api/v1/orders/"+url.PathEscape(o.ID)+"/actions/close", nil)
	return err
}
func (p *PerPay) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("perpay does not execute refunds; refund externally and reconcile manually")
}

func (p *PerPay) VerifyNotification(ctx context.Context, raw string, headers map[string]string) (*payment.PaymentNotification, error) {
	h := func(key string) string { return headers["x-perpay-webhook-"+key] }
	for _, key := range []string{"version", "key-id", "timestamp", "delivery-id", "event-id", "attempt", "signature"} {
		if h(key) == "" {
			return nil, fmt.Errorf("perpay missing webhook header")
		}
	}
	ts, err := strconv.ParseInt(h("timestamp"), 10, 64)
	now := time.Now().UnixMilli()
	if err != nil || ts < now-300000 || ts > now+300000 || h("version") != "1" {
		return nil, fmt.Errorf("perpay invalid webhook timestamp or version")
	}
	digest := sha256.Sum256([]byte(raw))
	text := strings.Join([]string{"perpay:webhook:v1", h("key-id"), h("timestamp"), h("delivery-id"), h("event-id"), h("attempt"), hex.EncodeToString(digest[:])}, "\n")
	mac := hmac.New(sha256.New, p.webhookSecret)
	mac.Write([]byte(text))
	expected := "v1=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(h("signature"))) {
		return nil, fmt.Errorf("perpay invalid webhook signature")
	}
	var e struct {
		Schema     string `json:"schema"`
		ID         string `json:"event_id"`
		Type       string `json:"event_type"`
		OrderID    string `json:"order_id"`
		MerchantNo string `json:"merchant_order_no"`
		Version    int64  `json:"order_version"`
	}
	if json.Unmarshal([]byte(raw), &e) != nil || e.ID != h("event-id") || e.MerchantNo == "" || e.OrderID == "" || e.Version < 1 || (e.Schema != "perpay:outbox-event:v2" && e.Schema != "perpay:outbox-event:v1") {
		return nil, fmt.Errorf("perpay invalid webhook event")
	}
	switch e.Type {
	case "PAYMENT_CONFIRMED", "PAYMENT_DISPUTED", "REFUND_UPDATED":
	default:
		return nil, fmt.Errorf("perpay unsupported event")
	}
	// Read current authenticated state instead of trusting a possibly stale event.
	// Replayed confirmations cannot roll a disputed order back to paid.
	o, err := p.order(ctx, e.MerchantNo)
	if err != nil {
		return nil, err
	}
	if o.ID != e.OrderID || o.Version < e.Version {
		return nil, fmt.Errorf("perpay webhook order identity or version mismatch")
	}
	r, err := p.result(o)
	if err != nil {
		return nil, err
	}
	r.Metadata["event_id"] = e.ID
	status := r.Status
	if status == payment.ProviderStatusPaid {
		status = payment.NotificationStatusSuccess
	}
	return &payment.PaymentNotification{TradeNo: r.TradeNo, OrderID: e.MerchantNo, Amount: r.Amount, Status: status, RawData: raw, Metadata: r.Metadata}, nil
}
