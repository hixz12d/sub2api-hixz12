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

func TestOpenAIAccountScopedIdentityDerivation(t *testing.T) {
	const (
		accountID   = int64(101)
		apiKeyID    = int64(202)
		rawSession  = "client-session"
		rawWindowID = "client-window"
	)

	sessionID := deriveOpenAIAccountScopedSessionID(accountID, apiKeyID, rawSession)
	require.Equal(t, sessionID, deriveOpenAIAccountScopedSessionID(accountID, apiKeyID, rawSession))
	require.NotEqual(t, sessionID, deriveOpenAIAccountScopedSessionID(accountID+1, apiKeyID, rawSession))
	require.NotContains(t, sessionID, rawSession)
	require.Empty(t, deriveOpenAIAccountScopedSessionID(accountID, apiKeyID, ""))

	windowID := deriveOpenAIAccountScopedWindowID(accountID, apiKeyID, rawSession, rawWindowID)
	require.Equal(t, windowID, deriveOpenAIAccountScopedWindowID(accountID, apiKeyID, rawSession, rawWindowID))
	require.NotEqual(t, windowID, deriveOpenAIAccountScopedWindowID(accountID+1, apiKeyID, rawSession, rawWindowID))
	require.NotContains(t, windowID, rawWindowID)
	require.Empty(t, deriveOpenAIAccountScopedWindowID(accountID, apiKeyID, rawSession, ""))
}

func TestOpenAIAccountScopedIdentityDoesNotChangeLocalSchedulingHash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session_id", "local-scheduler-session")
	c.Set("api_key", &APIKey{ID: 303})
	body := []byte(`{"model":"gpt-5.4","input":"hello"}`)

	disabled := (&OpenAIGatewayService{}).GenerateSessionHash(c, body)
	enabled := (&OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(true)}).GenerateSessionHash(c, body)
	require.NotEmpty(t, disabled)
	require.Equal(t, disabled, enabled)
}

func TestOpenAIAccountScopedIdentityGateSupportsAccountCanary(t *testing.T) {
	account := accountScopedIdentityTestAccount(401, "device-401")
	require.False(t, (&OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(false)}).isOpenAIAccountScopedIdentityEnabled(account))

	account.Extra[openAIAccountScopedIdentityExtraKey] = true
	require.True(t, (&OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(false)}).isOpenAIAccountScopedIdentityEnabled(account))

	account.Extra[openAIAccountScopedIdentityExtraKey] = false
	require.False(t, (&OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(true)}).isOpenAIAccountScopedIdentityEnabled(account))
}

func TestOpenAIAccountScopedIdentityGateExcludesNonOAuthAuthModes(t *testing.T) {
	svc := &OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(true)}
	require.False(t, svc.isOpenAIAccountScopedIdentityEnabled(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}))
	require.False(t, svc.isOpenAIAccountScopedIdentityEnabled(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModePersonalAccessToken,
		},
	}))
	require.False(t, svc.isOpenAIAccountScopedIdentityEnabled(&Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			openAIAuthModeCredentialKey: OpenAIAuthModeAgentIdentity,
		},
	}))
}

