package service

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent/paymentproviderinstance"
	"github.com/Wei-Shaw/sub2api/internal/payment"
)

// purchaseEntryAvailable gates new purchases only. Existing orders continue to
// use their provider snapshots for queries, callbacks and reconciliation.
func (s *PaymentConfigService) purchaseEntryAvailable(ctx context.Context, cfg *PaymentConfig) (bool, error) {
	if !cfg.Enabled || !cfg.PurchaseEntryEnabled {
		return false, nil
	}
	if !cfg.PurchaseEntryPerPayLinked {
		return true, nil
	}
	if s.entClient == nil {
		return false, nil
	}
	enabled, err := s.entClient.PaymentProviderInstance.Query().Where(
		paymentproviderinstance.ProviderKeyEQ(payment.TypePerPay),
		paymentproviderinstance.EnabledEQ(true),
	).Exist(ctx)
	if err != nil {
		return false, fmt.Errorf("check PerPay purchase entry: %w", err)
	}
	return enabled, nil
}
