package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOAuthAccountDailyQuotaStopsNonExemptModels(t *testing.T) {
	account := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Type:        AccountTypeOAuth,
		Extra: map[string]any{
			"quota_daily_limit": 10.0,
			"quota_daily_used":  10.0,
			"quota_daily_start": time.Now().Add(-time.Hour).Format(time.RFC3339),
			"quota_exempt_models": []any{
				"gpt-4o-mini",
			},
		},
	}

	require.True(t, account.SupportsQuotaLimit())
	require.False(t, account.IsSchedulableForModel("gpt-5"))
	require.True(t, account.IsSchedulableForModel("GPT-4O-MINI"))
	require.False(t, account.IsSchedulable())
}

func TestOAuthAccountQuotaExemptModelsAcceptCommaSeparatedConfig(t *testing.T) {
	account := &Account{Type: AccountTypeOAuth, Extra: map[string]any{
		"quota_exempt_models": "gpt-4o,\ngpt-4o-mini, gpt-4o",
	}}

	require.Equal(t, []string{"gpt-4o", "gpt-4o-mini"}, account.GetQuotaExemptModels())
	require.True(t, account.IsQuotaModelExempt("GPT-4O"))
	require.False(t, account.IsQuotaModelExempt("gpt-5"))
}

func TestOAuthAccountQuotaUsageIsNotLimitedWhenPeriodExpired(t *testing.T) {
	account := &Account{
		Status:      StatusActive,
		Schedulable: true,
		Type:        AccountTypeOAuth,
		Extra: map[string]any{
			"quota_daily_limit": 10.0,
			"quota_daily_used":  10.0,
			"quota_daily_start": time.Now().Add(-25 * time.Hour).Format(time.RFC3339),
		},
	}

	require.True(t, account.IsSchedulableForModel("gpt-5"))
}
