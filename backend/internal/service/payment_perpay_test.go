//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"
)

func TestPerPayRoutingAndSecrets(t *testing.T) {
	require.Equal(t, VisibleMethodSourcePerPayAlipay, NormalizeVisibleMethodSource("alipay", "perpay"))
	key, ok := VisibleMethodProviderKeyForSource("alipay", "perpay_alipay")
	require.True(t, ok)
	require.Equal(t, payment.TypePerPay, key)
	_, ok = VisibleMethodProviderKeyForSource("wxpay", "perpay_alipay")
	require.False(t, ok)
	inst := &dbent.PaymentProviderInstance{ProviderKey: payment.TypePerPay, SupportedTypes: "alipay", Enabled: true}
	require.True(t, providerSupportsVisibleMethod(inst, "alipay"))
	require.False(t, providerSupportsVisibleMethod(inst, "wxpay"))
	require.True(t, buildVisibleMethodSourceAvailability([]*dbent.PaymentProviderInstance{inst})[VisibleMethodSourcePerPayAlipay])
	svc := &PaymentConfigService{}
	masked, err := svc.decryptAndMaskConfig(payment.TypePerPay, `{"apiSecret":"secret","webhookSecret":"secret2","apiBase":"https://pay.example.com"}`)
	require.NoError(t, err)
	require.NotContains(t, masked, "apiSecret")
	require.NotContains(t, masked, "webhookSecret")
	require.True(t, hasPendingOrderProtectedConfigChange(payment.TypePerPay, map[string]string{"apiBase": "old"}, map[string]string{"apiBase": "new"}))
}

func TestPerPayNotificationCreditsOnceAndAuditsDispute(t *testing.T) {
	for _, mode := range []string{"legacy", "checkout already persisted"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			client := newPaymentConfigServiceTestClient(t)
			ensurePaymentAuditOrderActionUniqueIndex(t, ctx, client)
			order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
			order, err := client.PaymentOrder.UpdateOneID(order.ID).SetOrderType(payment.OrderTypeBalance).ClearPlanID().ClearSubscriptionGroupID().ClearSubscriptionDays().SetProviderKey(payment.TypePerPay).SetPaymentTradeNo("perpay-order").SetProviderSnapshot(map[string]any{"schema_version": 2, "provider_key": payment.TypePerPay, "perpay_origin": "https://pay.example.com", "currency": "CNY"}).Save(ctx)
			require.NoError(t, err)
			repo := &paymentFulfillmentRedeemRepo{}
			credited := 0.0
			userRepo := &mockUserRepo{getByIDUser: &User{ID: order.UserID}}
			userRepo.updateBalanceFn = func(_ context.Context, _ int64, amount float64) error { credited += amount; return nil }
			cache := &paymentFulfillmentRedeemCacheStub{}
			svc := &PaymentService{entClient: client, userRepo: userRepo, redeemService: NewRedeemService(repo, userRepo, nil, cache, nil, client, nil, nil)}
			if mode != "legacy" {
				checkoutCopy := *order
				updated, err := svc.persistPerPayCheckoutAmount(ctx, &checkoutCopy, 8002)
				require.NoError(t, err)
				require.Equal(t, 80.02, updated.Amount)
				require.Equal(t, 80.02, updated.PayAmount)
			}
			meta := map[string]string{"perpay_origin": "https://pay.example.com", "currency": "CNY", "merchant_order_no": order.OutTradeNo, "perpay_order_id": "perpay-order", "event_id": "event-1", "order_version": "2", "requested_amount_cents": "8000", "payable_amount_cents": "8002", "received_amount_cents": "8002"}
			n := &payment.PaymentNotification{TradeNo: "perpay-order", OrderID: order.OutTradeNo, Amount: order.PayAmount, Status: payment.NotificationStatusSuccess, Metadata: meta}
			require.NoError(t, svc.HandlePaymentNotification(ctx, n, payment.TypePerPay))
			require.NoError(t, svc.HandlePaymentNotification(ctx, n, payment.TypePerPay))
			require.Equal(t, order.Amount+0.02, credited)
			require.Len(t, repo.useCalls, 1)
			stored, err := client.PaymentOrder.Get(ctx, order.ID)
			require.NoError(t, err)
			require.Equal(t, 80.02, stored.PayAmount)
			require.Equal(t, 80.02, stored.Amount)
			require.Equal(t, "8000", stored.ProviderSnapshot["perpay_requested_amount_cents"])
			n.Status = "disputed"
			meta["order_version"] = "3"
			require.NoError(t, svc.HandlePaymentNotification(ctx, n, payment.TypePerPay))
			require.Equal(t, order.Amount+0.02, credited, "dispute must not trigger a second credit or an unapproved balance debit")
			n.Status = payment.NotificationStatusSuccess
			meta["merchant_order_no"] = "wrong-order"
			require.Error(t, svc.HandlePaymentNotification(ctx, n, payment.TypePerPay))
			require.Equal(t, order.Amount+0.02, credited)
		})
	}
}
