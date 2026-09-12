package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type wsWaitCache struct {
	helperConcurrencyCacheStub
	accountWaitAllowed            bool
	accountWaitIn, accountWaitOut int
	userHeldOnQueue               bool
}

func (s *wsWaitCache) IncrementAccountWaitCount(context.Context, int64, int) (bool, error) {
	s.accountWaitIn++
	s.userHeldOnQueue = s.userAcquireCalls > s.userReleaseCalls
	return s.accountWaitAllowed, nil
}
func (s *wsWaitCache) DecrementAccountWaitCount(context.Context, int64) error {
	s.accountWaitOut++
	return nil
}

func TestWSTurnSlots_ReleaseUserWhileAccountBusy(t *testing.T) {
	cache := &wsWaitCache{helperConcurrencyCacheStub: helperConcurrencyCacheStub{
		userSeq: []bool{true, true}, accountSeq: []bool{false, true}, waitAllowed: true,
	}, accountWaitAllowed: true}
	h := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
	userRelease, accountRelease, err := h.acquireWSTurnSlots(context.Background(), 1, 1, 2, 3, 1, time.Second)
	require.NoError(t, err)
	require.False(t, cache.userHeldOnQueue)
	require.Equal(t, 1, cache.userReleaseCalls)
	require.Equal(t, 1, cache.apiKeyTrackCalls)
	require.Equal(t, 1, cache.waitDecrementCalls)
	require.Equal(t, 1, cache.accountWaitOut)
	userRelease()
	accountRelease()
	require.Equal(t, 2, cache.userReleaseCalls)
	require.Equal(t, 1, cache.accountReleaseCalls)
	require.Equal(t, 1, cache.apiKeyReleaseCalls)
}

func TestWSTurnSlots_TimeoutCancellationAndQueueLimit(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel", "user_queue_full", "account_queue_full"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			cause := errors.New("client disconnected")
			cache := &wsWaitCache{helperConcurrencyCacheStub: helperConcurrencyCacheStub{waitAllowed: mode != "user_queue_full"}, accountWaitAllowed: mode != "account_queue_full"}
			if mode == "cancel" {
				cache.waitIncrementHook = func() { cancel(cause) }
			}
			h := NewConcurrencyHelper(service.NewConcurrencyService(cache), SSEPingFormatNone, time.Second)
			u, a, err := h.acquireWSTurnSlots(ctx, 1, 1, 2, 3, 1, 30*time.Millisecond)
			require.Nil(t, u)
			require.Nil(t, a)
			require.Error(t, err)
			if mode == "timeout" {
				var e *ConcurrencyError
				require.ErrorAs(t, err, &e)
				require.True(t, e.IsTimeout)
			}
			if mode == "cancel" {
				require.ErrorIs(t, err, cause)
			}
			if mode == "user_queue_full" || mode == "account_queue_full" {
				var e *WaitQueueFullError
				require.ErrorAs(t, err, &e)
			}
			if mode != "user_queue_full" {
				require.Equal(t, 1, cache.waitDecrementCalls)
			}
			if mode == "timeout" || mode == "cancel" {
				require.Equal(t, 1, cache.accountWaitOut)
			}
			require.Zero(t, cache.apiKeyTrackCalls)
		})
	}
}
