package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIRetryBudgetV2ExtraKey = "openai_retry_budget_v2"
	openAIRetryBudgetContextKey = "openai_retry_budget_v2_state"
	openAIRetryBudgetActiveKey  = "openai_retry_budget_v2_active"
	openAIRetryBudgetConfigKey  = "openai_retry_budget_v2_config"
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
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 520:
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
	MaxElapsed           time.Duration
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
	boundedHTTP           bool
	streamingHTTP         bool
	deadlineFrozen        bool
	highElapsed           time.Duration
	firstOutputLimit      time.Duration
	highFirstOutputLimit  time.Duration
}

func openAIRetryBudgetMaxElapsed(cfg *config.Config) time.Duration {
	legacyElapsed := 20 * time.Second
	boundedDefaultElapsed := 110 * time.Second
	if cfg == nil {
		return legacyElapsed
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Gateway.OpenAIPreoutputRecoveryMode))
	if override := cfg.Gateway.OpenAIPreoutputRecoveryMaxElapsedSeconds; override > 0 {
		return time.Duration(override) * time.Second
	}
	if mode == "bounded_preoutput" {
		return boundedDefaultElapsed
	}
	return legacyElapsed
}

func NewOpenAIRetryBudget(stateful bool) *OpenAIRetryBudget {
	return newOpenAIRetryBudget(stateful, false, openAIRetryBudgetMaxElapsed(nil))
}

func newOpenAIRetryBudget(stateful, fullContextRecoverable bool, maxElapsed time.Duration) *OpenAIRetryBudget {
	maxDistinct := 2
	// previous_response_id alone is sticky; a rebuildable local transcript may still
	// move once after OAuth death or intentional account/group switch.
	if stateful && !fullContextRecoverable {
		maxDistinct = 1
	}
	if maxElapsed <= 0 {
		maxElapsed = openAIRetryBudgetMaxElapsed(nil)
	}
	return &OpenAIRetryBudget{
		maxAttempts:         2,
		maxDistinctAccounts: maxDistinct,
		seenAccounts:        make(map[int64]struct{}),
		stateful:            stateful,
		replaySafe:          true,
		startedAt:           time.Now(),
		maxElapsed:          maxElapsed,
	}
}

func openAIRetryBudgetV2Enabled(account *Account) bool {
	if account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	if account.Extra == nil {
		return true
	}
	raw, exists := account.Extra[openAIRetryBudgetV2ExtraKey]
	if !exists {
		return true
	}
	enabled, ok := raw.(bool)
	return ok && enabled
}

func openAIRetryRequestIsStateful(body []byte) bool {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return true
	}
	if !gjson.ParseBytes(body).IsObject() || strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String()) != "" {
		return true
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "encrypted_content") || strings.Contains(lower, "encrypted_reasoning") {
		return true
	}
	var containsState func(gjson.Result) bool
	containsState = func(value gjson.Result) bool {
		found := false
		value.ForEach(func(key, child gjson.Result) bool {
			if key.String() == "encrypted_content" || key.String() == "encrypted_reasoning" ||
				(key.String() == "type" && (strings.HasSuffix(child.String(), "_call_output") ||
					child.String() == "tool_search_output" || child.String() == "item_reference" || child.String() == "mcp_approval_response")) {
				found = true
			} else if child.IsObject() || child.IsArray() {
				found = containsState(child)
			}
			return !found
		})
		return found
	}
	return containsState(gjson.ParseBytes(body))
}

func OpenAIRetryRequestIsStateful(c *gin.Context, body []byte) bool {
	if c != nil && c.Request != nil && strings.TrimSpace(c.GetHeader(openAIWSTurnStateHeader)) != "" {
		return true
	}
	if value, ok := openAIAffinityFromGin(c); ok {
		return value.Identity.Stateful || value.Identity.Strength == AffinityStrong
	}
	return openAIRetryRequestIsStateful(body)
}

func PrepareOpenAIRetryBudget(c *gin.Context, body []byte) *OpenAIRetryBudget {
	return PrepareOpenAIRetryBudgetWithConfig(c, body, nil)
}

func PrepareOpenAIRetryBudgetWithConfig(c *gin.Context, body []byte, cfg *config.Config) *OpenAIRetryBudget {
	if c == nil {
		return nil
	}
	PrepareOpenAIAttemptState(c, body, "", "", "")
	if existing := openAIRetryBudgetFromContextRaw(c); existing != nil {
		return existing
	}
	stateful := OpenAIRetryRequestIsStateful(c, body)
	fullContext := codexBodyHasLocalRebuildableContext(body) && (c.Request == nil || strings.TrimSpace(c.GetHeader(openAIWSTurnStateHeader)) == "")
	// Strong affinity / sticky stateful sessions must stay single-account even when
	// the body carries a rebuildable local transcript.
	if value, ok := openAIAffinityFromGin(c); ok {
		if value.Identity.Strength == AffinityStrong || (value.Identity.Stateful && !value.Identity.ReplaySafe) {
			fullContext = false
		}
	}
	if cfg == nil {
		if raw, ok := c.Get(openAIRetryBudgetConfigKey); ok {
			cfg, _ = raw.(*config.Config)
		}
	}
	if cfg != nil {
		c.Set(openAIRetryBudgetConfigKey, cfg)
	}
	budget := newOpenAIRetryBudget(stateful, fullContext, openAIRetryBudgetMaxElapsed(cfg))
	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Gateway.OpenAIPreoutputRecoveryMode), "bounded_preoutput") && c.Request != nil {
		path := strings.TrimSuffix(c.Request.URL.Path, "/")
		budget.boundedHTTP = strings.HasSuffix(path, "/responses") || strings.HasSuffix(path, "/messages")
		budget.streamingHTTP = c.Request.Method == http.MethodPost && gjson.GetBytes(body, "stream").Bool()
		budget.highElapsed = time.Duration(cfg.Gateway.OpenAIPreoutputRecoveryHighEffortMaxElapsedSeconds) * time.Second
		budget.firstOutputLimit = time.Duration(cfg.Gateway.OpenAIFirstOutputTimeoutSeconds) * time.Second
		budget.highFirstOutputLimit = time.Duration(cfg.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds) * time.Second
	}
	// Only bounded recovery uses an ingress-relative deadline. Legacy
	// retries start at preparation: counting a slow body upload against their
	// 20-second window can reject the first upstream attempt before dispatch.
	if cfg != nil && strings.EqualFold(strings.TrimSpace(cfg.Gateway.OpenAIPreoutputRecoveryMode), "bounded_preoutput") {
		if raw, ok := c.Get(openAILogicalStartKey); ok {
			if started, ok := raw.(time.Time); ok && !started.IsZero() {
				budget.startedAt = started
			}
		}
	}
	c.Set(openAIRetryBudgetContextKey, budget)
	c.Set(openAIRetryBudgetActiveKey, false)
	return budget
}

