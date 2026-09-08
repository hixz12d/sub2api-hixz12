//go:build unit

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

type messagesH2StreamReporter struct {
	httpUpstreamRecorder
	failures []error
}

func (u *messagesH2StreamReporter) RecordOpenAIHTTP2StreamFailure(proxyURL string, err error) {
	u.failures = append(u.failures, err)
}

func TestHandleAnthropicStreamingResponse_PreOutputUnexpectedEOF_FailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","max_tokens":32,"messages":[{"role":"user","content":"hello"}],"stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_ping_eof"}},
		Body:       io.NopCloser(&passthroughErrReadCloser{err: io.ErrUnexpectedEOF}),
	}
	upstream := &messagesH2StreamReporter{}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := rawChatCompletionsTestAccount()

	ctx := withOpenAIStreamProxyURL(context.Background(), "http://proxy.local:8080")
	_, err := svc.handleAnthropicStreamingResponse(ctx, resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())

	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "pre-output scanner EOF must be failover-safe, got %T: %v", err, err)
	require.NotEmpty(t, upstream.failures, "messages stream EOF must feed H2 stream failure reporter")
	require.NotContains(t, rec.Body.String(), `"type":"text"`)
}

func TestHandleAnthropicStreamingResponse_KeepalivePingDoesNotCloseRecovery(t *testing.T) {
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
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_ping"}},
		Body:       reader,
	}
	upstream := &messagesH2StreamReporter{}
	cfg := rawChatCompletionsTestConfig()
	cfg.Gateway = config.GatewayConfig{StreamKeepaliveInterval: 1}
	svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream}
	account := rawChatCompletionsTestAccount()

	_, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, account, "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr), "ping-only disconnect must remain failover-safe: %T %v", err, err)
	bodyOut := rec.Body.String()
	if strings.Contains(bodyOut, "event: ping") {
		require.False(t, OpenAIAttemptWireStateSnapshot(c).SemanticOutputStarted)
		require.True(t, openAIStreamKeepaliveBytes(c) > 0 || OpenAIAttemptWireStateSnapshot(c).HeartbeatOnly)
	}
}

func TestOpenAIRetryBudgetAllowsSecondAttemptAfterFirstOutputWindow(t *testing.T) {
	cfg := &config.Config{Gateway: config.GatewayConfig{OpenAIPreoutputRecoveryMode: "bounded_preoutput"}}
	budget := newOpenAIRetryBudget(false, false, openAIRetryBudgetMaxElapsed(cfg))
	require.NoError(t, budget.Reserve(11))
	budget.mu.Lock()
	budget.startedAt = time.Now().Add(-35 * time.Second)
	budget.mu.Unlock()
	require.NoError(t, budget.Reserve(11), "second attempt must remain admissible after 30s first-output wait")
}
