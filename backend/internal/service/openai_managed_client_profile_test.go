package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClientReleasePinsOldConversationAndFinalWire(t *testing.T) {
	for _, family := range []string{"pi", "opencode"} {
		t.Run(family, func(t *testing.T) {
			repo := &clientUpdateRepoStub{}
			update := defaultClientProfileUpdate()
			update.Active = testClientRelease(family, "9.2.3")
			update.Status = "current"
			repo.put(t, family, update)
			settings := &SettingService{settingRepo: repo}
			svc, registry := migrationTestService()
			svc.settingService = settings
			account := migrationTestAccount()
			extra, err := NormalizeCodexClientPresetExtra(map[string]any{CodexClientPresetExtraKey: family})
			require.NoError(t, err)
			account.Extra = extra
			firstContext := migrationTestContext(t, "first-managed-turn", CodexTransportHTTP)
			_, err = svc.finalizeCodexOAuthIdentity(account, firstContext, firstContext.Request.Header, "")
			require.NoError(t, err)
			first, ok := codexAttemptStateFromGin(firstContext)
			require.True(t, ok)
			require.NotNil(t, first.Profile().ClientRelease)
			require.Equal(t, "9.2.3", first.Profile().ClientRelease.Version)
			returned := first.Profile()
			returned.ClientRelease.Version = "corrupt"
			require.Equal(t, "9.2.3", first.Profile().ClientRelease.Version)
			before, err := PublicCodexClientCatalogWithUpdates("0.199.0", settings.ClientProfileUpdates(context.Background()))
			require.NoError(t, err)
			update.Active = testClientRelease(family, "9.3.0")
			repo.put(t, family, update)
			settings.rememberClientProfileUpdate(family, update)
			after, err := PublicCodexClientCatalogWithUpdates("0.199.0", settings.ClientProfileUpdates(context.Background()))
			require.NoError(t, err)
			require.NotEqual(t, before.Revision, after.Revision)
			for _, profile := range after.Profiles {
				if profile.ID == first.Profile().ID {
					require.Equal(t, "9.3.0", profile.AppVersion)
				}
			}
			// Reload the persisted JSON, not merely the in-memory attempt.
			saved, err := json.Marshal(registry.state)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(saved, &registry.state))
			continued := migrationTestContext(t, "continued-managed-turn", CodexTransportHTTP)
			_, err = svc.finalizeCodexOAuthIdentity(account, continued, continued.Request.Header, "")
			require.NoError(t, err)
			pinned, _ := codexAttemptStateFromGin(continued)
			require.Equal(t, first.Profile(), pinned.Profile())
			require.Equal(t, first.TransportKey(), pinned.TransportKey())
			fresh, _ := migrationTestService()
			fresh.settingService = settings
			newContext := migrationTestContext(t, "new-managed-conversation", CodexTransportHTTP)
			newPlan := mustCodexPlanForTest(t, "new-managed-conversation", "different-conversation", CodexTransportHTTP, time.Now())
			newContext.Request = newContext.Request.WithContext(ContextWithCodexRequestPlan(newContext.Request.Context(), newPlan))
			_, err = fresh.finalizeCodexOAuthIdentity(account, newContext, newContext.Request.Header, "")
			require.NoError(t, err)
			newAttempt, _ := codexAttemptStateFromGin(newContext)
			require.Equal(t, "9.3.0", newAttempt.Profile().ClientRelease.Version)
			require.NotEqual(t, first.TransportKey(), newAttempt.TransportKey())
			require.Equal(t, first.Identity().InstallationID(), newAttempt.Identity().InstallationID())
			require.NotEqual(t, first.Identity().SessionID(), newAttempt.Identity().SessionID())
			captured := make(chan http.Header, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured <- r.Header.Clone()
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			plan, ok := CodexRequestPlanFromContext(continued.Request.Context())
			require.True(t, ok)
			ctx := ContextWithCodexAttemptState(ContextWithCodexRequestPlan(context.Background(), plan), pinned)
			request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL, strings.NewReader(string(plan.Body())))
			require.NoError(t, err)
			request.Header.Set("Version", "9.3.0")
			request.Header.Set("x-openai-client-version", "9.3.0")
			svc.httpUpstream = bundleLoopbackSender{}
			response, err := svc.doOpenAIUpstream(request, "", account)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			headers := <-captured
			wantUA, err := testClientRelease(family, "9.2.3").UserAgent()
			require.NoError(t, err)
			require.Equal(t, wantUA, headers.Get("User-Agent"))
			require.Empty(t, headers.Get("Version"))
			require.Empty(t, headers.Get("x-openai-client-version"))
		})
	}
}
