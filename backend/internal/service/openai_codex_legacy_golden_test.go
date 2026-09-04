package service

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCodexLegacyV1FingerprintGolden(t *testing.T) {
	originalNow := codexFingerprintNow
	codexFingerprintNow = func() time.Time {
		return time.Date(2026, time.September, 4, 10, 30, 0, 0, time.UTC)
	}
	t.Cleanup(func() { codexFingerprintNow = originalNow })

	const clientSeed = "client-session"
	const installationID = "3a479c33-60a4-4c91-8479-62da0fecf605"

	tests := []struct {
		mode       codexFingerprintMode
		sessionID  string
		threadID   string
		windowID   string
		expectsIDs bool
	}{
		{mode: codexFingerprintOff},
		{
			mode:       codexFingerprintDevice,
			sessionID:  "2a2d0b25-6607-4261-8937-ea2716cc5a82",
			threadID:   "bcfee90a-4f53-4bca-84b3-27c6c5d6302d",
			windowID:   "bcfee90a-4f53-4bca-84b3-27c6c5d6302d:0",
			expectsIDs: true,
		},
		{
			mode:       codexFingerprintSession,
			sessionID:  "4065c8ec-c2ce-48bd-9198-53c4c5e30d6e",
			threadID:   "5483e12a-a512-4334-899e-d8d3a9b29dc8",
			windowID:   "5483e12a-a512-4334-899e-d8d3a9b29dc8:0",
			expectsIDs: true,
		},
		{
			mode:       codexFingerprintWindow,
			sessionID:  "4065c8ec-c2ce-48bd-9198-53c4c5e30d6e",
			threadID:   "9b7e58fe-c5ba-4e12-b69a-ac34a7cded4b",
			windowID:   "9b7e58fe-c5ba-4e12-b69a-ac34a7cded4b:0",
			expectsIDs: true,
		},
		{
			mode:       codexFingerprintWindow40,
			sessionID:  "0195c2e9-13de-7a52-9492-9a92d37a9298",
			threadID:   "0195c2e9-13de-7a52-9492-9a92d37a9298",
			windowID:   "0195c2e9-13df-7cf9-9f61-8bab03ceeb6b",
			expectsIDs: true,
		},
		{
			mode:       codexFingerprintFull,
			sessionID:  "4065c8ec-c2ce-48bd-9198-53c4c5e30d6e",
			threadID:   "4065c8ec-c2ce-48bd-9198-53c4c5e30d6e",
			windowID:   "4065c8ec-c2ce-48bd-9198-53c4c5e30d6e:0",
			expectsIDs: true,
		},
	}

	for _, tt := range tests {
		t.Run(string(tt.mode), func(t *testing.T) {
			account := newTestOAuthAccount(901, map[string]any{codexFingerprintModeExtraKey: string(tt.mode)})
			ids := resolveCodexFingerprintIDs(account, clientSeed, tt.mode)
			if !tt.expectsIDs {
				require.Nil(t, ids)
				return
			}

			require.NotNil(t, ids)
			require.Equal(t, installationID, ids.installationID)
			require.Equal(t, tt.sessionID, ids.sessionID)
			require.Equal(t, tt.threadID, ids.threadID)
			require.Equal(t, tt.windowID, ids.windowID)
			require.Equal(t, time.Date(2026, time.September, 4, 10, 30, 0, 0, time.UTC).UnixMilli(), ids.turnStartedAtUnixMs)
			turnID, err := uuid.Parse(ids.turnID)
			require.NoError(t, err)
			require.Equal(t, uuid.Version(7), turnID.Version())
		})
	}
}

func TestCodexLegacyV1DisabledGoldenSanitizesWithoutInventingIdentity(t *testing.T) {
	account := newTestOAuthAccount(902, map[string]any{codexFingerprintModeExtraKey: string(codexFingerprintOff)})
	snapshot, err := finalizeCodexOAuthIdentity(account, nil, http.Header{
		"session-id": {"caller-session"},
		"thread-id":  {"caller-thread"},
	}, "caller-cache")
	require.NoError(t, err)
	require.Nil(t, snapshot)

	body := []byte(`{"model":"gpt-5","prompt_cache_key":"caller-cache","client_metadata":{"session_id":"caller-session","thread_id":"caller-thread","hostname":"private-host"}}`)
	updated, err := applyCodexFingerprintToRawBody(body, snapshot)
	require.NoError(t, err)

	var got map[string]any
	require.NoError(t, json.Unmarshal(updated, &got))
	metadata := got["client_metadata"].(map[string]any)
	require.Equal(t, "caller-session", metadata["session_id"])
	require.Equal(t, "caller-thread", metadata["thread_id"])
	require.NotContains(t, metadata, "hostname")
	require.Equal(t, "caller-cache", got["prompt_cache_key"])
}
