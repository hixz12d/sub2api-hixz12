package service

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestCodexClientPresetNormalizesCompleteTuple(t *testing.T) {
	for _, preset := range CodexClientPresets() {
		t.Run(preset.ID, func(t *testing.T) {
			input := map[string]any{CodexClientPresetExtraKey: preset.ID, CodexRelayModeExtraKey: "legacy", codexFingerprintModeExtraKey: "full", "unrelated": "retained"}
			before, err := json.Marshal(input)
			require.NoError(t, err)
			extra, err := NormalizeCodexClientPresetExtra(input)
			require.NoError(t, err)
			require.Equal(t, "retained", extra["unrelated"])
			for key, value := range preset.Extra {
				require.Equal(t, value, extra[key], key)
			}
			after, err := json.Marshal(input)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoError(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, extra, testCodexRelaySecret))
			require.Error(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeOAuth, extra, "short"))
			require.Error(t, ValidateCodexRelayAccountExtra(PlatformOpenAI, AccountTypeAPIKey, extra, testCodexRelaySecret))
			catalog, err := PublicCodexClientCatalog("0.199.0")
			require.NoError(t, err)
			preview := PreviewCodexClientProfile(CodexProfilePreviewInput{Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Extra: map[string]any{CodexClientPresetExtraKey: preset.ID}, CatalogRevision: catalog.Revision,
				Operation: CodexOperationResponses, Transport: CodexTransportHTTP}, "builtin", "0.199.0")
			require.True(t, preview.Valid, preview.Conflicts)
			require.Equal(t, preset.Profile, preview.Profile.ID)
			account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: extra, Concurrency: 1}
			for _, modeRouter := range []bool{false, true} {
				cfg := &config.Config{}
				cfg.Gateway.OpenAIWS.Enabled = true
				cfg.Gateway.OpenAIWS.OAuthEnabled = true
				cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
				cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = modeRouter
				decision := NewOpenAIWSProtocolResolver(cfg).Resolve(account)
				want := OpenAIUpstreamTransportHTTPSSE
				if preset.ID == "codex" {
					want = OpenAIUpstreamTransportResponsesWebsocketV2
				}
				require.Equal(t, want, decision.Transport)
			}
		})
	}
}

func TestCodexClientPresetPreservesLegacyAndRejectsUnknown(t *testing.T) {
	legacy := map[string]any{CodexClientProfileExtraKey: "pi", CodexRelayModeExtraKey: "legacy"}
	got, err := NormalizeCodexClientPresetExtra(legacy)
	require.NoError(t, err)
	require.Equal(t, legacy, got)
	for _, value := range []any{true, nil, "future-client"} {
		_, err := NormalizeCodexClientPresetExtra(map[string]any{CodexClientPresetExtraKey: value})
		require.Error(t, err)
	}
}

func TestCodexPresetRejectsPartialTupleOverrides(t *testing.T) {
	current, err := NormalizeCodexClientPresetExtra(map[string]any{CodexClientPresetExtraKey: "pi"})
	require.NoError(t, err)
	for _, patch := range []map[string]any{
		{"enable_tls_fingerprint": true},
		{"openai_oauth_responses_websockets_v2_mode": "ctx_pool"},
		{codexFingerprintModeExtraKey: "full"},
	} {
		require.True(t, hasCodexRelayAccountExtraUpdate(patch))
		require.Error(t, validateCodexClientPresetPatch(current, patch))
		patch[CodexClientPresetExtraKey] = ""
		require.NoError(t, validateCodexClientPresetPatch(current, patch))
	}
	require.NoError(t, validateCodexClientPresetPatch(current, map[string]any{"unrelated": true}))
}

func TestCodexPresetVersionSnapshotAndPinnedTransport(t *testing.T) {
	plan := mustCodexPlanForTest(t, "preset-request", "preset-conversation", CodexTransportHTTP, time.Unix(1_800_000_000, 0))
	enabled := true
	input := CodexAttemptInput{AccountID: 44, ProfileID: CodexProfileCLI, ClientVersion: "0.199.0", FingerprintMode: "device",
		InstallationPolicy: CodexInstallationStableV1, TLSFingerprintEnabled: &enabled, TransportConfigVersion: "tls:0;enabled:true"}
	first, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.Equal(t, "0.199.0", first.Profile().App.Version)
	require.Contains(t, first.Profile().App.UserAgent, "/0.199.0 ")
	saved, err := codexConversationStateFromAttempt(plan, first, input)
	require.NoError(t, err)
	input.ClientVersion = "0.200.0"
	next, err := FinalizeCodexAttempt(plan, input, testCodexRelaySecret)
	require.NoError(t, err)
	require.NotEqual(t, first.TransportKey(), next.TransportKey())
	require.Equal(t, first.Identity().InstallationID(), next.Identity().InstallationID())
	input.ProfileID = CodexProfilePiBundle
	input.TransportConfigVersion = "tls:0;enabled:false"
	disabled := false
	input.TLSFingerprintEnabled = &disabled
	pinnedInput, err := pinCodexInputToConversation(input, saved)
	require.NoError(t, err)
	pinned, err := FinalizeCodexAttempt(plan, pinnedInput, testCodexRelaySecret)
	require.NoError(t, err)
	require.Equal(t, first.Profile(), pinned.Profile())
	require.True(t, *pinned.tlsFingerprintEnabled)
	require.Equal(t, first.TransportKey(), pinned.TransportKey())
	headers := http.Header{"Version": {"0.200.0"}, "X-Openai-Client-Version": {"0.200.0"}}
	applyCodexAttemptProfile(pinned.Profile(), headers)
	require.Equal(t, "0.199.0", headers.Get("Version"))
	require.Equal(t, headers.Get("Version"), headers.Get("x-openai-client-version"))
	pi, err := ResolveCodexClientProfile(CodexProfilePiBundle)
	require.NoError(t, err)
	applyCodexAttemptProfile(pi, headers)
	require.Empty(t, headers.Get("Version"))
	require.Empty(t, headers.Get("x-openai-client-version"))
	require.Empty(t, headers.Get("OpenAI-Beta"))
}

func TestCodexPresetCatalogUsesEffectiveVersion(t *testing.T) {
	before, err := PublicCodexClientCatalog("0.199.0")
	require.NoError(t, err)
	after, err := PublicCodexClientCatalog("0.200.0")
	require.NoError(t, err)
	require.NotEqual(t, before.Revision, after.Revision)
	for _, profile := range after.Profiles {
		if profile.ID == CodexProfileCLI || profile.ID == CodexProfileExec {
			require.Equal(t, "0.200.0", profile.AppVersion)
		}
		if profile.ID == CodexProfileDesktop {
			require.Equal(t, "0.148.0", profile.AppVersion)
		}
	}
}
