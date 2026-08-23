package admin

import (
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

type accountCreateTemplateValuesRequest struct {
	ProxyID                  *int64   `json:"proxy_id"`
	Concurrency              int      `json:"concurrency"`
	LoadFactor               *int     `json:"load_factor"`
	Priority                 int      `json:"priority"`
	RateMultiplier           float64  `json:"rate_multiplier"`
	GroupIDs                 []int64  `json:"group_ids"`
	QuotaLimit               *float64 `json:"quota_limit"`
	QuotaDailyLimit          *float64 `json:"quota_daily_limit"`
	QuotaWeeklyLimit         *float64 `json:"quota_weekly_limit"`
	AutoPauseOnExpired       bool     `json:"auto_pause_on_expired"`
	InterceptWarmup          bool     `json:"intercept_warmup"`
	OpenAIPassthrough        bool     `json:"openai_passthrough"`
	OpenAIFlattenNamespaces  bool     `json:"openai_flatten_namespaces"`
	OpenAILongContextBilling bool     `json:"openai_long_context_billing"`
	OpenAIWSMode             string   `json:"openai_ws_mode"`
	OpenAICompactMode        string   `json:"openai_compact_mode"`
	CodexCLIOnly             bool     `json:"codex_cli_only"`
	CodexCLIOnlyAppServer    bool     `json:"codex_cli_only_app_server"`
	CodexFingerprintMode     string   `json:"codex_fingerprint_mode"`
	TLSFingerprintEnabled    bool     `json:"tls_fingerprint_enabled"`
	TLSFingerprintProfileID  *int64   `json:"tls_fingerprint_profile_id"`
}

type accountCreateTemplateWriteRequest struct {
	Name          string                             `json:"name"`
	Platform      string                             `json:"platform"`
	Type          string                             `json:"type"`
	IsDefault     bool                               `json:"is_default"`
	IncludeGroups bool                               `json:"include_groups"`
	Values        accountCreateTemplateValuesRequest `json:"values"`
}

func accountCreateTemplateInputFromRequest(req accountCreateTemplateWriteRequest) service.AccountCreateTemplateInput {
	return service.AccountCreateTemplateInput{
		Name:          req.Name,
		Platform:      req.Platform,
		Type:          req.Type,
		IsDefault:     req.IsDefault,
		IncludeGroups: req.IncludeGroups,
		Values: service.AccountCreateTemplateValues{
			ProxyID:                  req.Values.ProxyID,
			Concurrency:              req.Values.Concurrency,
			LoadFactor:               req.Values.LoadFactor,
			Priority:                 req.Values.Priority,
			RateMultiplier:           req.Values.RateMultiplier,
			GroupIDs:                 req.Values.GroupIDs,
			QuotaLimit:               req.Values.QuotaLimit,
			QuotaDailyLimit:          req.Values.QuotaDailyLimit,
			QuotaWeeklyLimit:         req.Values.QuotaWeeklyLimit,
			AutoPauseOnExpired:       req.Values.AutoPauseOnExpired,
			InterceptWarmup:          req.Values.InterceptWarmup,
			OpenAIPassthrough:        req.Values.OpenAIPassthrough,
			OpenAIFlattenNamespaces:  req.Values.OpenAIFlattenNamespaces,
			OpenAILongContextBilling: req.Values.OpenAILongContextBilling,
			OpenAIWSMode:             req.Values.OpenAIWSMode,
			OpenAICompactMode:        req.Values.OpenAICompactMode,
			CodexCLIOnly:             req.Values.CodexCLIOnly,
			CodexCLIOnlyAppServer:    req.Values.CodexCLIOnlyAppServer,
			CodexFingerprintMode:     req.Values.CodexFingerprintMode,
			TLSFingerprintEnabled:    req.Values.TLSFingerprintEnabled,
			TLSFingerprintProfileID:  req.Values.TLSFingerprintProfileID,
		},
	}
}

// ListAccountCreateTemplates GET /api/v1/admin/settings/account-create-templates
func (h *SettingHandler) ListAccountCreateTemplates(c *gin.Context) {
	items, err := h.settingService.ListAccountCreateTemplates(
		c.Request.Context(),
		strings.TrimSpace(c.Query("platform")),
		strings.TrimSpace(c.Query("type")),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if items == nil {
		items = []service.AccountCreateTemplate{}
	}
	response.Success(c, gin.H{"items": items})
}

// CreateAccountCreateTemplate POST /api/v1/admin/settings/account-create-templates
func (h *SettingHandler) CreateAccountCreateTemplate(c *gin.Context) {
	var req accountCreateTemplateWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	item, err := h.settingService.CreateAccountCreateTemplate(c.Request.Context(), accountCreateTemplateInputFromRequest(req))
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Created(c, item)
}

// UpdateAccountCreateTemplate PUT /api/v1/admin/settings/account-create-templates/:id
func (h *SettingHandler) UpdateAccountCreateTemplate(c *gin.Context) {
	var req accountCreateTemplateWriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorFrom(c, infraerrors.BadRequest("VALIDATION_ERROR", err.Error()))
		return
	}
	item, err := h.settingService.UpdateAccountCreateTemplate(
		c.Request.Context(),
		c.Param("id"),
		accountCreateTemplateInputFromRequest(req),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

// DeleteAccountCreateTemplate DELETE /api/v1/admin/settings/account-create-templates/:id
func (h *SettingHandler) DeleteAccountCreateTemplate(c *gin.Context) {
	if err := h.settingService.DeleteAccountCreateTemplate(c.Request.Context(), c.Param("id")); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"ok": true})
}
