package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/gin-gonic/gin"
)

// Completions adapts the legacy OpenAI text completions protocol to the
// existing Chat Completions gateway. Account selection, failover, billing and
// upstream compatibility therefore remain identical to /v1/chat/completions.
//
// POST /v1/completions
func (h *OpenAIGatewayHandler) Completions(c *gin.Context) {
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	chatBody, stream, err := legacyCompletionsToChatCompletions(body)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	originalBody := c.Request.Body
	originalContentLength := c.Request.ContentLength
	originalWriter := c.Writer
	adapter := &openAICompletionsResponseWriter{
		ResponseWriter: originalWriter,
		stream:         stream,
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(chatBody))
	c.Request.ContentLength = int64(len(chatBody))
	c.Writer = adapter
	defer func() {
		c.Request.Body = originalBody
		c.Request.ContentLength = originalContentLength
		c.Writer = originalWriter
		adapter.finalize()
	}()

	h.ChatCompletions(c)
}

type legacyCompletionsRequest struct {
	Model            string          `json:"model"`
	Prompt           json.RawMessage `json:"prompt"`
	Suffix           string          `json:"suffix,omitempty"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	N                *int            `json:"n,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Logprobs         json.RawMessage `json:"logprobs,omitempty"`
	Echo             bool            `json:"echo,omitempty"`
	Stop             json.RawMessage `json:"stop,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	BestOf           *int            `json:"best_of,omitempty"`
	LogitBias        json.RawMessage `json:"logit_bias,omitempty"`
	User             string          `json:"user,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
}

func legacyCompletionsToChatCompletions(body []byte) ([]byte, bool, error) {
	var req legacyCompletionsRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false, fmt.Errorf("Failed to parse request body")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, false, fmt.Errorf("model is required")
	}
	if len(req.Prompt) == 0 || bytes.Equal(bytes.TrimSpace(req.Prompt), []byte("null")) {
		return nil, false, fmt.Errorf("prompt is required")
	}

	var prompt string
	if err := json.Unmarshal(req.Prompt, &prompt); err != nil {
		return nil, false, fmt.Errorf("prompt must be a string; array prompts are not supported by this compatible endpoint")
	}
	if req.N != nil && *req.N != 1 {
		return nil, false, fmt.Errorf("n is not supported; use n=1")
	}
	if req.BestOf != nil && *req.BestOf != 1 {
		return nil, false, fmt.Errorf("best_of is not supported; use best_of=1")
	}
	if req.Echo {
		return nil, false, fmt.Errorf("echo is not supported by this compatible endpoint")
	}
	if hasNonNullJSON(req.Logprobs) {
		return nil, false, fmt.Errorf("logprobs is not supported by this compatible endpoint")
	}
	if strings.TrimSpace(req.Suffix) != "" {
		return nil, false, fmt.Errorf("suffix is not supported by this compatible endpoint")
	}

	payload := map[string]any{
		"model": req.Model,
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": prompt,
			},
		},
		"stream": req.Stream,
	}
	if req.MaxTokens != nil {
		payload["max_tokens"] = *req.MaxTokens
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	if hasNonNullJSON(req.Stop) {
		var stop any
		if err := json.Unmarshal(req.Stop, &stop); err != nil {
			return nil, false, fmt.Errorf("stop must be a string or an array of strings")
		}
		payload["stop"] = stop
	}

	// These fields are preserved for raw OpenAI-compatible upstreams. The
	// existing Responses bridge intentionally drops fields it cannot represent.
	if req.PresencePenalty != nil {
		payload["presence_penalty"] = *req.PresencePenalty
	}
	if req.FrequencyPenalty != nil {
		payload["frequency_penalty"] = *req.FrequencyPenalty
	}
	if hasNonNullJSON(req.LogitBias) {
		var logitBias any
		if err := json.Unmarshal(req.LogitBias, &logitBias); err != nil {
			return nil, false, fmt.Errorf("logit_bias must be an object")
		}
		payload["logit_bias"] = logitBias
	}
	if req.User != "" {
		payload["user"] = req.User
	}
	if req.Seed != nil {
		payload["seed"] = *req.Seed
	}

	chatBody, err := json.Marshal(payload)
	if err != nil {
		return nil, false, fmt.Errorf("Failed to build compatible Chat Completions request")
	}
	return chatBody, req.Stream, nil
}

