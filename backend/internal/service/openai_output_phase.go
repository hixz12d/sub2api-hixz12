package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

var ErrOpenAIRecoveryDeadline = errors.New("openai pre-output recovery deadline exceeded")

type openAIPhaseWatchdogContextKey struct{}
type openAIRecoveryPreparationKey struct{}

// Keep request-builder metadata while restoring the caller's cancellation.
// WithoutCancel hides the inherited cancel key, so context.Cause still belongs
// to the active phase instead of a detached or obsolete request context.
type openAIPhaseParentContext struct {
	context.Context
	requestValues context.Context
}

func (c openAIPhaseParentContext) Value(key any) any {
	if value := c.requestValues.Value(key); value != nil {
		return value
	}
	return c.Context.Value(key)
}

func openAIPhaseParent(parent, request context.Context) context.Context {
	return openAIPhaseParentContext{Context: parent, requestValues: context.WithoutCancel(request)}
}

const openAILogicalStartKey = "openai_logical_request_start"

// RecordOpenAILogicalStart preserves ingress time before preparation and queues.
func RecordOpenAILogicalStart(c *gin.Context, started time.Time) {
	if _, exists := c.Get(openAILogicalStartKey); !exists {
		c.Set(openAILogicalStartKey, started)
	}
}

func openAIPhaseWatchdogFromContext(ctx context.Context) *openAIFirstOutputHeaderGuard {
	if ctx == nil {
		return nil
	}
	guard, _ := ctx.Value(openAIPhaseWatchdogContextKey{}).(*openAIFirstOutputHeaderGuard)
	return guard
}

func (g *openAIFirstOutputHeaderGuard) failure() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.expired {
		return g.timeoutCause
	}
	return context.Cause(g.ctx)
}

func (g *openAIFirstOutputHeaderGuard) timeoutFailure() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.expired {
		return g.timeoutCause
	}
	return nil
}

// tryCommit and expiry have one winner. Once committed, only the original
// request lifetime can cancel the stream; the phase deadline is disarmed.
func (g *openAIFirstOutputHeaderGuard) tryCommit() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.expired {
		return g.timeoutCause
	}
	if err := context.Cause(g.ctx); err != nil {
		return err
	}
	if !g.stopped && !time.Now().Before(g.deadline) {
		g.expired, g.stopped = true, true
		g.timer.Stop()
		g.done <- time.Now()
		g.cancel(g.timeoutCause)
		return g.timeoutCause
	}
	g.stopped = true
	g.timer.Stop()
	return nil
}

// OpenAIRecoveryPreparationContext bounds blocking work after OAuth selection.
// Before effective effort is known, use the largest configured total; dispatch
// freezes the effective tier and may only tighten that ingress-relative ceiling.
func OpenAIRecoveryPreparationContext(c *gin.Context, parent context.Context, account *Account) (context.Context, context.CancelFunc) {
	budget := openAIRetryBudgetFromContextRaw(c)
	if budget == nil || !budget.boundedHTTP || !budget.streamingHTTP {
		return parent, func() {}
	}
	if account != nil {
		if account.Platform != PlatformOpenAI || account.IsOpenAIPassthroughEnabled() {
			return parent, func() {}
		}
		if account.IsOpenAIOAuth() && openAIRetryBudgetV2Enabled(account) {
			// Selection activates the existing cap, not a new upstream attempt.
			c.Set(openAIRetryBudgetActiveKey, true)
		} else if OpenAIRetryBudgetFromContext(c) == nil {
			return parent, func() {}
		}
	} else if OpenAIRetryBudgetFromContext(c) == nil {
		// Retry waits may reuse an activated OAuth budget, but cannot activate one.
		return parent, func() {}
	}
	budget.mu.Lock()
	elapsed := budget.maxElapsed
	if !budget.deadlineFrozen && budget.highElapsed > elapsed {
		elapsed = budget.highElapsed
	}
	deadline := budget.startedAt.Add(elapsed)
	budget.mu.Unlock()
	ctx, cancel := context.WithDeadlineCause(parent, deadline, ErrOpenAIRecoveryDeadline)
	return context.WithValue(ctx, openAIRecoveryPreparationKey{}, true), cancel
}

func (s *OpenAIGatewayService) beginOpenAIHTTPOutputPhase(ctx context.Context, c *gin.Context, account *Account, effort string) (context.Context, *openAIFirstOutputHeaderGuard, error) {
	if account == nil || account.Platform != PlatformOpenAI {
		return ctx, nil, nil
	}
	now := time.Now()
	deadline := time.Time{}
	cause := error(ErrOpenAIFirstOutputTimeout)
	if first := s.openAIFirstOutputTimeout(effort); first > 0 {
		deadline = now.Add(first)
	}
	if budget := OpenAIRetryBudgetFromContext(c); budget != nil {
		budget.mu.Lock()
		if budget.boundedHTTP {
			if !budget.deadlineFrozen {
				budget.deadlineFrozen = true
				if isOpenAIHighReasoningEffort(effort) {
					if budget.highElapsed > 0 {
						budget.maxElapsed = budget.highElapsed
					}
					if budget.highFirstOutputLimit > 0 {
						budget.firstOutputLimit = budget.highFirstOutputLimit
					}
				}
			}
			deadline = time.Time{}
			if budget.firstOutputLimit > 0 {
				deadline = now.Add(budget.firstOutputLimit)
			}
			total := budget.startedAt.Add(budget.maxElapsed)
			if deadline.IsZero() || !deadline.Before(total) {
				deadline, cause = total, ErrOpenAIRecoveryDeadline
			}
		}
		budget.mu.Unlock()
	}
	if err := ctx.Err(); err != nil {
		return ctx, nil, err
	}
	if deadline.IsZero() {
		return ctx, nil, nil
	}
	if !now.Before(deadline) {
		return ctx, nil, cause
	}
	phaseCtx, guard := newOpenAIPhaseWatchdog(ctx, func() {}, deadline, cause)
	return context.WithValue(phaseCtx, openAIPhaseWatchdogContextKey{}, guard), guard, nil
}

func isOpenAIHighReasoningEffort(effort string) bool {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "high", "xhigh", "max":
		return true
	}
	return false
}

func openAIOutputPhaseFailure(c *gin.Context, err error, headers http.Header) error {
	if !errors.Is(err, ErrOpenAIFirstOutputTimeout) && !errors.Is(err, ErrOpenAIRecoveryDeadline) {
		return err
	}
	cause := OpenAIFailureCauseFirstOutputTimeout
	body := []byte(`{"error":{"type":"first_output_timeout","message":"Upstream produced no output before the deadline"}}`)
	if errors.Is(err, ErrOpenAIRecoveryDeadline) {
		cause = "recovery_deadline"
		body = []byte(`{"error":{"type":"recovery_deadline","message":"Pre-output recovery deadline exceeded"}}`)
	}
	failure := &UpstreamFailoverError{
		StatusCode:               http.StatusGatewayTimeout,
		Err:                      err,
		ResponseHeaders:          headers.Clone(),
		ResponseBody:             body,
		SafeToFailoverAfterWrite: true,
	}
	return annotateOpenAIPreOutputFailover(c, failure, cause, OpenAIRetryDecisionFailClosed)
}
