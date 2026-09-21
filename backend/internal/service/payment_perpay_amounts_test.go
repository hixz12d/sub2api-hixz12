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

func TestPerPayOffsetCreditTerms(t *testing.T) {
	for _, tt := range []struct {
		name       string
		base, rate float64
		kind       string
		want       float64
	}{
		{"one yuan", 1, 1, payment.OrderTypeBalance, 1.02},
		{"multiplier", 7, 7, payment.OrderTypeBalance, 7.14},
		{"fractional multiplier", 0.2, 0.2, payment.OrderTypeBalance, 0.2},
		{"subscription unchanged", 1, 7, payment.OrderTypeSubscription, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := &dbent.PaymentOrder{Amount: tt.base, PayAmount: 1, OrderType: tt.kind, ProviderSnapshot: map[string]any{}}
			snapshotPerPayCredit(o.ProviderSnapshot, 1, tt.base, tt.rate)
			require.NoError(t, applyPerPayPayable(o, 102))
			require.Equal(t, tt.want, o.Amount)
			require.Equal(t, 1.02, o.PayAmount)
			require.NoError(t, applyPerPayPayable(o, 102))
			require.Equal(t, tt.want, o.Amount, "a retry must not add the offset again")
			require.Error(t, applyPerPayPayable(o, 103), "allocated amount must not change")
		})
	}
}

func TestPerPayRejectsUnboundAmounts(t *testing.T) {
	for _, tt := range []struct{ name, key, value string }{
		{"wrong requested", "requested_amount_cents", "101"},
		{"underpaid", "received_amount_cents", "101"},
		{"overpaid", "received_amount_cents", "103"},
		{"missing received", "received_amount_cents", ""},
		{"excessive offset", "payable_amount_cents", "200"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			o := &dbent.PaymentOrder{Amount: 1, PayAmount: 1, OrderType: payment.OrderTypeBalance, ProviderSnapshot: map[string]any{}}
			snapshotPerPayCredit(o.ProviderSnapshot, 1, 1, 1)
			meta := map[string]string{"requested_amount_cents": "100", "payable_amount_cents": "102", "received_amount_cents": "102"}
			meta[tt.key] = tt.value
			if tt.name == "excessive offset" {
				meta["received_amount_cents"] = "200"
			}
			_, err := preparePerPayConfirmation(o, 1, meta)
			require.Error(t, err)
			require.Equal(t, 1.0, o.Amount)
		})
	}
}

func TestPerPayCheckoutPreservesCompletedSettlement(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	order := createPaymentFulfillmentSubscriptionOrder(t, ctx, client, OrderStatusPending, time.Now())
	order.OrderType = payment.OrderTypeBalance
	order.ProviderSnapshot = map[string]any{}
	snapshotPerPayCredit(order.ProviderSnapshot, 80, 80, 1)
	require.NoError(t, client.PaymentOrder.UpdateOneID(order.ID).SetStatus(OrderStatusCompleted).SetAmount(80.02).SetPayAmount(80.02).SetProviderSnapshot(order.ProviderSnapshot).Exec(ctx))
	svc := &PaymentService{entClient: client}
	stored, err := svc.persistPerPayCheckoutAmount(ctx, order, 8001)
	require.NoError(t, err)
	require.Equal(t, OrderStatusCompleted, stored.Status)
	require.Equal(t, 80.02, stored.Amount)
	require.Equal(t, 80.02, stored.PayAmount)
}
