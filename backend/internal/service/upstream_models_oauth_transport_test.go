package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncUpstreamOAuthUsesCodexDiscoveryTransport(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "Bearer test-access-token", r.Header.Get("Authorization"))
				require.Equal(t, "acc-123", r.Header.Get("chatgpt-account-id"))
				require.Equal(t, r.Header.Get("Version"), r.URL.Query().Get("client_version"))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"models":[{"slug":"gpt-6-sol","reasoning":true,"supported_reasoning_levels":[{"effort":"medium"},{"effort":"max"}],"input_modalities":["text","image"],"context_window":1050000}]}`))
			}))
			defer server.Close()
			original := chatgptCodexModelsURL
			chatgptCodexModelsURL = server.URL
			defer func() { chatgptCodexModelsURL = original }()
			repo := &upstreamModelMetadataRepoStub{}
			svc := &AccountTestService{
				accountRepo:          repo,
				openaiGatewayService: &OpenAIGatewayService{},
				// The generic client is deliberately absent: OAuth must use Codex discovery.
			}
			catalog, err := svc.SyncUpstreamModelCatalog(context.Background(), newCodexModelsTestAccount())
			if status != http.StatusOK {
				var syncErr *UpstreamModelSyncError
				require.True(t, errors.As(err, &syncErr))
				require.Equal(t, status, syncErr.StatusCode)
				require.Equal(t, "Upstream model list request failed with HTTP 403", syncErr.SafeMessage())
				require.Nil(t, repo.updates)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"gpt-6-sol"}, catalog.Models)
			require.Empty(t, catalog.Warnings)
			require.Equal(t, int64(1050000), catalog.Metadata["gpt-6-sol"].ContextWindow)
			require.Contains(t, repo.updates, UpstreamModelMetadataExtraKey)
		})
	}
}
