package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// OpenAIAttemptState is the request-local state machine for one logical OpenAI
// request. It owns retry identity, account ownership and the state that must
// be discarded before a cross-account replay. The canonical request body is
// represented by a digest so this state never becomes a second mutable body.
type OpenAIAttemptState struct {
	CanonicalBodySHA256 string
	Stateful            bool
	ReplaySafe          bool
	Attempt             int
	AccountSwitches     int
	CurrentAccountID    int64
	PreviousAccountID   int64
	SessionHash         string
	PreviousResponseID  string
	PromptCacheKey      string
	RouteKey            string
	TurnState           string
	ResponseIDs         []string
	ResponseConnIDs     []string
	LastResetReason     string

	attemptActive bool
}

// OpenAIAttemptWireState is request-local evidence about what the current
// upstream attempt exposed to the downstream client. It is deliberately
// separate from ResponseCommitted: HTTP headers may be committed by a
// heartbeat while semantic output is still replayable.
type OpenAIAttemptWireState struct {
	TransportCommitted    bool
	HeartbeatOnly         bool
	SemanticOutputStarted bool
	TerminalEvent         string
}

const (
	openAIAttemptStateKey     = "openai_attempt_state"
	openAIAttemptWireStateKey = "openai_attempt_wire_state"
)

type openAIAttemptStateContextKey struct{}

var openAIAttemptStateCtxKey = openAIAttemptStateContextKey{}

const OpenAIAttemptFailureReasonCapacity GatewayFailureReason = "openai_capacity"

func attachOpenAIAttemptStateToRequest(c *gin.Context, state *OpenAIAttemptState) {
	if c == nil || c.Request == nil || state == nil {
		return
	}
	c.Request = c.Request.WithContext(context.WithValue(c.Request.Context(), openAIAttemptStateCtxKey, state))
}

func openAIAttemptStateFromContext(ctx context.Context) *OpenAIAttemptState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(openAIAttemptStateCtxKey).(*OpenAIAttemptState)
	return state
}

func TrackOpenAIResponseIDFromContext(ctx context.Context, responseID string) {
	trackOpenAIResponseID(openAIAttemptStateFromContext(ctx), responseID)
}

func TrackOpenAIResponseConnIDFromContext(ctx context.Context, connID string) {
	trackOpenAIResponseConnID(openAIAttemptStateFromContext(ctx), connID)
}

func SetOpenAIAttemptRouteKeyFromContext(ctx context.Context, routeKey string) {
	if state := openAIAttemptStateFromContext(ctx); state != nil {
		state.RouteKey = strings.TrimSpace(routeKey)
	}
}

// PrepareOpenAIAttemptState initializes the logical request once. Later calls
// preserve the original body digest and stateful/replay classification.
func PrepareOpenAIAttemptState(c *gin.Context, body []byte, sessionHash, previousResponseID, promptCacheKey string) *OpenAIAttemptState {
	if c == nil {
		return nil
	}
	if existing := OpenAIAttemptStateFromContext(c); existing != nil {
		SetOpenAIAttemptRouting(c, sessionHash, previousResponseID, promptCacheKey)
		attachOpenAIAttemptStateToRequest(c, existing)
		return existing
	}

	digest := sha256.Sum256(body)
	stateful := OpenAIRetryRequestIsStateful(c, body)
	state := &OpenAIAttemptState{
		CanonicalBodySHA256: hex.EncodeToString(digest[:]),
		Stateful:            stateful,
		ReplaySafe:          !stateful,
		SessionHash:         strings.TrimSpace(sessionHash),
		PreviousResponseID:  strings.TrimSpace(previousResponseID),
		PromptCacheKey:      strings.TrimSpace(promptCacheKey),
	}
	c.Set(openAIAttemptStateKey, state)
	attachOpenAIAttemptStateToRequest(c, state)
	ResetOpenAIAttemptWireState(c)
	return state
}

