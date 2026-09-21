package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/gin-gonic/gin"
)

// PerPayNotify acknowledges only a verified event whose current upstream state
// was reconciled successfully. An exact JSON ACK is required by PerPay.
func (h *PaymentWebhookHandler) PerPayNotify(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxWebhookBodySize+1))
	if err != nil || len(body) > maxWebhookBodySize {
		c.Status(http.StatusBadRequest)
		return
	}
	var hint struct {
		MerchantNo string `json:"merchant_order_no"`
	}
	if json.Unmarshal(body, &hint) != nil || hint.MerchantNo == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	providers, err := h.paymentService.GetWebhookProviders(c.Request.Context(), payment.TypePerPay, hint.MerchantNo)
	if err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	headers := make(map[string]string)
	for k := range c.Request.Header {
		headers[strings.ToLower(k)] = c.GetHeader(k)
	}
	resolvedKey, notification, err := verifyNotificationWithProviders(c.Request.Context(), providers, string(body), headers)
	// Verification also queries PerPay. Use a retryable status for temporary
	// network errors instead of permanently dead-lettering legitimate callbacks.
	if err != nil || notification == nil || resolvedKey != payment.TypePerPay {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	if err := h.paymentService.HandlePaymentNotification(c.Request.Context(), notification, payment.TypePerPay); err != nil {
		c.Status(http.StatusServiceUnavailable)
		return
	}
	c.JSON(http.StatusOK, gin.H{"schema": "perpay:webhook-ack:v1", "ack": true, "event_id": headers["x-perpay-webhook-event-id"], "delivery_id": headers["x-perpay-webhook-delivery-id"]})
}
