package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientProfileEndpointsAreBoundedAndReadOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountHandler{}
	router := gin.New()
	router.GET("/catalog", h.GetClientProfiles)
	router.POST("/preview", h.PreviewClientProfile)
	catalog, err := service.PublicCodexClientCatalog()
	require.NoError(t, err)
	request := service.CodexProfilePreviewInput{Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Operation: service.CodexOperationResponses, Transport: service.CodexTransportHTTP, CatalogRevision: catalog.Revision,
		Extra: map[string]any{service.CodexClientProfileExtraKey: service.CodexProfileCLI}}
	payload, err := json.Marshal(request)
	require.NoError(t, err)
	for _, tc := range []struct {
		method, path string
		body         []byte
		status       int
	}{
		{http.MethodGet, "/catalog", nil, http.StatusOK},
		{http.MethodPost, "/preview", payload, http.StatusOK},
		{http.MethodPost, "/preview", []byte(`{"access_token":"private-value"}`), http.StatusBadRequest},
		{http.MethodPost, "/preview", append(append([]byte{}, payload...), []byte(` {}`)...), http.StatusBadRequest},
		{http.MethodPost, "/preview", bytes.Repeat([]byte("x"), 128*1024+1), http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(tc.method, tc.path, bytes.NewReader(tc.body)))
		require.Equal(t, tc.status, response.Code, response.Body.String())
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		require.NotContains(t, response.Body.String(), "private-value")
	}
}
