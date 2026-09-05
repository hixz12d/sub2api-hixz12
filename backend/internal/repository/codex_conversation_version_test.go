package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func testPinnedCodexState(t *testing.T, profileID string) service.CodexConversationState {
	t.Helper()
	state := testCodexConversationState(44)
	profile, err := service.ResolveCodexClientProfile(profileID)
	require.NoError(t, err)
	encoded, err := json.Marshal(profile)
	require.NoError(t, err)
	digest := sha256.Sum256(encoded)
	state.ProfileID = profile.ID
	state.ProfileSnapshot = &profile
	state.ProfileDigest = hex.EncodeToString(digest[:])
	state.FingerprintMode = "device"
	state.InstallationPolicy = service.CodexInstallationStableV1
	return state
}

func TestCodexPinnedProfileSurvivesLuaAndActivityTTL(t *testing.T) {
	for _, profile := range service.CodexClientProfiles() {
		t.Run(profile.ID, func(t *testing.T) {
			mr := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			registry := NewGatewayCache(client).(service.CodexConversationRegistry)
			digest := testCodexConversationDigest(profile.ID)
			candidate := testPinnedCodexState(t, profile.ID)
			stored, created, err := registry.ResolveOrCreateCodexConversation(context.Background(), digest, candidate, 2*time.Hour)
			require.NoError(t, err)
			require.True(t, created)
			mr.FastForward(119 * time.Minute)
			stored.Committed = true
			stored.LastActivityUnixMS++
			updated, err := registry.CompareAndSwapCodexConversation(context.Background(), digest, stored.Revision, stored.AccountID, stored, 2*time.Hour)
			require.NoError(t, err)
			require.Equal(t, candidate.ProfileSnapshot, updated.ProfileSnapshot)
			require.Equal(t, candidate.ProfileDigest, updated.ProfileDigest)
			require.Equal(t, service.CodexInstallationStableV1, updated.InstallationPolicy)
			require.Equal(t, 2*time.Hour, mr.TTL(codexConversationPrefix+digest))
			mr.FastForward(2 * time.Minute)
			_, err = registry.GetCodexConversation(context.Background(), digest)
			require.NoError(t, err, "active record survives its original two-hour deadline")
			mr.FastForward(2 * time.Hour)
			_, err = registry.GetCodexConversation(context.Background(), digest)
			require.ErrorIs(t, err, service.ErrCodexConversationNotFound)
		})
	}
}

func TestCodexPinnedProfileConcurrentCreationKeepsOneSnapshot(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	registry := NewGatewayCache(client).(service.CodexConversationRegistry)
	digest := testCodexConversationDigest("snapshot-race")
	candidates := []service.CodexConversationState{testPinnedCodexState(t, service.CodexProfileCLI), testPinnedCodexState(t, service.CodexProfileExec)}
	const workers = 24
	states := make([]service.CodexConversationState, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range states {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			states[i], _, errs[i] = registry.ResolveOrCreateCodexConversation(context.Background(), digest, candidates[i%2], time.Hour)
		}(i)
	}
	wg.Wait()
	for i := range states {
		require.NoError(t, errs[i])
		require.Equal(t, states[0].ProfileDigest, states[i].ProfileDigest)
		require.Equal(t, states[0].ProfileSnapshot, states[i].ProfileSnapshot)
	}
}

func TestCodexRegistryRejectsCorruptPinnedDigest(t *testing.T) {
	state := testPinnedCodexState(t, service.CodexProfileCLI)
	state.ProfileDigest = "not-the-snapshot-digest"
	payload, err := json.Marshal(state)
	require.NoError(t, err)
	_, err = decodeCodexConversationState(string(payload))
	require.Error(t, err)
	state.ProfileSnapshot = nil
	payload, err = json.Marshal(state)
	require.NoError(t, err)
	_, err = decodeCodexConversationState(string(payload))
	require.Error(t, err)
}
