package service

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const officialDesktopInstallationID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func officialCodexDesktopHeaders(installationID, turnMetadata string) http.Header {
	h := http.Header{}
	h.Set("User-Agent", "codex_cli_rs/0.151.0 (Windows NT 10.0; Win64; x64) electron")
	h.Set("originator", "codex_cli_rs")
	if installationID != "" {
		h.Set("x-codex-installation-id", installationID)
	}
	if turnMetadata != "" {
		h.Set("x-codex-turn-metadata", turnMetadata)
	}
	return h
}

func TestCanonicalCodexInstallationID(t *testing.T) {
	assert.Equal(t, officialDesktopInstallationID, canonicalCodexInstallationID("  "+officialDesktopInstallationID+"  "))
	assert.Empty(t, canonicalCodexInstallationID("real-device-id"))
	assert.Empty(t, canonicalCodexInstallationID("11111111-1111-7111-8111-111111111111"), "UUIDv7 不是官方 installation")
	assert.Empty(t, canonicalCodexInstallationID(""))
}

func TestAdoptOfficialCodexInstallationID(t *testing.T) {
	account := newTestOAuthAccount(9, map[string]any{codexFingerprintModeExtraKey: "window40"})
	derived := resolveConvergedInstallationID(account)
	require.NotEmpty(t, derived)

	headers := officialCodexDesktopHeaders(officialDesktopInstallationID, "")
	ids := resolveCodexFingerprintIDsFromRequest(account, headers)
	require.NotNil(t, ids)
	assert.Equal(t, officialDesktopInstallationID, ids.installationID)
	assert.True(t, ids.learnedOfficialInstallation)
	assert.NotEqual(t, derived, ids.installationID)

	apiHeaders := http.Header{}
	apiHeaders.Set("session-id", "python-session")
	apiHeaders.Set("x-codex-installation-id", officialDesktopInstallationID)
	apiIDs := resolveCodexFingerprintIDsFromRequest(account, apiHeaders)
	require.NotNil(t, apiIDs)
	assert.Equal(t, derived, apiIDs.installationID, "非官方客户端不得改写设备身份")
	assert.False(t, apiIDs.learnedOfficialInstallation)
}

func TestAdoptOfficialCodexInstallationIDKeepsStoredDevice(t *testing.T) {
	stored := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	account := newTestOAuthAccount(9, map[string]any{
		codexFingerprintModeExtraKey: "window40",
		openaiDeviceIDExtraKey:       stored,
	})
	headers := officialCodexDesktopHeaders(officialDesktopInstallationID, "")
	ids := resolveCodexFingerprintIDsFromRequest(account, headers)
	require.NotNil(t, ids)
	assert.Equal(t, stored, ids.installationID)
	assert.False(t, ids.learnedOfficialInstallation)
}

func TestAdoptOfficialCodexInstallationIDRejectsHeaderMetadataMismatch(t *testing.T) {
	account := newTestOAuthAccount(9, map[string]any{codexFingerprintModeExtraKey: "window40"})
	derived := resolveConvergedInstallationID(account)
	meta, err := json.Marshal(map[string]any{"installation_id": "cccccccc-cccc-4ccc-8ccc-cccccccccccc"})
	require.NoError(t, err)
	headers := officialCodexDesktopHeaders(officialDesktopInstallationID, string(meta))
	ids := resolveCodexFingerprintIDsFromRequest(account, headers)
	require.NotNil(t, ids)
	assert.Equal(t, derived, ids.installationID)
	assert.False(t, ids.learnedOfficialInstallation)
}

func TestPersistLearnedCodexDeviceID(t *testing.T) {
	account := newTestOAuthAccount(31, map[string]any{codexFingerprintModeExtraKey: "window40"})
	repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	svc := &OpenAIGatewayService{accountRepo: repo}

	headers := officialCodexDesktopHeaders(officialDesktopInstallationID, "")
	snapshot, err := finalizeCodexOAuthIdentity(account, nil, headers, "")
	require.NoError(t, err)
	require.NotNil(t, snapshot)
	assert.Equal(t, officialDesktopInstallationID, snapshot.installationID)

	svc.persistLearnedCodexDeviceID(context.Background(), account, snapshot)
	assert.Equal(t, officialDesktopInstallationID, account.GetOpenAIDeviceID())
	require.NotEmpty(t, repo.updates[account.ID])
	assert.Equal(t, officialDesktopInstallationID, repo.updates[account.ID][0][openaiDeviceIDExtraKey])

	svc.persistLearnedCodexDeviceID(context.Background(), account, snapshot)
	assert.Len(t, repo.updates[account.ID], 1, "已记住的设备身份不得反复写入")

	later := resolveCodexFingerprintIDsFromRequest(account, http.Header{"session-id": []string{"api-client"}})
	require.NotNil(t, later)
	assert.Equal(t, officialDesktopInstallationID, later.installationID)
}
