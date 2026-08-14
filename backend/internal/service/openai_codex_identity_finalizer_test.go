package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestFinalizeCodexOAuthIdentityUnknownModePolicy(t *testing.T) {
	legacy := newTestOAuthAccount(41, map[string]any{codexFingerprintModeExtraKey: "typo"})
	require.Equal(t, codexFingerprintOff, legacy.GetCodexFingerprintMode())
	snapshot, err := finalizeCodexOAuthIdentity(legacy, nil, nil, "")
	require.NoError(t, err)
	require.Nil(t, snapshot, "legacy unknown mode must fail safe to off")

	strict := newTestOAuthAccount(42, map[string]any{
		codexFingerprintModeExtraKey:     "typo",
		codexIdentityFinalizerV2ExtraKey: true,
	})
	snapshot, err = finalizeCodexOAuthIdentity(strict, nil, nil, "")
	require.ErrorContains(t, err, codexFingerprintModeExtraKey)
	require.Nil(t, snapshot)
}

func TestCodexIdentityFinalizerSanitizesMalformedManagedMetadata(t *testing.T) {
	account := newTestOAuthAccount(43, map[string]any{codexFingerprintModeExtraKey: "session"})
	snapshot, err := finalizeCodexOAuthIdentity(account, nil, http.Header{"session-id": {"client-session"}}, "")
	require.NoError(t, err)
	require.NotNil(t, snapshot)

	headers := http.Header{"x-codex-turn-metadata": {`{"broken"`}}
	applyCodexFingerprintHeaders(headers, snapshot)
	require.Empty(t, headers.Get("x-codex-turn-metadata"))

	body := map[string]any{"client_metadata": map[string]any{"x-codex-turn-metadata": `{"broken"`}}
	require.True(t, applyCodexFingerprintClientMetadata(body, snapshot))
	metadata := body["client_metadata"].(map[string]any)
	_, retained := metadata["x-codex-turn-metadata"]
	require.False(t, retained)
}

func TestCodexOAuthStableEnvironmentHeadersUseAccountPolicy(t *testing.T) {
	account := newTestOAuthAccount(46, map[string]any{
		"codex_accept_language": "zh-CN",
		"codex_beta_features":   "remote_compaction_v2",
	})
	headers := http.Header{
		"Accept-Language":       {"client-language"},
		"X-Codex-Beta-Features": {"client-feature"},
	}
	applyCodexOAuthStableEnvironmentHeaders(headers, account)
	require.Equal(t, "zh-CN", headers.Get("accept-language"))
	require.Equal(t, "remote_compaction_v2", headers.Get("x-codex-beta-features"))

	account = newTestOAuthAccount(47, nil)
	headers = http.Header{
		"Accept-Language":       {"client-language"},
		"X-Codex-Beta-Features": {"client-feature"},
	}
	applyCodexOAuthStableEnvironmentHeaders(headers, account)
	require.Equal(t, defaultCodexAcceptLanguage, headers.Get("accept-language"))
	require.Empty(t, headers.Get("x-codex-beta-features"))
}

func TestCodexIdentityFinalizerHTTPFinalWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	builders := []struct {
		name  string
		build func(*OpenAIGatewayService, context.Context, *gin.Context, *Account, []byte) (*http.Request, error)
	}{
		{
			name: "responses",
			build: func(s *OpenAIGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return s.buildUpstreamRequest(ctx, c, account, body, "token", true, "cache-seed", true)
			},
		},
		{
			name: "passthrough",
			build: func(s *OpenAIGatewayService, ctx context.Context, c *gin.Context, account *Account, body []byte) (*http.Request, error) {
				return s.buildUpstreamRequestOpenAIPassthrough(ctx, c, account, body, "token")
			},
		},
	}

	for _, tt := range builders {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			body := []byte(`{"model":"gpt-5","prompt_cache_key":"cache-seed","client_metadata":{"x-codex-turn-metadata":"{\"turn_id\":\"old\"}"}}`)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("session_id", "legacy-client-session")
			c.Request.Header.Set("thread_id", "legacy-client-thread")
			c.Request.Header.Set("x-codex-turn-metadata", `{"turn_id":"old"}`)
			account := newTestOAuthAccount(44, map[string]any{
				codexFingerprintModeExtraKey:     "session",
				codexIdentityFinalizerV2ExtraKey: true,
			})
			account.Credentials = map[string]any{"chatgpt_account_id": "chatgpt-account"}

			snapshot, err := finalizeCodexOAuthIdentity(account, c, c.Request.Header, "cache-seed")
			require.NoError(t, err)
			require.NotNil(t, snapshot)
			c.Set("codex_fingerprint_ids", snapshot)

			svc := &OpenAIGatewayService{}
			req, err := tt.build(svc, context.Background(), c, account, body)
			require.NoError(t, err)
			snapshot = codexFingerprintIDsFromContext(c)
			require.NotNil(t, snapshot)
			wireBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)

			require.Equal(t, snapshot.sessionID, req.Header.Get("session-id"))
			require.Equal(t, snapshot.threadID, req.Header.Get("thread-id"))
			require.Equal(t, snapshot.turnID, req.Header.Get("x-client-request-id"))
			require.Empty(t, req.Header.Get("session_id"))
			require.Empty(t, req.Header.Get("thread_id"))
			require.False(t, gjson.GetBytes(wireBody, "prompt_cache_key").Exists())
			require.Equal(t, snapshot.sessionID, gjson.GetBytes(wireBody, "client_metadata.session_id").String())
			require.Equal(t, snapshot.threadID, gjson.GetBytes(wireBody, "client_metadata.thread_id").String())
			require.Equal(t, snapshot.turnID, gjson.GetBytes(wireBody, "client_metadata.turn_id").String())

			var headerTurn, bodyTurn map[string]any
			require.NoError(t, json.Unmarshal([]byte(req.Header.Get("x-codex-turn-metadata")), &headerTurn))
			require.NoError(t, json.Unmarshal([]byte(gjson.GetBytes(wireBody, "client_metadata.x-codex-turn-metadata").String()), &bodyTurn))
			require.Equal(t, headerTurn["turn_id"], bodyTurn["turn_id"])
			require.Equal(t, headerTurn["turn_started_at_unix_ms"], bodyTurn["turn_started_at_unix_ms"])
		})
	}
}

