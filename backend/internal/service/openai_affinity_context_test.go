package service

import (
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAffinityUsesSeparateGinAndRequestContextKeys(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	identity := SessionIdentity{OwnerScopeHash: "owner", PrimaryHash: "session"}
	attachOpenAIAffinityIdentity(c, identity, true, true)
	fromGin, ok := openAIAffinityFromGin(c)
	require.True(t, ok)
	fromRequest, ok := openAIAffinityFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, fromGin, fromRequest)
	require.Equal(t, identity, fromRequest.Identity)
	require.Nil(t, c.Request.Context().Value(openAIAffinityContextKey))
}

func TestOpenAIAffinityPreparationPreservesConfigurationError(t *testing.T) {
	cfg := &config.Config{}
	cfg.Gateway.OpenAIAffinity.Enabled = true
	cfg.Gateway.OpenAIAffinity.WritesEnabled = true
	cfg.Gateway.OpenAIAffinity.Secret = "short"
	svc := &OpenAIGatewayService{cfg: cfg}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
	err := svc.prepareOpenAIAffinityIdentity(c, nil, "session")
	require.ErrorIs(t, err, ErrOpenAIAffinityConfiguration)
	fromGin, ok := openAIAffinityFromGin(c)
	require.True(t, ok)
	require.ErrorIs(t, fromGin.Err, ErrOpenAIAffinityConfiguration)
	fromRequest, ok := openAIAffinityFromContext(c.Request.Context())
	require.True(t, ok)
	require.Equal(t, fromGin, fromRequest)
	require.ErrorIs(t, svc.prepareOpenAIAffinityIdentity(nil, nil, "session"), ErrOpenAIAffinityConfiguration)
}
