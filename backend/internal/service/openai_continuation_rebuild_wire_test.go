package service

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexLegacyRecoveryReachesActualHTTPWire(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		svc, _ := migrationTestService()
		svc.cache = &codexResponseRegistryTestCache{states: make(map[string]CodexConversationState)}
		svc.openaiWSStateStore = NewOpenAIWSStateStore(nil)
		account := migrationTestAccount()
		account.Status, account.Schedulable = StatusActive, true
		account.Extra[CodexInstallationPolicyExtraKey] = CodexInstallationStableV1
		c := rebuildTestContext(t, 1)
		ownerCtx := WithOpenAIWSRequestOwner(context.Background(), c)
		require.NoError(t, bindOpenAIWSResponseAccount(ownerCtx, svc.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), "resp_old", account.ID+1, svc.openAIAffinityResponseTTL()))
		original, _ := CodexRequestPlanFromContext(c.Request.Context())
		var request *http.Request
		var err error
		if passthrough {
			request, err = svc.buildUpstreamRequestOpenAIPassthrough(c.Request.Context(), c, account, []byte(rebuildTranscript), "test-wire-token")
		} else {
			snapshot, identityErr := svc.finalizeCodexOAuthIdentity(account, c, c.Request.Header, "")
			require.NoError(t, identityErr)
			stageCodexFingerprintIDs(c, snapshot)
			request, err = svc.buildUpstreamRequest(c.Request.Context(), c, account, []byte(rebuildTranscript), "test-wire-token", true, "", true)
		}
		require.NoError(t, err)
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		require.NoError(t, request.Body.Close())
		require.False(t, gjson.GetBytes(body, "previous_response_id").Exists(), string(body))
		require.Len(t, gjson.GetBytes(body, "input").Array(), 3)
		require.Equal(t, int64(len(body)), request.ContentLength)
		replay, err := request.GetBody()
		require.NoError(t, err)
		replayBody, err := io.ReadAll(replay)
		require.NoError(t, err)
		require.NoError(t, replay.Close())
		require.Equal(t, body, replayBody)
		attempt, ok := CodexAttemptStateFromContext(request.Context())
		require.True(t, ok)
		require.Equal(t, body, attempt.FinalHTTPBody())
		require.Equal(t, "resp_old", original.previousResponseID)
		owner, err := getOpenAIWSResponseAccount(ownerCtx, svc.getOpenAIWSStateStore(), getOpenAIGroupIDFromContext(c), "resp_old")
		require.NoError(t, err)
		require.Equal(t, account.ID+1, owner)
	}
}

func TestCodexLegacyRebuildDoesNotOptWebSocketIntoHTTPRecovery(t *testing.T) {
	c := rebuildTestContext(t, 1)
	plan, _ := CodexRequestPlanFromContext(c.Request.Context())
	plan.transport = CodexTransportWS
	_, err := PrepareCodexFullContextRecovery(c, 12, plan.body)
	require.Error(t, err)
	require.False(t, plan.rebuildFromLocalHistory)
	require.Equal(t, "resp_old", plan.previousResponseID)
}
