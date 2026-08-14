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
// openAIOutboundIdentity remains as an internal compatibility alias while the
// complete, versioned protocol tuple is represented by CodexProtocolProfile.
type openAIOutboundIdentity = CodexProtocolProfile

type openAIOutboundEndpointPolicy struct {
	UseCodexIdentity bool
	RequireVersion   bool
}

var (
	openAIOutboundOAuthPolicy              = openAIOutboundEndpointPolicy{UseCodexIdentity: true, RequireVersion: true}
	openAIOutboundAPIKeyPolicy             = openAIOutboundEndpointPolicy{}
	openAIOutboundAPIKeyCodexVersionPolicy = openAIOutboundEndpointPolicy{RequireVersion: true}
)

type openAIOutboundIdentitySnapshotKey struct{}

func openAIOutboundIdentityFromContext(ctx context.Context) (openAIOutboundIdentity, bool) {
	if ctx == nil {
		return openAIOutboundIdentity{}, false
	}
	identity, ok := ctx.Value(openAIOutboundIdentitySnapshotKey{}).(openAIOutboundIdentity)
	return identity, ok && identity.UserAgent != "" && identity.Version != ""
}

func withOpenAIOutboundIdentitySnapshot(ctx context.Context, identity openAIOutboundIdentity) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := openAIOutboundIdentityFromContext(ctx); ok {
		return ctx
	}
	if identity.UserAgent == "" || identity.Version == "" {
		return ctx
	}
	return context.WithValue(ctx, openAIOutboundIdentitySnapshotKey{}, identity)
}

// withoutOpenAIOutboundIdentitySnapshot starts a new account attempt while
// retaining cancellation, tracing, and other request-local values.
func withoutOpenAIOutboundIdentitySnapshot(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, openAIOutboundIdentitySnapshotKey{}, nil)
}

// resolveOpenAIOutboundIdentityFromSettings is the single source of truth for
// selecting an outbound OpenAI identity. A valid account identity wins over
// the global identity, followed by the compiled-in default. The selected
// client family and platform fingerprint are preserved while its version is
// synchronized with the currently effective version setting.
func resolveOpenAIOutboundIdentityFromSettings(ctx context.Context, account *Account, settingService *SettingService) openAIOutboundIdentity {
	return resolveOpenAIOutboundIdentityWithPolicy(ctx, account, nil, settingService, false, "")
}

func resolveOpenAIOutboundIdentityWithPolicy(
	ctx context.Context,
	account *Account,
	accountRepo AccountRepository,
	settingService *SettingService,
	forceCodexCLI bool,
	inboundUserAgent string,
) openAIOutboundIdentity {
	if identity, ok := openAIOutboundIdentityFromContext(ctx); ok {
		return identity
	}
	if account != nil && account.IsCredentialShadow() && accountRepo != nil {
		if credentialAccount, err := resolveCredentialAccount(ctx, accountRepo, account); err == nil && credentialAccount != nil {
			account = credentialAccount
		} else {
			account = nil
		}
	}
	accountUA := ""
	if account != nil {
		accountUA = account.GetOpenAIUserAgent()
	}
	systemUA := codexCanonicalUserAgent()
	if settingService != nil {
		systemUA = settingService.GetOpenAICodexCanonicalUserAgent(ctx)
	}
	if forceCodexCLI {
		return resolveOpenAIOutboundIdentityWithVersion("", codexCLIUserAgent, codexCLIVersion)
	}
	if !codexIdentityEnforcement.Load() && strings.TrimSpace(accountUA) == "" {
		if identity, ok := validOpenAIOutboundIdentity(inboundUserAgent); ok {
			version := identity.Version
			if CompareVersions(version, codexUpstreamMinVersion) < 0 {
				version = codexCLIVersion
			}
			return resolveOpenAIOutboundIdentityWithVersion(inboundUserAgent, inboundUserAgent, version)
		}
	}
	return resolveOpenAIOutboundIdentityWithVersion(accountUA, systemUA, codexClientVersionFromUA(systemUA))
}

// resolveOpenAIOutboundIdentity resolves shadow accounts to their credential
// owner before selecting the account-level identity. ForceCodexCLI remains a
// local compatibility switch and deliberately wins over account/global UA.
func (s *OpenAIGatewayService) resolveOpenAIOutboundIdentity(ctx context.Context, account *Account) openAIOutboundIdentity {
	var accountRepo AccountRepository
	var settingService *SettingService
	forceCodexCLI := false
	if s != nil {
		accountRepo = s.accountRepo
		settingService = s.settingService
		forceCodexCLI = s.cfg != nil && s.cfg.Gateway.ForceCodexCLI
	}
	return resolveOpenAIOutboundIdentityWithPolicy(ctx, account, accountRepo, settingService, forceCodexCLI, "")
}

func (s *OpenAIGatewayService) snapshotOpenAIOutboundIdentity(ctx context.Context, account *Account, inboundUserAgent string) context.Context {
	if _, ok := openAIOutboundIdentityFromContext(ctx); ok {
		return ctx
	}
	var accountRepo AccountRepository
	var settingService *SettingService
	forceCodexCLI := false
	if s != nil {
		accountRepo = s.accountRepo
		settingService = s.settingService
		forceCodexCLI = s.cfg != nil && s.cfg.Gateway.ForceCodexCLI
	}
	identity := resolveOpenAIOutboundIdentityWithPolicy(ctx, account, accountRepo, settingService, forceCodexCLI, inboundUserAgent)
	return withOpenAIOutboundIdentitySnapshot(ctx, identity)
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
	return completeCodexProtocolProfile(identity)
}

