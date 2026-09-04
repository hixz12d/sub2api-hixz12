package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCodexConversationRegistryResolveOrCreateIsAtomic(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	registry := NewGatewayCache(client).(service.CodexConversationRegistry)
	digest := testCodexConversationDigest("race")

	const workers = 24
	var createdCount atomic.Int32
	states := make([]service.CodexConversationState, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			candidate := testCodexConversationState(int64(index + 1))
			state, created, err := registry.ResolveOrCreateCodexConversation(context.Background(), digest, candidate, time.Hour)
			states[index] = state
			errs[index] = err
			if created {
				createdCount.Add(1)
			}
		}(i)
	}
	wg.Wait()

	require.Equal(t, int32(1), createdCount.Load())
	winner := states[0].AccountID
	for i := range states {
		require.NoError(t, errs[i])
		require.Equal(t, winner, states[i].AccountID)
		require.Equal(t, int64(1), states[i].Revision)
	}
	require.Equal(t, time.Hour, mr.TTL(codexConversationPrefix+digest))
}

func TestCodexConversationRegistryCASAndInvalidate(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	registry := NewGatewayCache(client).(service.CodexConversationRegistry)
	digest := testCodexConversationDigest("cas")
	candidate := testCodexConversationState(77)

	created, wasCreated, err := registry.ResolveOrCreateCodexConversation(context.Background(), digest, candidate, 0)
	require.NoError(t, err)
	require.True(t, wasCreated)
	require.Equal(t, int64(1), created.Revision)

	next := created
	next.Committed = true
	next.LastActivityUnixMS++
	updated, err := registry.CompareAndSwapCodexConversation(context.Background(), digest, created.Revision, created.AccountID, next, 3*time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(2), updated.Revision)
	require.True(t, updated.Committed)
	require.Equal(t, created.CreatedAtUnixMS, updated.CreatedAtUnixMS)

	_, err = registry.CompareAndSwapCodexConversation(context.Background(), digest, created.Revision, created.AccountID, next, time.Hour)
	require.ErrorIs(t, err, service.ErrCodexConversationCASConflict)

	invalidated, err := registry.InvalidateCodexConversation(context.Background(), digest, updated.Revision, updated.AccountID)
	require.NoError(t, err)
	require.True(t, invalidated)
	_, err = registry.GetCodexConversation(context.Background(), digest)
	require.ErrorIs(t, err, service.ErrCodexConversationNotFound)
}

func TestCodexConversationRegistryRejectsRawConversationKey(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	registry := NewGatewayCache(client).(service.CodexConversationRegistry)
	_, _, err := registry.ResolveOrCreateCodexConversation(context.Background(), "raw-conversation-id", testCodexConversationState(1), time.Hour)
	require.ErrorContains(t, err, "digest")
}

func testCodexConversationDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func testCodexConversationState(accountID int64) service.CodexConversationState {
	return service.CodexConversationState{
		Revision:               1,
		AccountID:              accountID,
		ProxyIdentity:          "proxy:1",
		ProfileID:              service.CodexProfileCLI,
		IdentityPolicyVersion:  service.CodexIdentityPolicyV2,
		PoolSlot:               3,
		DeviceID:               "11111111-1111-4111-8111-111111111111",
		SessionID:              "22222222-2222-4222-8222-222222222222",
		ThreadID:               "33333333-3333-4333-8333-333333333333",
		WindowID:               "44444444-4444-4444-8444-444444444444",
		EgressRoute:            "proxy",
		TransportConfigVersion: "tls:1",
		Active:                 true,
		CreatedAtUnixMS:        1_800_000_000_000,
		LastActivityUnixMS:     1_800_000_000_000,
	}
}
