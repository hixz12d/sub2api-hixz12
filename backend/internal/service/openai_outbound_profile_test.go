package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/stretchr/testify/require"
)

const testOutboundCodexUserAgent = "codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color"

func TestResolveOpenAIOutboundIdentityCandidates(t *testing.T) {
	accountUA := "codex-tui/0.150.0 (Ubuntu 22.4.0; x86_64) xterm-256color (codex-tui; 0.150.0)"

	tests := []struct {
		name           string
		accountUA      string
		systemUA       string
		wantUserAgent  string
		wantOriginator string
		wantVersion    string
	}{
		{
			name:           "account identity wins",
			accountUA:      accountUA,
			systemUA:       testOutboundCodexUserAgent,
			wantUserAgent:  accountUA,
			wantOriginator: "codex-tui",
			wantVersion:    "0.150.0",
		},
		{
			name:           "system identity is fallback",
			accountUA:      "Mozilla/5.0",
			systemUA:       testOutboundCodexUserAgent,
			wantUserAgent:  testOutboundCodexUserAgent,
			wantOriginator: "codex_cli_rs",
			wantVersion:    "0.144.1",
		},
		{
			name:           "compiled identity is final fallback",
			accountUA:      "curl/8.7.1",
			systemUA:       "not-a-codex-client/1.0",
			wantUserAgent:  DefaultOpenAICodexUserAgent,
			wantOriginator: openai.CodexDefaultOriginator,
			wantVersion:    codexCLIVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := resolveOpenAIOutboundIdentityCandidates(tt.accountUA, tt.systemUA)
			require.Equal(t, tt.wantUserAgent, identity.UserAgent)
			require.Equal(t, tt.wantOriginator, identity.Originator)
			require.Equal(t, tt.wantVersion, identity.Version)
		})
	}
}

func TestResolveOpenAIOutboundIdentityWithVersion(t *testing.T) {
	identity := resolveOpenAIOutboundIdentityWithVersion(
		"codex_vscode/0.120.0 (Windows 11; x86_64) vscode",
		testOutboundCodexUserAgent,
		"0.151.0",
	)
	require.Equal(t, "codex_vscode/0.151.0 (Windows 11; x86_64) vscode", identity.UserAgent)
	require.Equal(t, "codex_vscode", identity.Originator)
	require.Equal(t, "0.151.0", identity.Version)
}

func TestNormalizeOpenAIAccountUserAgent(t *testing.T) {
	credentials := map[string]any{"user_agent": "  " + testOutboundCodexUserAgent + "  "}
	require.NoError(t, NormalizeOpenAIAccountUserAgent(PlatformOpenAI, credentials))
	require.Equal(t, testOutboundCodexUserAgent, credentials["user_agent"])

	err := NormalizeOpenAIAccountUserAgent(PlatformOpenAI, map[string]any{"user_agent": "curl/8.0"})
	require.Error(t, err)
}

func TestApplyResolvedOpenAIOutboundIdentity(t *testing.T) {
	identity := resolveOpenAIOutboundIdentityCandidates("", testOutboundCodexUserAgent)

	t.Run("oauth identity triple", func(t *testing.T) {
		headers := http.Header{
			"User-Agent": {"Mozilla/5.0"},
			"Originator": {"client-controlled"},
		}
		applyResolvedOpenAIOutboundIdentity(headers, identity, true)
		require.Equal(t, testOutboundCodexUserAgent, headers.Get("User-Agent"))
		require.Equal(t, "codex_cli_rs", headers.Get("Originator"))
		require.Equal(t, "0.144.1", headers.Get("Version"))
	})

	t.Run("api key omits oauth headers", func(t *testing.T) {
		headers := http.Header{
			"User-Agent": {"Mozilla/5.0"},
			"Originator": {"client-controlled"},
		}
		applyResolvedOpenAIOutboundIdentity(headers, identity, false)
		require.Equal(t, testOutboundCodexUserAgent, headers.Get("User-Agent"))
		require.Empty(t, headers.Get("Originator"))
		require.Empty(t, headers.Get("Version"))
	})

	t.Run("compact request keeps version synchronized", func(t *testing.T) {
		headers := http.Header{"Version": {"0.1.0"}}
		applyResolvedOpenAIOutboundIdentityWithPolicy(headers, identity, openAIOutboundAPIKeyCodexVersionPolicy)
		require.Equal(t, "0.144.1", headers.Get("Version"))
	})
}

func TestOpenAIOutboundIdentitySnapshotAndEnforcement(t *testing.T) {
	original := codexIdentityEnforcement.Load()
	t.Cleanup(func() { codexIdentityEnforcement.Store(original) })

	inbound := "codex_cli_rs/0.155.0 (Ubuntu 22.4.0; x86_64) xterm"
	codexIdentityEnforcement.Store(false)
	compat := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), nil, nil, nil, false, inbound)
	require.Equal(t, inbound, compat.UserAgent)
	require.Equal(t, "codex_cli_rs", compat.Originator)

	codexIdentityEnforcement.Store(true)
	enforced := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), nil, nil, nil, false, inbound)
	require.NotEqual(t, inbound, enforced.UserAgent)
	require.Equal(t, enforced.Version, openai.CodexUserAgentVersion(enforced.UserAgent))

	ctx := withOpenAIOutboundIdentitySnapshot(context.Background(), compat)
	codexIdentityEnforcement.Store(true)
	snapshot := resolveOpenAIOutboundIdentityWithPolicy(ctx, nil, nil, nil, false, "different/0.160.0")
	require.Equal(t, compat, snapshot)
}

func TestResolveOpenAIOutboundIdentityInheritsCredentialParent(t *testing.T) {
	parentID := int64(501)
	parent := &Account{
		ID:       parentID,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"user_agent": "codex_vscode/0.144.1 (Windows 11; x86_64) vscode",
		},
	}
	shadow := &Account{
		ID:              502,
		Platform:        PlatformOpenAI,
		Type:            AccountTypeOAuth,
		ParentAccountID: &parentID,
		QuotaDimension:  QuotaDimensionSpark,
	}
	repo := &stubOpenAIAccountRepo{accounts: []Account{*parent}}

	identity := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), shadow, repo, nil, false, "")
	require.Equal(t, "codex_vscode", identity.Originator)
	require.Contains(t, identity.UserAgent, "codex_vscode/")
	require.Equal(t, identity.Version, openai.CodexUserAgentVersion(identity.UserAgent))
}

func TestApplyResolvedOpenAIOutboundIdentityPolicies(t *testing.T) {
	identity := resolveOpenAIOutboundIdentityCandidates("", testOutboundCodexUserAgent)
	headers := http.Header{
		"User-Agent": {"client/0.1"},
		"Originator": {"client-controlled"},
		"Version":    {"0.1.0"},
	}
	applyResolvedOpenAIOutboundIdentityWithPolicy(headers, identity, openAIOutboundAPIKeyPolicy)
	require.Equal(t, testOutboundCodexUserAgent, headers.Get("User-Agent"))
	require.Empty(t, headers.Get("Originator"))
	require.Empty(t, headers.Get("Version"))

	headers.Set("Originator", "client-controlled")
	applyResolvedOpenAIOutboundIdentityWithPolicy(headers, identity, openAIOutboundOAuthPolicy)
	require.Equal(t, "codex_cli_rs", headers.Get("Originator"))
	require.Equal(t, "0.144.1", headers.Get("Version"))
}
