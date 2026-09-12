package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOAuthCredentialSyncAuditOmitsEntireBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &auditCaptureRepository{}
	audit := service.NewAuditLogService(repository, nil)
	audit.Start()
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(audit)))
	router.POST("/api/v1/admin/accounts/:id/sync-oauth-credentials", func(c *gin.Context) { c.Status(http.StatusBadRequest) })
	router.POST("/api/v1/admin/accounts/:id/credential-sync-access-token", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"access_token": "fixture-secret"}) })
	router.POST("/api/v1/admin/accounts/:id/credential-refresh-handoff", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"refresh_token": "fixture-secret"}) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/sync-oauth-credentials", bytes.NewBufferString(`{"credentials":{"access_token":"fixture-secret"},"unexpected":"fixture-secret"}`)))
	readback := httptest.NewRecorder()
	router.ServeHTTP(readback, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/credential-sync-access-token", bytes.NewBufferString(`{"unexpected":"fixture-secret"}`)))
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/42/credential-refresh-handoff", bytes.NewBufferString(`{"unexpected":"fixture-secret"}`)))
	audit.Stop()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	require.Len(t, repository.logs, 3)
	for _, entry := range repository.logs {
		require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
		require.NotContains(t, entry.RequestBody, "fixture-secret")
	}
}