func EnsureOpenAIRetryBudget(c *gin.Context, account *Account, body []byte) *OpenAIRetryBudget {
	if c == nil {
		return nil
	}
	PrepareOpenAIRetryBudget(c, body)
	if account != nil {
		BeginOpenAIAttempt(c, account.ID, body)
	}
	if account == nil || !account.IsOpenAIOAuth() || !openAIRetryBudgetV2Enabled(account) {
		// Once activated, a mixed-pool candidate cannot remove this request's cap.
		if active := OpenAIRetryBudgetFromContext(c); active != nil {
			return active
		}
		return nil
	}
	budget := openAIRetryBudgetFromContextRaw(c)
	c.Set(openAIRetryBudgetActiveKey, budget != nil)
	return budget
}

// StartOpenAIRetryBudgetTurn replaces the previous WS turn's budget. Retries
// of the same turn must keep using the returned object and must not call this.
func StartOpenAIRetryBudgetTurn(c *gin.Context, account *Account, body []byte) *OpenAIRetryBudget {
	if c == nil {
		return nil
	}
	if account == nil || !account.IsOpenAIOAuth() || !openAIRetryBudgetV2Enabled(account) {
		c.Set(openAIRetryBudgetActiveKey, false)
		return nil
	}
	// A WS response.create is a new logical turn, not permission to replay
	// the previous turn. Replace its ledger together with its fresh budget.
	sessionHash, promptCacheKey, routeKey := "", "", ""
	if previous := OpenAIAttemptStateFromContext(c); previous != nil {
		sessionHash, promptCacheKey, routeKey = previous.SessionHash, previous.PromptCacheKey, previous.RouteKey
	}
	if key := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String()); key != "" {
		promptCacheKey = key
	}
	state := newOpenAIAttemptState(c, body, sessionHash,
		gjson.GetBytes(body, "previous_response_id").String(), promptCacheKey)
	state.CurrentAccountID, state.Attempt, state.attemptActive = account.ID, 1, true
	state.RouteKey = routeKey
	c.Set(openAIAttemptStateKey, state)
	attachOpenAIAttemptStateToRequest(c, state)
	ResetOpenAIAttemptWireState(c)
	var cfg *config.Config
	if raw, ok := c.Get(openAIRetryBudgetConfigKey); ok {
		cfg, _ = raw.(*config.Config)
	}
	budget := newOpenAIRetryBudget(state.Stateful, false, openAIRetryBudgetMaxElapsed(cfg))
	c.Set(openAIRetryBudgetContextKey, budget)
	c.Set(openAIRetryBudgetActiveKey, true)
	return budget
}

func openAIRetryBudgetFromContextRaw(c *gin.Context) *OpenAIRetryBudget {
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

func OpenAIRetryBudgetFromContext(c *gin.Context) *OpenAIRetryBudget {
	if c == nil {
		return nil
	}
	active, _ := c.Get(openAIRetryBudgetActiveKey)
	if enabled, _ := active.(bool); !enabled {
		return nil
	}
	return openAIRetryBudgetFromContextRaw(c)
}

func ReserveOpenAIUpstreamAttempt(c *gin.Context, accountID int64) error {
	if c == nil {
		return nil
	}
	active, _ := c.Get(openAIRetryBudgetActiveKey)
	if enabled, _ := active.(bool); !enabled {
		return nil
	}
	budget := OpenAIRetryBudgetFromContext(c)
	if budget == nil {
		return nil
	}
	if err := NewCodexCommitGuard(c).CanStartAttempt(accountID); err != nil {
		return err
	}
	return budget.reserve(accountID, NewCodexCommitGuard(c).Snapshot().ReplaySafe)
}

func (b *OpenAIRetryBudget) Reserve(accountID int64) error {
	return b.reserve(accountID, true)
}

func (b *OpenAIRetryBudget) reserve(accountID int64, replaySafe bool) error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if !replaySafe && b.attempts > 0 {
		return fmt.Errorf("%w: request replay is not permitted", ErrOpenAIRetryBudgetExhausted)
	}
	if !b.replaySafe || b.streamStarted || b.bytesEmitted {
		return fmt.Errorf("%w: replay is closed after downstream output", ErrOpenAIRetryBudgetExhausted)
	}
	if b.maxElapsed > 0 && time.Since(b.startedAt) >= b.maxElapsed {
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
		StartedAt: b.startedAt, MaxElapsed: b.maxElapsed,
	}
}
