package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/Wei-Shaw/sub2api/internal/pkg/clientprofile"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type PublicCodexProfile struct {
	ID               string `json:"id"`
	Family           string `json:"family"`
	Variant          string `json:"variant"`
	AppVersion       string `json:"appVersion"`
	HTTP             *bool  `json:"http"`
	WS               *bool  `json:"ws"`
	Compact          *bool  `json:"compact"`
	Fidelity         string `json:"fidelity"`
	Recipe           string `json:"recipe"`
	Digest           string `json:"digest"`
	RelayDigest      string `json:"relay_digest,omitempty"`
	NativeValidation string `json:"native_validation"`
}
type PublicCodexCatalog struct {
	Revision                 string                            `json:"revision"`
	RelayContract            string                            `json:"relay_contract"`
	RelayDigest              string                            `json:"relay_digest"`
	RelayReferences          []clientprofile.RelayCatalogEntry `json:"relay_references"`
	Profiles                 []PublicCodexProfile              `json:"profiles"`
	NewManagedDefaultEnabled bool                              `json:"new_managed_default_enabled"`
	ActivationRequirements   []string                          `json:"activation_requirements"`
}

func PublicCodexClientCatalog() (PublicCodexCatalog, error) {
	relay, err := clientprofile.LoadRelayCatalog()
	if err != nil {
		return PublicCodexCatalog{}, err
	}
	result := PublicCodexCatalog{RelayContract: relay.Contract, RelayDigest: clientprofile.RelayCatalogSHA256,
		RelayReferences: relay.Profiles, ActivationRequirements: []string{"affinity-secret", "distributed-registry", "installation-migration-approval"}}
	result.Profiles = append(result.Profiles, PublicCodexProfile{ID: CodexProfileAuto, Family: "caller", Variant: "auto", AppVersion: "dynamic", Fidelity: "caller-resolved", NativeValidation: "untested"})
	for _, profile := range CodexClientProfiles() {
		digest, err := codexProfileSnapshotDigest(profile)
		if err != nil {
			return PublicCodexCatalog{}, err
		}
		httpOK, wsOK, compactOK := profile.Supports(CodexCapabilityHTTP), profile.Supports(CodexCapabilityWebSocket), profile.Supports(CodexCapabilityCompact)
		item := PublicCodexProfile{ID: profile.ID, Family: codexClientFamily(profile.ID), Variant: "legacy", AppVersion: profile.App.Version,
			HTTP: &httpOK, WS: &wsOK, Compact: &compactOK, Fidelity: "degraded", Recipe: profile.ID, Digest: digest, NativeValidation: "untested"}
		switch profile.ID {
		case CodexProfileAuto:
			item.AppVersion = "dynamic"
		case CodexProfilePassthrough:
			item.AppVersion, item.Fidelity = "caller supplied", "passthrough/degraded"
		case CodexProfilePi:
			item.AppVersion, item.Fidelity = "not asserted", "unsupported strict parity"
		}
		if profile.BundleID != "" {
			item.Variant = "shared-r1"
			for _, entry := range relay.Profiles {
				if entry.Descriptor.Selector == profile.ID {
					item.AppVersion, item.RelayDigest = entry.Descriptor.Release, entry.Digest
				}
			}
		}
		result.Profiles = append(result.Profiles, item)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return PublicCodexCatalog{}, err
	}
	digest := sha256.Sum256(data)
	result.Revision = hex.EncodeToString(digest[:])
	return result, nil
}

func codexClientFamily(id string) string {
	switch id {
	case CodexProfileCLI, CodexProfileExec, CodexProfileDesktop:
		return "codex"
	case CodexProfilePi, CodexProfilePiBundle:
		return "pi"
	case CodexProfileOpenCode, CodexProfileOpenCodeBundle:
		return "opencode"
	default:
		return "caller"
	}
}

type CodexEffectiveTransportPolicy struct {
	Owner            string `json:"owner"`
	Sender           string `json:"sender"`
	TLSRecipe        string `json:"tls_recipe"`
	HTTP2Recipe      string `json:"http2_recipe"`
	NativeValidation string `json:"native_validation"`
}

// Resolve once from the attempt's profile and captured flags. Native still uses HTTPS.
func ResolveCodexEffectiveTransport(profile CodexClientProfile, tlsEnabled, pluginRoute bool, transport CodexEgressTransport) (CodexEffectiveTransportPolicy, error) {
	policy := CodexEffectiveTransportPolicy{Owner: "builtin", Sender: "go-http", TLSRecipe: "native-default", HTTP2Recipe: "native-default", NativeValidation: "untested"}
	if transport != CodexTransportHTTP && transport != CodexTransportWS {
		return policy, errors.New("unsupported transport")
	}
	if transport == CodexTransportWS {
		if !profile.Supports(CodexCapabilityWebSocket) {
			return policy, errors.New("profile does not support WebSocket")
		}
		policy.Sender, policy.TLSRecipe, policy.HTTP2Recipe = "go-websocket", "websocket-driver", "not-applicable"
		if pluginRoute {
			return policy, errors.New("plugin WebSocket path requires a separate HTTP bridge preview")
		}
		return policy, nil
	}
	if profile.BundleID != "" {
		if tlsEnabled {
			return policy, errors.New("shared client bundle conflicts with TLS impersonation; explicitly select native HTTP before saving")
		}
		if pluginRoute {
			return policy, errors.New("shared client bundle requires the built-in sender; plugin route is unsupported")
		}
	}
	if pluginRoute {
		policy.Owner, policy.Sender, policy.TLSRecipe, policy.HTTP2Recipe = "plugin", "plugin-managed", "plugin-defined", "plugin-defined"
	} else if tlsEnabled && profile.Transport.TLSProfileID == tlsfingerprint.HelloPresetChromeAuto {
		policy.Sender, policy.TLSRecipe, policy.HTTP2Recipe = "go-utls", profile.Transport.TLSProfileID, profile.Transport.HTTP2ProfileID
	}
	return policy, nil
}

