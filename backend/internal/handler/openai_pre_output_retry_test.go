package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestWaitOpenAIPreOutputAutoRetryOnlyRetriesUncommittedCapacity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	firstRecorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(firstRecorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	failoverErr := &service.UpstreamFailoverError{
		StatusCode: http.StatusBadGateway,
		// The stream error wrapper intentionally hides the upstream code from
		// the client; the preserved internal reason must still trigger retry.
		ResponseBody: []byte(`{"error":{"type":"upstream_error","message":"Please retry later."}}`),
		Reason:       service.OpenAIAttemptFailureReasonCapacity,
	}
	writerBefore := service.OpenAICompactKeepaliveAdjustedWrittenSize(c)

	require.True(t, waitOpenAIPreOutputAutoRetry(c, nil, failoverErr, writerBefore, 0, 1))
	require.Empty(t, firstRecorder.Body.String())

	secondRecorder := httptest.NewRecorder()
	c, _ = gin.CreateTestContext(secondRecorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	writerBefore = service.OpenAICompactKeepaliveAdjustedWrittenSize(c)
	_, _ = c.Writer.Write([]byte("data: semantic output\n\n"))
	require.False(t, waitOpenAIPreOutputAutoRetry(c, nil, failoverErr, writerBefore, 0, 1))
	require.False(t, waitOpenAIPreOutputAutoRetry(c, nil, failoverErr, writerBefore, 1, 1))
}

func TestIsOpenAIPoolRetrySessionHash(t *testing.T) {
	require.True(t, isOpenAIPoolRetrySessionHash("openai-pool-retry-abc"))
	require.False(t, isOpenAIPoolRetrySessionHash("real-session-hash"))
	require.False(t, isOpenAIPoolRetrySessionHash(""))
}
