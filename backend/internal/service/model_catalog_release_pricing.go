package service

import "strings"

// Published 2026-09-22 (GPT-6.1 Sol: 2026-09-29). USD per token; explicit operator prices take precedence.
// Sources: https://developers.openai.com/api/docs/models/gpt-6.1-sol
// https://developers.openai.com/api/docs/models/gpt-6.1-terra
// https://developers.openai.com/api/docs/models/gpt-6-sol
// https://developers.openai.com/api/docs/models/gpt-6-luna
// https://platform.claude.com/docs/en/models/opus-5-5/overview
var (
	// GPT-6.1 / GPT-6 Sol、Luna 与 GPT-6.2 的卡价随上游定义在 pricing_service.go。
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
	for _, base := range []string{"gpt-6.1-sol", "gpt-6.1-terra", "gpt-6-sol", "gpt-6-luna"} {
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

func openAIGPT6SolLunaFallbackPricing(base string) *LiteLLMModelPricing {
	switch base {
	case "gpt-6.1-sol":
		return openAIGPT61SolFallbackPricing
	case "gpt-6.1-terra":
		return openAIGPT61TerraFallbackPricing
	case "gpt-6-sol":
		return openAIGPT6SolFallbackPricing
	default:
		return openAIGPT6LunaFallbackPricing
	}
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
