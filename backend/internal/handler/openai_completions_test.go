package handler

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestLegacyCompletionsToChatCompletions(t *testing.T) {
	body, stream, err := legacyCompletionsToChatCompletions([]byte(`{
		"model":"gpt-5.4",
		"prompt":"Write a haiku",
		"max_tokens":64,
		"temperature":0.2,
		"top_p":0.9,
		"stop":["\\n\\n"],
		"presence_penalty":0.1,
		"frequency_penalty":0.2,
		"user":"user-1",
		"seed":7,
		"stream":true
	}`))
	require.NoError(t, err)
	require.True(t, stream)

	var got map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "gpt-5.4", got["model"])
	require.Equal(t, true, got["stream"])
	require.Equal(t, "user-1", got["user"])
	require.Equal(t, float64(7), got["seed"])
	require.Equal(t, float64(64), got["max_tokens"])
	require.Equal(t, "Write a haiku", got["messages"].([]any)[0].(map[string]any)["content"])
}

func TestLegacyCompletionsToChatCompletionsRejectsUnsupportedSemantics(t *testing.T) {
	for name, body := range map[string]string{
		"array prompt":     `{"model":"gpt-5.4","prompt":["one","two"]}`,
		"multiple choices": `{"model":"gpt-5.4","prompt":"hi","n":2}`,
		"echo":             `{"model":"gpt-5.4","prompt":"hi","echo":true}`,
		"logprobs":         `{"model":"gpt-5.4","prompt":"hi","logprobs":2}`,
		"suffix":           `{"model":"gpt-5.4","prompt":"hi","suffix":"stop"}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := legacyCompletionsToChatCompletions([]byte(body))
			require.Error(t, err)
		})
	}
}

func TestChatResponseToLegacyCompletion(t *testing.T) {
	body, err := chatResponseToLegacyCompletion([]byte(`{
		"id":"chatcmpl-1",
		"object":"chat.completion",
		"created":123,
		"model":"gpt-5.4",
		"choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],
		"usage":{"prompt_tokens":4,"completion_tokens":2,"total_tokens":6}
	}`))
	require.NoError(t, err)

	var got legacyCompletionResponse
	require.NoError(t, json.Unmarshal(body, &got))
	require.Equal(t, "chatcmpl-1", got.ID)
	require.Equal(t, "text_completion", got.Object)
	require.Equal(t, "hello", got.Choices[0].Text)
	require.NotNil(t, got.Choices[0].FinishReason)
	require.Equal(t, "stop", *got.Choices[0].FinishReason)
	require.Equal(t, 6, got.Usage.TotalTokens)
}

func TestConvertChatSSEEventToCompletion(t *testing.T) {
	content := "hello"
	finish := (*string)(nil)
	payload, err := json.Marshal(apicompat.ChatCompletionsChunk{
		ID:      "chatcmpl-1",
		Object:  "chat.completion.chunk",
		Created: 123,
		Model:   "gpt-5.4",
		Choices: []apicompat.ChatChunkChoice{{
			Index:        0,
			Delta:        apicompat.ChatDelta{Content: &content},
			FinishReason: finish,
		}},
	})
	require.NoError(t, err)

	converted := convertChatSSEEventToCompletion(append([]byte("data: "), append(payload, []byte("\n\n")...)...))
	require.Contains(t, string(converted), `"object":"text_completion"`)
	require.Contains(t, string(converted), `"text":"hello"`)
	require.Contains(t, string(converted), `"finish_reason":null`)
	require.Equal(t, "data: [DONE]\n\n", string(convertChatSSEEventToCompletion([]byte("data: [DONE]\n\n"))))
	require.Equal(t, ": keepalive\n\n", string(convertChatSSEEventToCompletion([]byte(": keepalive\n\n"))))
}

func TestChatResponseToLegacyCompletionPreservesErrors(t *testing.T) {
	body := []byte(`{"error":{"type":"upstream_error","message":"failed"}}`)
	converted, err := chatResponseToLegacyCompletion(body)
	require.NoError(t, err)
	require.Equal(t, string(body), string(converted))
}
