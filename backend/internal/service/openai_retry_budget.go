package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIRetryBudgetV2ExtraKey = "openai_retry_budget_v2"
	openAIRetryBudgetContextKey = "openai_retry_budget_v2_state"
)

var ErrOpenAIRetryBudgetExhausted = errors.New("openai retry budget exhausted")

type OpenAIRetryFailureClass string

type OpenAIRetryFailureScope string

const (
	OpenAIRetryFailureNone       OpenAIRetryFailureClass = "none"
	OpenAIRetryFailureRequest    OpenAIRetryFailureClass = "request"
	OpenAIRetryFailureCredential OpenAIRetryFailureClass = "credential"
	OpenAIRetryFailureRateLimit  OpenAIRetryFailureClass = "rate_limit"
	OpenAIRetryFailureTransient  OpenAIRetryFailureClass = "transient"
	OpenAIRetryFailureTransport  OpenAIRetryFailureClass = "transport"
	OpenAIRetryFailureState      OpenAIRetryFailureClass = "state"
	OpenAIRetryFailureCanceled   OpenAIRetryFailureClass = "canceled"
)

const (
	OpenAIRetryScopeRequest   OpenAIRetryFailureScope = "request"
	OpenAIRetryScopeAccount   OpenAIRetryFailureScope = "account"
	OpenAIRetryScopeTransport OpenAIRetryFailureScope = "transport"
	OpenAIRetryScopeState     OpenAIRetryFailureScope = "state"
)

type OpenAIRetryDecision struct {
	Class             OpenAIRetryFailureClass
	Scope             OpenAIRetryFailureScope
	RetrySameAccount  bool
	RetryOtherAccount bool
	RefreshCredential bool
}

// ClassifyOpenAIRetryFailure is the single Phase 2 retry-policy classifier.
// Callers still have to reserve the shared budget and pass the output gate.
func ClassifyOpenAIRetryFailure(ctx context.Context, status int, err error, stateful bool, beforeOutput bool) OpenAIRetryDecision {
	if ctx != nil && ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) && !beforeOutput {
		return OpenAIRetryDecision{Class: OpenAIRetryFailureCanceled, Scope: OpenAIRetryScopeRequest}
	}
	if !beforeOutput {
		return OpenAIRetryDecision{Class: OpenAIRetryFailureState, Scope: OpenAIRetryScopeState}
	}
	decision := OpenAIRetryDecision{Class: OpenAIRetryFailureRequest, Scope: OpenAIRetryScopeRequest}
	switch status {
	case http.StatusUnauthorized:
		decision.Class = OpenAIRetryFailureCredential
		decision.Scope = OpenAIRetryScopeAccount
		decision.RetrySameAccount = true
		decision.RefreshCredential = true
	case http.StatusRequestTimeout:
		decision.Class = OpenAIRetryFailureTransient
		decision.Scope = OpenAIRetryScopeTransport
		decision.RetrySameAccount = true
		decision.RetryOtherAccount = !stateful
	case http.StatusTooManyRequests:
		decision.Class = OpenAIRetryFailureRateLimit
		decision.Scope = OpenAIRetryScopeAccount
		decision.RetrySameAccount = true
		decision.RetryOtherAccount = !stateful
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		decision.Class = OpenAIRetryFailureTransient
		decision.Scope = OpenAIRetryScopeAccount
		decision.RetrySameAccount = true
		decision.RetryOtherAccount = !stateful
	case 0:
		if err != nil {
			decision.Class = OpenAIRetryFailureTransport
			decision.Scope = OpenAIRetryScopeTransport
			decision.RetrySameAccount = true
			decision.RetryOtherAccount = !stateful
		}
	}
	return decision
}

type OpenAIRetryBudgetSnapshot struct {
	MaxAttempts          int
	MaxDistinctAccounts  int
	Attempts             int
	DistinctAccounts     int
	StreamStarted        bool
	BytesEmitted         bool
	Stateful             bool
	ReplaySafe           bool
	RefreshUsed          bool
	PreviousRecoveryUsed bool
	LastFailureClass     OpenAIRetryFailureClass
	LastFailureScope     OpenAIRetryFailureScope
	StartedAt            time.Time
}

