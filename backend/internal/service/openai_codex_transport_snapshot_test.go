package service

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodexAttemptFreezesTLSOptOutAndSeparatesPoolKeys(t *testing.T) {
	plan := mustCodexPlanForTest(t, "same-request", "same-conversation", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	keys := map[string]bool{}
	for _, enabled := range []bool{false, true} {
		supplied := enabled
		state, err := FinalizeCodexAttempt(plan, CodexAttemptInput{AccountID: 707, ProfileID: CodexProfileCLI, FingerprintMode: "device", TLSFingerprintEnabled: &supplied}, testCodexRelaySecret)
		require.NoError(t, err)
		require.False(t, keys[state.TransportKey()])
		keys[state.TransportKey()] = true
		supplied = !enabled
		require.Equal(t, enabled, *state.tlsFingerprintEnabled)
		upstream := &codexTransportCountingUpstream{}
		svc := &OpenAIGatewayService{httpUpstream: upstream}
		account := newTestOAuthAccount(707, map[string]any{"enable_tls_fingerprint": !enabled})
		req := httptest.NewRequest(http.MethodPost, "https://chatgpt.com/backend-api/codex/responses", nil)
		req = req.WithContext(ContextWithCodexAttemptState(req.Context(), state))
		_, err = svc.doOpenAIUpstream(req, "", account)
		require.ErrorContains(t, err, "stop")
		if enabled {
			require.Equal(t, int32(1), upstream.tlsCalls.Load())
			require.Zero(t, upstream.plainCalls.Load())
		} else {
			require.Equal(t, int32(1), upstream.plainCalls.Load())
			require.Zero(t, upstream.tlsCalls.Load())
		}
	}
}
