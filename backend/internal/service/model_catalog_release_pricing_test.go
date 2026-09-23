package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/claude"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestNewModelCatalogPricingAndFallbacks(t *testing.T) {
	bundled, _ := loadGPT6AstraPricingForTest(t)
	stale := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"gpt-6":         openAIGPT6AstraFallbackPricing,
		"claude-opus-5": {InputCostPerToken: 5e-6, OutputCostPerToken: 25e-6},
	}})
	for _, tc := range []struct {
		model                      string
		input, output, read, write float64
	}{
		{"gpt-6-sol", 2e-6, 10e-6, 0.2e-6, 2.5e-6},
		{"gpt-6-luna", 0.1e-6, 0.5e-6, 0.01e-6, 0.125e-6},
		{"claude-opus-5-5", 4e-6, 20e-6, 0.2e-6, 5e-6},
	} {
		for source, svc := range map[string]*BillingService{"bundled": bundled, "stale": stale, "offline": NewBillingService(&config.Config{}, nil)} {
			for _, model := range []string{tc.model, "provider/" + tc.model, tc.model + "-20260922"} {
				t.Run(source+"/"+model, func(t *testing.T) {
					p, err := svc.GetModelPricing(model)
					require.NoError(t, err)
					require.InDelta(t, tc.input, p.InputPricePerToken, 1e-14)
					require.InDelta(t, tc.output, p.OutputPricePerToken, 1e-14)
					require.InDelta(t, tc.read, p.CacheReadPricePerToken, 1e-14)
					require.InDelta(t, tc.write, p.CacheCreationPricePerToken, 1e-14)
					require.InDelta(t, 2*tc.input, p.InputPricePerTokenPriority, 1e-14)
					require.InDelta(t, 2*tc.output, p.OutputPricePerTokenPriority, 1e-14)
					if tc.model == "claude-opus-5-5" {
						require.True(t, p.SupportsCacheBreakdown)
						require.InDelta(t, 8e-6, p.CacheCreation1hPrice, 1e-14)
						require.Zero(t, p.LongContextInputThreshold)
					} else {
						require.Equal(t, 272000, p.LongContextInputThreshold)
					}
				})
			}
		}
	}
	// A remote catalog containing only the new Opus must never change Opus 5's rate.
	svc := NewBillingService(&config.Config{}, &PricingService{pricingData: map[string]*LiteLLMModelPricing{"claude-opus-5-5": claudeOpus55FallbackPricing}})
	old, err := svc.GetModelPricing("claude-opus-5")
	require.NoError(t, err)
	require.InDelta(t, 5e-6, old.InputPricePerToken, 1e-14)
	// Exact operator overrides still win over the release fallbacks.
	custom := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"gpt-6-sol": {InputCostPerToken: 7e-6}, "claude-opus-5-5": {InputCostPerToken: 9e-6}}}
	require.InDelta(t, 7e-6, custom.GetModelPricing("gpt-6-sol-20260922").InputCostPerToken, 1e-14)
	require.InDelta(t, 9e-6, custom.GetModelPricing("claude-opus-5.5").InputCostPerToken, 1e-14)
}