type CodexProfilePreviewInput struct {
	AccountID       int64                           `json:"account_id,omitempty"`
	Platform        string                          `json:"platform"`
	Type            string                          `json:"type"`
	Extra           map[string]any                  `json:"extra"`
	Operation       CodexOperationKind              `json:"operation"`
	Transport       CodexEgressTransport            `json:"transport"`
	CatalogRevision string                          `json:"catalog_revision"`
	Device          *clientprofile.RelayDeviceTuple `json:"device,omitempty"`
}
type CodexProfilePreview struct {
	Valid         bool                           `json:"valid"`
	Scope         string                         `json:"scope"`
	Profile       *PublicCodexProfile            `json:"profile,omitempty"`
	Transport     *CodexEffectiveTransportPolicy `json:"transport,omitempty"`
	Conflicts     []string                       `json:"conflicts"`
	Requirements  []string                       `json:"requirements"`
	SessionEffect string                         `json:"session_effect"`
	PluginStatus  string                         `json:"plugin_status"`
}

func PreviewCodexClientProfile(input CodexProfilePreviewInput, pluginStatus string) CodexProfilePreview {
	result := CodexProfilePreview{Scope: "configuration-only-no-upstream", Conflicts: []string{},
		Requirements:  []string{"server-validates-secret-on-save", "distributed-registry-required", "review-active-pins-before-transport-change"},
		SessionEffect: "existing-pins-retained; transport changes require migration review", PluginStatus: pluginStatus}
	reject := func(message string) CodexProfilePreview {
		result.Conflicts = append(result.Conflicts, message)
		return result
	}
	catalog, err := PublicCodexClientCatalog()
	if err != nil {
		return reject("reviewed relay contract unavailable")
	}
	if input.CatalogRevision != catalog.Revision {
		return reject("catalog revision changed; reload preview")
	}
	allowed := map[string]bool{"enable_tls_fingerprint": true, "tls_fingerprint_profile_id": true}
	for _, key := range codexRelayAccountExtraKeys {
		allowed[key] = true
	}
	for key := range input.Extra {
		if !allowed[key] {
			return reject("unknown preview configuration field")
		}
	}
	if input.Operation != CodexOperationResponses && input.Operation != CodexOperationCompact && input.Operation != CodexOperationResume {
		return reject("unsupported operation")
	}
	if err := validateCodexRelayAccountExtra(input.Platform, input.Type, input.Extra, "", false); err != nil {
		return reject(err.Error())
	}
	account := &Account{Platform: input.Platform, Type: input.Type, Extra: input.Extra}
	if input.Platform != PlatformOpenAI || (input.Type != AccountTypeOAuth && input.Type != AccountTypeSetupToken) {
		return reject("profile preview requires OpenAI OAuth or setup-token")
	}
	settings, err := ResolveCodexRelaySettings(account)
	if err != nil {
		return reject(err.Error())
	}
	if settings.ProfileID == CodexProfileAuto {
		result.Requirements = append(result.Requirements, "auto-resolves-original-inbound-only; unknown-caller-passthrough")
		return result
	}
	profile, err := ResolveCodexClientProfile(settings.ProfileID)
	if err != nil {
		return reject("unknown client profile")
	}
	if input.Operation == CodexOperationCompact && !profile.Supports(CodexCapabilityCompact) {
		return reject("profile does not support Compact")
	}
	if input.Operation == CodexOperationResume && !profile.Supports(CodexCapabilityResume) {
		return reject("profile does not support resume")
	}
	if input.Device != nil {
		if profile.BundleID == "" {
			return reject("logical device override not adapted for this legacy recipe")
		}
		if *input.Device != (clientprofile.RelayDeviceTuple{Platform: "win32", Release: "10.0.26200", Arch: "x64"}) {
			return reject("device tuple conflicts with frozen shared artifact")
		}
	}
	for _, item := range catalog.Profiles {
		if item.ID == profile.ID {
			value := item
			result.Profile = &value
		}
	}
	policy, err := ResolveCodexEffectiveTransport(profile, account.IsTLSFingerprintEnabled(), pluginStatus == "routed", input.Transport)
	if err != nil {
		return reject(err.Error())
	}
	if settings.Mode != CodexRelayModeKernel {
		policy.Sender, policy.TLSRecipe, policy.HTTP2Recipe = "legacy-path", "operation-dependent", "operation-dependent"
		result.Requirements = append(result.Requirements, "legacy-execution-not-the-managed-recipe")
	}
	result.Transport = &policy
	if pluginStatus == "unknown" {
		result.Requirements = append(result.Requirements, "plugin-routing-must-be-checked-for-selected-account")
	}
	result.Valid = true
	return result
}

// This only reads local routing state; it neither runs a plugin nor sends a test request.
func (s *AccountTestService) CodexProfilePluginStatus(account *Account) string {
	if s == nil || account == nil || account.ID <= 0 {
		return "unknown"
	}
	if s.pluginManager != nil && s.pluginManager.ShouldRouteOpenAIOAuth(account) {
		return "routed"
	}
	return "builtin"
}
