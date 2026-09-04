package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func loadGPT6AstraPricingForTest(t *testing.T) (*BillingService, *ModelPricingResolver) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	pricingService := &PricingService{}
	pricingData, err := pricingService.parsePricingData(data)
	require.NoError(t, err)
	pricingService.pricingData = pricingData
	billing := NewBillingService(&config.Config{}, pricingService)
	return billing, NewModelPricingResolver(nil, billing)
}

func TestGPT6AstraOfficialPricingCard(t *testing.T) {
	billing, _ := loadGPT6AstraPricingForTest(t)
	pricing, err := billing.GetModelPricing("gpt-6-astra")
	require.NoError(t, err)

	require.InDelta(t, 10e-6, pricing.InputPricePerToken, 1e-12)
	require.InDelta(t, 1e-6, pricing.CacheReadPricePerToken, 1e-12)
	require.InDelta(t, 12.5e-6, pricing.CacheCreationPricePerToken, 1e-12)
	require.InDelta(t, 50e-6, pricing.OutputPricePerToken, 1e-12)
	require.InDelta(t, 20e-6, pricing.InputPricePerTokenPriority, 1e-12)
	require.InDelta(t, 2e-6, pricing.CacheReadPricePerTokenPriority, 1e-12)
	require.InDelta(t, 25e-6, pricing.CacheCreationPricePerTokenPriority, 1e-12)
	require.InDelta(t, 100e-6, pricing.OutputPricePerTokenPriority, 1e-12)
	require.Equal(t, 272000, pricing.LongContextInputThreshold)
	require.InDelta(t, 2.0, pricing.LongContextInputMultiplier, 1e-12)
	require.InDelta(t, 1.5, pricing.LongContextOutputMultiplier, 1e-12)
}

func TestGPT6AstraPricingBoundaryAndServiceTiers(t *testing.T) {
	billing, resolver := loadGPT6AstraPricingForTest(t)
	group := &Group{ID: 1, Platform: PlatformOpenAI, LongContextPricingEnabled: true}
	groupID := group.ID
	accountEnabled := true

	calculate := func(tokens UsageTokens, tier string) *CostBreakdown {
		t.Helper()
		cost, err := billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-6-astra", GroupID: &groupID, Group: group,
			Tokens: tokens, RateMultiplier: 1, ServiceTier: tier, Resolver: resolver,
			LongContextBillingEnabled: &accountEnabled,
		})
		require.NoError(t, err)
		return cost
	}

	atBoundary := calculate(UsageTokens{
		InputTokens: 100000, CacheCreationTokens: 100000, CacheReadTokens: 72000, OutputTokens: 1000,
	}, "")
	require.InDelta(t, 100000*10e-6, atBoundary.InputCost, 1e-12)
	require.InDelta(t, 100000*12.5e-6, atBoundary.CacheCreationCost, 1e-12)
	require.InDelta(t, 72000*1e-6, atBoundary.CacheReadCost, 1e-12)
	require.InDelta(t, 1000*50e-6, atBoundary.OutputCost, 1e-12)
	require.False(t, atBoundary.LongContextBillingApplied)

	above := UsageTokens{InputTokens: 100000, CacheCreationTokens: 100000, CacheReadTokens: 72001, OutputTokens: 1000}
	standard := calculate(above, "")
	require.InDelta(t, 100000*10e-6*2, standard.InputCost, 1e-12)
	require.InDelta(t, 100000*12.5e-6*2, standard.CacheCreationCost, 1e-12)
	require.InDelta(t, 72001*1e-6*2, standard.CacheReadCost, 1e-12)
	require.InDelta(t, 1000*50e-6*1.5, standard.OutputCost, 1e-12)
	require.True(t, standard.LongContextBillingApplied)

	fast := calculate(above, "fast")
	require.InDelta(t, standard.TotalCost*2, fast.TotalCost, 1e-12)
	flex := calculate(above, "flex")
	require.InDelta(t, standard.TotalCost*0.5, flex.TotalCost, 1e-12)
}

func TestGPT6AstraSubscriptionAccountDefaultsToNoLongContextSurcharge(t *testing.T) {
	billing, resolver := loadGPT6AstraPricingForTest(t)
	group := &Group{
		ID: 1, Platform: PlatformOpenAI, SubscriptionType: SubscriptionTypeSubscription,
		LongContextPricingEnabled: true,
	}
	groupID := group.ID
	accountDefault := false
	cost, err := billing.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "gpt-6-astra", GroupID: &groupID, Group: group,
		Tokens: UsageTokens{InputTokens: 300000, OutputTokens: 1000}, RateMultiplier: 1,
		Resolver: resolver, LongContextBillingEnabled: &accountDefault,
	})
	require.NoError(t, err)
	require.InDelta(t, 300000*10e-6, cost.InputCost, 1e-12)
	require.InDelta(t, 1000*50e-6, cost.OutputCost, 1e-12)
	require.False(t, cost.LongContextBillingApplied)
}

func TestGPT6AstraStaticFallbackNeverFallsThroughToOlderGPT(t *testing.T) {
	billing := NewBillingService(&config.Config{}, nil)
	for _, model := range []string{"gpt-6-astra", "openai/gpt-6-astra", "gpt-6-astra-preview"} {
		pricing, err := billing.GetModelPricing(model)
		require.NoError(t, err, model)
		require.InDelta(t, 10e-6, pricing.InputPricePerToken, 1e-12, model)
		require.InDelta(t, 50e-6, pricing.OutputPricePerToken, 1e-12, model)
	}
}

func TestGPT6AstraCodexDescriptorCapabilities(t *testing.T) {
	descriptor := newConfiguredCodexModelDescriptor("gpt-6-astra")
	require.Equal(t, "GPT-6 Astra", descriptor.DisplayName)
	require.Equal(t, int64(1_050_000), descriptor.ContextWindow)
	require.Equal(t, int64(1_050_000), descriptor.MaxContextWindow)
	require.NotNil(t, descriptor.DefaultReasoningLevel)
	require.Equal(t, "low", *descriptor.DefaultReasoningLevel)
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, effortsFromConfiguredCodexLevels(descriptor.SupportedReasoningLevels))
	require.True(t, descriptor.SupportsParallelToolCalls)
	require.True(t, descriptor.SupportVerbosity)
	require.True(t, configuredCodexSupportsPriorityServiceTier("gpt-6-astra"))
	require.True(t, isOpenAICodexImageInputModel("gpt-6-astra"))
	require.True(t, supportsOpenAIReasoningEffortMax("gpt-6-astra"))
}
