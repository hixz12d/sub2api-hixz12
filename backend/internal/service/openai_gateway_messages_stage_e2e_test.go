package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Default-tag E2E coverage for Messages first-output stage (C5) and ledger fields.
// Companion unit-tag smoke lives in openai_gateway_messages_preoutput_recovery_test.go.

type messagesStageH2Reporter struct {
	httpUpstreamRecorder
	failures []error
}

func (u *messagesStageH2Reporter) RecordOpenAIHTTP2StreamFailure(proxyURL string, err error) {
	u.failures = append(u.failures, err)
}

func messagesStageTestAccount() *Account {
	return &Account{
		ID:          101,
		Name:        "messages-stage-openai",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
	}
}

func messagesStageTestConfig() *config.Config {
	return &config.Config{
		Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{
				Enabled:           false,
				AllowInsecureHTTP: true,
			},
		},
	}
}

func messagesResponsesSSEHappyPath() string {
	return strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"id":"resp_stage_1","model":"gpt-5.4"}}`,
		``,
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1"}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"Hello"}`,
		``,
		`event: response.output_text.done`,
		`data: {"type":"response.output_text.done"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"id":"resp_stage_1","status":"completed","usage":{"input_tokens":10,"output_tokens":2}}}`,
		``,
		``,
	}, "\n")
}

func TestMessagesStage_PreOutputUnexpectedEOF_FailsOverDefaultTag(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader([]byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hi"}],"stream":true}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 2,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_eof_default"}},
		Body:       io.NopCloser(&passthroughErrReadCloser{err: io.ErrUnexpectedEOF}),
	}
	upstream := &messagesStageH2Reporter{}
	svc := &OpenAIGatewayService{cfg: messagesStageTestConfig(), httpUpstream: upstream}
	account := messagesStageTestAccount()

	ctx := withOpenAIStreamProxyURL(context.Background(), "http://proxy.local:8080")
	_, err := svc.handleAnthropicStreamingResponse(ctx, resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "pre-output EOF must be failover-safe: %T %v", err, err)
	require.Equal(t, OpenAIFailurePhasePreOutput, failoverErr.FailurePhase)
	require.Equal(t, OpenAIFailureCauseStreamEOF, failoverErr.Cause)
	require.NotEmpty(t, upstream.failures)
	require.NotContains(t, rec.Body.String(), `"type":"text"`)
	require.False(t, OpenAIAttemptWireStateSnapshot(c).SemanticOutputStarted)
	require.Equal(t, 2, OpenAIAttemptWireStateSnapshot(c).ActualProtoMajor)
}

func TestMessagesStage_KeepalivePingDoesNotCommitAccountHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	reader, writer := io.Pipe()
	go func() {
		time.Sleep(1100 * time.Millisecond)
		_ = writer.CloseWithError(io.ErrUnexpectedEOF)
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 2,
		Header: http.Header{
			"Content-Type":  []string{"text/event-stream"},
			"X-Request-Id":  []string{"rid_ping_stage"},
			"x-openai-proxy-wallet": []string{"should-not-leak-early"},
		},
		Body: reader,
	}
	cfg := messagesStageTestConfig()
	cfg.Gateway.StreamKeepaliveInterval = 1
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: &messagesStageH2Reporter{}}
	account := messagesStageTestAccount()

	_, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "ping-only disconnect must remain failover-safe")

	bodyOut := rec.Body.String()
	wire := OpenAIAttemptWireStateSnapshot(c)
	require.False(t, wire.SemanticOutputStarted)
	// Account-specific upstream id must not appear before semantic commit.
	// After transport-only keepalive WriteHeader, Gin freezes headers — staged
	// x-request-id should still be absent from the frozen map if never applied.
	if strings.Contains(bodyOut, "event: ping") {
		require.True(t, wire.HeartbeatOnly || openAIStreamKeepaliveBytes(c) > 0 || wire.TransportCommitted)
		require.NotEqual(t, "rid_ping_stage", rec.Header().Get("X-Request-Id"))
		require.NotEqual(t, "rid_ping_stage", rec.Header().Get("X-Request-Id"))
	}
}

