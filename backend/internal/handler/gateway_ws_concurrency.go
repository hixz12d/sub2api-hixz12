package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const openAIWSTurnConcurrencyWait = 3 * time.Second

// acquireWSTurnSlots waits for a pair of slots without retaining either slot
// between attempts. Queue counters are shared across instances and always
// released before returning. Key concurrency is tracked only after admission.
func (h *ConcurrencyHelper) acquireWSTurnSlots(parent context.Context, userID int64, userLimit int, keyID, accountID int64, accountLimit int, timeout time.Duration) (func(), func(), error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	blockedSlot := "user"
	tryPair := func() (func(), func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		userRelease, acquired, err := h.TryAcquireUserSlot(ctx, userID, userLimit)
		if err != nil || !acquired {
			blockedSlot = "user"
			return nil, nil, err
		}
		accountRelease, acquired, err := h.TryAcquireAccountSlot(ctx, accountID, accountLimit)
		if err != nil || !acquired {
			userRelease()
			blockedSlot = "account"
			return nil, nil, err
		}
		if err := ctx.Err(); err != nil {
			userRelease()
			accountRelease()
			return nil, nil, err
		}
		return h.withAPIKeySlot(ctx, keyID, userRelease), accountRelease, nil
	}
	userRelease, accountRelease, err := tryPair()
	if err != nil || userRelease != nil {
		return userRelease, accountRelease, err
	}
	userQueueLimit := max(1, service.CalculateMaxWait(userLimit)-userLimit)
	allowed, err := h.IncrementWaitCount(ctx, userID, userQueueLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("join user websocket wait queue: %w", err)
	}
	if !allowed {
		return nil, nil, &WaitQueueFullError{SlotType: "user"}
	}
	defer h.DecrementWaitCount(parent, userID)
	accountQueueLimit := max(1, service.CalculateMaxWait(accountLimit)-accountLimit)
	allowed, err = h.IncrementAccountWaitCount(ctx, accountID, accountQueueLimit)
	if err != nil {
		return nil, nil, fmt.Errorf("join account websocket wait queue: %w", err)
	}
	if !allowed {
		return nil, nil, &WaitQueueFullError{SlotType: "account"}
	}
	defer h.DecrementAccountWaitCount(parent, accountID)

	backoff := initialBackoff
	timer := time.NewTimer(backoff)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			if parent.Err() != nil {
				return nil, nil, context.Cause(parent)
			}
			return nil, nil, &ConcurrencyError{SlotType: blockedSlot, IsTimeout: true}
		case <-timer.C:
			userRelease, accountRelease, err = tryPair()
			if err != nil || userRelease != nil {
				return userRelease, accountRelease, err
			}
			backoff = nextBackoff(backoff)
			timer.Reset(backoff)
		}
	}
}
