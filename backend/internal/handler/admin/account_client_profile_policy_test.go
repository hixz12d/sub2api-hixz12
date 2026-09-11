package admin

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestClientProfilePolicyEndpointIsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &AccountHandler{}
	router := gin.New()
	router.POST("/profiles/:family/policy", h.SetClientProfileUpdatePolicy)
	for _, test := range []struct {
		family, body string
		status       int
	}{
		{"pi", `{"action":"hold","extra":"must-not-echo"}`, http.StatusBadRequest},
		{"pi", `{"action":"hold"} {}`, http.StatusBadRequest},
		{"pi", strings.Repeat("x", 4097), http.StatusBadRequest},
		{"unknown", `{"action":"hold"}`, http.StatusBadRequest},
		{"pi", `{"action":"hold"}`, http.StatusServiceUnavailable},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/profiles/"+test.family+"/policy", bytes.NewBufferString(test.body)))
		require.Equal(t, test.status, response.Code, response.Body.String())
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		require.NotContains(t, response.Body.String(), "must-not-echo")
	}
}
