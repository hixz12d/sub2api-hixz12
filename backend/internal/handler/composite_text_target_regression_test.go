package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAICompatibleTextTargetPreservesCompositeRoutingBoundaries(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name, model, resolved string
		allowed               bool
	}{
		{name: "unknown", model: "llama-4-maverick"},
		{name: "anthropic", model: "claude-sonnet-4-5"},
		{name: "gemini", model: "gemini-2.5-pro"},
		{name: "explicit unsupported route", model: "gpt-5.6-sol", resolved: service.PlatformAnthropic},
		{name: "explicit kimi alias", model: "public-alias", resolved: service.PlatformKimi, allowed: true},
		{name: "explicit zhipu alias", model: "public-alias", resolved: service.PlatformZhipu, allowed: true},
		{name: "explicit deepseek alias", model: "public-alias", resolved: service.PlatformDeepseek, allowed: true},
	}
	for _, path := range []string{"/v1/messages", "/v1/chat/completions", "/v1/responses", "/v1/responses/input_tokens", "/v1/messages/count_tokens"} {
		for _, tt := range tests {
			t.Run(path+"/"+tt.name, func(t *testing.T) {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				if tt.resolved != "" {
					c.Request = c.Request.WithContext(service.WithResolvedTargetPlatform(c.Request.Context(), tt.resolved))
				}
				key := &service.APIKey{Group: &service.Group{Platform: service.PlatformComposite}}
				require.Equal(t, tt.allowed, openAICompatibleTextTargetAllowed(c, key, tt.model))
				if tt.resolved != "" {
					platform, ok := service.ResolvedTargetPlatformFromContext(c.Request.Context())
					require.True(t, ok)
					require.Equal(t, tt.resolved, platform, "explicit routing must take precedence over model-name detection")
				}
			})
		}
	}
}
