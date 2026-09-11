package service

import (
	"context"
	"maps"
	"reflect"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const CodexClientPresetExtraKey = "codex_client_preset"

// Presets own the complete configuration tuple. Legacy selectors remain opt-in
// compatibility settings; reading an old account never activates a preset.
type CodexClientPreset struct {
	ID      string         `json:"id"`
	Profile string         `json:"profile"`
	Extra   map[string]any `json:"extra"`
}

func CodexClientPresets() []CodexClientPreset {
	presets := []CodexClientPreset{
		{ID: "codex", Profile: CodexProfileCLI},
		{ID: "pi", Profile: CodexProfilePiManaged},
		{ID: "opencode", Profile: CodexProfileOpenCodeManaged},
	}
	for i := range presets {
		preset := &presets[i]
		preset.Extra = map[string]any{
			CodexClientPresetExtraKey:                      preset.ID,
			CodexClientProfileExtraKey:                     preset.Profile,
			CodexRelayModeExtraKey:                         string(CodexRelayModeKernel),
			CodexIdentityPolicyVersionExtraKey:             CodexIdentityPolicyV2,
			CodexInstallationPolicyExtraKey:                CodexInstallationStableV1,
			codexFingerprintModeExtraKey:                   string(codexFingerprintDevice),
			CodexRelayShadowEnabledExtraKey:                false,
			"enable_tls_fingerprint":                       preset.ID == "codex",
			"openai_ws_force_http":                         false,
			"openai_oauth_responses_websockets_v2_enabled": preset.ID == "codex",
		}
		mode := OpenAIWSIngressModeOff
		if preset.ID == "codex" {
			mode = OpenAIWSIngressModeCtxPool
		}
		preset.Extra["openai_oauth_responses_websockets_v2_mode"] = mode
	}
	return presets
}

func NormalizeCodexClientPresetExtra(extra map[string]any) (map[string]any, error) {
	raw, exists := extra[CodexClientPresetExtraKey]
	if !exists {
		return extra, nil
	}
	id, ok := raw.(string)
	if !ok {
		return nil, infraerrors.BadRequest("CODEX_CLIENT_PRESET_INVALID", "codex_client_preset must be a string")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return extra, nil
	}
	for _, preset := range CodexClientPresets() {
		if id == preset.ID {
			result := maps.Clone(extra)
			maps.Copy(result, preset.Extra)
			return result, nil
		}
	}
	return nil, infraerrors.BadRequest("CODEX_CLIENT_PRESET_INVALID", "unknown client preset")
}

// Partial JSONB updates must not silently break a managed tuple. A caller can
// explicitly clear the preset before editing individual compatibility fields.
func validateCodexClientPresetPatch(current, updates map[string]any) error {
	if _, explicit := updates[CodexClientPresetExtraKey]; explicit {
		return nil
	}
	id, _ := current[CodexClientPresetExtraKey].(string)
	for _, preset := range CodexClientPresets() {
		if id != preset.ID {
			continue
		}
		for key, expected := range preset.Extra {
			if value, changed := updates[key]; changed && !reflect.DeepEqual(value, expected) {
				return infraerrors.BadRequest("CODEX_CLIENT_PRESET_MANAGED", "clear codex_client_preset before changing preset-managed fields")
			}
		}
	}
	return nil
}

// A release refresh only affects new snapshots. Desktop needs a separately
// verified bundled Core/Desktop tuple and must not borrow the CLI release.
func codexProfileWithClientVersion(profile CodexClientProfile, version string) CodexClientProfile {
	if profile.ID != CodexProfileCLI && profile.ID != CodexProfileExec {
		return profile
	}
	version = NormalizeCodexClientVersion(version)
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		return profile
	}
	if ua := openai.SetCodexUserAgentVersion(profile.App.UserAgent, version); ua != "" {
		profile.App.UserAgent = ua
		profile.App.Version = version
	}
	return profile
}

func (s *AccountTestService) CodexClientVersion(ctx context.Context) string {
	if s == nil {
		return codexCLIVersion
	}
	return s.settingService.GetOpenAICodexClientVersion(ctx)
}
