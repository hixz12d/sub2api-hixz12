package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

func TestCodexPlatformPersonaPoolLooksOfficial(t *testing.T) {
	require.GreaterOrEqual(t, len(codexPlatformPersonaPool), 80)

	var windows, mac, linux int
	for _, persona := range codexPlatformPersonaPool {
		ua := persona.UserAgent(codexCLIVersion)
		originator, paired, ok := openai.PairCodexClientIdentity(ua)
		require.Truef(t, ok, "persona must pair: %+v ua=%q", persona, ua)
		require.Equal(t, persona.Originator, originator)
		require.Equal(t, ua, paired)
		require.NotContains(t, strings.ToLower(ua), "sub2api")
		switch {
		case strings.Contains(persona.Platform, "Windows"):
			windows++
			require.NotContains(t, persona.Terminal, "iTerm")
			require.NotEqual(t, "xterm-256color", persona.Terminal)
		case strings.HasPrefix(persona.Platform, "Mac OS X"):
			mac++
			require.NotEqual(t, "WindowsTerminal", persona.Terminal)
			require.NotEqual(t, "xterm-256color", persona.Terminal)
		case strings.HasPrefix(persona.Platform, "Ubuntu"):
			linux++
			require.NotContains(t, persona.Terminal, "iTerm")
			require.NotEqual(t, "WindowsTerminal", persona.Terminal)
		default:
			t.Fatalf("unexpected platform %q", persona.Platform)
		}
	}
	require.Greater(t, windows, 0)
	require.Greater(t, mac, 0)
	require.Greater(t, linux, 0)
}

func TestAccountStableCodexPersonaIsDurableAndDiverse(t *testing.T) {
	account := &Account{
		ID:       42,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			codexFingerprintSeedExtraKey: testCodexFingerprintSeed,
		},
	}

	first := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), account, nil, nil, false, "sub2api/1.0")
	second := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), account, nil, nil, false, "curl/8.0")
	require.Equal(t, first, second)
	requireOfficialCodexOutboundIdentityFromProfile(t, first)

	seen := map[string]struct{}{}
	for id := int64(1); id <= 48; id++ {
		identity := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), &Account{
			ID:       id,
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
		}, nil, nil, false, "")
		requireOfficialCodexOutboundIdentityFromProfile(t, identity)
		seen[identity.UserAgent] = struct{}{}
	}
	require.Greater(t, len(seen), 10, "different OAuth accounts should not collapse to one platform shell")
}

func TestAccountStableCodexPersonaKeepsExplicitAccountUA(t *testing.T) {
	const pinned = "codex-tui/0.144.1 (Mac OS X 15.1.0; arm64) iTerm.app"
	account := &Account{
		ID:       77,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"user_agent": pinned,
		},
	}
	identity := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), account, nil, nil, false, "")
	require.Equal(t, "codex-tui", identity.Originator)
	require.Contains(t, identity.UserAgent, "(Mac OS X 15.1.0; arm64) iTerm.app")
	require.Equal(t, identity.Version, openai.CodexUserAgentVersion(identity.UserAgent))
}

func TestAccountStableCodexPersonaDoesNotApplyWithoutOAuthAccount(t *testing.T) {
	identity := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), nil, nil, nil, false, "")
	require.Equal(t, DefaultOpenAICodexUserAgent, identity.UserAgent)
	require.Equal(t, openai.CodexDefaultOriginator, identity.Originator)
}

func requireOfficialCodexOutboundHeaders(t *testing.T, headers http.Header) {
	t.Helper()
	require.NotNil(t, headers)
	ua := headers.Get("User-Agent")
	originator, paired, ok := openai.PairCodexClientIdentity(ua)
	require.True(t, ok, ua)
	require.Equal(t, originator, headers.Get("originator"))
	require.Equal(t, paired, ua)
	require.Equal(t, openai.CodexUserAgentVersion(ua), headers.Get("version"))
	require.NotContains(t, strings.ToLower(ua), "sub2api")
	require.NotContains(t, strings.ToLower(headers.Get("originator")), "sub2api")
}

func requireOfficialCodexOutboundIdentityFromProfile(t *testing.T, identity openAIOutboundIdentity) {
	t.Helper()
	originator, paired, ok := openai.PairCodexClientIdentity(identity.UserAgent)
	require.True(t, ok, identity.UserAgent)
	require.Equal(t, originator, identity.Originator)
	require.Equal(t, paired, identity.UserAgent)
	require.Equal(t, identity.Version, openai.CodexUserAgentVersion(identity.UserAgent))
	require.NotContains(t, strings.ToLower(identity.UserAgent), "sub2api")
	require.NotContains(t, strings.ToLower(identity.Originator), "sub2api")
}
