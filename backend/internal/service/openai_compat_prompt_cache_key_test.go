package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func mustRawJSON(t *testing.T, s string) json.RawMessage {
	t.Helper()
	return json.RawMessage(s)
}

func TestShouldAutoInjectPromptCacheKeyForCompat(t *testing.T) {
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.5-pro"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.4-mini"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.2"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-5.3-codex-spark"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-4o"))
}

func TestDeriveOpenAIResponsesPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	first := []byte(`{"model":"gpt-5.4","instructions":"You are helpful.","input":[{"role":"user","content":"Hello"}]}`)
	later := []byte(`{"model":"gpt-5.4","instructions":"You are helpful.","input":[{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi"},{"role":"user","content":"Continue"}]}`)

	key1 := deriveOpenAIResponsesPromptCacheKey(first)
	key2 := deriveOpenAIResponsesPromptCacheKey(later)
	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2, "Responses cache key should be stable across later turns")
	require.True(t, strings.HasPrefix(key1, responsesPromptCacheKeyPrefix))
	require.Len(t, key1, len(responsesPromptCacheKeyPrefix)+16)
}

func TestDeriveOpenAIResponsesPromptCacheKey_UsesStablePrefixAcrossPrompts(t *testing.T) {
	first := []byte(`{"model":"gpt-5.4","instructions":"You are helpful.","input":[{"role":"user","content":"Question A"}]}`)
	second := []byte(`{"model":"gpt-5.4","instructions":"You are helpful.","input":[{"role":"user","content":"Question B"}]}`)

	key1 := deriveOpenAIResponsesPromptCacheKey(first)
	key2 := deriveOpenAIResponsesPromptCacheKey(second)
	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2, "Responses cache key should follow the reusable prefix, not changing user content")
}

func TestDeriveOpenAIResponsesPromptCacheKey_RequiresStablePrefixOrUserAnchor(t *testing.T) {
	require.Empty(t, deriveOpenAIResponsesPromptCacheKey([]byte(`{"model":"gpt-5.4"}`)))
	require.NotEmpty(t, deriveOpenAIResponsesPromptCacheKey([]byte(`{"model":"gpt-5.4","instructions":"You are helpful."}`)))
	require.NotEmpty(t, deriveOpenAIResponsesPromptCacheKey([]byte(`{"model":"gpt-5.4","input":"Hello"}`)))
}

func TestDeriveCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
		},
	}
	extended := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "user", Content: mustRawJSON(t, `"Hello"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Hi there!"`)},
			{Role: "user", Content: mustRawJSON(t, `"How are you?"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(extended, "gpt-5.4")
	require.Equal(t, k1, k2, "cache key should be stable across later turns")
	require.NotEmpty(t, k1)
}

func TestDeriveCompatPromptCacheKey_StableAcrossIndependentPrompts(t *testing.T) {
	first := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "developer", Content: mustRawJSON(t, `"Use the repository tools."`)},
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}
	second := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: mustRawJSON(t, `"You are helpful."`)},
			{Role: "developer", Content: mustRawJSON(t, `"Use the repository tools."`)},
			{Role: "user", Content: mustRawJSON(t, `"Question B"`)},
		},
	}

	key1 := deriveCompatPromptCacheKey(first, "gpt-5.4")
	key2 := deriveCompatPromptCacheKey(second, "gpt-5.4")
	require.NotEmpty(t, key1)
	require.Equal(t, key1, key2, "a reusable system/developer prefix should ignore changing user content")
	require.Empty(t, deriveCompatPromptCacheKey(&apicompat.ChatCompletionsRequest{Model: "gpt-5.4"}, "gpt-5.4"))
}

func TestDeriveCompatPromptCacheKey_DiffersAcrossSessions(t *testing.T) {
	req1 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}
	req2 := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.4",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question B"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req1, "gpt-5.4")
	k2 := deriveCompatPromptCacheKey(req2, "gpt-5.4")
	require.NotEqual(t, k1, k2, "different first user messages should yield different keys")
}

func TestDeriveCompatPromptCacheKey_UsesResolvedSparkFamily(t *testing.T) {
	req := &apicompat.ChatCompletionsRequest{
		Model: "gpt-5.3-codex-spark",
		Messages: []apicompat.ChatMessage{
			{Role: "user", Content: mustRawJSON(t, `"Question A"`)},
		},
	}

	k1 := deriveCompatPromptCacheKey(req, "gpt-5.3-codex-spark")
	k2 := deriveCompatPromptCacheKey(req, " openai/gpt-5.3-codex-spark ")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "resolved spark family should derive a stable compat cache key")
}

func TestDeriveAnthropicCompatPromptCacheKey_StableAcrossLaterTurns(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  "claude-sonnet-4-5",
		System: mustRawJSON(t, `"You are helpful."`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `"Open repo"`)},
			{Role: "assistant", Content: mustRawJSON(t, `"Opened."`)},
			{Role: "user", Content: mustRawJSON(t, `"Run tests"`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.3-codex")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.3-codex")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2, "cache key should stay stable as later Claude Code turns append history")
}

func TestDeriveAnthropicCompatPromptCacheKey_UsesCacheControlAnchors(t *testing.T) {
	base := &apicompat.AnthropicRequest{
		Model: "claude-sonnet-4-5",
		System: mustRawJSON(t, `[
			{"type":"text","text":"project instructions","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []apicompat.AnthropicMessage{
			{Role: "user", Content: mustRawJSON(t, `[
				{"type":"text","text":"repo anchor","cache_control":{"type":"ephemeral"}}
			]`)},
		},
	}
	extended := &apicompat.AnthropicRequest{
		Model:  base.Model,
		System: base.System,
		Messages: []apicompat.AnthropicMessage{
			base.Messages[0],
			{Role: "assistant", Content: mustRawJSON(t, `[{"type":"text","text":"Opened."}]`)},
			{Role: "user", Content: mustRawJSON(t, `[{"type":"text","text":"Run tests"}]`)},
		},
	}

	k1 := deriveAnthropicCompatPromptCacheKey(base, "gpt-5.4")
	k2 := deriveAnthropicCompatPromptCacheKey(extended, "gpt-5.4")
	require.NotEmpty(t, k1)
	require.Equal(t, k1, k2)
	require.True(t, strings.HasPrefix(k1, "anthropic-cache-"))
	require.False(t, strings.HasPrefix(k1, compatPromptCacheKeyPrefix))
}