func validOpenAIOutboundIdentity(userAgent string) (openAIOutboundIdentity, bool) {
	originator, pairedUserAgent, ok := openai.PairCodexClientIdentity(strings.TrimSpace(userAgent))
	if !ok {
		return openAIOutboundIdentity{}, false
	}
	version := NormalizeCodexClientVersion(openai.CodexUserAgentVersion(pairedUserAgent))
	if version == "" {
		return openAIOutboundIdentity{}, false
	}
	return openAIOutboundIdentity{UserAgent: pairedUserAgent, Originator: originator, Version: version}, true
}

// applyOpenAIOutboundIdentity is the final identity stage for an OpenAI
// request. Header overrides and inbound headers must run before this stage.
func (s *OpenAIGatewayService) applyOpenAIOutboundIdentity(ctx context.Context, account *Account, headers http.Header, useCodexIdentity bool) {
	policy := openAIOutboundAPIKeyPolicy
	if useCodexIdentity {
		policy = openAIOutboundOAuthPolicy
	}
	s.applyOpenAIOutboundIdentityPolicy(ctx, account, headers, policy)
}

func (s *OpenAIGatewayService) applyOpenAIOutboundIdentityPolicy(ctx context.Context, account *Account, headers http.Header, policy openAIOutboundEndpointPolicy) {
	if headers == nil {
		return
	}
	identity, ok := openAIOutboundIdentityFromContext(ctx)
	if !ok {
		var settingService *SettingService
		var accountRepo AccountRepository
		forceCodexCLI := false
		if s != nil {
			settingService = s.settingService
			accountRepo = s.accountRepo
			forceCodexCLI = s.cfg != nil && s.cfg.Gateway.ForceCodexCLI
		}
		identity = resolveOpenAIOutboundIdentityWithPolicy(ctx, account, accountRepo, settingService, forceCodexCLI, headers.Get("User-Agent"))
	}
	applyResolvedOpenAIOutboundIdentityWithPolicy(headers, identity, policy)
}

func applyResolvedOpenAIOutboundIdentity(headers http.Header, identity openAIOutboundIdentity, useCodexIdentity bool) {
	policy := openAIOutboundAPIKeyPolicy
	if useCodexIdentity {
		policy = openAIOutboundOAuthPolicy
	}
	applyResolvedOpenAIOutboundIdentityWithPolicy(headers, identity, policy)
}

func applyResolvedOpenAIOutboundIdentityWithPolicy(headers http.Header, identity openAIOutboundIdentity, policy openAIOutboundEndpointPolicy) {
	if headers == nil {
		return
	}
	headers.Set("User-Agent", identity.UserAgent)
	if policy.RequireVersion {
		headers.Set("Version", identity.Version)
	} else {
		headers.Del("Version")
	}
	if !policy.UseCodexIdentity {
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
	identity, ok := openAIOutboundIdentityFromContext(ctx)
	if !ok {
		identity = resolveOpenAIOutboundIdentityWithPolicy(ctx, account, nil, nil, false, "")
	}
	headers["User-Agent"] = identity.UserAgent
	headers["Originator"] = identity.Originator
	headers["Version"] = identity.Version
}

func (s *adminServiceImpl) resolveOpenAIOutboundIdentity(ctx context.Context, account *Account) openAIOutboundIdentity {
	var accountRepo AccountRepository
	var settingService *SettingService
	if s != nil {
		accountRepo = s.accountRepo
		settingService = s.settingService
	}
	return resolveOpenAIOutboundIdentityWithPolicy(ctx, account, accountRepo, settingService, false, "")
}

func (s *TokenRefreshService) resolveOpenAIOutboundIdentity(ctx context.Context, account *Account) openAIOutboundIdentity {
	var accountRepo AccountRepository
	if s != nil {
		accountRepo = s.accountRepo
	}
	return resolveOpenAIOutboundIdentityWithPolicy(ctx, account, accountRepo, nil, false, "")
}

// ResolveOpenAIOAuthIdentity normalizes a repository OAuth client's identity
// through the same policy primitives used by gateway requests.
func ResolveOpenAIOAuthIdentity(userAgent, version string) (string, string, string) {
	resolvedVersion := NormalizeCodexClientVersion(version)
	if resolvedVersion == "" {
		resolvedVersion = codexClientVersionFromUA(userAgent)
	}
	identity := resolveOpenAIOutboundIdentityWithVersion(userAgent, codexCanonicalUserAgent(), resolvedVersion)
	return identity.UserAgent, identity.Originator, identity.Version
}

// OpenAICodexUpstreamMinVersion exposes the lower bound for repository-level
// OAuth clients that must pair User-Agent, Originator, and Version.
const (
	OpenAICodexUpstreamMinVersion = codexUpstreamMinVersion
	DefaultOpenAICodexVersion     = codexCLIVersion
)
