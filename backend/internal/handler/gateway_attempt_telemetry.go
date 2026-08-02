package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// gatewayRequestAttemptTelemetry is shadow-only telemetry for the Responses
// failover loop. It emits structured log records and deliberately owns no
// scheduler, Redis, billing, or response-writing behavior.
type gatewayRequestAttemptTelemetry struct {
	log              *zap.Logger
	logicalRequestID string
	requestID        string
	clientRequestID  string
	endpoint         string
	requestedModel   string
	stream           bool
	userID           int64
	apiKeyID         int64
	groupID          int64
	groupIDPresent   bool
	startedAt        time.Time
	attemptCount     atomic.Int64
	switchCount      atomic.Int64
}

type gatewayAttemptTelemetry struct {
	parent            *gatewayRequestAttemptTelemetry
	attemptID         string
	attemptIndex      int
	accountID         int64
	accountPlatform   string
	accountType       string
	proxyID           int64
	proxyIDPresent    bool
	canonicalModel    string
	routeLayer        string
	stickyPreviousHit bool
	stickySessionHit  bool
	startedAt         time.Time
}

func newGatewayRequestAttemptTelemetry(
	c *gin.Context,
	log *zap.Logger,
	endpoint, requestedModel string,
	stream bool,
	userID, apiKeyID int64,
	groupID *int64,
) *gatewayRequestAttemptTelemetry {
	requestID := gatewayTelemetryContextString(c, ctxkey.RequestID)
	clientRequestID := gatewayTelemetryContextString(c, ctxkey.ClientRequestID)
	logicalRequestID := requestID
	if logicalRequestID == "" {
		logicalRequestID = uuid.NewString()
	}
	if log == nil {
		log = requestLogger(c, "handler.gateway_attempt_telemetry")
	}
	groupValue, groupPresent := gatewayTelemetryGroupID(groupID)
	return &gatewayRequestAttemptTelemetry{
		log:              log,
		logicalRequestID: logicalRequestID,
		requestID:        requestID,
		clientRequestID:  clientRequestID,
		endpoint:         strings.TrimSpace(endpoint),
		requestedModel:   strings.TrimSpace(requestedModel),
		stream:           stream,
		userID:           userID,
		apiKeyID:         apiKeyID,
		groupID:          groupValue,
		groupIDPresent:   groupPresent,
		startedAt:        time.Now(),
	}
}

func (t *gatewayRequestAttemptTelemetry) beginAttempt(
	attemptIndex int,
	account *service.Account,
	canonicalModel, routeLayer string,
	stickyPreviousHit, stickySessionHit bool,
) *gatewayAttemptTelemetry {
	if t == nil {
		return nil
	}
	attempt := &gatewayAttemptTelemetry{
		parent:            t,
		attemptID:         uuid.NewString(),
		attemptIndex:      attemptIndex,
		canonicalModel:    strings.TrimSpace(canonicalModel),
		routeLayer:        strings.TrimSpace(routeLayer),
		stickyPreviousHit: stickyPreviousHit,
		stickySessionHit:  stickySessionHit,
		startedAt:         time.Now(),
	}
	if account != nil {
		attempt.accountID = account.ID
		attempt.accountPlatform = strings.TrimSpace(account.Platform)
		attempt.accountType = strings.TrimSpace(account.Type)
		if account.ProxyID != nil {
			attempt.proxyID = *account.ProxyID
			attempt.proxyIDPresent = attempt.proxyID > 0
		}
	}
	t.attemptCount.Add(1)
	return attempt
}

func (t *gatewayRequestAttemptTelemetry) finish(c *gin.Context, streamStarted bool) {
	if t == nil || t.log == nil {
		return
	}
	terminalState := "completed"
	if c != nil {
		if streamError, ok := service.GetOpsStreamError(c); ok && streamError.CountTowardsSLA {
			terminalState = "failed"
		} else if c.Writer != nil && c.Writer.Status() >= http.StatusBadRequest {
			terminalState = "failed"
		} else if c.Request != nil && errors.Is(c.Request.Context().Err(), context.Canceled) {
			terminalState = "canceled"
		} else if streamStarted && service.IsResponseCommitted(c) && c.Writer.Status() == http.StatusOK {
			terminalState = "completed_or_in_band_failed"
		}
	}
	fields := []zap.Field{
		zap.String("event", "request_complete"),
		zap.String("logical_request_id", t.logicalRequestID),
		zap.String("request_id", t.requestID),
		zap.String("client_request_id", t.clientRequestID),
		zap.String("endpoint", t.endpoint),
		zap.String("requested_model", t.requestedModel),
		zap.Bool("stream", t.stream),
		zap.Int64("user_id", t.userID),
		zap.Int64("api_key_id", t.apiKeyID),
		zap.Int64("group_id", t.groupID),
		zap.Bool("group_id_present", t.groupIDPresent),
		zap.Int("http_status", gatewayTelemetryStatus(c)),
		zap.String("terminal_state", terminalState),
		zap.Int64("attempt_count", t.attemptCount.Load()),
		zap.Int64("switch_count", t.switchCount.Load()),
		zap.Int64("duration_ms", time.Since(t.startedAt).Milliseconds()),
	}
	if state := service.OpenAIAttemptWireStateSnapshot(c); state.TerminalEvent != "" {
		fields = append(fields, zap.String("terminal_event", state.TerminalEvent))
	}
	t.log.Info("openai.gateway_attempt", fields...)
}

