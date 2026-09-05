//go:build unit

package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthRecoveryErrorsReturnFixedSafeResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		reason  service.GatewayFailureReason
		status  int
		message string
	}{
		{service.OpenAIOAuthRefreshFailedReason, http.StatusServiceUnavailable, service.OpenAIOAuthUnavailableClientMessage},
		{service.OpenAIConversationRecoveryRequiredReason, http.StatusConflict, service.OpenAIConversationRecoveryClientMessage},
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		failure := &service.UpstreamFailoverError{Stage: service.GatewayFailureStageAccountAuth, Reason: tc.reason,
			NextAccountAction: service.NextAccountStop, ClientStatusCode: http.StatusTeapot,
			ClientMessage: "refresh_token=secret", ResponseBody: []byte(`{"error":{"message":"secret"}}`)}
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, failure, false)
		require.Equal(t, tc.status, recorder.Code)
		require.Contains(t, recorder.Body.String(), tc.message)
		require.NotContains(t, recorder.Body.String(), "secret")
		require.NotContains(t, recorder.Body.String(), "Grok")
	}
}
