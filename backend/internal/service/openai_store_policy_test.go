package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func storePolicyAccount(enabled bool) *Account {
	return &Account{ID: 17, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": "https://codex.example/v1", "api_key": "test-key"},
		Extra:       map[string]any{openAIForceStoreFalseKey: enabled}}
}

func TestOpenAIStorePolicy(t *testing.T) {
	for _, raw := range []string{
		`{"model":"gpt-5.6-sol","input":[],"future":{"n":9007199254740993}}`,
		`{"store":true,"previous_response_id":"resp_keep","stream":false}`,
		`{"store":null,"instructions":"keep"}`, `{"store":"false"}`, `{"store":false}`,
	} {
		t.Run(raw, func(t *testing.T) {
			body, err := applyOpenAIStorePolicy([]byte(raw), storePolicyAccount(true), false)
			require.NoError(t, err)
			require.Equal(t, gjson.False, gjson.GetBytes(body, "store").Type)
			for _, field := range []string{"model", "input", "future", "previous_response_id", "stream", "instructions"} {
				require.Equal(t, gjson.Get(raw, field).Raw, gjson.GetBytes(body, field).Raw)
			}
			again, err := applyOpenAIStorePolicy(body, storePolicyAccount(true), false)
			require.NoError(t, err)
			require.Equal(t, body, again)
		})
	}
	for _, account := range []*Account{nil, storePolicyAccount(false),
		{Platform: PlatformAnthropic, Type: AccountTypeAPIKey, Extra: map[string]any{openAIForceStoreFalseKey: true}},
		{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{openAIForceStoreFalseKey: true}},
		{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{openAIForceStoreFalseKey: "true"}},
	} {
		body := []byte(`{"store":true}`)
		result, err := applyOpenAIStorePolicy(body, account, false)
		require.NoError(t, err)
		require.Equal(t, body, result)
	}
	for _, raw := range []string{`{"store":true}`, `{"input":[]}`} {
		result, err := applyOpenAIStorePolicy([]byte(raw), storePolicyAccount(true), true)
		require.NoError(t, err)
		require.Equal(t, raw, string(result))
	}
	for _, raw := range []string{`{"private":"broken`, `[]`} {
		_, err := applyOpenAIStorePolicy([]byte(raw), storePolicyAccount(true), false)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "private")
	}
}

func TestOpenAIStorePolicyHTTPBuilders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := &OpenAIGatewayService{cfg: &config.Config{}}
	for _, passthrough := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = httptest.NewRequest(http.MethodPost, path, nil)
				account := storePolicyAccount(enabled)
				body := []byte(`{"model":"gpt-5.6-sol","store":true,"input":[]}`)
				var request *http.Request
				var err error
				if passthrough {
					request, err = s.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "test-key")
				} else {
					request, err = s.buildUpstreamRequest(context.Background(), c, account, body, "test-key", false, "", false)
				}
				require.NoError(t, err)
				actual, err := io.ReadAll(request.Body)
				require.NoError(t, err)
				require.NoError(t, request.Body.Close())
				require.Equal(t, !enabled || path != "/v1/responses", gjson.GetBytes(actual, "store").Bool())
				require.Equal(t, "Bearer test-key", request.Header.Get("Authorization"))
			}
		}
	}
}

func TestOpenAIStorePolicyWebSocketCompatibility(t *testing.T) {
	for _, raw := range []string{`{"type":"response.create","model":"gpt-5.6-sol","input":[]}`, `{"type":"response.create","store":true,"input":[]}`} {
		body, changed, err := normalizeOpenAIResponsesWebSocketCompatibilityBody([]byte(raw), storePolicyAccount(true), false)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, gjson.False, gjson.GetBytes(body, "store").Type)
		require.Equal(t, "response.create", gjson.GetBytes(body, "type").String())
	}
	body, _, err := normalizeOpenAIResponsesWebSocketCompatibilityBody([]byte(`{"input":[]}`), storePolicyAccount(true), false, true)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(body, "store").Exists())
}

func TestOpenAIConnectionProbeNeverStoresResponses(t *testing.T) {
	for _, oauth := range []bool{false, true} {
		payload := createOpenAITestPayload("gpt-5.6-sol", oauth)
		require.Equal(t, false, payload["store"])
		require.Equal(t, true, payload["stream"])
		require.NotEmpty(t, payload["instructions"])
		applyAccountQuestionPayload(payload, "hi", false, oauth, "")
		require.Equal(t, false, payload["store"])
	}
}
