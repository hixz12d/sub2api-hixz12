package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestGPTLongContextPolicyPreservesCatalogConfiguration(t *testing.T) {
	tests := []struct {
		name                              string
		threshold                         int
		inputMultiplier, outputMultiplier float64
		wantInputCost, wantOutputCost     float64
		applied                           bool
	}{
		{name: "absent", wantInputCost: 1.5, wantOutputCost: .03},
		{name: "disabled", inputMultiplier: 2, outputMultiplier: 1.5, wantInputCost: 1.5, wantOutputCost: .03},
		{name: "unit multipliers", threshold: 100000, inputMultiplier: 1, outputMultiplier: 1, wantInputCost: 1.5, wantOutputCost: .03},
		{name: "configured", threshold: 100000, inputMultiplier: 2, outputMultiplier: 1.5, wantInputCost: 3, wantOutputCost: .045, applied: true},
		{name: "input only", threshold: 100000, inputMultiplier: 2, wantInputCost: 3, wantOutputCost: .03, applied: true},
		{name: "output only", threshold: 100000, outputMultiplier: 1.5, wantInputCost: 1.5, wantOutputCost: .045, applied: true},
		{name: "below threshold", threshold: 400000, inputMultiplier: 2, outputMultiplier: 1.5, wantInputCost: 1.5, wantOutputCost: .03},
	}
	billing := &BillingService{}
	for _, model := range []string{"gpt-5.4", "gpt-5.5", "gpt-5.5-pro"} {
		for _, tt := range tests {
			t.Run(model+"/"+tt.name, func(t *testing.T) {
				original := ModelPricing{InputPricePerToken: 5e-6, OutputPricePerToken: 30e-6,
					LongContextInputThreshold: tt.threshold, LongContextInputMultiplier: tt.inputMultiplier, LongContextOutputMultiplier: tt.outputMultiplier}
				pricing := original
				for _, defaultCard := range []bool{true, false} {
					resolved := billing.applyModelSpecificPricingPolicyEx(model, &pricing, defaultCard)
					require.Equal(t, tt.threshold, resolved.LongContextInputThreshold)
					require.Equal(t, tt.inputMultiplier, resolved.LongContextInputMultiplier)
					require.Equal(t, tt.outputMultiplier, resolved.LongContextOutputMultiplier)
					require.Equal(t, original, pricing, "shared source prices must remain immutable")
					cost := billing.computeTokenBreakdown(resolved, UsageTokens{InputTokens: 300000, OutputTokens: 1000}, 1, "", true)
					require.Equal(t, tt.applied, cost.LongContextBillingApplied)
					require.InDelta(t, tt.wantInputCost, cost.InputCost, 1e-12)
					require.InDelta(t, tt.wantOutputCost, cost.OutputCost, 1e-12)
				}
			})
		}
	}
}

func TestGPTLongContextOverrideStaysDisabledInUnifiedBilling(t *testing.T) {
	service := newPricingServiceWithOverride(t, `{"gpt-5.5":{"long_context_input_token_threshold":0}}`)
	data, err := service.parsePricingData([]byte(gpt55OverrideCatalogJSON))
	require.NoError(t, err)
	service.pricingData = data
	billing := NewBillingService(&config.Config{}, service)
	group := &Group{ID: 1, Platform: PlatformOpenAI, LongContextPricingEnabled: true}
	enabled := true
	for _, tier := range []string{"", "fast", "flex"} {
		cost, err := billing.CalculateCostUnified(CostInput{
			Ctx: context.Background(), Model: "gpt-5.5", Group: group, GroupID: &group.ID,
			Tokens:         UsageTokens{InputTokens: 300000, OutputTokens: 1000, CacheReadTokens: 10000},
			RateMultiplier: 1, ServiceTier: tier, Resolver: NewModelPricingResolver(nil, billing), LongContextBillingEnabled: &enabled,
		})
		require.NoError(t, err)
		require.False(t, cost.LongContextBillingApplied, tier)
		expected := 1.535
		switch tier {
		case "fast":
			expected *= 2.5
		case "flex":
			expected *= .5
		}
		require.InDelta(t, expected, cost.TotalCost, 1e-12, tier)
		require.InDelta(t, expected, cost.ActualCost, 1e-12, tier)
	}
}
