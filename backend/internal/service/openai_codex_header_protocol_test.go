package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newCodexHeaderProtocolContext(t *testing.T, path string, headers map[string]string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	for name, value := range headers {
		c.Request.Header.Set(name, value)
	}
	c.Set("api_key", &APIKey{ID: 71})
	return c
}

func newCodexHeaderProtocolOAuthAccount(mode codexFingerprintMode) *Account {
	return &Account{
		ID:       72,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "codex-header-protocol-account",
		},
		Extra: map[string]any{codexFingerprintModeExtraKey: string(mode)},
	}
}

func requireCodexOAuthHeaderProtocol(t *testing.T, headers http.Header, wantSession, wantThread string) {
	t.Helper()
	require.Equal(t, wantSession, headers.Get("session-id"))
	require.Empty(t, headers.Get("session_id"))
	require.Equal(t, wantThread, headers.Get("thread-id"))
	require.Empty(t, headers.Get("thread_id"))
}

func TestCodexCompatibleHeaderResolversPreferCurrentNames(t *testing.T) {
	headers := http.Header{}
	headers.Set("session-id", "current-session")
	headers.Set("session_id", "legacy-session")
	headers.Set("thread-id", "current-thread")
	headers.Set("thread_id", "legacy-thread")

	require.Equal(t, "current-session", resolveCodexSessionHeader(headers))
	require.Equal(t, "current-thread", resolveCodexThreadHeader(headers))

	headers.Del("session-id")
	headers.Del("thread-id")
	require.Equal(t, "legacy-session", resolveCodexSessionHeader(headers))
	require.Equal(t, "legacy-thread", resolveCodexThreadHeader(headers))
}

func TestCodexCLIOnlyDiagnosticSnapshotIncludesCurrentHeaders(t *testing.T) {
	headers := http.Header{}
	headers.Set(codexSessionHeader, "diagnostic-session")
	headers.Set(codexThreadHeader, "diagnostic-thread")
	snapshot := snapshotCodexCLIOnlyHeaders(headers)
	require.Equal(t, "diagnostic-session", snapshot[codexSessionHeader])
	require.Equal(t, "diagnostic-thread", snapshot[codexThreadHeader])
}

func TestOpenAIOAuthHTTPHeaderProtocolAllFingerprintModes(t *testing.T) {
	for _, mode := range []codexFingerprintMode{
		codexFingerprintOff,
		codexFingerprintDevice,
		codexFingerprintSession,
		codexFingerprintFull,
	} {
		t.Run(string(mode), func(t *testing.T) {
			c := newCodexHeaderProtocolContext(t, "/v1/responses", map[string]string{
				"session-id": "current-session",
				"session_id": "legacy-session",
				"thread-id":  "current-thread",
				"thread_id":  "legacy-thread",
			})
			account := newCodexHeaderProtocolOAuthAccount(mode)
			ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
			if ids != nil {
				c.Set("codex_fingerprint_ids", ids)
			}

			req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
				context.Background(), c, account,
				[]byte(`{"model":"gpt-5.4","input":"hello"}`), "oauth-token", true, "", true,
			)
			require.NoError(t, err)

			wantSession := isolateOpenAISessionID(71, "current-session")
			wantThread := "current-thread"
			if ids != nil && ids.sessionID != "" {
				wantSession = ids.sessionID
				wantThread = ids.threadID
			}
			requireCodexOAuthHeaderProtocol(t, req.Header, wantSession, wantThread)
		})
	}
}

func TestOpenAIOAuthCompactAndPassthroughHeaderProtocol(t *testing.T) {
	t.Run("compact ordinary", func(t *testing.T) {
		c := newCodexHeaderProtocolContext(t, "/v1/responses/compact", map[string]string{
			"session_id": "legacy-session",
			"thread_id":  "legacy-thread",
		})
		account := newCodexHeaderProtocolOAuthAccount(codexFingerprintOff)
		req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
			context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "oauth-token", false, "", true,
		)
		require.NoError(t, err)
		require.NotEmpty(t, req.Header.Get("session-id"))
		requireCodexOAuthHeaderProtocol(t, req.Header, req.Header.Get("session-id"), "legacy-thread")
	})

	t.Run("passthrough reuses fingerprint ids", func(t *testing.T) {
		c := newCodexHeaderProtocolContext(t, "/v1/responses", map[string]string{
			"session-id": "passthrough-session",
			"thread-id":  "passthrough-thread",
		})
		account := newCodexHeaderProtocolOAuthAccount(codexFingerprintSession)
		ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
		require.NotNil(t, ids)
		c.Set("codex_fingerprint_ids", ids)
		bodyMap := map[string]any{
			"model": "gpt-5.4",
			"client_metadata": map[string]any{
				"session_id": "client-body-session",
				"thread_id":  "client-body-thread",
			},
		}
		require.True(t, applyCodexFingerprintClientMetadata(bodyMap, ids))
		body, err := json.Marshal(bodyMap)
		require.NoError(t, err)

		req, err := (&OpenAIGatewayService{}).buildUpstreamRequestOpenAIPassthrough(
			context.Background(), c, account, body, "oauth-token",
		)
		require.NoError(t, err)
		requireCodexOAuthHeaderProtocol(t, req.Header, ids.sessionID, ids.threadID)
		outBody, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(outBody, &decoded))
		metadata := decoded["client_metadata"].(map[string]any)
		require.Equal(t, ids.sessionID, metadata["session_id"])
		require.Equal(t, ids.threadID, metadata["thread_id"])
		require.NotContains(t, metadata, codexSessionHeader)
		require.NotContains(t, metadata, codexThreadHeader)
	})
}

