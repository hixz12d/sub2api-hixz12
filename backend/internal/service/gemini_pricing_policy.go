package service

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

var (
	// 2026-12-01 00:00 PST (UTC 08:00): Gemini 4 Carbon / Argon / Orchestrator introductory promotion ends.
	gemini4PromoEndAt = time.Date(2026, time.December, 1, 8, 0, 0, 0, time.UTC)
	// 2027-01-01 00:00 PST (UTC 08:00): Gemini 3.8 / 3.7 / 3.6 Flash introductory promotion ends.
	gemini38FlashPromoEndAt = time.Date(2027, time.January, 1, 8, 0, 0, 0, time.UTC)
)

const (
	geminiPriorityMultiplier = 1.8

	gemini4NeonInputPrice     = 3e-6
	gemini4NeonOutputPrice    = 15e-6
	gemini4NeonCacheReadPrice = 0.15e-6

	gemini4PromoInputPrice     = 2e-6
	gemini4PromoOutputPrice    = 10e-6
	gemini4PromoCacheReadPrice = 0.1e-6

	gemini4StandardInputPrice     = 4e-6
	gemini4StandardOutputPrice    = 20e-6
	gemini4StandardCacheReadPrice = 0.2e-6

	gemini38FlashPromoInputPrice     = 0.75e-6
	gemini38FlashPromoOutputPrice    = 3.75e-6
	gemini38FlashPromoCacheReadPrice = 0.075e-6

	gemini38FlashStandardInputPrice     = 1.5e-6
	gemini38FlashStandardOutputPrice    = 7.5e-6
	gemini38FlashStandardCacheReadPrice = 0.15e-6
)

func canonicalGeminiPolicyModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" {
		return ""
	}
	m = lastSegment(m)
	for _, pair := range [][2]string{
		{"gemini-3-6-flash", "gemini-3.6-flash"},
		{"gemini-3-7-flash", "gemini-3.7-flash"},
		{"gemini-3-8-flash", "gemini-3.8-flash"},
	} {
		if m == pair[0] {
			m = pair[1]
			break
		}
		if rest, ok := strings.CutPrefix(m, pair[0]+"-"); ok {
			m = pair[1] + "-" + rest
			break
		}
	}
	return normalizeGeminiThinkingTierAlias(m)
}

func isGemini4ZeroCacheWriteModel(base string) bool {
	return strings.HasPrefix(base, "gemini-4-neon") ||
		strings.HasPrefix(base, "gemini-4-carbon") ||
		strings.HasPrefix(base, "gemini-4-argon") ||
		strings.HasPrefix(base, "gemini-4-hydrogen") ||
		strings.HasPrefix(base, "gemini-pro-orchestrator")
}

func setGeminiStandardAndPriorityRates(dst *ModelPricing, input, output, cacheRead float64) {
	dst.InputPricePerToken = input
	dst.OutputPricePerToken = output
	dst.CacheReadPricePerToken = cacheRead
	dst.InputPricePerTokenPriority = input * geminiPriorityMultiplier
	dst.OutputPricePerTokenPriority = output * geminiPriorityMultiplier
	dst.CacheReadPricePerTokenPriority = cacheRead * geminiPriorityMultiplier
}

func clearGemini4CacheWriteFields(dst *ModelPricing) {
	dst.CacheCreationPricePerToken = 0
	dst.CacheCreationPricePerTokenPriority = 0
	dst.CacheCreation5mPrice = 0
	dst.CacheCreation1hPrice = 0
	dst.CacheCreation12hPricePerToken = nil
	dst.SupportsCacheBreakdown = false
}

// applyGeminiCatalogPricingPolicy enforces official scheduled rates and
// Gemini 4 cache-write suppression on default catalog cards (PricingSourceLiteLLM).
// It always clones pricing before mutating so shared cache/fallback pointers
// are never modified in place.
func applyGeminiCatalogPricingPolicy(model string, pricing *ModelPricing, pricingAt time.Time) (*ModelPricing, bool) {
	if pricing == nil {
		return nil, false
	}
	base := canonicalGeminiPolicyModel(model)
	if base == "" {
		return pricing, false
	}

	var matchedRate bool
	switch base {
	case "gemini-4-neon",
		"gemini-pro-orchestrator-neon",
		"gemini-4-carbon",
		"gemini-4-argon",
		"gemini-pro-orchestrator",
		"gemini-pro-orchestrator-argon",
		"gemini-4-argon-cyber-permissive",
		"gemini-3.8-flash",
		"gemini-3.7-flash",
		"gemini-3.6-flash",
		"gemini-flash-worker-agy":
		matchedRate = true
	}

	clearCacheWrite := isGemini4ZeroCacheWriteModel(base)
	if !matchedRate && !clearCacheWrite {
		return pricing, false
	}

	at := pricingAt
	if at.IsZero() {
		at = timezone.Now()
	}

	cloned := *pricing
	switch base {
	case "gemini-4-neon", "gemini-pro-orchestrator-neon":
		setGeminiStandardAndPriorityRates(&cloned, gemini4NeonInputPrice, gemini4NeonOutputPrice, gemini4NeonCacheReadPrice)
	case "gemini-4-carbon", "gemini-4-argon", "gemini-pro-orchestrator", "gemini-pro-orchestrator-argon":
		if at.Before(gemini4PromoEndAt) {
			setGeminiStandardAndPriorityRates(&cloned, gemini4PromoInputPrice, gemini4PromoOutputPrice, gemini4PromoCacheReadPrice)
		} else {
			setGeminiStandardAndPriorityRates(&cloned, gemini4StandardInputPrice, gemini4StandardOutputPrice, gemini4StandardCacheReadPrice)
		}
	case "gemini-4-argon-cyber-permissive":
		setGeminiStandardAndPriorityRates(&cloned, gemini4StandardInputPrice, gemini4StandardOutputPrice, gemini4StandardCacheReadPrice)
	case "gemini-3.8-flash", "gemini-3.7-flash", "gemini-3.6-flash", "gemini-flash-worker-agy":
		if at.Before(gemini38FlashPromoEndAt) {
			setGeminiStandardAndPriorityRates(&cloned, gemini38FlashPromoInputPrice, gemini38FlashPromoOutputPrice, gemini38FlashPromoCacheReadPrice)
		} else {
			setGeminiStandardAndPriorityRates(&cloned, gemini38FlashStandardInputPrice, gemini38FlashStandardOutputPrice, gemini38FlashStandardCacheReadPrice)
		}
	}

	if clearCacheWrite {
		clearGemini4CacheWriteFields(&cloned)
	}
	return &cloned, true
}