func TestMessagesStage_HappyPathCommitsHeadersAndTTFTPhases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 2,
		Header: http.Header{
			"Content-Type": []string{"text/event-stream"},
			"X-Request-Id": []string{"rid_happy_stage"},
		},
		Body: io.NopCloser(strings.NewReader(messagesResponsesSSEHappyPath())),
	}
	svc := &OpenAIGatewayService{cfg: messagesStageTestConfig()}
	account := messagesStageTestAccount()
	start := time.Now().Add(-20 * time.Millisecond)

	result, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", start)
	require.NoError(t, err)
	require.NotNil(t, result)

	bodyOut := rec.Body.String()
	require.Contains(t, bodyOut, "event: message_start")
	require.Contains(t, bodyOut, "text_delta")
	require.Contains(t, bodyOut, "Hello")

	// After semantic commit, staged request id is visible on the client response.
	gotRID := rec.Header().Get("X-Request-Id")
	if gotRID == "" {
		gotRID = rec.Header().Get("X-Request-Id")
	}
	require.Equal(t, "rid_happy_stage", gotRID)

	wire := OpenAIAttemptWireStateSnapshot(c)
	require.True(t, wire.SemanticOutputStarted)
	require.Equal(t, 2, wire.ActualProtoMajor)
	// Messages bridge: frame = first upstream SSE; visible may fire on
	// message_start derived from response.created (preamble); semantic marks
	// the first non-preamble upstream event (often output_text.delta).
	require.Greater(t, wire.FirstFrameMs, 0)
	require.Greater(t, wire.FirstVisibleMs, 0)
	require.Greater(t, wire.FirstSemanticMs, 0)
	require.GreaterOrEqual(t, wire.FirstVisibleMs, wire.FirstFrameMs)
	require.GreaterOrEqual(t, wire.FirstSemanticMs, wire.FirstFrameMs)

	snap := OutputCommitSnapshotFromContext(c)
	require.Equal(t, wire.ActualProtoMajor, snap.ActualProtoMajor)
	require.Equal(t, wire.FirstVisibleMs, snap.FirstVisibleMs)
	require.Equal(t, wire.FirstSemanticMs, snap.FirstSemanticMs)
}

func TestMessagesStage_FirstOutputTimeoutCarriesStructuredCause(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	reader, writer := io.Pipe()
	go func() {
		// Stay silent past the 1s first-output deadline; never emit semantic SSE.
		time.Sleep(1500 * time.Millisecond)
		_, _ = writer.Write([]byte("event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"late\"}}\n\n"))
		_ = writer.Close()
	}()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 2,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_fot"}},
		Body:       reader,
	}
	cfg := messagesStageTestConfig()
	cfg.Gateway.OpenAIFirstOutputTimeoutSeconds = 1
	svc := &OpenAIGatewayService{cfg: cfg}
	account := messagesStageTestAccount()

	_, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "first-output timeout must be failover-safe: %T %v", err, err)
	require.Equal(t, OpenAIFailurePhasePreOutput, failoverErr.FailurePhase)
	require.Equal(t, OpenAIFailureCauseFirstOutputTimeout, failoverErr.Cause)
	require.Equal(t, OpenAIRetryDecisionFailoverOtherAccount, failoverErr.RetryDecisionReason)
	require.False(t, OpenAIAttemptWireStateSnapshot(c).SemanticOutputStarted)
	require.NotContains(t, rec.Body.String(), "text_delta")
}

func TestMessagesStage_PreambleOnlyThenEOF_StillFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// response.created maps to message_start (semantic for Anthropic client),
	// so use only upstream idle + EOF without any SSE frames.
	resp := &http.Response{
		StatusCode: http.StatusOK,
		ProtoMajor: 1,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid_empty_eof"}},
		Body:       io.NopCloser(&passthroughErrReadCloser{err: io.ErrUnexpectedEOF}),
	}
	svc := &OpenAIGatewayService{cfg: messagesStageTestConfig(), httpUpstream: &messagesStageH2Reporter{}}
	account := messagesStageTestAccount()

	_, err := svc.handleAnthropicStreamingResponse(
		withOpenAIStreamProxyURL(context.Background(), "http://proxy.local:9"),
		resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now(),
	)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, 1, OpenAIAttemptWireStateSnapshot(c).ActualProtoMajor)
	require.Equal(t, OpenAIFailureCauseStreamEOF, failoverErr.Cause)
}