func TestGPT6SolLunaBillingTiersAndLongContext(t *testing.T) {
	billing, resolver := loadGPT6AstraPricingForTest(t)
	group := &Group{ID: 1, Platform: PlatformOpenAI, LongContextPricingEnabled: true}
	enabled := true
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		t.Run(model, func(t *testing.T) {
			calculate := func(cached int, tier string) *CostBreakdown {
				cost, err := billing.CalculateCostUnified(CostInput{Ctx: context.Background(), Model: model, GroupID: &group.ID, Group: group,
					Tokens:         UsageTokens{InputTokens: 100000, CacheCreationTokens: 100000, CacheReadTokens: cached, OutputTokens: 1000},
					RateMultiplier: 1, ServiceTier: tier, Resolver: resolver, LongContextBillingEnabled: &enabled})
				require.NoError(t, err)
				return cost
			}
			p, err := billing.GetModelPricing(model)
			require.NoError(t, err)
			boundary := calculate(72000, "")
			require.False(t, boundary.LongContextBillingApplied)
			require.InDelta(t, 100000*p.InputPricePerToken, boundary.InputCost, 1e-12)
			standard := calculate(72001, "")
			require.True(t, standard.LongContextBillingApplied)
			require.InDelta(t, 100000*p.InputPricePerToken*2, standard.InputCost, 1e-12)
			require.InDelta(t, 100000*p.CacheCreationPricePerToken*2, standard.CacheCreationCost, 1e-12)
			require.InDelta(t, 72001*p.CacheReadPricePerToken*2, standard.CacheReadCost, 1e-12)
			require.InDelta(t, 1000*p.OutputPricePerToken*1.5, standard.OutputCost, 1e-12)
			require.InDelta(t, standard.TotalCost*2, calculate(72001, "fast").TotalCost, 1e-12)
			require.InDelta(t, standard.TotalCost*0.5, calculate(72001, "flex").TotalCost, 1e-12)
		})
	}
}

func TestNewModelCatalogOpus55CacheTTLPricing(t *testing.T) {
	bundled, _ := loadGPT6AstraPricingForTest(t)
	for _, billing := range []*BillingService{bundled, NewBillingService(&config.Config{}, nil)} {
		for _, tc := range []struct {
			tier       string
			multiplier float64
		}{{"", 1}, {"fast", 2}, {"priority", 2}, {"flex", 0.5}} {
			cost, err := billing.CalculateCostWithServiceTier("claude-opus-5-5", UsageTokens{
				InputTokens: 1000, OutputTokens: 1000, CacheReadTokens: 1000,
				CacheCreationTokens: 3000, CacheCreation5mTokens: 1000, CacheCreation1hTokens: 2000,
			}, 1, tc.tier)
			require.NoError(t, err)
			require.InDelta(t, (1000*5e-6+2000*8e-6)*tc.multiplier, cost.CacheCreationCost, 1e-12, tc.tier)
			require.InDelta(t, 1000*0.2e-6*tc.multiplier, cost.CacheReadCost, 1e-12, tc.tier)
			require.InDelta(t, 1000*4e-6*tc.multiplier, cost.InputCost, 1e-12, tc.tier)
			require.InDelta(t, 1000*20e-6*tc.multiplier, cost.OutputCost, 1e-12, tc.tier)
		}
	}
}

func TestNewModelCatalogCapabilities(t *testing.T) {
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		require.Contains(t, openai.DefaultModelIDs(), model)
		for _, alias := range []string{model, "openai/" + model, model + "-max", model + "-20260922"} {
			require.Equal(t, model, normalizeKnownOpenAICodexModel(alias))
			require.True(t, supportsOpenAIReasoningEffortMax(alias))
			require.True(t, isOpenAICodexImageInputModel(alias))
		}
		d := newConfiguredCodexModelDescriptor(model)
		require.Equal(t, int64(1050000), d.ContextWindow)
		require.Equal(t, int64(1050000), d.MaxContextWindow)
		require.Equal(t, "medium", *d.DefaultReasoningLevel)
		require.Equal(t, []string{"none", "low", "medium", "high", "xhigh", "max"}, effortsFromConfiguredCodexLevels(d.SupportedReasoningLevels))
		require.True(t, configuredCodexSupportsPriorityServiceTier(model))
	}
	require.Contains(t, claude.DefaultModelIDs(), "claude-opus-5-5")
	require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.EffortLevelsForModel("claude-opus-5-5"))
	require.Equal(t, int64(1000000), newConfiguredCodexModelDescriptor("claude-opus-5-5").ContextWindow)
	require.NotContains(t, openai.DefaultModelIDs(), "gpt-6-terra")
	require.False(t, isOpenAIGPT6SolLunaModel("gpt-6-terra"))
	require.False(t, isClaudeOpus55Model("claude-opus-5-50"))
}
