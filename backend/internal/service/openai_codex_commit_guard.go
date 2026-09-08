package service

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// OutputCommitSnapshot is the unified pre-output commit ledger (C4).
// CodexCommitSnapshot remains an alias for call-site compatibility.
type OutputCommitSnapshot struct {
	TransportCommitted     bool
	HeartbeatOnly          bool
	SemanticOutputStarted  bool
	ResponseOwnershipBound bool
	Stateful               bool
	ReplaySafe             bool
	CurrentAccountID       int64
	ActualProtoMajor       int
	FirstFrameMs           int
	FirstSemanticMs        int
	FirstVisibleMs         int
	FailurePhase           string
	FailureCause           string
	RetryDecisionReason    string
}

// CodexCommitSnapshot is retained as the historical name for OutputCommitSnapshot.
type CodexCommitSnapshot = OutputCommitSnapshot

type CodexCommitGuard struct {
	c *gin.Context
}

func NewCodexCommitGuard(c *gin.Context) CodexCommitGuard {
	return CodexCommitGuard{c: c}
}

func (g CodexCommitGuard) Snapshot() OutputCommitSnapshot {
	wire := OpenAIAttemptWireStateSnapshot(g.c)
	attempt := OpenAIAttemptStateSnapshot(g.c)
	snapshot := OutputCommitSnapshot{
		TransportCommitted:     wire.TransportCommitted,
		HeartbeatOnly:          wire.HeartbeatOnly,
		SemanticOutputStarted:  wire.SemanticOutputStarted,
		ResponseOwnershipBound: len(attempt.ResponseIDs) > 0 || len(attempt.ResponseConnIDs) > 0,
		Stateful:               attempt.Stateful,
		ReplaySafe:             attempt.ReplaySafe,
		CurrentAccountID:       attempt.CurrentAccountID,
		ActualProtoMajor:       wire.ActualProtoMajor,
		FirstFrameMs:           wire.FirstFrameMs,
		FirstSemanticMs:        wire.FirstSemanticMs,
		FirstVisibleMs:         wire.FirstVisibleMs,
		FailurePhase:           wire.FailurePhase,
		FailureCause:           wire.FailureCause,
		RetryDecisionReason:    wire.RetryDecisionReason,
	}
	if budget := OpenAIRetryBudgetFromContext(g.c); budget != nil {
		budgetSnapshot := budget.Snapshot()
		snapshot.Stateful = snapshot.Stateful || budgetSnapshot.Stateful
		snapshot.ReplaySafe = snapshot.ReplaySafe && budgetSnapshot.ReplaySafe && !budgetSnapshot.BytesEmitted
	}
	if snapshot.SemanticOutputStarted || snapshot.ResponseOwnershipBound {
		snapshot.ReplaySafe = false
	}
	return snapshot
}

func (g CodexCommitGuard) CanStartAttempt(accountID int64) error {
	snapshot := g.Snapshot()
	if snapshot.SemanticOutputStarted {
		return fmt.Errorf("%w: semantic output already started", ErrOpenAIRetryBudgetExhausted)
	}
	if snapshot.ResponseOwnershipBound {
		return fmt.Errorf("%w: response ownership already bound", ErrOpenAIRetryBudgetExhausted)
	}
	if snapshot.Stateful && snapshot.CurrentAccountID > 0 && accountID > 0 && snapshot.CurrentAccountID != accountID {
		return fmt.Errorf("%w: stateful request cannot switch accounts", ErrOpenAIRetryBudgetExhausted)
	}
	if !snapshot.ReplaySafe && !g.isInitialStatefulDispatch(accountID, snapshot) {
		return fmt.Errorf("%w: request is not replay safe", ErrOpenAIRetryBudgetExhausted)
	}
	return nil
}

// A first dispatch is not a replay. Keep ReplaySafe false and let the budget
// atomically enforce one admission when the request cannot be replayed.
func (g CodexCommitGuard) isInitialStatefulDispatch(accountID int64, snapshot OutputCommitSnapshot) bool {
	if !snapshot.Stateful || accountID <= 0 || snapshot.CurrentAccountID != accountID ||
		(snapshot.TransportCommitted && !snapshot.HeartbeatOnly) {
		return false
	}
	attempt := OpenAIAttemptStateSnapshot(g.c)
	if attempt.PreviousAccountID > 0 && attempt.PreviousAccountID != accountID {
		return false
	}
	budget := OpenAIRetryBudgetFromContext(g.c)
	if budget == nil {
		return false
	}
	state := budget.Snapshot()
	return state.Attempts == 0 && state.ReplaySafe && !state.StreamStarted && !state.BytesEmitted
}

func (g CodexCommitGuard) MarkHeartbeat() {
	MarkOpenAIAttemptHeartbeat(g.c)
}

func (g CodexCommitGuard) MarkSemanticOutput() {
	MarkOpenAISemanticOutputStarted(g.c)
}

func (g CodexCommitGuard) MarkTerminal(event string) {
	MarkOpenAIAttemptTerminal(g.c, event)
}

// OutputCommitSnapshotFromContext returns the unified commit ledger for the
// current request attempt.
func OutputCommitSnapshotFromContext(c *gin.Context) OutputCommitSnapshot {
	return NewCodexCommitGuard(c).Snapshot()
}