// OpenAIRetryBudget is one race-safe budget for a logical HTTP request or WS
// turn. It is intentionally request-local and never persisted.
type OpenAIRetryBudget struct {
	mu sync.Mutex

	maxAttempts           int
	maxDistinctAccounts   int
	attempts              int
	seenAccounts          map[int64]struct{}
	streamStarted         bool
	bytesEmitted          bool
	stateful              bool
	replaySafe            bool
	refreshUsed           bool
	previousRecoveryUsed  bool
	lastFailureClass      OpenAIRetryFailureClass
	lastFailureScope      OpenAIRetryFailureScope
	failureRecorded       bool
	lastRetrySameAccount  bool
	lastRetryOtherAccount bool
	startedAt             time.Time
	maxElapsed            time.Duration
}

func NewOpenAIRetryBudget(stateful bool) *OpenAIRetryBudget {
	maxDistinct := 2
	if stateful {
		maxDistinct = 1
	}
	return &OpenAIRetryBudget{
		maxAttempts:         2,
		maxDistinctAccounts: maxDistinct,
		seenAccounts:        make(map[int64]struct{}),
		stateful:            stateful,
		replaySafe:          true,
		startedAt:           time.Now(),
		maxElapsed:          20 * time.Second,
	}
}

func openAIRetryBudgetV2Enabled(account *Account) bool {
	if account == nil || account.Extra == nil {
		return false
	}
	enabled, _ := account.Extra[openAIRetryBudgetV2ExtraKey].(bool)
	return enabled
}

func openAIRetryRequestIsStateful(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return true
	}
	if strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "" {
		return true
	}
	lower := strings.ToLower(string(body))
	return strings.Contains(lower, `"type":"function_call_output"`) ||
		strings.Contains(lower, `"type": "function_call_output"`) ||
		strings.Contains(lower, "encrypted_content") ||
		strings.Contains(lower, "encrypted_reasoning")
}

func EnsureOpenAIRetryBudget(c *gin.Context, account *Account, body []byte) *OpenAIRetryBudget {
	if c == nil {
		return nil
	}
	if existing := OpenAIRetryBudgetFromContext(c); existing != nil {
		return existing
	}
	if account == nil || !account.IsOpenAIOAuth() || !openAIRetryBudgetV2Enabled(account) {
		return nil
	}
	budget := NewOpenAIRetryBudget(openAIRetryRequestIsStateful(body))
	c.Set(openAIRetryBudgetContextKey, budget)
	return budget
}

// StartOpenAIRetryBudgetTurn replaces the previous WS turn's budget. Retries
// of the same turn must keep using the returned object and must not call this.
func StartOpenAIRetryBudgetTurn(c *gin.Context, account *Account, body []byte) *OpenAIRetryBudget {
	if c == nil || account == nil || !account.IsOpenAIOAuth() || !openAIRetryBudgetV2Enabled(account) {
		return nil
	}
	budget := NewOpenAIRetryBudget(openAIRetryRequestIsStateful(body))
	c.Set(openAIRetryBudgetContextKey, budget)
	return budget
}

func OpenAIRetryBudgetFromContext(c *gin.Context) *OpenAIRetryBudget {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIRetryBudgetContextKey)
	if !ok {
		return nil
	}
	budget, _ := value.(*OpenAIRetryBudget)
	return budget
}

func ReserveOpenAIUpstreamAttempt(c *gin.Context, accountID int64) error {
	budget := OpenAIRetryBudgetFromContext(c)
	if budget == nil {
		return nil
	}
	return budget.Reserve(accountID)
}

