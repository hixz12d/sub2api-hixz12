package handler

import (
	"context"
	"crypto/sha256"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const securityAuditCompletedContextKey = "sub2api.security_audit.completed"
const securityAuditAllowedContextKey = "sub2api.security_audit.allowed"
const selectedAccountContentModerationCompletedContextKey = "sub2api.content_moderation.selected_account.completed"
const securityAuditWSTurnContextKey = "sub2api.security_audit.ws_turn"
const securityAuditWSDedupeContextKey = "sub2api.security_audit.ws_dedupe"
const selectedAccountContentModerationWSDedupeContextKey = "sub2api.content_moderation.selected_account.ws_dedupe"

type securityAuditWSDedupeEntry struct {
	stage    string
	turn     int
	bodyHash [sha256.Size]byte
	decision securityaudit.Decision
}

type selectedAccountContentModerationWSDedupeEntry struct {
	accountID int64
	stage     string
	turn      int
	bodyHash  [sha256.Size]byte
	decision  securityaudit.Decision
}

// cachesSecurityAuditCompletion reports whether a successful audit may be
// reused for the rest of the gin request. WebSocket turns share one Context
// across many response.create frames and must be audited independently.
func cachesSecurityAuditCompletion(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "", "http":
		return true
	default:
		return false
	}
}

func isSecurityAuditWebSocketStage(stage string) bool {
	switch strings.TrimSpace(stage) {
	case "first_turn", "subsequent_turn":
		return true
	default:
		return false
	}
}