func TestCodexIdentityFinalizerWSTurnAndReconnectWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "ws-client-session")
	c.Request.Header.Set("x-codex-turn-metadata", `{"turn_id":"old"}`)
	account := newTestOAuthAccount(45, map[string]any{
		codexFingerprintModeExtraKey:     "session",
		codexIdentityFinalizerV2ExtraKey: true,
	})
	account.Credentials = map[string]any{"chatgpt_account_id": "chatgpt-account"}
	svc := &OpenAIGatewayService{}
	ctx := svc.snapshotOpenAIOutboundIdentity(context.Background(), account, "")

	buildTurn := func(cacheKey string) (*CodexIdentitySnapshot, http.Header, map[string]any) {
		snapshot, err := finalizeCodexOAuthIdentity(account, c, c.Request.Header, cacheKey)
		require.NoError(t, err)
		c.Set("codex_fingerprint_ids", snapshot)
		headers, _, err := svc.buildOpenAIWSHeaders(ctx, c, account, "token", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, true, "", `{"turn_id":"old"}`, cacheKey, "gpt-5", "")
		require.NoError(t, err)
		payload := map[string]any{"type": "response.create", "client_metadata": map[string]any{"x-codex-turn-metadata": `{"turn_id":"old"}`}}
		require.True(t, applyCodexFingerprintClientMetadata(payload, snapshot))
		return snapshot, headers, payload
	}

	first, firstHeaders, firstPayload := buildTurn("turn-one")
	second, reconnectHeaders, secondPayload := buildTurn("turn-two")
	require.NotEqual(t, first.turnID, second.turnID)
	for _, wire := range []struct {
		snapshot *CodexIdentitySnapshot
		headers  http.Header
		payload  map[string]any
	}{{first, firstHeaders, firstPayload}, {second, reconnectHeaders, secondPayload}} {
		metadata := wire.payload["client_metadata"].(map[string]any)
		require.Equal(t, wire.snapshot.sessionID, wire.headers.Get("session-id"))
		require.Equal(t, wire.snapshot.threadID, wire.headers.Get("thread-id"))
		require.Equal(t, wire.snapshot.turnID, wire.headers.Get("x-client-request-id"))
		require.Equal(t, wire.snapshot.turnID, metadata["turn_id"])
		require.Equal(t, wire.snapshot.protocolProfile, codexProtocolProfileName)
	}
}

func TestOpenAIWSLogSensitiveIDDigestUsesDomainSeparatedHMAC(t *testing.T) {
	raw := "session-secret-prefix-123456"
	sessionDigest := openAIWSSensitiveIDDigest("session", raw)
	conversationDigest := openAIWSSensitiveIDDigest("conversation", raw)
	require.NotEmpty(t, sessionDigest)
	require.NotContains(t, sessionDigest, "session-secret-prefix")
	require.NotEqual(t, sessionDigest, conversationDigest)
	require.Equal(t, sessionDigest, openAIWSSensitiveIDDigest("session", raw))
}

func TestResolvedCodexProtocolProfileIsAtomic(t *testing.T) {
	profile := resolveOpenAIOutboundIdentityWithVersion("", codexCLIUserAgent, codexCLIVersion)
	require.Equal(t, codexProtocolProfileName, profile.Name)
	require.Equal(t, codexHTTPBetaValue, profile.HTTPBeta)
	require.Equal(t, openAIWSBetaV1Value, profile.WSBetaV1)
	require.Equal(t, openAIWSBetaV2Value, profile.WSBetaV2)
	require.Equal(t, codexBodyPolicyVersion, profile.BodyPolicyVersion)
	require.NotEmpty(t, profile.UserAgent)
	require.NotEmpty(t, profile.Version)
	require.NotEmpty(t, profile.Originator)
}
