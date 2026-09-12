package admin

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type readbackHandlerStub struct {
	oauthSyncHandlerStub
	reads int
}

func (s *readbackHandlerStub) ReadOAuthSyncAccessToken(_ context.Context, id int64, instance string, version int64, stamp string) (map[string]any, error) {
	s.reads++
	return map[string]any{"access_token": "fixture-readback-AT", "remote_account_id": id, "instance_id": instance, "credential_version": version}, nil
}
func TestOAuthSyncAccessTokenHandlerRequiresBridgeKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &readbackHandlerStub{}
	h := &AccountHandler{oauthSync: stub}
	for _, method := range []string{"", "jwt", "admin_api_key"} {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("auth_method", method); c.Next() })
		r.POST("/accounts/:id/credential-sync-access-token", h.ReadOAuthSyncAccessToken)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts/42/credential-sync-access-token", bytes.NewBufferString(`{"expected_instance_id":"fixture-instance","expected_credential_version":7,"expected_updated_at":"2026-09-11T00:00:00Z"}`)))
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
		if method == "admin_api_key" {
			require.Equal(t, 200, rec.Code)
		} else {
			require.Equal(t, 403, rec.Code)
			require.NotContains(t, rec.Body.String(), "fixture-readback-AT")
		}
	}
	require.Equal(t, 1, stub.reads)
}
