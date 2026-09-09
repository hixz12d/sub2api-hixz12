package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func astraOldCatalogForTest(t *testing.T, model string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{model: map[string]any{
		"input_cost_per_token": 10e-6, "output_cost_per_token": 50e-6,
		"cache_creation_input_token_cost": 12.5e-6, "cache_read_input_token_cost": 1e-6,
		"input_cost_per_token_priority": 20e-6, "output_cost_per_token_priority": 100e-6,
		"cache_creation_input_token_cost_priority": 25e-6, "cache_read_input_token_cost_priority": 2e-6,
		"input_cost_per_token_above_272k_tokens":  20e-6,
		"output_cost_per_token_above_272k_tokens": 75e-6,
		"litellm_provider":                        "openai", "mode": "chat",
	}})
	require.NoError(t, err)
	return data
}

func TestAstraRemoteAndCachedCardsUseLocalRetailPrices(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6-astra-fast", "gpt-6-astra-preview", "gpt-6-astra-high", "openai/gpt-6-astra", "models/gpt-6-astra"} {
		t.Run(model, func(t *testing.T) {
			service := &PricingService{}
			data, err := service.parsePricingData(astraOldCatalogForTest(t, model))
			require.NoError(t, err)
			service.pricingData = data
			billing := NewBillingService(&config.Config{}, service)
			prices, err := billing.GetModelPricing(model)
			require.NoError(t, err)
			require.InDelta(t, 10e-6, prices.InputPricePerToken, 1e-12)
			require.InDelta(t, 12.5e-6, prices.CacheCreationPricePerToken, 1e-12)
			require.InDelta(t, 2e-6, prices.CacheReadPricePerToken, 1e-12)
			require.InDelta(t, 50e-6, prices.OutputPricePerToken, 1e-12)
			require.InDelta(t, 20e-6, prices.InputPricePerTokenPriority, 1e-12)
			require.InDelta(t, 25e-6, prices.CacheCreationPricePerTokenPriority, 1e-12)
			require.InDelta(t, 4e-6, prices.CacheReadPricePerTokenPriority, 1e-12)
			require.InDelta(t, 100e-6, prices.OutputPricePerTokenPriority, 1e-12)
			require.Equal(t, 272000, prices.LongContextInputThreshold)
			require.Equal(t, 2.0, prices.LongContextInputMultiplier)
			require.InDelta(t, 1.5, prices.LongContextOutputMultiplier, 1e-12)
			encoded, err := json.Marshal(data)
			require.NoError(t, err)
			reloaded, err := service.parsePricingData(encoded)
			require.NoError(t, err)
			require.Equal(t, data[model], reloaded[model], "reloading an adjusted card must not multiply it again")
		})
	}
}

func TestAstraScreenshotUsageBillsWithTwoDollarCacheRead(t *testing.T) {
	service := &PricingService{}
	data, err := service.parsePricingData(astraOldCatalogForTest(t, "gpt-6-astra"))
	require.NoError(t, err)
	service.pricingData = data
	billing := NewBillingService(&config.Config{}, service)
	resolver := NewModelPricingResolver(nil, billing)
	for _, tier := range []string{"", "fast"} {
		cost, err := billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-6-astra", Tokens: UsageTokens{InputTokens: 294, OutputTokens: 372, CacheReadTokens: 142976},
			RateMultiplier: 1, ServiceTier: tier, Resolver: resolver,
		})
		require.NoError(t, err)
		expected := 294*10e-6 + 372*50e-6 + 142976*2e-6
		if tier == "fast" {
			expected *= 2
		}
		require.InDelta(t, expected, cost.TotalCost, 1e-12)
		require.InDelta(t, expected, cost.ActualCost, 1e-12)
	}
}

func TestAstraPricingPreservesExplicitOverridesAndUnrelatedModels(t *testing.T) {
	overridePath := filepath.Join(t.TempDir(), "pricing-override.json")
	require.NoError(t, os.WriteFile(overridePath, []byte(`{"gpt-6-astra":{"input_cost_per_token":0.000021,"long_context_input_token_threshold":0}}`), 0600))
	cfg := &config.Config{}
	cfg.Pricing.OverrideFile = overridePath
	service := &PricingService{cfg: cfg}
	data, err := service.parsePricingData(astraOldCatalogForTest(t, "gpt-6-astra"))
	require.NoError(t, err)
	require.Equal(t, 21e-6, data["gpt-6-astra"].InputCostPerToken)
	require.Equal(t, 0, data["gpt-6-astra"].LongContextInputTokenThreshold)
	for _, model := range []string{"gpt-5.6-sol", "gpt-6-astral", "gpt-6-other"} {
		data, err := service.parsePricingData(astraOldCatalogForTest(t, model))
		require.NoError(t, err)
		require.Equal(t, 10e-6, data[model].InputCostPerToken)
		require.Equal(t, 50e-6, data[model].OutputCostPerToken)
	}
}