func (b *OpenAIRetryBudget) Reserve(accountID int64) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.replaySafe || b.streamStarted || b.bytesEmitted {
		return fmt.Errorf("%w: replay is closed after downstream output", ErrOpenAIRetryBudgetExhausted)
	}
	if b.maxElapsed > 0 && time.Since(b.startedAt) > b.maxElapsed {
		return fmt.Errorf("%w: elapsed limit exceeded", ErrOpenAIRetryBudgetExhausted)
	}
	if b.attempts >= b.maxAttempts {
		return fmt.Errorf("%w: attempts=%d max=%d", ErrOpenAIRetryBudgetExhausted, b.attempts, b.maxAttempts)
	}
	if accountID > 0 {
		_, seen := b.seenAccounts[accountID]
		if b.failureRecorded {
			if seen && !b.lastRetrySameAccount {
				return fmt.Errorf("%w: failure policy forbids same-account retry", ErrOpenAIRetryBudgetExhausted)
			}
			if !seen && !b.lastRetryOtherAccount {
				return fmt.Errorf("%w: failure policy forbids cross-account retry", ErrOpenAIRetryBudgetExhausted)
			}
		}
		if !seen && len(b.seenAccounts) >= b.maxDistinctAccounts {
			return fmt.Errorf("%w: distinct_accounts=%d max=%d", ErrOpenAIRetryBudgetExhausted, len(b.seenAccounts), b.maxDistinctAccounts)
		}
		b.seenAccounts[accountID] = struct{}{}
	}
	b.attempts++
	return nil
}

func (b *OpenAIRetryBudget) MarkSemanticOutput() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.streamStarted = true
	b.replaySafe = false
	b.mu.Unlock()
}

func (b *OpenAIRetryBudget) MarkBytesEmitted() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.streamStarted = true
	b.bytesEmitted = true
	b.replaySafe = false
	b.mu.Unlock()
}

func (b *OpenAIRetryBudget) RecordFailure(decision OpenAIRetryDecision) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.lastFailureClass = decision.Class
	b.lastFailureScope = decision.Scope
	b.failureRecorded = true
	b.lastRetrySameAccount = decision.RetrySameAccount
	b.lastRetryOtherAccount = decision.RetryOtherAccount
	b.mu.Unlock()
}

func RecordOpenAIRetryFailure(c *gin.Context, status int, err error) OpenAIRetryDecision {
	budget := OpenAIRetryBudgetFromContext(c)
	if budget == nil {
		return OpenAIRetryDecision{}
	}
	snapshot := budget.Snapshot()
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = c.Request.Context()
	}
	decision := ClassifyOpenAIRetryFailure(ctx, status, err, snapshot.Stateful, snapshot.ReplaySafe && !snapshot.BytesEmitted)
	budget.RecordFailure(decision)
	return decision
}

func (b *OpenAIRetryBudget) UseRefresh() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.refreshUsed || !b.replaySafe {
		return false
	}
	b.refreshUsed = true
	return true
}

func (b *OpenAIRetryBudget) UsePreviousResponseRecovery() bool {
	if b == nil {
		return true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.previousRecoveryUsed || !b.replaySafe || b.bytesEmitted {
		return false
	}
	b.previousRecoveryUsed = true
	return true
}

func (b *OpenAIRetryBudget) Snapshot() OpenAIRetryBudgetSnapshot {
	if b == nil {
		return OpenAIRetryBudgetSnapshot{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return OpenAIRetryBudgetSnapshot{
		MaxAttempts: b.maxAttempts, MaxDistinctAccounts: b.maxDistinctAccounts,
		Attempts: b.attempts, DistinctAccounts: len(b.seenAccounts),
		StreamStarted: b.streamStarted, BytesEmitted: b.bytesEmitted,
		Stateful: b.stateful, ReplaySafe: b.replaySafe,
		RefreshUsed: b.refreshUsed, PreviousRecoveryUsed: b.previousRecoveryUsed,
		LastFailureClass: b.lastFailureClass, LastFailureScope: b.lastFailureScope,
		StartedAt: b.startedAt,
	}
}
