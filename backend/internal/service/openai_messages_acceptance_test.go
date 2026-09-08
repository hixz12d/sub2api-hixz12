package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type messagesAcceptanceFailWriter struct {
	gin.ResponseWriter
	accepted int
}

func (w *messagesAcceptanceFailWriter) Write(p []byte) (int, error) {
	n := min(w.accepted, len(p))
	if n > 0 {
		_, _ = w.ResponseWriter.Write(p[:n])
	}
	return n, io.ErrClosedPipe
}

func TestMessagesAcceptanceEmptyCompleted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	payload := "data: {\"type\":\"response.created\",\"response\":{\"id\":\"empty_response\"}}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"empty_response\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":0}}}\n\n"
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(payload))}
	svc := &OpenAIGatewayService{cfg: messagesStageTestConfig()}
	result, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, messagesStageTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now())
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), "event: message_start")
	require.Contains(t, rec.Body.String(), "event: message_stop")
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Nil(t, result.FirstTokenMs)
}

func TestMessagesAcceptanceFailedWriteDoesNotRecordVisibleTTFT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, accepted := range []int{0, 9} {
		t.Run(string(rune('0'+accepted)), func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Writer = &messagesAcceptanceFailWriter{ResponseWriter: c.Writer, accepted: accepted}
			resp := &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(messagesResponsesSSEHappyPath()))}
			svc := &OpenAIGatewayService{cfg: messagesStageTestConfig()}
			result, err := svc.handleAnthropicStreamingResponse(context.Background(), resp, c, messagesStageTestAccount(), "gpt-5.4", "gpt-5.4", "gpt-5.4", time.Now().Add(-time.Second))
			var failover *UpstreamFailoverError
			require.False(t, errors.As(err, &failover), "a attempted public write must never trigger replay")
			require.True(t, result.ClientDisconnect)
			require.Equal(t, 10, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.Nil(t, result.FirstTokenMs)
			require.Zero(t, OpenAIAttemptWireStateSnapshot(c).FirstVisibleMs)
		})
	}
}
