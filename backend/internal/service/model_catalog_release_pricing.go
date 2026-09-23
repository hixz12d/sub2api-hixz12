package service

import "strings"

// Published 2026-09-22. USD per token; explicit operator prices take precedence.
// Sources: https://developers.openai.com/api/docs/models/gpt-6-sol
// https://developers.openai.com/api/docs/models/gpt-6-luna
// https://platform.claude.com/docs/en/models/opus-5-5/overview
var (
	openAIGPT6SolFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken: 2e-6, OutputCostPerToken: 10e-6,
		CacheCreationInputTokenCost: 2.5e-6, CacheReadInputTokenCost: 0.2e-6,
		InputCostPerTokenPriority: 4e-6, OutputCostPerTokenPriority: 20e-6,
		CacheCreationInputTokenCostPriority: 5e-6, CacheReadInputTokenCostPriority: 0.4e-6,
		LongContextInputTokenThreshold: 272000, LongContextInputCostMultiplier: 2, LongContextOutputCostMultiplier: 1.5,
		LiteLLMProvider: "openai", Mode: "chat", SupportsPromptCaching: true, SupportsServiceTier: true,
	}
	openAIGPT6LunaFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken: 0.1e-6, OutputCostPerToken: 0.5e-6,
		CacheCreationInputTokenCost: 0.125e-6, CacheReadInputTokenCost: 0.01e-6,
		InputCostPerTokenPriority: 0.2e-6, OutputCostPerTokenPriority: 1e-6,
		CacheCreationInputTokenCostPriority: 0.25e-6, CacheReadInputTokenCostPriority: 0.02e-6,
		LongContextInputTokenThreshold: 272000, LongContextInputCostMultiplier: 2, LongContextOutputCostMultiplier: 1.5,
		LiteLLMProvider: "openai", Mode: "chat", SupportsPromptCaching: true, SupportsServiceTier: true,
	}
	claudeOpus55FallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken: 4e-6, OutputCostPerToken: 20e-6,
		CacheCreationInputTokenCost: 5e-6, CacheCreationInputTokenCostAbove1hr: 8e-6, CacheReadInputTokenCost: 0.2e-6,
		InputCostPerTokenPriority: 8e-6, OutputCostPerTokenPriority: 40e-6,
		CacheCreationInputTokenCostPriority: 10e-6, CacheReadInputTokenCostPriority: 0.4e-6,
		LiteLLMProvider: "anthropic", Mode: "chat", SupportsPromptCaching: true, SupportsServiceTier: true,
	}
)

func isOpenAIGPT6SolLunaModel(model string) bool {
	return openAIGPT6SolLunaBase(model) != ""
}

func openAIGPT6SolLunaBase(model string) string {
	canonical := canonicalizeOpenAIModelAliasSpelling(model)
	for _, base := range []string{"gpt-6-sol", "gpt-6-luna"} {
		if canonical == base || strings.HasPrefix(canonical, base+"-") {
			return base
		}
	}
	return ""
}

func isClaudeOpus55Model(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, base := range []string{"claude-opus-5-5", "claude-opus-5.5"} {
		if at := strings.Index(model, base); at >= 0 {
			end := at + len(base)
			if end == len(model) || model[end] == '-' || model[end] == ':' || model[end] == '[' {
				return true
			}
		}
	}
	return false
}

func catalogFallbackBillingPricing(card *LiteLLMModelPricing) *ModelPricing {
	return &ModelPricing{
		InputPricePerToken: card.InputCostPerToken, OutputPricePerToken: card.OutputCostPerToken,
		CacheCreationPricePerToken: card.CacheCreationInputTokenCost, CacheReadPricePerToken: card.CacheReadInputTokenCost,
		InputPricePerTokenPriority: card.InputCostPerTokenPriority, OutputPricePerTokenPriority: card.OutputCostPerTokenPriority,
		CacheCreationPricePerTokenPriority: card.CacheCreationInputTokenCostPriority, CacheReadPricePerTokenPriority: card.CacheReadInputTokenCostPriority,
		CacheCreation5mPrice: card.CacheCreationInputTokenCost, CacheCreation1hPrice: card.CacheCreationInputTokenCostAbove1hr,
		SupportsCacheBreakdown:     card.CacheCreationInputTokenCostAbove1hr > card.CacheCreationInputTokenCost,
		LongContextInputThreshold:  card.LongContextInputTokenThreshold,
		LongContextInputMultiplier: card.LongContextInputCostMultiplier, LongContextOutputMultiplier: card.LongContextOutputCostMultiplier,
	}
}