func (h *GatewayHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSecurityAudit(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSecurityAuditStage(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSecurityAudit(c, reqLog, h.securityAuditCoordinator, h.contentModerationService, apiKey, subject, protocol, model, body, stage)
}

func (h *GatewayHandler) checkSelectedAccountContentModeration(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSelectedAccountSecurityAudit(c, reqLog, h.contentModerationService, apiKey, subject, account, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSelectedAccountContentModeration(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte) *securityaudit.Decision {
	return h.checkSelectedAccountContentModerationStage(c, reqLog, apiKey, subject, account, protocol, model, body, "http")
}

func (h *OpenAIGatewayHandler) checkSelectedAccountContentModerationStage(c *gin.Context, reqLog *zap.Logger, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if h == nil {
		return nil
	}
	return runSelectedAccountSecurityAudit(c, reqLog, h.contentModerationService, apiKey, subject, account, protocol, model, body, stage)
}

func runSelectedAccountSecurityAudit(c *gin.Context, reqLog *zap.Logger, legacy *service.ContentModerationService, apiKey *service.APIKey, subject middleware2.AuthSubject, account *service.Account, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if c == nil || c.Request == nil || legacy == nil || account == nil {
		return nil
	}
	cacheCompletion := cachesSecurityAuditCompletion(stage)
	if cacheCompletion {
		if completed, exists := c.Get(selectedAccountContentModerationCompletedContextKey); exists && completed == true {
			return nil
		}
	}
	if isSecurityAuditWebSocketStage(stage) {
		turnNo, ok := securityAuditWSTurn(c)
		if !ok && strings.TrimSpace(stage) == "first_turn" {
			turnNo, ok = 1, true
		}
		if ok {
			bodyHash := sha256.Sum256(body)
			if cached, exists := c.Get(selectedAccountContentModerationWSDedupeContextKey); exists {
				if entry, ok := cached.(selectedAccountContentModerationWSDedupeEntry); ok &&
					entry.accountID == account.ID && entry.stage == strings.TrimSpace(stage) && entry.turn == turnNo && entry.bodyHash == bodyHash {
					decision := entry.decision
					return &decision
				}
			}
			legacyDecision, inScope := runSelectedAccountContentModeration(c, reqLog, legacy, apiKey, subject, account, protocol, model, body)
			if !inScope || legacyDecision == nil {
				return nil
			}
			decision := securityDecisionFromContentModeration(legacyDecision)
			if decision.AllowNextStage {
				c.Set(selectedAccountContentModerationWSDedupeContextKey, selectedAccountContentModerationWSDedupeEntry{
					accountID: account.ID, stage: strings.TrimSpace(stage), turn: turnNo, bodyHash: bodyHash, decision: *decision,
				})
			}
			return decision
		}
	}
	legacyDecision, inScope := runSelectedAccountContentModeration(c, reqLog, legacy, apiKey, subject, account, protocol, model, body)
	if !inScope || legacyDecision == nil {
		return nil
	}
	decision := securityDecisionFromContentModeration(legacyDecision)
	if decision.AllowNextStage && cacheCompletion {
		c.Set(selectedAccountContentModerationCompletedContextKey, true)
	}
	return decision
}

func securityDecisionFromContentModeration(legacyDecision *service.ContentModerationDecision) *securityaudit.Decision {
	if legacyDecision == nil {
		return nil
	}
	decision := &securityaudit.Decision{Kind: securityaudit.DecisionAllow, HTTPStatus: http.StatusOK, AllowNextStage: true}
	decision.Legacy = &securityaudit.LegacyDecision{
		Allowed: legacyDecision.Allowed, Blocked: legacyDecision.Blocked, Flagged: legacyDecision.Flagged,
		Message: legacyDecision.Message, StatusCode: legacyDecision.StatusCode,
		ErrorCode: "content_policy_violation", Action: legacyDecision.Action,
	}
	if legacyDecision.Blocked {
		decision.Kind, decision.HTTPStatus, decision.ErrorCode, decision.ClientMessage, decision.AllowNextStage = securityaudit.DecisionBlock, contentModerationStatus(legacyDecision), "content_policy_violation", legacyDecision.Message, false
	}
	return decision
}

func runSecurityAudit(c *gin.Context, reqLog *zap.Logger, coordinator *securityaudit.Coordinator, legacy *service.ContentModerationService, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if c == nil || c.Request == nil {
		return nil
	}
	cacheCompletion := cachesSecurityAuditCompletion(stage)
	if cacheCompletion {
		if completed, exists := c.Get(securityAuditCompletedContextKey); exists && completed == true {
			return nil
		}
	}
	if coordinator == nil {
		legacyDecision := runContentModeration(c, reqLog, legacy, apiKey, subject, protocol, model, body)
		if legacyDecision == nil {
			return nil
		}
		decision := securityDecisionFromContentModeration(legacyDecision)
		if decision.AllowNextStage && cacheCompletion {
			c.Set(securityAuditCompletedContextKey, true)
		}
		return decision
	}
	request := buildSecurityAuditRequest(c, apiKey, subject, protocol, model, body, stage)
	if isSecurityAuditWebSocketStage(request.Stage) {
		if turnNo, ok := securityAuditWSTurn(c); ok {
			bodyHash := sha256.Sum256(body)
			if cached, exists := c.Get(securityAuditWSDedupeContextKey); exists {
				if entry, ok := cached.(securityAuditWSDedupeEntry); ok &&
					entry.stage == request.Stage && entry.turn == turnNo && entry.bodyHash == bodyHash {
					decision := entry.decision
					logSecurityAuditDone(reqLog, request, decision, true)
					return &decision
				}
			}
			logSecurityAuditStart(reqLog, request, len(body), false)
			decision := coordinator.Check(c.Request.Context(), request)
			if decision.Kind == securityaudit.DecisionAllow {
				c.Set(securityAuditWSDedupeContextKey, securityAuditWSDedupeEntry{
					stage: request.Stage, turn: turnNo, bodyHash: bodyHash, decision: decision,
				})
			}
			logSecurityAuditDone(reqLog, request, decision, false)
			return &decision
		}
	}
	logSecurityAuditStart(reqLog, request, len(body), false)
	decision := coordinator.Check(c.Request.Context(), request)
	if decision.AllowNextStage && cacheCompletion {
		c.Set(securityAuditCompletedContextKey, true)
	}
	if decision.AllowNextStage && decision.Prompt != nil && decision.Prompt.AllowVia != "" {
		c.Set(securityAuditAllowedContextKey, securityAuditAllowedRecord{
			request: request, configVersion: decision.Prompt.ConfigVersion, allowVia: decision.Prompt.AllowVia,
		})
	}
	logSecurityAuditDone(reqLog, request, decision, false)
	return &decision
}

// securityAuditAllowedRecord remembers a request the prompt guard audited and
// let through, so an upstream refusal can be traced back to that verdict.
type securityAuditAllowedRecord struct {
	request       securityaudit.Request
	configVersion int64
	allowVia      string
}

// handleSecurityAuditUpstreamRefusal runs after forwarding: when the prompt
// guard allowed this request and the last forwarding attempt ended in an
// upstream refusal, it hands the request to the prompt audit for archiving and
// the refused cache. It never touches the response already sent.
func handleSecurityAuditUpstreamRefusal(c *gin.Context, coordinator *securityaudit.Coordinator) {
	if c == nil || c.Request == nil || coordinator == nil {
		return
	}
	value, exists := c.Get(securityAuditAllowedContextKey)
	if !exists {
		return
	}
	record, ok := value.(securityAuditAllowedRecord)
	if !ok {
		return
	}
	kind := service.ObservedUpstreamRefusalKind(c)
	if kind == "" {
		return
	}
	coordinator.HandleUpstreamRefusal(securityaudit.UpstreamRefusal{
		Request: record.request.Clone(), AuditConfigVersion: record.configVersion, AllowVia: record.allowVia,
		RefusalKind: kind, ClientRequestID: securityAuditUsageRequestID(c.Request.Context(), record.request.RequestID),
	})
}

// securityAuditUsageRequestID mirrors the usage_logs.request_id derivation
// for a forwarded request ("client:<id>", else "local:<id>").
func securityAuditUsageRequestID(ctx context.Context, requestID string) string {
	if ctx != nil {
		if clientRequestID, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			return "client:" + strings.TrimSpace(clientRequestID)
		}
	}
	if requestID = strings.TrimSpace(requestID); requestID != "" {
		return "local:" + requestID
	}
	return ""
}

func logSecurityAuditStart(reqLog *zap.Logger, request securityaudit.Request, bodyBytes int, cached bool) {
	if reqLog == nil {
		return
	}
	reqLog.Info("security_audit.gateway_check_start",
		zap.String("request_id", request.RequestID), zap.Int64("user_id", request.UserID),
		zap.Int64("api_key_id", request.APIKeyID), zap.Int64p("group_id", request.GroupID),
		zap.String("endpoint", request.Endpoint), zap.String("provider", request.Provider),
		zap.String("protocol", request.Protocol), zap.String("model", request.Model), zap.String("stage", request.Stage),
		zap.Int("body_bytes", bodyBytes), zap.Bool("cached", cached))
}

func logSecurityAuditDone(reqLog *zap.Logger, request securityaudit.Request, decision securityaudit.Decision, cached bool) {
	if reqLog == nil {
		return
	}
	reqLog.Info("security_audit.gateway_check_done",
		zap.String("request_id", request.RequestID), zap.String("decision", string(decision.Kind)),
		zap.String("error_code", decision.ErrorCode), zap.Bool("allow_next_stage", decision.AllowNextStage),
		zap.String("stage", request.Stage), zap.Bool("cached", cached))
}

func securityAuditWSTurn(c *gin.Context) (int, bool) {
	turn, exists := c.Get(securityAuditWSTurnContextKey)
	if !exists {
		return 0, false
	}
	turnNo, ok := turn.(int)
	return turnNo, ok
}

func buildSecurityAuditRequest(c *gin.Context, apiKey *service.APIKey, subject middleware2.AuthSubject, protocol, model string, body []byte, stage string) securityaudit.Request {
	legacy := buildContentModerationInput(c, apiKey, subject, protocol, model, body)
	request := securityaudit.Request{
		RequestID: legacy.RequestID, UserID: legacy.UserID, UserEmail: legacy.UserEmail,
		APIKeyID: legacy.APIKeyID, APIKeyName: legacy.APIKeyName, GroupID: cloneSecurityAuditGroupID(legacy.GroupID),
		GroupName: legacy.GroupName, Provider: legacy.Provider, Endpoint: legacy.Endpoint,
		Protocol: legacy.Protocol, Model: legacy.Model, Body: body, Stage: strings.TrimSpace(stage),
	}
	if apiKey != nil && apiKey.User != nil {
		request.Username = apiKey.User.Username
		if request.UserEmail == "" {
			request.UserEmail = apiKey.User.Email
		}
	}
	if request.Stage == "" {
		request.Stage = "http"
	}
	return request
}

func securityAuditStatus(decision *securityaudit.Decision) int {
	if decision == nil || decision.HTTPStatus < 400 || decision.HTTPStatus > 599 {
		return http.StatusForbidden
	}
	return decision.HTTPStatus
}

func securityAuditErrorCode(decision *securityaudit.Decision) string {
	if decision == nil || strings.TrimSpace(decision.ErrorCode) == "" {
		return "content_policy_violation"
	}
	return decision.ErrorCode
}

func securityAuditMessage(decision *securityaudit.Decision) string {
	if decision == nil {
		return "Request blocked by content policy"
	}
	if decision.Legacy != nil && decision.Legacy.Blocked && strings.TrimSpace(decision.Legacy.Message) != "" {
		return decision.Legacy.Message
	}
	if strings.TrimSpace(decision.ClientMessage) != "" {
		return decision.ClientMessage
	}
	return "Request blocked by content policy"
}

func cloneSecurityAuditGroupID(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
