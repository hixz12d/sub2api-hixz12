package service

import (
	"context"
	"fmt"
	"strconv"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/paymentorder"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/shopspring/decimal"
)

// Persist the original amount and conversion rate before calling PerPay. Never
// read the current site multiplier when settling an already-created order.
func snapshotPerPayCredit(snapshot map[string]any, requested, baseCredit, multiplier float64) {
	cents, _ := payment.YuanToFen(strconv.FormatFloat(requested, 'f', 2, 64))
	snapshot["perpay_requested_amount_cents"] = strconv.FormatInt(cents, 10)
	snapshot["perpay_base_credit"] = strconv.FormatFloat(baseCredit, 'f', 2, 64)
	snapshot["perpay_credit_multiplier"] = strconv.FormatFloat(normalizeBalanceRechargeMultiplier(multiplier), 'f', -1, 64)
}

// The offset is not a fee: balance orders receive the offset at the order's
// snapshotted conversion rate. Subscription entitlements remain unchanged.
func applyPerPayPayable(order *dbent.PaymentOrder, payable int64) error {
	snapshot := make(map[string]any, len(order.ProviderSnapshot)+3)
	for k, v := range order.ProviderSnapshot {
		snapshot[k] = v
	}
	if psSnapshotStringValue(snapshot["perpay_requested_amount_cents"]) == "" {
		// Compatibility for orders opened before offset credit was introduced.
		if !isValidProviderAmount(order.PayAmount) || !isValidProviderAmount(order.Amount) {
			return fmt.Errorf("perpay invalid legacy amounts")
		}
		snapshotPerPayCredit(snapshot, order.PayAmount, order.Amount, order.Amount/order.PayAmount)
	}
	requested, err := strconv.ParseInt(psSnapshotStringValue(snapshot["perpay_requested_amount_cents"]), 10, 64)
	if err != nil || requested < 1 || payable <= requested || payable-requested > 99 || payable > 9999999999 {
		return fmt.Errorf("perpay invalid matching offset")
	}
	expected, err := payment.YuanToFen(strconv.FormatFloat(order.PayAmount, 'f', 2, 64))
	if err != nil || (expected != requested && expected != payable) {
		return fmt.Errorf("perpay payable amount changed")
	}
	if order.OrderType == payment.OrderTypeBalance {
		base, e1 := decimal.NewFromString(psSnapshotStringValue(snapshot["perpay_base_credit"]))
		rate, e2 := decimal.NewFromString(psSnapshotStringValue(snapshot["perpay_credit_multiplier"]))
		if e1 != nil || e2 != nil || !base.IsPositive() || !rate.IsPositive() {
			return fmt.Errorf("perpay invalid credit terms")
		}
		order.Amount = base.Add(decimal.NewFromInt(payable - requested).Div(decimal.NewFromInt(100)).Mul(rate)).Round(2).InexactFloat64()
	}
	order.PayAmount = payment.FenToYuan(payable)
	order.ProviderSnapshot = snapshot
	return nil
}

func preparePerPayConfirmation(order *dbent.PaymentOrder, requestedAmount float64, metadata map[string]string) (float64, error) {
	requested, e1 := strconv.ParseInt(metadata["requested_amount_cents"], 10, 64)
	payable, e2 := strconv.ParseInt(metadata["payable_amount_cents"], 10, 64)
	received, e3 := strconv.ParseInt(metadata["received_amount_cents"], 10, 64)
	notified, e4 := payment.YuanToFen(strconv.FormatFloat(requestedAmount, 'f', 2, 64))
	expected := psSnapshotStringValue(order.ProviderSnapshot["perpay_requested_amount_cents"])
	if expected == "" {
		cents, err := payment.YuanToFen(strconv.FormatFloat(order.PayAmount, 'f', 2, 64))
		if err != nil {
			return 0, err
		}
		expected = strconv.FormatInt(cents, 10)
	}
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil || requested != notified || expected != strconv.FormatInt(requested, 10) || received != payable {
		return 0, fmt.Errorf("perpay requested or received amount mismatch")
	}
	if err := applyPerPayPayable(order, payable); err != nil {
		return 0, err
	}
	return order.PayAmount, nil
}

func (s *PaymentService) persistPerPayCheckoutAmount(ctx context.Context, order *dbent.PaymentOrder, payable int64) (*dbent.PaymentOrder, error) {
	if err := applyPerPayPayable(order, payable); err != nil {
		return nil, err
	}
	// A fast callback may already have fulfilled the order. Never overwrite its
	// amount or settlement snapshot after it leaves PENDING.
	_, err := s.entClient.PaymentOrder.Update().Where(paymentorder.IDEQ(order.ID), paymentorder.StatusEQ(OrderStatusPending)).
		SetAmount(order.Amount).SetPayAmount(order.PayAmount).SetProviderSnapshot(order.ProviderSnapshot).Save(ctx)
	if err != nil {
		return nil, err
	}
	return s.entClient.PaymentOrder.Get(ctx, order.ID)
}
