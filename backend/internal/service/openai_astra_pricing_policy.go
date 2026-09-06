package service

import "encoding/json"

// applyAstraCatalogPricing pins the local retail card before explicit operator
// overrides. Assigning prices, rather than multiplying a fetched card, makes
// bundled data, remote refreshes and cached reloads idempotent.
func applyAstraCatalogPricing(data map[string]json.RawMessage) {
	for model, raw := range data {
		if !isOpenAIGPT6AstraModel(model) {
			continue
		}
		var fields map[string]any
		var original LiteLLMModelPricing
		if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &original) != nil {
			continue
		}
		// Preserve the original ladder ratios before replacing the base prices.
		if fields["long_context_input_token_threshold"] == nil && fields["long_context_input_cost_multiplier"] == nil && fields["long_context_output_cost_multiplier"] == nil {
			deriveLongContextFromAboveTierFields(raw, &original)
			if original.LongContextInputTokenThreshold > 0 {
				fields["long_context_input_token_threshold"] = original.LongContextInputTokenThreshold
				fields["long_context_input_cost_multiplier"] = original.LongContextInputCostMultiplier
				fields["long_context_output_cost_multiplier"] = original.LongContextOutputCostMultiplier
			}
		}
		card := openAIGPT6AstraFallbackPricing
		if original.CacheCreationInputTokenCostAbove1hr > 0 && original.CacheCreationInputTokenCost > 0 {
			fields["cache_creation_input_token_cost_above_1hr"] = original.CacheCreationInputTokenCostAbove1hr * (card.CacheCreationInputTokenCost / original.CacheCreationInputTokenCost)
		}
		fields["input_cost_per_token"] = card.InputCostPerToken
		fields["output_cost_per_token"] = card.OutputCostPerToken
		fields["cache_creation_input_token_cost"] = card.CacheCreationInputTokenCost
		fields["cache_read_input_token_cost"] = card.CacheReadInputTokenCost
		fields["input_cost_per_token_priority"] = card.InputCostPerTokenPriority
		fields["output_cost_per_token_priority"] = card.OutputCostPerTokenPriority
		fields["cache_creation_input_token_cost_priority"] = card.CacheCreationInputTokenCostPriority
		fields["cache_read_input_token_cost_priority"] = card.CacheReadInputTokenCostPriority
		if updated, err := json.Marshal(fields); err == nil {
			data[model] = updated
		}
	}
}
