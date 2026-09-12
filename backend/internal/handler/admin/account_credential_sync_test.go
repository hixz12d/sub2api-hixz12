package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type oauthSyncHandlerStub struct {
	writes, retries int
	scope, instance string
}

func (s *oauthSyncHandlerStub) InstanceID(context.Context) (string, error) {
	return "fixture-instance", nil
}
func (s *oauthSyncHandlerStub) Submit(_ context.Context, scope string, id int64, req *service.SyncOAuthCredentialsRequest) (*service.OAuthSyncOperation, bool, error) {
	s.writes++
	s.scope = scope
	s.instance = req.ExpectedInstanceID
	return &service.OAuthSyncOperation{AccountID: id, OperationID: req.OperationID, State: "pending"}, false, nil
}
func (s *oauthSyncHandlerStub) Operation(_ context.Context, scope string, id int64, key string) (*service.OAuthSyncOperation, error) {
	s.scope = scope
	return &service.OAuthSyncOperation{AccountID: id, OperationID: key, State: "pending"}, nil
}
func (s *oauthSyncHandlerStub) Receipt(_ context.Context, op *service.OAuthSyncOperation) map[string]any {
	return map[string]any{"operation_id": op.OperationID, "credential_write": "succeeded", "state": "pending", "partial": true}
}
func (s *oauthSyncHandlerStub) Snapshot(_ context.Context, scope string, id int64) (map[string]any, error) {
	s.scope = scope
	return map[string]any{"remote_account_id": id, "instance_id": "fixture-instance", "remaining_blockers": []string{"schedulable_off"}}, nil
}
func (s *oauthSyncHandlerStub) Retry(_ context.Context, scope string, id int64, key, instance string) (*service.OAuthSyncOperation, error) {
	s.retries++
	s.scope = scope
	s.instance = instance
	return &service.OAuthSyncOperation{AccountID: id, OperationID: key, State: "pending"}, nil
}
func oauthSyncRouter(stub *oauthSyncHandlerStub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &AccountHandler{oauthSync: stub}
	r := gin.New()
	r.POST("/accounts/:id/sync-oauth-credentials", h.SyncOAuthCredentials)
	r.GET("/accounts/:id/credential-sync-operations/:operation_id", h.GetOAuthSyncOperation)
	r.POST("/accounts/:id/credential-sync-operations/:operation_id/retry", h.RetryOAuthSyncOperation)
	r.GET("/accounts/:id/credential-sync-state", h.GetOAuthSyncState)
	r.GET("/integration/capabilities", h.IntegrationCapabilities)
	return r
}
func TestOAuthCredentialSyncHandlerDurableReceiptAndReadOnlyState(t *testing.T) {
	stub := &oauthSyncHandlerStub{}
	router := oauthSyncRouter(stub)
	body := `{"contract_version":1,"operation_id":"fixture-operation","expected_instance_id":"fixture-instance","credentials":{"access_token":"fixture-secret"}}`
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts/42/sync-oauth-credentials", bytes.NewBufferString(body)))
	require.Equal(t, 200, rec.Code)
	require.Equal(t, "fixture-instance", stub.instance)
	require.Equal(t, "oauth-sync:admin:0:42", stub.scope)
	require.NotContains(t, rec.Body.String(), "fixture-secret")
	for _, path := range []string{"/accounts/42/credential-sync-state", "/accounts/42/credential-sync-operations/fixture-operation", "/integration/capabilities"} {
		rec = httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, 200, rec.Code)
		require.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
	}
	require.Equal(t, 1, stub.writes)
	require.Zero(t, stub.retries)
}
func TestOAuthCredentialSyncHandlerRetryAcceptsOnlyInstancePrecondition(t *testing.T) {
	stub := &oauthSyncHandlerStub{}
	router := oauthSyncRouter(stub)
	for _, body := range []string{`{"credentials":{"access_token":"fixture-secret"}}`, `{"expected_instance_id":"fixture-instance"} {}`} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts/42/credential-sync-operations/fixture-operation/retry", bytes.NewBufferString(body)))
		require.Equal(t, 400, rec.Code)
		require.NotContains(t, rec.Body.String(), "fixture-secret")
	}
	require.Zero(t, stub.retries)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts/42/credential-sync-operations/fixture-operation/retry", bytes.NewBufferString(`{"expected_instance_id":"fixture-instance"}`)))
	require.Equal(t, 200, rec.Code)
	require.Equal(t, 1, stub.retries)
	require.Zero(t, stub.writes)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, "recorded", payload["data"].(map[string]any)["state"])
}
func TestOAuthCredentialSyncHandlerUnavailableFailsClosed(t *testing.T) {
	router := gin.New()
	handler := &AccountHandler{}
	router.POST("/accounts/:id/sync-oauth-credentials", handler.SyncOAuthCredentials)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/accounts/42/sync-oauth-credentials", bytes.NewBufferString(`{}`)))
	require.Equal(t, 503, rec.Code)
}