func TestOpenAIWSHeaderProtocolAndAPIKeyCompatibility(t *testing.T) {
	decision := OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}
	svc := &OpenAIGatewayService{}

	t.Run("oauth reuses fingerprint ids", func(t *testing.T) {
		c := newCodexHeaderProtocolContext(t, "/v1/responses", map[string]string{
			"session-id": "ws-session",
			"session_id": "legacy-ws-session",
			"thread-id":  "ws-thread",
			"thread_id":  "legacy-ws-thread",
		})
		account := newCodexHeaderProtocolOAuthAccount(codexFingerprintFull)
		ids := resolveCodexFingerprintIDsFromRequest(account, c.Request.Header)
		require.NotNil(t, ids)
		c.Set("codex_fingerprint_ids", ids)

		headers, resolution, err := svc.buildOpenAIWSHeaders(
			context.Background(), c, account, "oauth-token", decision, true,
			"", "", "", "gpt-5.4", "default",
		)
		require.NoError(t, err)
		require.Equal(t, "ws-session", resolution.SessionID)
		require.Equal(t, "header_session-id", resolution.SessionSource)
		requireCodexOAuthHeaderProtocol(t, headers, ids.sessionID, ids.threadID)
	})

	t.Run("api key keeps legacy upstream contract", func(t *testing.T) {
		c := newCodexHeaderProtocolContext(t, "/v1/responses", map[string]string{
			"session_id": "api-key-session",
			"thread_id":  "api-key-thread",
		})
		account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
		headers, _, err := svc.buildOpenAIWSHeaders(
			context.Background(), c, account, "api-key-token", decision, true,
			"", "", "", "gpt-5.4", "default",
		)
		require.NoError(t, err)
		require.Equal(t, "api-key-session", headers.Get("session_id"))
		require.Empty(t, headers.Get("session-id"))
	})
}

func TestOpenAIHTTPAPIKeyCustomUpstreamKeepsLegacySessionHeader(t *testing.T) {
	c := newCodexHeaderProtocolContext(t, "/v1/responses", map[string]string{
		"session_id": "custom-upstream-session",
		"thread_id":  "custom-upstream-thread",
	})
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"base_url": "https://example.com/v1",
		},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{Enabled: false},
	}}}
	req, err := svc.buildUpstreamRequest(
		context.Background(), c, account, []byte(`{"model":"gpt-5.4"}`), "api-key-token", false, "", false,
	)
	require.NoError(t, err)
	require.Equal(t, "custom-upstream-session", req.Header.Get("session_id"))
	require.Equal(t, "custom-upstream-thread", req.Header.Get("thread_id"))
	require.Empty(t, req.Header.Get("session-id"))
	require.Empty(t, req.Header.Get("thread-id"))
}

func TestOpenAIOAuthCompatibilityForwardersUseCurrentSessionHeader(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	for _, tc := range []struct {
		name    string
		forward func(*OpenAIGatewayService, *gin.Context, *Account) error
	}{
		{
			name: "chat completions",
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account) error {
				_, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "compat-cache", "gpt-5.4")
				return err
			},
		},
		{
			name: "messages",
			forward: func(svc *OpenAIGatewayService, c *gin.Context, account *Account) error {
				messagesBody := []byte(`{"model":"claude-sonnet-4-5","max_tokens":16,"messages":[{"role":"user","content":"hello"}],"stream":false}`)
				_, err := svc.ForwardAsAnthropic(context.Background(), c, account, messagesBody, "compat-cache", "gpt-5.4")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newCodexHeaderProtocolContext(t, "/v1/compat", nil)
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"stop after capture"}}`)),
			}}
			svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
			account := newCodexHeaderProtocolOAuthAccount(codexFingerprintOff)
			account.Credentials["access_token"] = "oauth-token"
			err := tc.forward(svc, c, account)
			require.Error(t, err)
			require.NotNil(t, upstream.lastReq)
			require.NotEmpty(t, upstream.lastReq.Header.Get(codexSessionHeader))
			require.Empty(t, upstream.lastReq.Header.Get(legacyCodexSessionHeader))
		})
	}
}
