package service

import (
	"context"
	"net/http"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
)

const maxOpenAIAccountUserAgentLength = 512

// openAIOutboundIdentity is the trusted Codex identity used for upstream
// requests. It is resolved from account and system settings only; inbound
// caller headers never participate in the decision.
type openAIOutboundIdentity struct {
	UserAgent  string
	Originator string
	Version    string
}

// resolveOpenAIOutboundIdentityFromSettings is the single source of truth for
// selecting an outbound OpenAI identity. A valid account identity wins over
// the global identity, followed by the compiled-in default. The selected
// client family and platform fingerprint are preserved while its version is
// synchronized with the currently effective version setting.
func resolveOpenAIOutboundIdentityFromSettings(ctx context.Context, account *Account, settingService *SettingService) openAIOutboundIdentity {
	accountUA := ""
	if account != nil {
		accountUA = account.GetOpenAIUserAgent()
	}

	// The gateway installs a cached canonical resolver during wiring. Using it
	// here keeps helper services on the same global identity without requiring
	// every service to own a SettingService field.
	systemUA := codexCanonicalUserAgent()
	version := codexClientVersionFromUA(systemUA)
	if settingService != nil {
		systemUA = settingService.GetOpenAICodexUserAgent(ctx)
		version = settingService.GetOpenAICodexClientVersion(ctx)
	}
	return resolveOpenAIOutboundIdentityWithVersion(accountUA, systemUA, version)
}

// resolveOpenAIOutboundIdentity resolves shadow accounts to their credential
// owner before selecting the account-level identity. ForceCodexCLI remains a
// local compatibility switch and deliberately wins over account/global UA.
func (s *OpenAIGatewayService) resolveOpenAIOutboundIdentity(ctx context.Context, account *Account) openAIOutboundIdentity {
	if s != nil && s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		return resolveOpenAIOutboundIdentityWithVersion("", codexCLIUserAgent, codexCLIVersion)
	}
	if account != nil && account.IsCredentialShadow() && s != nil && s.accountRepo != nil {
		if credentialAccount, err := resolveCredentialAccount(ctx, s.accountRepo, account); err == nil && credentialAccount != nil {
			account = credentialAccount
		}
	}
	var settingService *SettingService
	if s != nil && account != nil && account.Type == AccountTypeOAuth {
		settingService = s.settingService
	}
	return resolveOpenAIOutboundIdentityFromSettings(ctx, account, settingService)
}

// NormalizeOpenAIAccountUserAgent validates and canonicalizes an optional
// account-level Codex identity. Empty means inherit the global/default value.
func NormalizeOpenAIAccountUserAgent(platform string, credentials map[string]any) error {
	if platform != PlatformOpenAI || credentials == nil {
		return nil
	}
	raw, configured := credentials["user_agent"]
	if !configured || raw == nil {
		delete(credentials, "user_agent")
		return nil
	}
	userAgent, ok := raw.(string)
	if !ok {
		return infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_USER_AGENT_INVALID", "OpenAI Codex user_agent must be a string")
	}
	userAgent, err := NormalizeOpenAICodexUserAgent(userAgent)
	if err != nil {
		return err
	}
	if userAgent == "" {
		delete(credentials, "user_agent")
		return nil
	}
	credentials["user_agent"] = userAgent
	return nil
}

// NormalizeOpenAICodexUserAgent validates and canonicalizes a configured Codex
// identity. Only supported official Codex identities are accepted.
func NormalizeOpenAICodexUserAgent(userAgent string) (string, error) {
	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		return "", nil
	}
	if len(userAgent) > maxOpenAIAccountUserAgentLength {
		return "", infraerrors.Newf(http.StatusBadRequest, "OPENAI_CODEX_USER_AGENT_INVALID", "OpenAI Codex user_agent must be at most %d characters", maxOpenAIAccountUserAgentLength)
	}
	_, pairedUserAgent, ok := openai.PairCodexClientIdentity(userAgent)
	if !ok {
		return "", infraerrors.New(http.StatusBadRequest, "OPENAI_CODEX_USER_AGENT_INVALID", "OpenAI Codex user_agent must be a supported Codex User-Agent")
	}
	return pairedUserAgent, nil
}

