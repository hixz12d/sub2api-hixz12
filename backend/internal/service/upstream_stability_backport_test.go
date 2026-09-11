//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRawChatUnsupportedWebAccessIsGrokOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, platform := range []string{PlatformGrok, PlatformOpenAI} {
		t.Run(platform, func(t *testing.T) {
			body := []byte(`{"model":"grok-4.6","messages":[{"role":"user","content":"external_web_access must remain in text"}],"external_web_access":true,"tools":[{"type":"function","function":{"name":"lookup","external_web_access":true,"parameters":{"type":"object"}}}],"stream":false}`)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
			account := rawChatCompletionsTestAccount()
			account.Platform = platform
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl","object":"chat.completion","model":"grok-4.6","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			result, err := svc.forwardAsRawChatCompletions(context.Background(), c, account, body, "")
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, upstream.lastReq)
			require.Equal(t, platform != PlatformGrok, gjson.GetBytes(upstream.lastBody, "external_web_access").Exists())
			require.Equal(t, platform != PlatformGrok, gjson.GetBytes(upstream.lastBody, "tools.0.function.external_web_access").Exists())
			require.Equal(t, "lookup", gjson.GetBytes(upstream.lastBody, "tools.0.function.name").String())
			require.Equal(t, "external_web_access must remain in text", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
		})
	}
}

func TestGeminiMonitorBodyIncludesExplicitUserRoleBackport(t *testing.T) {
	body, err := providerAdapters[MonitorProviderGemini].buildBody("gemini-3.6-flash", "Reply with only 7.")
	require.NoError(t, err)
	require.Equal(t, int64(1), gjson.GetBytes(body, "contents.#").Int())
	require.Equal(t, "user", gjson.GetBytes(body, "contents.0.role").String())
	require.Equal(t, "Reply with only 7.", gjson.GetBytes(body, "contents.0.parts.0.text").String())
}
