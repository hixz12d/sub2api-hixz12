package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type bundleLoopbackSender struct{ HTTPUpstream }

func (bundleLoopbackSender) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return (&http.Client{Transport: &http.Transport{Proxy: nil}}).Do(req)
}

type bundleWireCapture struct {
	Digest  string            `json:"digest"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

func TestCodexSharedBundleFinalSender(t *testing.T) {
	fixtureBytes, err := os.ReadFile("../pkg/clientprofile/testdata/shared-wire.json")
	require.NoError(t, err)
	var fixture struct {
		Session string
		Body    json.RawMessage
	}
	require.NoError(t, json.Unmarshal(fixtureBytes, &fixture))
	captures := map[string]bundleWireCapture{}
	for _, id := range []string{CodexProfilePiBundle, CodexProfileOpenCodeBundle} {
		t.Run(id, func(t *testing.T) {
			captured := make(chan bundleWireCapture, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				headers := map[string]string{}
				for key, values := range r.Header {
					lower := strings.ToLower(key)
					if lower != "content-length" && lower != "accept-encoding" && lower != "connection" {
						headers[lower] = strings.Join(values, ",")
					}
				}
				captured <- bundleWireCapture{Headers: headers, Body: body}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			plan, err := NewCodexRequestPlan(CodexRequestPlanInput{LogicalRequestID: "fixture", SessionHash: "fixture", Body: fixture.Body, DerivationSecret: testCodexRelaySecret})
			require.NoError(t, err)
			state, err := FinalizeCodexAttempt(plan, CodexAttemptInput{AccountID: 44, ProfileID: id, FingerprintMode: "device"}, testCodexRelaySecret)
			require.NoError(t, err)
			state.identity.sessionID = fixture.Session
			c.Request = c.Request.WithContext(ContextWithCodexAttemptState(ContextWithCodexRequestPlan(context.Background(), plan), state))
			svc := &OpenAIGatewayService{httpUpstream: bundleLoopbackSender{}}
			account := migrationTestAccount()
			account.Extra["enable_tls_fingerprint"] = false
			body, err := svc.finalizeCodexOAuthBody(c.Request.Context(), c, account, fixture.Body, state.Identity(), "")
			require.NoError(t, err)
			req, err := http.NewRequest("POST", server.URL, strings.NewReader(string(body)))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer fixture-token")
			req.Header.Set("chatgpt-account-id", "fixture-account")
			req.Header.Set("session-id", fixture.Session)
			req.Header.Set("x-codex-window-id", "must-not-leak")
			req.Header.Set("OpenAI-Beta", "must-not-leak")
			svc.finalizeCodexAttemptHTTPWire(c, req, body)
			response, err := svc.doOpenAIUpstream(req, "", account)
			require.NoError(t, err)
			_, err = io.ReadAll(response.Body)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			wire := <-captured
			bundle, digest, err := clientprofile.LoadCandidate(id)
			require.NoError(t, err)
			wire.Digest = digest
			require.Equal(t, bundle.UserAgent, wire.Headers["user-agent"])
			require.Equal(t, fixture.Session, wire.Headers["session_id"])
			require.Empty(t, wire.Headers["session-id"])
			require.Empty(t, wire.Headers["x-codex-window-id"])
			require.Empty(t, wire.Headers["openai-beta"])
			var actual, expected map[string]any
			require.NoError(t, json.Unmarshal(wire.Body, &actual))
			require.NoError(t, json.Unmarshal(fixture.Body, &expected))
			if id == CodexProfilePiBundle {
				expected["prompt_cache_key"] = fixture.Session
				expected["text"] = map[string]any{"verbosity": "medium"}
			}
			require.Equal(t, expected, actual)
			require.Equal(t, digest, state.Profile().BundleDigest)
			captures[id] = wire
			svc.pluginManager = &PluginManager{}
			svc.pluginManager.route.Store(&pluginRoute{rolloutPercent: 100})
			_, err = svc.doOpenAIUpstream(req, "", account)
			require.ErrorContains(t, err, "plugin transport is not verified")
			svc.pluginManager = nil
			account.Extra["enable_tls_fingerprint"] = true
			_, err = svc.doOpenAIUpstream(req, "", account)
			require.ErrorContains(t, err, "disable enable_tls_fingerprint")
			account.Extra[CodexClientProfileExtraKey] = id
			require.True(t, hasCodexRelayAccountExtraUpdate(map[string]any{"enable_tls_fingerprint": true}))
			require.Error(t, ValidateCodexRelayAccountExtra(account.Platform, account.Type, account.Extra, testCodexRelaySecret))
			account.Extra["enable_tls_fingerprint"] = false
			badPlan := *plan
			badPlan.body = []byte(`{"input":[],"client_metadata":{"keep":"caller-owned"}}`)
			badReq := req.Clone(ContextWithCodexRequestPlan(req.Context(), &badPlan))
			_, err = svc.doOpenAIUpstream(badReq, "", account)
			require.ErrorContains(t, err, "client_metadata")
		})
	}
	if output := os.Getenv("HIXZ12_BUNDLE_CAPTURE_OUT"); output != "" {
		data, err := json.MarshalIndent(captures, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(output, data, 0600))
	}
}

func TestCodexBundlePinsAndRejectsUnsupportedTransport(t *testing.T) {
	svc, _ := migrationTestService()
	account := migrationTestAccount()
	account.Extra["enable_tls_fingerprint"] = false
	account.Extra[CodexClientProfileExtraKey] = CodexProfilePiBundle
	c := migrationTestContext(t, "bundle-pinned", CodexTransportHTTP)
	_, err := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	require.NoError(t, err)
	first, _ := codexAttemptStateFromGin(c)
	account.Extra[CodexClientProfileExtraKey] = CodexProfileCLI
	_, err = svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
	require.NoError(t, err)
	second, _ := codexAttemptStateFromGin(c)
	require.Equal(t, first.Profile().BundleDigest, second.Profile().BundleDigest)
	plan := mustCodexPlanForTest(t, "ws", "new", CodexTransportWS, time.Now())
	_, err = FinalizeCodexAttempt(plan, CodexAttemptInput{AccountID: 44, ProfileID: CodexProfilePiBundle, FingerprintMode: "device"}, testCodexRelaySecret)
	require.Error(t, err)
}