func TestOpenAIAccountScopedIdentityHTTPBuilders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		apiKeyID        = int64(501)
		rawSession      = "client-session-http"
		rawConversation = "client-conversation-http"
		rawHeaderWindow = "client-window-http"
		rawBodyWindow   = "client-window-body"
		clientInstall   = "client-installation-http"
		deviceID        = "account-device-http"
		promptCacheKey  = "prompt-cache-http"
	)

	tests := []struct {
		name        string
		path        string
		promptCache string
		bridge      bool
	}{
		{name: "responses", path: "/v1/responses", promptCache: promptCacheKey},
		{name: "messages bridge", path: "/v1/messages", promptCache: promptCacheKey, bridge: true},
		{name: "chat completions bridge", path: "/v1/chat/completions", promptCache: promptCacheKey},
		{name: "compact", path: "/v1/responses/compact"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, tt.path, nil)
			c.Request.Header.Set("session_id", rawSession)
			c.Request.Header.Set("conversation_id", rawConversation)
			c.Request.Header.Set(openAICodexWindowIDHeader, rawHeaderWindow)
			c.Request.Header.Set(openAICodexInstallationIDHeader, clientInstall)
			c.Set("api_key", &APIKey{ID: apiKeyID})
			if tt.bridge {
				setOpenAICompatMessagesBridgeContext(c, true)
			}

			account := accountScopedIdentityTestAccount(502, deviceID)
			svc := &OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(true)}
			body := []byte(`{"model":"gpt-5.4","input":[],"client_metadata":{"x-codex-installation-id":"` + clientInstall + `","x-codex-window-id":"` + rawBodyWindow + `"}}`)
			req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-token", true, tt.promptCache, true)
			require.NoError(t, err)

			identitySession := tt.promptCache
			if identitySession == "" {
				identitySession = rawSession
			}
			require.Equal(t, deviceID, req.Header.Get(openAICodexInstallationIDHeader))
			require.Equal(t, deriveOpenAIAccountScopedWindowID(account.ID, apiKeyID, identitySession, rawHeaderWindow), req.Header.Get(openAICodexWindowIDHeader))
			require.NotEqual(t, rawSession, req.Header.Get(codexSessionHeader))
			requestBody, readErr := io.ReadAll(req.Body)
			require.NoError(t, readErr)
			require.Equal(t, deviceID, gjson.GetBytes(requestBody, "client_metadata.x-codex-installation-id").String())
			require.Equal(t, deriveOpenAIAccountScopedWindowID(account.ID, apiKeyID, identitySession, rawBodyWindow), gjson.GetBytes(requestBody, "client_metadata.x-codex-window-id").String())
			require.NotContains(t, string(requestBody), clientInstall)
		})
	}
}

func TestOpenAIAccountScopedIdentityDisabledPreservesExistingOutboundBehavior(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session_id", "client-session-disabled")
	c.Request.Header.Set(openAICodexWindowIDHeader, "client-window-disabled")
	c.Request.Header.Set(openAICodexInstallationIDHeader, "client-installation-disabled")
	c.Set("api_key", &APIKey{ID: 601})

	account := accountScopedIdentityTestAccount(602, "account-device-disabled")
	svc := &OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(false)}
	body := []byte(`{"model":"gpt-5.4","client_metadata":{"x-codex-installation-id":"client-body-installation-disabled","x-codex-window-id":"client-body-window-disabled"}}`)
	req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-token", true, "cache-disabled", true)
	require.NoError(t, err)
	require.Equal(t, isolateOpenAISessionID(601, "client-session-disabled"), req.Header.Get(codexSessionHeader))
	require.Equal(t, "client-window-disabled", req.Header.Get(openAICodexWindowIDHeader))
	require.Equal(t, "client-installation-disabled", req.Header.Get(openAICodexInstallationIDHeader))
	requestBody, readErr := io.ReadAll(req.Body)
	require.NoError(t, readErr)
	require.Equal(t, isolateOpenAISessionID(601, "cache-disabled"), gjson.GetBytes(requestBody, "prompt_cache_key").String())
	require.Equal(t, "client-body-installation-disabled", gjson.GetBytes(requestBody, "client_metadata.x-codex-installation-id").String())
	require.Equal(t, "client-body-window-disabled", gjson.GetBytes(requestBody, "client_metadata.x-codex-window-id").String())
}

func TestOpenAIAccountScopedIdentityMissingDeviceOmitsClientInstallation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	build := func() (*http.Request, []byte) {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		c.Request.Header.Set("session_id", "missing-device-session")
		c.Request.Header.Set(openAICodexInstallationIDHeader, "client-installation-must-not-pass")
		c.Set("api_key", &APIKey{ID: 701})
		account := accountScopedIdentityTestAccount(702, "")
		svc := &OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(true)}
		body := []byte(`{"model":"gpt-5.4","client_metadata":{"x-codex-installation-id":"client-body-installation-must-not-pass"}}`)
		req, err := svc.buildUpstreamRequest(context.Background(), c, account, body, "oauth-token", true, "missing-device-cache", true)
		require.NoError(t, err)
		requestBody, readErr := io.ReadAll(req.Body)
		require.NoError(t, readErr)
		return req, requestBody
	}

	firstReq, firstBody := build()
	secondReq, secondBody := build()
	require.Empty(t, firstReq.Header.Get(openAICodexInstallationIDHeader))
	require.False(t, gjson.GetBytes(firstBody, "client_metadata.x-codex-installation-id").Exists())
	require.Empty(t, firstReq.Header.Get(openAICodexWindowIDHeader))
	require.False(t, gjson.GetBytes(firstBody, "client_metadata.x-codex-window-id").Exists())
	require.Equal(t, firstReq.Header.Get(codexSessionHeader), secondReq.Header.Get(codexSessionHeader))
	require.Equal(t, string(firstBody), string(secondBody))
}

