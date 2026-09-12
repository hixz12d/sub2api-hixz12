package admin

import (
	"bytes"
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

type handoffHandlerStub struct {
	oauthSyncHandlerStub
	calls int
}

func (s *handoffHandlerStub) RefreshHandoff(_ context.Context, _ string, _ int64, _ *service.OpenAIRefreshHandoffRequest) (map[string]any, error) {
	s.calls++
	return map[string]any{"credentials": map[string]string{"refresh_token": "fixture-handoff-RT"}}, nil
}
func TestOpenAIRefreshHandoffRequiresBridgeKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &handoffHandlerStub{}
	handler := &AccountHandler{oauthSync: stub}
	for _, method := range []string{"", "jwt", "admin_api_key"} {
		router := gin.New()
		router.Use(func(c *gin.Context) { c.Set("auth_method", method); c.Next() })
		router.POST("/accounts/:id/credential-refresh-handoff", handler.OAuthRefreshHandoff)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/accounts/42/credential-refresh-handoff", bytes.NewBufferString(`{"action":"read","operation_id":"fixture-handoff"}`)))
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		if method == "admin_api_key" {
			require.Equal(t, 200, response.Code)
		} else {
			require.Equal(t, 403, response.Code)
			require.NotContains(t, response.Body.String(), "fixture-handoff-RT")
		}
	}
	require.Equal(t, 1, stub.calls)
}
