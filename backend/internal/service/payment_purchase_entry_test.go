package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/payment"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPurchaseEntryDefaultsAndPartialUpdates(t *testing.T) {
	ctx := context.Background()
	repo := &paymentConfigSettingRepoStub{values: map[string]string{SettingPaymentEnabled: "true"}}
	svc := &PaymentConfigService{settingRepo: repo}
	cfg, err := svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.True(t, cfg.PurchaseEntryEnabled)
	require.False(t, cfg.PurchaseEntryPerPayLinked)
	require.True(t, cfg.PurchaseEntryAvailable)

	closed, linked := false, true
	require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{
		PurchaseEntryEnabled: &closed, PurchaseEntryPerPayLinked: &linked,
	}))
	require.Equal(t, "false", repo.values[SettingPurchaseEntryEnabled])
	require.Equal(t, "true", repo.values[SettingPurchaseEntryPerPayLinked])
	prefix := "Updated"
	require.NoError(t, svc.UpdatePaymentConfig(ctx, UpdatePaymentConfigRequest{ProductNamePrefix: &prefix}))
	require.NotContains(t, repo.updates, SettingPurchaseEntryEnabled)
	require.NotContains(t, repo.updates, SettingPurchaseEntryPerPayLinked)
	cfg, err = svc.GetPaymentConfig(ctx)
	require.NoError(t, err)
	require.False(t, cfg.PurchaseEntryAvailable)
	require.True(t, cfg.PurchaseEntryPerPayLinked)
}

func TestPurchaseEntryFollowsEnabledPerPayProviders(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	repo := &paymentConfigSettingRepoStub{values: map[string]string{
		SettingPaymentEnabled: "true", SettingPurchaseEntryPerPayLinked: "true",
	}}
	svc := &PaymentConfigService{entClient: client, settingRepo: repo}
	check := func(want bool) {
		t.Helper()
		cfg, err := svc.GetPaymentConfig(ctx)
		require.NoError(t, err)
		require.Equal(t, want, cfg.PurchaseEntryAvailable)
	}
	check(false)
	_, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypeAlipay).
		SetName("Official Alipay").SetConfig("{}").SetSupportedTypes("alipay").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	check(false)
	first, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypePerPay).
		SetName("PerPay first").SetConfig("{}").SetSupportedTypes("alipay").SetEnabled(false).Save(ctx)
	require.NoError(t, err)
	check(false)
	require.NoError(t, client.PaymentProviderInstance.UpdateOne(first).SetEnabled(true).Exec(ctx))
	check(true)
	second, err := client.PaymentProviderInstance.Create().SetProviderKey(payment.TypePerPay).
		SetName("PerPay second").SetConfig("{}").SetSupportedTypes("alipay").SetEnabled(true).Save(ctx)
	require.NoError(t, err)
	require.NoError(t, client.PaymentProviderInstance.UpdateOne(first).SetEnabled(false).Exec(ctx))
	check(true)
	require.NoError(t, client.PaymentProviderInstance.DeleteOne(second).Exec(ctx))
	check(false)
	repo.values[SettingPurchaseEntryPerPayLinked] = "false"
	check(true)
	repo.values[SettingPurchaseEntryEnabled] = "false"
	require.NoError(t, client.PaymentProviderInstance.UpdateOne(first).SetEnabled(true).Exec(ctx))
	repo.values[SettingPurchaseEntryPerPayLinked] = "true"
	check(false)
	repo.values[SettingPurchaseEntryEnabled] = "true"
	repo.values[SettingPaymentEnabled] = "false"
	check(false)
}

func TestCreateOrderRejectsClosedPurchaseEntryBeforeCreatingOrder(t *testing.T) {
	for _, settings := range []map[string]string{
		{SettingPaymentEnabled: "true", SettingPurchaseEntryEnabled: "false"},
		{SettingPaymentEnabled: "true", SettingPurchaseEntryPerPayLinked: "true"},
	} {
		svc := &PaymentService{configService: &PaymentConfigService{
			settingRepo: &paymentConfigSettingRepoStub{values: settings},
		}}
		order, err := svc.CreateOrder(context.Background(), CreateOrderRequest{UserID: 1, Amount: 1, PaymentType: "alipay"})
		require.Nil(t, order)
		require.Equal(t, "PURCHASE_ENTRY_CLOSED", infraerrors.Reason(err))
	}
}
