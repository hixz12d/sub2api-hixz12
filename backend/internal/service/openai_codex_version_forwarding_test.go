package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForcedCodexIdentityUsesEffectiveVersion(t *testing.T) {
	for _, tc := range []struct {
		name, override, synced, want string
	}{
		{"synced", "", "0.200.1", "0.200.1"},
		{"manual override", "0.150.0", "0.200.1", "0.150.0"},
		{"fallback", "", "", codexCLIVersion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexClientVersion:       tc.override,
				SettingKeyOpenAICodexClientVersionSynced: tc.synced,
				SettingKeyOpenAICodexUserAgent:           "codex_vscode/0.144.1 (Windows 11; x86_64) vscode",
			}}, nil)
			identity := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), nil, nil, settings, true, "")
			require.Equal(t, tc.want, identity.Version)
			require.Equal(t, tc.want, openai.CodexUserAgentVersion(identity.UserAgent))
			require.Equal(t, openai.CodexDefaultOriginator, identity.Originator)
		})
	}
	identity := resolveOpenAIOutboundIdentityWithPolicy(context.Background(), nil, nil, nil, true, "")
	require.Equal(t, codexCLIVersion, identity.Version)
}

func TestMessagesBridgeIdentityUsesEffectiveVersion(t *testing.T) {
	for _, useSnapshot := range []bool{false, true} {
		name := "settings"
		if useSnapshot {
			name = "attempt snapshot"
		}
		t.Run(name, func(t *testing.T) {
			settings := NewSettingService(&codexVersionSettingRepoStub{values: map[string]string{
				SettingKeyOpenAICodexClientVersionSynced: "0.200.1",
			}}, nil)
			svc := &OpenAIGatewayService{settingService: settings}
			ctx := context.Background()
			want := "0.200.1"
			if useSnapshot {
				want = "0.199.0"
				ctx = withOpenAIOutboundIdentitySnapshot(ctx, resolveOpenAIOutboundIdentityWithVersion(
					"codex_vscode/0.199.0 (Windows 11; x86_64) vscode", "", want,
				))
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
			setOpenAICompatMessagesBridgeContext(c, true)
			headers := http.Header{"Version": {"0.1.0"}, "User-Agent": {"pi/0.1.0"}}
			svc.finalizeCodexOAuthHeaders(ctx, c, nil, headers, nil, "")
			require.Equal(t, want, headers.Get("Version"))
			require.Equal(t, want, openai.CodexUserAgentVersion(headers.Get("User-Agent")))
			require.Equal(t, openai.CodexDefaultOriginator, headers.Get("Originator"))
			require.Empty(t, headers.Get("conversation_id"))
		})
	}
}