func TestOpenAIAccountScopedIdentityPassthroughTransport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.150.0")
	c.Request.Header.Set("session_id", "passthrough-session")
	c.Request.Header.Set("conversation_id", "passthrough-conversation")
	c.Request.Header.Set(openAICodexWindowIDHeader, "passthrough-window")
	c.Request.Header.Set(openAICodexInstallationIDHeader, "passthrough-client-installation")
	c.Set("api_key", &APIKey{ID: 801})

	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_identity_http", "gpt-5.4")}
	svc := &OpenAIGatewayService{cfg: accountScopedIdentityTestConfig(true), httpUpstream: upstream}
	account := accountScopedIdentityTestAccount(802, "passthrough-account-device")
	account.Extra["openai_passthrough"] = true
	body := []byte(`{"model":"gpt-5.4","instructions":"reply","stream":false,"input":[],"client_metadata":{"x-codex-installation-id":"passthrough-body-installation","x-codex-window-id":"passthrough-body-window"}}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, account.GetOpenAIDeviceID(), upstream.lastReq.Header.Get(openAICodexInstallationIDHeader))
	require.Equal(t, deriveOpenAIAccountScopedSessionID(account.ID, 801, "passthrough-session"), upstream.lastReq.Header.Get(codexSessionHeader))
	require.Equal(t, account.GetOpenAIDeviceID(), gjson.GetBytes(upstream.lastBody, "client_metadata.x-codex-installation-id").String())
	require.NotContains(t, string(upstream.lastBody), "passthrough-body-installation")
}

func TestOpenAIAccountScopedIdentityWSV2OutboundMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.150.0")
	c.Request.Header.Set("session_id", "wsv2-session")
	c.Request.Header.Set("conversation_id", "wsv2-conversation")
	c.Request.Header.Set(openAICodexWindowIDHeader, "wsv2-window")
	c.Request.Header.Set(openAICodexInstallationIDHeader, "wsv2-client-installation")
	c.Set("api_key", &APIKey{ID: 901})

	cfg := accountScopedIdentityTestConfig(true)
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_identity_wsv2","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	defer pool.Close()
	pool.setClientDialerForTest(captureDialer)

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := accountScopedIdentityTestAccount(902, "wsv2-account-device")
	account.Extra["responses_websockets_v2_enabled"] = true
	body := []byte(`{"model":"gpt-5.4","stream":true,"input":[],"client_metadata":{"x-codex-installation-id":"wsv2-body-installation","x-codex-window-id":"wsv2-body-window"}}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, deriveOpenAIAccountScopedSessionID(account.ID, 901, "wsv2-session"), captureDialer.lastHeaders.Get(codexSessionHeader))
	require.Empty(t, captureDialer.lastHeaders.Get(legacyCodexSessionHeader))
	require.Equal(t, account.GetOpenAIDeviceID(), captureDialer.lastHeaders.Get(openAICodexInstallationIDHeader))
	require.Equal(t, deriveOpenAIAccountScopedWindowID(account.ID, 901, "wsv2-session", "wsv2-window"), captureDialer.lastHeaders.Get(openAICodexWindowIDHeader))

	requestJSON := requestToJSONString(captureConn.lastWrite)
	require.Equal(t, account.GetOpenAIDeviceID(), gjson.Get(requestJSON, "client_metadata.x-codex-installation-id").String())
	require.Equal(t, deriveOpenAIAccountScopedWindowID(account.ID, 901, "wsv2-session", "wsv2-body-window"), gjson.Get(requestJSON, "client_metadata.x-codex-window-id").String())
	require.NotContains(t, requestJSON, "wsv2-body-installation")
}

func accountScopedIdentityTestConfig(enabled bool) *config.Config {
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Gateway.OpenAIAccountScopedIdentity.Enabled = enabled
	return cfg
}

func accountScopedIdentityTestAccount(id int64, deviceID string) *Account {
	extra := map[string]any{}
	if deviceID != "" {
		extra["openai_device_id"] = deviceID
	}
	if _, ok := extra[codexFingerprintModeExtraKey]; !ok {
		extra[codexFingerprintModeExtraKey] = string(codexFingerprintOff)
	}
	return &Account{
		ID:          id,
		Name:        "account-scoped-identity-test",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "chatgpt-account",
		},
		Extra: extra,
	}
}