func (a *gatewayAttemptTelemetry) recordForwardComplete(
	c *gin.Context,
	upstreamModel, upstreamEndpoint string,
	inputTokens, outputTokens int,
	firstTokenMs *int,
	err error,
	writerCommitted bool,
	retryEligible bool,
	compactKeepaliveBytesBefore int,
) {
	if a == nil || a.parent == nil || a.parent.log == nil {
		return
	}
	state := service.OpenAIAttemptWireStateSnapshot(c)
	if writerCommitted {
		state.TransportCommitted = true
	}
	if service.OpenAICompactKeepaliveBytes(c) > compactKeepaliveBytesBefore && !state.SemanticOutputStarted {
		state.TransportCommitted = true
		state.HeartbeatOnly = true
	}
	semanticUnknown := state.TransportCommitted && !state.SemanticOutputStarted && err != nil
	var failoverErr *service.UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		failoverErr = nil
	}
	upstreamStatus := 0
	if failoverErr != nil {
		upstreamStatus = failoverErr.StatusCode
	}
	wireState := "not_started"
	switch {
	case state.SemanticOutputStarted:
		wireState = "semantic_output"
	case state.HeartbeatOnly:
		wireState = "heartbeat_only"
	case state.TransportCommitted:
		wireState = "transport_committed"
	}
	fields := a.baseFields(c)
	fields = append(fields,
		zap.String("event", "forward_complete"),
		zap.String("attempt_id", a.attemptID),
		zap.Int("attempt_index", a.attemptIndex),
		zap.String("upstream_model", strings.TrimSpace(upstreamModel)),
		zap.String("upstream_endpoint", strings.TrimSpace(upstreamEndpoint)),
		zap.Int("upstream_status", upstreamStatus),
		zap.String("failure_class", service.ClassifyOpenAIAttemptFailure(err)),
		zap.Bool("retry_eligible", retryEligible),
		zap.Bool("safe_after_nonsemantic_write", failoverErr != nil && failoverErr.SafeToFailoverAfterWrite),
		zap.Bool("same_account_retryable", failoverErr != nil && failoverErr.RetryableOnSameAccount),
		zap.Bool("transport_committed", state.TransportCommitted),
		zap.Bool("heartbeat_only", state.HeartbeatOnly),
		zap.Bool("semantic_output_started", state.SemanticOutputStarted),
		zap.Bool("semantic_output_unknown", semanticUnknown),
		zap.String("wire_state", wireState),
		zap.String("terminal_event", state.TerminalEvent),
		zap.Int("input_tokens", inputTokens),
		zap.Int("output_tokens", outputTokens),
		zap.Int64("duration_ms", time.Since(a.startedAt).Milliseconds()),
	)
	if firstTokenMs != nil {
		fields = append(fields, zap.Int("first_token_ms", *firstTokenMs))
	}
	a.parent.log.Info("openai.gateway_attempt", fields...)
}

func (a *gatewayAttemptTelemetry) recordDecision(decision string, switchTaken bool, switchCount int) {
	if a == nil || a.parent == nil || a.parent.log == nil {
		return
	}
	if switchTaken {
		a.parent.switchCount.Store(int64(switchCount))
	}
	fields := a.baseFields(nil)
	fields = append(fields,
		zap.String("event", "failover_decision"),
		zap.String("attempt_id", a.attemptID),
		zap.Int("attempt_index", a.attemptIndex),
		zap.String("decision", strings.TrimSpace(decision)),
		zap.Bool("switch_taken", switchTaken),
		zap.Int("switch_count", switchCount),
	)
	a.parent.log.Info("openai.gateway_attempt", fields...)
}

func (a *gatewayAttemptTelemetry) baseFields(c *gin.Context) []zap.Field {
	if a == nil || a.parent == nil {
		return nil
	}
	p := a.parent
	return []zap.Field{
		zap.String("logical_request_id", p.logicalRequestID),
		zap.String("request_id", p.requestID),
		zap.String("client_request_id", p.clientRequestID),
		zap.String("endpoint", p.endpoint),
		zap.String("requested_model", p.requestedModel),
		zap.Bool("stream", p.stream),
		zap.Int64("user_id", p.userID),
		zap.Int64("api_key_id", p.apiKeyID),
		zap.Int64("group_id", p.groupID),
		zap.Bool("group_id_present", p.groupIDPresent),
		zap.Int64("account_id", a.accountID),
		zap.String("account_platform", a.accountPlatform),
		zap.String("account_type", a.accountType),
		zap.Int64("proxy_id", a.proxyID),
		zap.Bool("proxy_id_present", a.proxyIDPresent),
		zap.String("canonical_model", a.canonicalModel),
		zap.String("route_layer", a.routeLayer),
		zap.Bool("sticky_previous_hit", a.stickyPreviousHit),
		zap.Bool("sticky_session_hit", a.stickySessionHit),
		zap.Int("http_status_at_emit", gatewayTelemetryStatus(c)),
	}
}

func gatewayTelemetryContextString(c *gin.Context, key ctxkey.Key) string {
	if c == nil || c.Request == nil {
		return ""
	}
	value, _ := c.Request.Context().Value(key).(string)
	return strings.TrimSpace(value)
}

func gatewayTelemetryGroupID(groupID *int64) (int64, bool) {
	if groupID == nil {
		return 0, false
	}
	return *groupID, true
}

func gatewayTelemetryStatus(c *gin.Context) int {
	if c == nil || c.Writer == nil {
		return 0
	}
	return c.Writer.Status()
}