// OpenAIAttemptStateFromContext returns the mutable request-local state.
func OpenAIAttemptStateFromContext(c *gin.Context) *OpenAIAttemptState {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIAttemptStateKey)
	if !ok {
		return nil
	}
	state, _ := value.(*OpenAIAttemptState)
	return state
}

// OpenAIAttemptStateSnapshot returns a detached copy for telemetry and tests.
func OpenAIAttemptStateSnapshot(c *gin.Context) OpenAIAttemptState {
	state := OpenAIAttemptStateFromContext(c)
	if state == nil {
		return OpenAIAttemptState{}
	}
	copy := *state
	copy.ResponseIDs = append([]string(nil), state.ResponseIDs...)
	copy.ResponseConnIDs = append([]string(nil), state.ResponseConnIDs...)
	return copy
}

// SetOpenAIAttemptRouting records the logical request's continuation identity.
// It is safe to call before or between protocol-specific routing phases.
func SetOpenAIAttemptRouting(c *gin.Context, sessionHash, previousResponseID, promptCacheKey string) {
	state := OpenAIAttemptStateFromContext(c)
	if state == nil {
		return
	}
	if value := strings.TrimSpace(sessionHash); value != "" {
		state.SessionHash = value
	}
	if value := strings.TrimSpace(previousResponseID); value != "" {
		state.PreviousResponseID = value
	}
	if value := strings.TrimSpace(promptCacheKey); value != "" {
		state.PromptCacheKey = value
	}
}

// BeginOpenAIAttempt advances the state exactly once for an account attempt.
// Repeated forwarding helpers for the same account do not consume attempts.
func BeginOpenAIAttempt(c *gin.Context, accountID int64, body []byte) {
	if c == nil || accountID <= 0 {
		return
	}
	state := OpenAIAttemptStateFromContext(c)
	if state == nil {
		state = PrepareOpenAIAttemptState(c, body, "", "", "")
	}
	if state == nil {
		return
	}
	if state.attemptActive && state.CurrentAccountID == accountID {
		return
	}
	state.PreviousAccountID = state.CurrentAccountID
	state.CurrentAccountID = accountID
	state.Attempt++
	state.attemptActive = true
	ResetOpenAIAttemptWireState(c)
}

// TrackOpenAIResponseID and TrackOpenAIResponseConnID make newly-created
// continuation bindings available to the centralized account-switch cleanup.
func trackOpenAIResponseID(state *OpenAIAttemptState, responseID string) {
	responseID = strings.TrimSpace(responseID)
	if state == nil || responseID == "" {
		return
	}
	for _, existing := range state.ResponseIDs {
		if existing == responseID {
			return
		}
	}
	state.ResponseIDs = append(state.ResponseIDs, responseID)
}

func TrackOpenAIResponseID(c *gin.Context, responseID string) {
	trackOpenAIResponseID(OpenAIAttemptStateFromContext(c), responseID)
}

func trackOpenAIResponseConnID(state *OpenAIAttemptState, connID string) {
	connID = strings.TrimSpace(connID)
	if state == nil || connID == "" {
		return
	}
	for _, existing := range state.ResponseConnIDs {
		if existing == connID {
			return
		}
	}
	state.ResponseConnIDs = append(state.ResponseConnIDs, connID)
}

func TrackOpenAIResponseConnID(c *gin.Context, connID string) {
	trackOpenAIResponseConnID(OpenAIAttemptStateFromContext(c), connID)
}

// ResetOpenAIAttemptWireState starts a fresh request-local wire snapshot for
// one account attempt. It does not release slots or clear continuation state.
func ResetOpenAIAttemptWireState(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openAIAttemptWireStateKey, OpenAIAttemptWireState{})
}

func openAIAttemptWireState(c *gin.Context) OpenAIAttemptWireState {
	if c == nil {
		return OpenAIAttemptWireState{}
	}
	value, ok := c.Get(openAIAttemptWireStateKey)
	if !ok {
		return OpenAIAttemptWireState{}
	}
	state, _ := value.(OpenAIAttemptWireState)
	return state
}

