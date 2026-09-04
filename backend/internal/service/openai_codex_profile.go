package service

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

type CodexProfileCapability uint32

const (
	CodexCapabilityResponses CodexProfileCapability = 1 << iota
	CodexCapabilityCompact
	CodexCapabilityResume
	CodexCapabilityHTTP
	CodexCapabilityWebSocket
)

type CodexProfileFidelity string

const (
	CodexProfileFidelityVerified    CodexProfileFidelity = "verified"
	CodexProfileFidelityDegraded    CodexProfileFidelity = "degraded"
	CodexProfileFidelityUnsupported CodexProfileFidelity = "unsupported"
)

const (
	CodexProfileAuto        = "auto"
	CodexProfilePassthrough = "passthrough"
	CodexProfileCLI         = "codex_cli"
	CodexProfileExec        = "codex_exec"
	CodexProfileDesktop     = "codex_desktop"
	CodexProfileOpenCode    = "opencode"
	CodexProfilePi          = "pi"
)

type CodexAppIdentityProfile struct {
	UserAgent         string
	Originator        string
	Version           string
	BetaFeatures      string
	ClientEnvironment string
}

type CodexTransportProfile struct {
	TLSProfileID   string
	HTTP2ProfileID string
	HeaderOrder    []string
}

type CodexClientProfile struct {
	ID           string
	Revision     int
	App          CodexAppIdentityProfile
	Transport    CodexTransportProfile
	Capabilities CodexProfileCapability
	Fidelity     CodexProfileFidelity
	FidelityNote string
}

var codexProfileVersionPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+){1,3}(?:[-+][0-9A-Za-z.-]+)?$`)

var codexDefaultHeaderOrder = tlsfingerprint.ChromeHTTP2HeaderOrder()

var codexProfileCatalog = map[string]CodexClientProfile{
	CodexProfilePassthrough: {
		ID:           CodexProfilePassthrough,
		Revision:     1,
		Capabilities: CodexCapabilityResponses | CodexCapabilityCompact | CodexCapabilityResume | CodexCapabilityHTTP | CodexCapabilityWebSocket,
		Fidelity:     CodexProfileFidelityDegraded,
		FidelityNote: "application identity is caller supplied; transport parity cannot be asserted",
	},
	CodexProfileCLI: {
		ID:       CodexProfileCLI,
		Revision: 1,
		App: CodexAppIdentityProfile{
			UserAgent:         "codex-tui/0.148.0 (Windows 10.0.26100; x86_64) WindowsTerminal/1.23.20211.0",
			Originator:        "codex_cli_rs",
			Version:           "0.148.0",
			BetaFeatures:      "responses_websockets=2026-02-06",
			ClientEnvironment: "windows-terminal",
		},
		Transport: CodexTransportProfile{
			TLSProfileID:   tlsfingerprint.HelloPresetChromeAuto,
			HTTP2ProfileID: tlsfingerprint.ChromeHTTP2ProfileID,
			HeaderOrder:    codexDefaultHeaderOrder,
		},
		Capabilities: CodexCapabilityResponses | CodexCapabilityCompact | CodexCapabilityResume | CodexCapabilityHTTP | CodexCapabilityWebSocket,
		Fidelity:     CodexProfileFidelityDegraded,
		FidelityNote: "application identity is known, but the built-in transport is Chrome/uTLS rather than the measured CLI SChannel ClientHello",
	},
	CodexProfileExec: {
		ID:       CodexProfileExec,
		Revision: 1,
		App: CodexAppIdentityProfile{
			UserAgent:    "codex_exec/0.148.0 (Windows 10.0.26100; x86_64) unknown/0",
			Originator:   "codex_exec",
			Version:      "0.148.0",
			BetaFeatures: "responses_websockets=2026-02-06",
		},
		Transport: CodexTransportProfile{
			TLSProfileID:   tlsfingerprint.HelloPresetChromeAuto,
			HTTP2ProfileID: tlsfingerprint.ChromeHTTP2ProfileID,
			HeaderOrder:    codexDefaultHeaderOrder,
		},
		Capabilities: CodexCapabilityResponses | CodexCapabilityCompact | CodexCapabilityResume | CodexCapabilityHTTP | CodexCapabilityWebSocket,
		Fidelity:     CodexProfileFidelityDegraded,
		FidelityNote: "application identity is known, but strict native-agent SChannel parity is not implemented",
	},
	CodexProfileDesktop: {
		ID:       CodexProfileDesktop,
		Revision: 1,
		App: CodexAppIdentityProfile{
			UserAgent:         "Codex Desktop/0.148.0 (Windows 10.0.26100; x86_64)",
			Originator:        "Codex Desktop",
			Version:           "0.148.0",
			BetaFeatures:      "responses_websockets=2026-02-06",
			ClientEnvironment: "electron",
		},
		Transport: CodexTransportProfile{
			TLSProfileID:   tlsfingerprint.HelloPresetChromeAuto,
			HTTP2ProfileID: tlsfingerprint.ChromeHTTP2ProfileID,
			HeaderOrder:    codexDefaultHeaderOrder,
		},
		Capabilities: CodexCapabilityResponses | CodexCapabilityCompact | CodexCapabilityResume | CodexCapabilityHTTP | CodexCapabilityWebSocket,
		Fidelity:     CodexProfileFidelityDegraded,
		FidelityNote: "Electron shell Chrome parity is available; Desktop agent SChannel parity is not",
	},
	CodexProfileOpenCode: {
		ID:       CodexProfileOpenCode,
		Revision: 1,
		App: CodexAppIdentityProfile{
			UserAgent:  "opencode/1.2.4 windows/x64",
			Originator: "opencode",
			Version:    "1.2.4",
		},
		Transport: CodexTransportProfile{
			TLSProfileID:   tlsfingerprint.HelloPresetChromeAuto,
			HTTP2ProfileID: tlsfingerprint.ChromeHTTP2ProfileID,
			HeaderOrder:    codexDefaultHeaderOrder,
		},
		Capabilities: CodexCapabilityResponses | CodexCapabilityResume | CodexCapabilityHTTP,
		Fidelity:     CodexProfileFidelityDegraded,
		FidelityNote: "application headers are observed; no official strict TLS or WebSocket profile is asserted",
	},
	CodexProfilePi: {
		ID:       CodexProfilePi,
		Revision: 1,
		App: CodexAppIdentityProfile{
			UserAgent:  "pi (windows; x64)",
			Originator: "pi",
		},
		Transport: CodexTransportProfile{
			TLSProfileID:   tlsfingerprint.HelloPresetChromeAuto,
			HTTP2ProfileID: tlsfingerprint.ChromeHTTP2ProfileID,
			HeaderOrder:    codexDefaultHeaderOrder,
		},
		Capabilities: CodexCapabilityResponses | CodexCapabilityResume | CodexCapabilityHTTP,
		Fidelity:     CodexProfileFidelityUnsupported,
		FidelityNote: "no measured official wire profile is available; HTTP application compatibility only",
	},
}

func CodexClientProfiles() []CodexClientProfile {
	ids := []string{CodexProfilePassthrough, CodexProfileCLI, CodexProfileExec, CodexProfileDesktop, CodexProfileOpenCode, CodexProfilePi}
	out := make([]CodexClientProfile, 0, len(ids))
	for _, id := range ids {
		profile, _ := ResolveCodexClientProfile(id)
		out = append(out, profile)
	}
	return out
}

func ResolveCodexClientProfile(id string) (CodexClientProfile, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		id = CodexProfileCLI
	}
	profile, ok := codexProfileCatalog[id]
	if !ok {
		return CodexClientProfile{}, fmt.Errorf("unknown codex client profile %q", id)
	}
	profile.Transport.HeaderOrder = append([]string(nil), profile.Transport.HeaderOrder...)
	if err := ValidateCodexClientProfile(profile); err != nil {
		return CodexClientProfile{}, err
	}
	return profile, nil
}

// ResolveCodexClientProfileForRequest resolves the auto selector from immutable
// inbound metadata. Unknown callers remain passthrough instead of being
// rewritten into a client they did not claim to be.
func ResolveCodexClientProfileForRequest(id string, inbound http.Header) (CodexClientProfile, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id != CodexProfileAuto {
		return ResolveCodexClientProfile(id)
	}
	originator := strings.ToLower(strings.TrimSpace(inbound.Get("originator")))
	userAgent := strings.ToLower(strings.TrimSpace(inbound.Get("user-agent")))
	switch {
	case strings.Contains(originator, "codex_exec") || strings.Contains(userAgent, "codex-exec"):
		id = CodexProfileExec
	case strings.Contains(originator, "codex_desktop") || strings.Contains(userAgent, "codex-desktop"):
		id = CodexProfileDesktop
	case strings.Contains(originator, "opencode") || strings.Contains(userAgent, "opencode"):
		id = CodexProfileOpenCode
	case originator == "pi" || strings.Contains(userAgent, "pi-coding-agent"):
		id = CodexProfilePi
	case strings.Contains(originator, "codex") || strings.Contains(userAgent, "codex_cli_rs"):
		id = CodexProfileCLI
	default:
		id = CodexProfilePassthrough
	}
	return ResolveCodexClientProfile(id)
}

func ValidateCodexClientProfile(profile CodexClientProfile) error {
	if strings.TrimSpace(profile.ID) == "" || profile.Revision <= 0 {
		return errors.New("codex profile id and positive revision are required")
	}
	if profile.ID == CodexProfilePassthrough {
		if strings.TrimSpace(profile.App.UserAgent) != "" || strings.TrimSpace(profile.App.Originator) != "" {
			return errors.New("passthrough profile cannot invent application identity")
		}
		return nil
	}
	if profile.Capabilities&CodexCapabilityResponses == 0 || profile.Capabilities&CodexCapabilityHTTP == 0 {
		return errors.New("codex profile must support responses over HTTP")
	}
	if strings.TrimSpace(profile.Transport.TLSProfileID) == "" || strings.TrimSpace(profile.Transport.HTTP2ProfileID) == "" || len(profile.Transport.HeaderOrder) == 0 {
		return errors.New("codex profile transport tuple is incomplete")
	}
	if strings.TrimSpace(profile.App.UserAgent) == "" || strings.TrimSpace(profile.App.Originator) == "" {
		return errors.New("codex profile application identity is incomplete")
	}
	if profile.App.Version != "" && !codexProfileVersionPattern.MatchString(profile.App.Version) {
		return errors.New("codex profile version is invalid")
	}
	if err := validateCodexProfileApplicationPair(profile); err != nil {
		return err
	}
	if (profile.ID == CodexProfileOpenCode || profile.ID == CodexProfilePi) && profile.Capabilities&CodexCapabilityWebSocket != 0 {
		return fmt.Errorf("profile %s cannot claim WebSocket parity", profile.ID)
	}
	return nil
}

func validateCodexProfileApplicationPair(profile CodexClientProfile) error {
	ua := strings.ToLower(strings.TrimSpace(profile.App.UserAgent))
	origin := strings.ToLower(strings.TrimSpace(profile.App.Originator))
	var uaPrefix, expectedOrigin string
	switch profile.ID {
	case CodexProfileCLI:
		uaPrefix, expectedOrigin = "codex-tui/", "codex_cli_rs"
	case CodexProfileExec:
		uaPrefix, expectedOrigin = "codex_exec/", "codex_exec"
	case CodexProfileDesktop:
		uaPrefix, expectedOrigin = "codex desktop/", "codex desktop"
	case CodexProfileOpenCode:
		uaPrefix, expectedOrigin = "opencode/", "opencode"
	case CodexProfilePi:
		uaPrefix, expectedOrigin = "pi ", "pi"
	default:
		return fmt.Errorf("profile %s has no validation rule", profile.ID)
	}
	if !strings.HasPrefix(ua, uaPrefix) || origin != expectedOrigin {
		return fmt.Errorf("profile %s has inconsistent user-agent/originator", profile.ID)
	}
	if profile.App.Version != "" && !strings.Contains(ua, strings.ToLower(profile.App.Version)) {
		return fmt.Errorf("profile %s user-agent does not contain its version", profile.ID)
	}
	return nil
}

func (p CodexClientProfile) Supports(capability CodexProfileCapability) bool {
	return p.Capabilities&capability != 0
}

const (
	CodexRelayModeExtraKey             = "codex_relay_mode"
	CodexIdentityPolicyVersionExtraKey = "codex_identity_policy_version"
	CodexClientProfileExtraKey         = "codex_client_profile"
	CodexRelayShadowEnabledExtraKey    = "codex_relay_shadow_enabled"
)

type CodexRelayMode string

const (
	CodexRelayModeLegacy CodexRelayMode = "legacy"
	CodexRelayModeKernel CodexRelayMode = "relay_kernel"
)

type CodexRelaySettings struct {
	Mode          CodexRelayMode
	PolicyVersion string
	ProfileID     string
	ShadowEnabled bool
}

func usesCodexRelayKernel(account *Account) bool {
	return account != nil && strings.EqualFold(
		strings.TrimSpace(account.GetExtraString(CodexRelayModeExtraKey)),
		string(CodexRelayModeKernel),
	)
}

func ResolveCodexRelaySettings(account *Account) (CodexRelaySettings, error) {
	settings := CodexRelaySettings{
		Mode:          CodexRelayModeLegacy,
		PolicyVersion: CodexIdentityPolicyV1,
		ProfileID:     CodexProfileCLI,
	}
	if account == nil || account.Extra == nil {
		return settings, nil
	}
	if raw := strings.ToLower(strings.TrimSpace(account.GetExtraString(CodexRelayModeExtraKey))); raw != "" {
		settings.Mode = CodexRelayMode(raw)
	}
	if raw := strings.ToLower(strings.TrimSpace(account.GetExtraString(CodexIdentityPolicyVersionExtraKey))); raw != "" {
		settings.PolicyVersion = raw
	}
	if raw := strings.ToLower(strings.TrimSpace(account.GetExtraString(CodexClientProfileExtraKey))); raw != "" {
		settings.ProfileID = raw
	}
	settings.ShadowEnabled, _ = account.Extra[CodexRelayShadowEnabledExtraKey].(bool)

	if settings.Mode != CodexRelayModeLegacy && settings.Mode != CodexRelayModeKernel {
		return CodexRelaySettings{}, fmt.Errorf("invalid %s %q", CodexRelayModeExtraKey, settings.Mode)
	}
	if settings.PolicyVersion != CodexIdentityPolicyV1 && settings.PolicyVersion != CodexIdentityPolicyV2 {
		return CodexRelaySettings{}, fmt.Errorf("invalid %s %q", CodexIdentityPolicyVersionExtraKey, settings.PolicyVersion)
	}
	if settings.ProfileID != CodexProfileAuto {
		if _, err := ResolveCodexClientProfile(settings.ProfileID); err != nil {
			return CodexRelaySettings{}, err
		}
	}
	if settings.Mode == CodexRelayModeKernel && settings.PolicyVersion != CodexIdentityPolicyV2 {
		return CodexRelaySettings{}, errors.New("relay_kernel requires codex identity policy v2")
	}
	return settings, nil
}

var codexRelayAccountExtraKeys = []string{
	CodexRelayModeExtraKey,
	CodexIdentityPolicyVersionExtraKey,
	CodexClientProfileExtraKey,
	CodexRelayShadowEnabledExtraKey,
	codexFingerprintModeExtraKey,
}

// hasCodexRelayAccountExtraUpdate keeps generic JSONB merge paths under the
// same validation contract as full account updates.
func hasCodexRelayAccountExtraUpdate(extra map[string]any) bool {
	for _, key := range codexRelayAccountExtraKeys {
		if _, ok := extra[key]; ok {
			return true
		}
	}
	return false
}

// ValidateCodexRelayAccountExtra validates the persisted admin configuration.
// Runtime-owned identity values are intentionally absent from this contract.
func ValidateCodexRelayAccountExtra(platform, accountType string, extra map[string]any, derivationSecret string) error {
	configured := false
	for _, key := range codexRelayAccountExtraKeys {
		if _, ok := extra[key]; ok {
			configured = true
			break
		}
	}
	if !configured {
		return nil
	}
	if platform != PlatformOpenAI || (accountType != AccountTypeOAuth && accountType != AccountTypeSetupToken) {
		return infraerrors.BadRequest("CODEX_RELAY_ACCOUNT_INVALID", "Codex relay settings require an OpenAI OAuth or setup-token account")
	}
	if raw, ok := extra[CodexRelayShadowEnabledExtraKey]; ok {
		if _, valid := raw.(bool); !valid {
			return infraerrors.BadRequest("CODEX_RELAY_SHADOW_INVALID", "codex_relay_shadow_enabled must be a boolean")
		}
	}
	if raw, ok := extra[codexFingerprintModeExtraKey]; ok {
		mode, valid := raw.(string)
		if !valid {
			return infraerrors.BadRequest("CODEX_FINGERPRINT_MODE_INVALID", "codex_fingerprint_mode must be a string")
		}
		normalized := codexFingerprintMode(strings.ToLower(strings.TrimSpace(mode)))
		switch normalized {
		case codexFingerprintOff, codexFingerprintDevice, codexFingerprintSession, codexFingerprintWindow, codexFingerprintWindow40, codexFingerprintFull:
		default:
			return infraerrors.BadRequest("CODEX_FINGERPRINT_MODE_INVALID", "invalid codex_fingerprint_mode")
		}
	}
	account := &Account{Platform: platform, Type: accountType, Extra: extra}
	settings, err := ResolveCodexRelaySettings(account)
	if err != nil {
		return infraerrors.BadRequest("CODEX_RELAY_SETTINGS_INVALID", err.Error())
	}
	if settings.Mode == CodexRelayModeKernel && codexFingerprintModeFromExtra(extra) == codexFingerprintOff {
		return infraerrors.BadRequest("CODEX_RELAY_IDENTITY_REQUIRED", "relay_kernel requires a managed codex_fingerprint_mode")
	}
	if settings.Mode == CodexRelayModeKernel || settings.ShadowEnabled {
		if _, err := NewCodexIdentityDeriver(derivationSecret); err != nil {
			return infraerrors.BadRequest("CODEX_RELAY_SECRET_INVALID", "gateway.openai_affinity.secret or jwt.secret must contain at least 32 bytes")
		}
	}
	return nil
}