func resolveOpenAIOutboundIdentityCandidates(accountUA, systemUA string) openAIOutboundIdentity {
	if identity, ok := validOpenAIOutboundIdentity(accountUA); ok {
		return identity
	}
	if identity, ok := validOpenAIOutboundIdentity(systemUA); ok {
		return identity
	}
	if identity, ok := validOpenAIOutboundIdentity(DefaultOpenAICodexUserAgent); ok {
		return identity
	}
	return openAIOutboundIdentity{
		UserAgent:  codexCLIUserAgent,
		Originator: openai.CodexDefaultOriginator,
		Version:    codexCLIVersion,
	}
}

// resolveOpenAIOutboundIdentityWithVersion preserves the selected identity's
// client family and platform details while rebuilding its version declarations.
func resolveOpenAIOutboundIdentityWithVersion(accountUA, systemUA, configuredVersion string) openAIOutboundIdentity {
	identity := resolveOpenAIOutboundIdentityCandidates(accountUA, systemUA)
	version := NormalizeCodexClientVersion(configuredVersion)
	if version == "" || CompareVersions(version, codexUpstreamMinVersion) < 0 {
		version = codexCLIVersion
	}
	if userAgent := openai.SetCodexUserAgentVersion(identity.UserAgent, version); userAgent != "" {
		identity.UserAgent = userAgent
		identity.Version = version
	}
	return identity
}

func validOpenAIOutboundIdentity(userAgent string) (openAIOutboundIdentity, bool) {
	originator, pairedUserAgent, ok := openai.PairCodexClientIdentity(strings.TrimSpace(userAgent))
	if !ok {
		return openAIOutboundIdentity{}, false
	}
	version := openAIOutboundIdentityVersion(pairedUserAgent)
	if version == "" {
		return openAIOutboundIdentity{}, false
	}
	return openAIOutboundIdentity{UserAgent: pairedUserAgent, Originator: originator, Version: version}, true
}

func openAIOutboundIdentityVersion(userAgent string) string {
	_, suffix, ok := strings.Cut(strings.TrimSpace(userAgent), "/")
	if !ok {
		return ""
	}
	version := strings.Fields(suffix)
	if len(version) == 0 {
		return ""
	}
	return version[0]
}

// applyOpenAIOutboundIdentity is the final identity stage for an OpenAI
// request. Header overrides and inbound headers must run before this stage.
func (s *OpenAIGatewayService) applyOpenAIOutboundIdentity(ctx context.Context, account *Account, headers http.Header, useCodexIdentity bool) {
	if headers == nil {
		return
	}
	identity := resolveOpenAIOutboundIdentityFromSettings(ctx, account, nil)
	if s != nil {
		identity = s.resolveOpenAIOutboundIdentity(ctx, account)
	}
	applyResolvedOpenAIOutboundIdentity(headers, identity, useCodexIdentity)
}

func applyResolvedOpenAIOutboundIdentity(headers http.Header, identity openAIOutboundIdentity, useCodexIdentity bool) {
	if headers == nil {
		return
	}
	headers.Set("User-Agent", identity.UserAgent)
	if useCodexIdentity || headers.Get("Version") != "" {
		headers.Set("Version", identity.Version)
	}
	if !useCodexIdentity {
		headers.Del("Originator")
		return
	}
	headers.Set("Originator", identity.Originator)
}

// applyResolvedOpenAIOutboundIdentityToMap adapts the shared identity stage to
// the map-based HTTP client used by the quota service.
func applyResolvedOpenAIOutboundIdentityToMap(ctx context.Context, account *Account, headers map[string]string) {
	if len(headers) == 0 {
		return
	}
	for key := range headers {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "user-agent", "originator", "version":
			delete(headers, key)
		}
	}
	identity := resolveOpenAIOutboundIdentityFromSettings(ctx, account, nil)
	headers["User-Agent"] = identity.UserAgent
	headers["Originator"] = identity.Originator
	headers["Version"] = identity.Version
}

func (s *adminServiceImpl) resolveOpenAIOutboundIdentity(ctx context.Context, account *Account) openAIOutboundIdentity {
	var settingService *SettingService
	if s != nil {
		settingService = s.settingService
	}
	return resolveOpenAIOutboundIdentityFromSettings(ctx, account, settingService)
}

func (s *TokenRefreshService) resolveOpenAIOutboundIdentity(ctx context.Context, account *Account) openAIOutboundIdentity {
	return resolveOpenAIOutboundIdentityFromSettings(ctx, account, nil)
}

// OpenAICodexUpstreamMinVersion exposes the lower bound for repository-level
// OAuth clients that must pair User-Agent, Originator, and Version.
const (
	OpenAICodexUpstreamMinVersion = codexUpstreamMinVersion
	DefaultOpenAICodexVersion     = codexCLIVersion
)