// OpenAIAttemptWireStateSnapshot returns a copy of the current attempt-local
// wire evidence for structured telemetry.
func OpenAIAttemptWireStateSnapshot(c *gin.Context) OpenAIAttemptWireState {
	return openAIAttemptWireState(c)
}

func updateOpenAIAttemptWireState(c *gin.Context, update func(*OpenAIAttemptWireState)) {
	if c == nil || update == nil {
		return
	}
	state := openAIAttemptWireState(c)
	update(&state)
	c.Set(openAIAttemptWireStateKey, state)
}

// MarkOpenAIAttemptHeartbeat records a downstream heartbeat. Heartbeats are
// transport-visible but remain non-semantic and therefore replayable.
func MarkOpenAIAttemptHeartbeat(c *gin.Context) {
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.TransportCommitted = true
		if !state.SemanticOutputStarted {
			state.HeartbeatOnly = true
		}
	})
}

// MarkOpenAISemanticOutputStarted records the first upstream event that can
// contribute model output, a tool call, or an output item.
func MarkOpenAISemanticOutputStarted(c *gin.Context) {
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.SemanticOutputStarted = true
		state.HeartbeatOnly = false
		if budget := OpenAIRetryBudgetFromContext(c); budget != nil {
			budget.MarkSemanticOutput()
		}
	})
}

// MarkOpenAIAttemptTerminal records the normalized upstream terminal event.
func MarkOpenAIAttemptTerminal(c *gin.Context, event string) {
	event = strings.TrimSpace(event)
	if event == "" {
		return
	}
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.TerminalEvent = event
	})
}

// MarkOpenAIAttemptTransportCommitted records wire commitment without declaring
// that semantic output has started.
func MarkOpenAIAttemptTransportCommitted(c *gin.Context) {
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.TransportCommitted = true
		if budget := OpenAIRetryBudgetFromContext(c); budget != nil {
			budget.MarkBytesEmitted()
		}
	})
}

// ClassifyOpenAIAttemptFailure maps existing upstream predicates into stable
// telemetry labels. It intentionally does not decide whether failover is safe.
func ClassifyOpenAIAttemptFailure(err error) string {
	if err == nil {
		return "none"
	}

	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if _, _, ok := OpenAIUpstreamStreamReadErrorDetails(err); ok {
		lower := strings.ToLower(strings.TrimSpace(err.Error()))
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(lower, "timeout") {
			return "timeout"
		}
		return "transport"
	}

	var failoverErr *UpstreamFailoverError
	if errors.As(err, &failoverErr) && failoverErr != nil {
		message := strings.TrimSpace(err.Error())
		body := failoverErr.ResponseBody
		switch {
		case isOpenAIContextWindowError(message, body):
			return "context_window"
		case failoverErr.Reason == OpenAIAttemptFailureReasonCapacity:
			return "capacity"
		case openAIStreamFailureIsCapacity(failoverErr.StatusCode, body, message):
			return "capacity"
		case failoverErr.IsCredentialFailure() || failoverErr.StatusCode == 401 || failoverErr.StatusCode == 403:
			return "credential"
		case failoverErr.StatusCode == 429:
			return "rate_limit"
		case failoverErr.StatusCode >= 500:
			return "upstream"
		}
	}

	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	if isOpenAITransientProcessingError(http.StatusBadGateway, lower, nil) {
		return "capacity"
	}
	switch {
	case strings.Contains(lower, "context") && (strings.Contains(lower, "length") || strings.Contains(lower, "window") || strings.Contains(lower, "token")):
		return "context_window"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "timeout"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "http/2") || strings.Contains(lower, "stream error: stream id ") || strings.Contains(lower, "broken pipe"):
		return "transport"
	default:
		return "unknown"
	}
}
