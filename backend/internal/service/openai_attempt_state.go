package service

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
)

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

const openAIAttemptWireStateKey = "openai_attempt_wire_state"

const OpenAIAttemptFailureReasonCapacity GatewayFailureReason = "openai_capacity"

// ResetOpenAIAttemptWireState starts a fresh request-local snapshot for one
// account attempt. It does not alter the response writer or scheduling state.
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
// contribute model output, a tool call, or an output item. The downstream
// writer may still reject the event, so this does not commit transport state.
func MarkOpenAISemanticOutputStarted(c *gin.Context) {
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.SemanticOutputStarted = true
		state.HeartbeatOnly = false
	})
}

// MarkOpenAIAttemptTerminal records the normalized upstream terminal event.
// A terminal event may be observed before it is presented downstream, so it
// must not by itself mark the transport as committed.
func MarkOpenAIAttemptTerminal(c *gin.Context, event string) {
	event = strings.TrimSpace(event)
	if event == "" {
		return
	}
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.TerminalEvent = event
	})
}

// MarkOpenAIAttemptTransportCommitted records wire commitment without
// declaring that semantic output has started.
func MarkOpenAIAttemptTransportCommitted(c *gin.Context) {
	updateOpenAIAttemptWireState(c, func(state *OpenAIAttemptWireState) {
		state.TransportCommitted = true
	})
}

// ClassifyOpenAIAttemptFailure maps existing upstream predicates into stable
// telemetry labels. It intentionally does not decide whether failover is safe.
func ClassifyOpenAIAttemptFailure(err error) string {
	if err == nil {
		return "none"
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
	switch {
	case strings.Contains(lower, "context") && (strings.Contains(lower, "length") || strings.Contains(lower, "window") || strings.Contains(lower, "token")):
		return "context_window"
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded"):
		return "timeout"
	case strings.Contains(lower, "connection") || strings.Contains(lower, "http/2") || strings.Contains(lower, "broken pipe"):
		return "transport"
	default:
		return "unknown"
	}
}
