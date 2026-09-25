//go:build unit

package service

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestAccountQuestionPreservesPromptWithoutStateUpdates(t *testing.T) {
	question := "  don't search the internet, who is Thibault Sottiaux on X\n"
	for _, chat := range []bool{false, true} {
		for _, status := range []int{200, 401, 429} {
			t.Run(strings.Join([]string{map[bool]string{true: "chat", false: "responses"}[chat], http.StatusText(status)}, "/"), func(t *testing.T) {
				body := "fixture upstream error"
				if status == 200 {
					body = "data: {\"type\":\"response.output_text.delta\",\"delta\":\"fixture answer\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"
					if chat {
						body = "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture answer\"},\"finish_reason\":null}]}\n\ndata: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
					}
				}
				response := newJSONResponse(status, body)
				response.Header.Set("Retry-After", "60")
				response.Header.Set("x-codex-primary-used-percent", "88")
				repo := &openAIAccountTestRepo{}
				upstream := &queuedHTTPUpstream{responses: []*http.Response{response}}
				svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{}}
				account := &Account{ID: 89, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1, Credentials: map[string]any{"access_token": "fixture-token"}}
				if chat {
					account.Type = AccountTypeAPIKey
					account.Credentials = map[string]any{"api_key": "fixture-token", "base_url": "https://fixture.invalid"}
					account.Extra = map[string]any{"openai_responses_mode": "force_chat_completions"}
				}
				ctx, recorder := newTestContext()
				err := svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", question, AccountTestModeQuestion)
				if status == 200 {
					require.NoError(t, err)
					require.Contains(t, recorder.Body.String(), "fixture answer")
				} else {
					require.Error(t, err)
				}
				require.Len(t, upstream.requests, 1)
				raw, err := io.ReadAll(upstream.requests[0].Body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(raw, &payload))
				if chat {
					require.Equal(t, question, payload["messages"].([]any)[0].(map[string]any)["content"])
				} else {
					require.Equal(t, question, payload["input"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"])
				}
				require.Empty(t, payload["tools"])
				require.Zero(t, repo.setErrorID)
				require.Zero(t, repo.rateLimitedID)
				require.Zero(t, repo.clearedErrorID)
				require.Empty(t, repo.updatedExtra)
				require.Empty(t, repo.bulkUpdatedIDs)
			})
		}
	}
}

func TestAccountQuestionRejectsUnsupportedBeforeRequest(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.NoError(t, validateAccountQuestion(account, "gpt-5.4", "question"))
	for _, prompt := range []string{"", "  ", strings.Repeat("x", 4097), "question\x00"} {
		require.Error(t, validateAccountQuestion(account, "gpt-5.4", prompt))
	}
	require.Error(t, validateAccountQuestion(account, "gpt-image-1", "question"))
	account.Platform = PlatformAnthropic
	upstream := &queuedHTTPUpstream{}
	svc := &AccountTestService{httpUpstream: upstream}
	ctx, _ := newTestContext()
	require.Error(t, svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "question", AccountTestModeQuestion))
	require.Empty(t, upstream.requests)
	require.Equal(t, AccountTestModeQuestion, normalizeAccountTestMode(" QUESTION "))
	require.True(t, IsAccountQuestionTest(" QUESTION "))
	require.False(t, IsAccountQuestionTest(AccountTestModeDefault))
}

func TestAccountQuestionReasoningEffortPayload(t *testing.T) {
	for _, chat := range []bool{false, true} {
		for _, effort := range []string{"", "xhigh"} {
			name := map[bool]string{true: "chat", false: "responses"}[chat] + "/" + effort
			t.Run(name, func(t *testing.T) {
				body := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\ndata: {\"type\":\"response.completed\"}\n\n"
				if chat {
					body = "data: {\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
				}
				upstream := &queuedHTTPUpstream{responses: []*http.Response{newJSONResponse(200, body)}}
				svc := &AccountTestService{accountRepo: &openAIAccountTestRepo{}, httpUpstream: upstream, cfg: &config.Config{}}
				account := &Account{ID: 90, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1, Credentials: map[string]any{"api_key": "fixture-token", "base_url": "https://fixture.invalid"}}
				if chat {
					account.Extra = map[string]any{"openai_responses_mode": "force_chat_completions"}
				}
				ctx, _ := newTestContext()
				ctx.Request = ctx.Request.WithContext(withAccountQuestionReasoningEffort(ctx.Request.Context(), effort))
				require.NoError(t, svc.testOpenAIAccountConnection(ctx, account, "gpt-5.4", "question", AccountTestModeQuestion))
				require.Len(t, upstream.requests, 1)
				raw, err := io.ReadAll(upstream.requests[0].Body)
				require.NoError(t, err)
				var payload map[string]any
				require.NoError(t, json.Unmarshal(raw, &payload))
				limitKey := "max_output_tokens"
				if chat {
					limitKey = "max_tokens"
				}
				if effort == "" {
					require.NotContains(t, payload, "reasoning")
					require.NotContains(t, payload, "reasoning_effort")
					require.EqualValues(t, accountQuestionMaxOutputTokens, payload[limitKey])
					return
				}
				require.EqualValues(t, accountQuestionReasoningMaxOutputTokens, payload[limitKey])
				if chat {
					require.Equal(t, effort, payload["reasoning_effort"])
					require.NotContains(t, payload, "reasoning")
				} else {
					require.Equal(t, effort, payload["reasoning"].(map[string]any)["effort"])
					require.NotContains(t, payload, "reasoning_effort")
				}
			})
		}
	}
}

func TestAccountQuestionOAuthReasoningHasNoOutputCap(t *testing.T) {
	payload := createOpenAITestPayload("gpt-5.4", true)
	applyAccountQuestionPayload(payload, "question", false, true, "high")
	require.Equal(t, "high", payload["reasoning"].(map[string]any)["effort"])
	require.NotContains(t, payload, "max_output_tokens")
}

func TestNormalizeAccountQuestionReasoningEffort(t *testing.T) {
	for input, want := range map[string]string{"": "", " HIGH ": "high", "none": "none", "minimal": "minimal", "xhigh": "xhigh"} {
		got, ok := NormalizeAccountQuestionReasoningEffort(input)
		require.True(t, ok, input)
		require.Equal(t, want, got)
	}
	for _, input := range []string{"max", "ultra", "extreme", "high\x00"} {
		_, ok := NormalizeAccountQuestionReasoningEffort(input)
		require.False(t, ok, input)
	}
}