func hasNonNullJSON(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

type legacyCompletionResponse struct {
	ID                string                   `json:"id"`
	Object            string                   `json:"object"`
	Created           int64                    `json:"created"`
	Model             string                   `json:"model"`
	Choices           []legacyCompletionChoice `json:"choices"`
	Usage             *legacyCompletionUsage   `json:"usage,omitempty"`
	SystemFingerprint string                   `json:"system_fingerprint,omitempty"`
}

type legacyCompletionChoice struct {
	Text         string  `json:"text"`
	Index        int     `json:"index"`
	Logprobs     any     `json:"logprobs"`
	FinishReason *string `json:"finish_reason"`
}

type legacyCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func chatResponseToLegacyCompletion(body []byte) ([]byte, error) {
	if hasJSONError(body) {
		return body, nil
	}
	var response apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return body, err
	}
	out := legacyCompletionResponse{
		ID:                response.ID,
		Object:            "text_completion",
		Created:           response.Created,
		Model:             response.Model,
		SystemFingerprint: response.SystemFingerprint,
	}
	for _, choice := range response.Choices {
		finishReason := choice.FinishReason
		out.Choices = append(out.Choices, legacyCompletionChoice{
			Text:         chatMessageText(choice.Message.Content),
			Index:        choice.Index,
			FinishReason: &finishReason,
		})
	}
	if response.Usage != nil {
		out.Usage = &legacyCompletionUsage{
			PromptTokens:     response.Usage.PromptTokens,
			CompletionTokens: response.Usage.CompletionTokens,
			TotalTokens:      response.Usage.TotalTokens,
		}
	}
	return json.Marshal(out)
}

func chatChunkToLegacyCompletion(chunk apicompat.ChatCompletionsChunk) legacyCompletionResponse {
	out := legacyCompletionResponse{
		ID:                chunk.ID,
		Object:            "text_completion",
		Created:           chunk.Created,
		Model:             chunk.Model,
		SystemFingerprint: chunk.SystemFingerprint,
	}
	for _, choice := range chunk.Choices {
		text := ""
		if choice.Delta.Content != nil {
			text = *choice.Delta.Content
		}
		var finishReason *string
		if choice.FinishReason != nil {
			value := *choice.FinishReason
			finishReason = &value
		}
		out.Choices = append(out.Choices, legacyCompletionChoice{
			Text:         text,
			Index:        choice.Index,
			FinishReason: finishReason,
		})
	}
	if chunk.Usage != nil {
		out.Usage = &legacyCompletionUsage{
			PromptTokens:     chunk.Usage.PromptTokens,
			CompletionTokens: chunk.Usage.CompletionTokens,
			TotalTokens:      chunk.Usage.TotalTokens,
		}
	}
	return out
}

func chatMessageText(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var parts []map[string]any
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range parts {
		if value, ok := part["text"].(string); ok {
			builder.WriteString(value)
		}
	}
	return builder.String()
}

func hasJSONError(body []byte) bool {
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	return json.Unmarshal(body, &envelope) == nil && len(envelope.Error) > 0 && !bytes.Equal(bytes.TrimSpace(envelope.Error), []byte("null"))
}

type openAICompletionsResponseWriter struct {
	gin.ResponseWriter
	stream       bool
	buffer       bytes.Buffer
	streamBuffer bytes.Buffer
}

func (w *openAICompletionsResponseWriter) Size() int {
	size := w.ResponseWriter.Size()
	if !w.stream && w.buffer.Len() > 0 {
		if size < 0 {
			return w.buffer.Len()
		}
		return size + w.buffer.Len()
	}
	return size
}

func (w *openAICompletionsResponseWriter) Written() bool {
	return w.ResponseWriter.Written() || (!w.stream && w.buffer.Len() > 0)
}

func (w *openAICompletionsResponseWriter) Write(data []byte) (int, error) {
	if !w.stream {
		_, _ = w.buffer.Write(data)
		return len(data), nil
	}
	_, _ = w.streamBuffer.Write(data)
	if err := w.writeCompleteSSEEvents(); err != nil {
		return len(data), err
	}
	return len(data), nil
}

func (w *openAICompletionsResponseWriter) WriteString(data string) (int, error) {
	return w.Write([]byte(data))
}

func (w *openAICompletionsResponseWriter) writeCompleteSSEEvents() error {
	for {
		data := w.streamBuffer.Bytes()
		idx := bytes.Index(data, []byte("\n\n"))
		if idx < 0 {
			return nil
		}
		event := append([]byte(nil), data[:idx+2]...)
		w.streamBuffer.Next(idx + 2)
		converted := convertChatSSEEventToCompletion(event)
		if _, err := w.ResponseWriter.Write(converted); err != nil {
			return err
		}
	}
}

func convertChatSSEEventToCompletion(event []byte) []byte {
	text := string(event)
	if strings.HasPrefix(strings.TrimSpace(text), ":") {
		return event
	}
	var dataLines []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) == 0 {
		return event
	}
	data := strings.Join(dataLines, "\n")
	if data == "[DONE]" || hasJSONError([]byte(data)) {
		return event
	}
	var chunk apicompat.ChatCompletionsChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return event
	}
	payload, err := json.Marshal(chatChunkToLegacyCompletion(chunk))
	if err != nil {
		return event
	}
	return append([]byte("data: "), append(payload, []byte("\n\n")...)...)
}

func (w *openAICompletionsResponseWriter) finalize() {
	if !w.stream {
		if w.buffer.Len() == 0 {
			return
		}
		body, err := chatResponseToLegacyCompletion(w.buffer.Bytes())
		if err != nil {
			body = w.buffer.Bytes()
		}
		_, _ = w.ResponseWriter.Write(body)
		return
	}
	if w.streamBuffer.Len() > 0 {
		_, _ = w.ResponseWriter.Write(w.streamBuffer.Bytes())
		w.streamBuffer.Reset()
	}
}
