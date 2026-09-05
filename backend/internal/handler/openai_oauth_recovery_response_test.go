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
		{service.OpenAIConversationRecoveryRequiredReason, http.StatusConflict, "The original conversation account is unavailable. Retry after it recovers, or restore the full conversation context."},
	} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		clientMessage := "refresh_token=secret"
		if tc.reason == service.OpenAIConversationRecoveryRequiredReason {
			clientMessage = tc.message
		}
		failure := &service.UpstreamFailoverError{Stage: service.GatewayFailureStageAccountAuth, Reason: tc.reason,
			NextAccountAction: service.NextAccountStop, ClientStatusCode: http.StatusTeapot,
			ClientMessage: clientMessage, ResponseBody: []byte(`{"error":{"message":"secret"}}`)}
		(&OpenAIGatewayHandler{}).handleFailoverExhausted(c, failure, false)
		require.Equal(t, tc.status, recorder.Code)
		require.Contains(t, recorder.Body.String(), tc.message)
		require.NotContains(t, recorder.Body.String(), "secret")
		require.NotContains(t, recorder.Body.String(), "Grok")
	}
}
