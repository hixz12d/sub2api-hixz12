package handler

import (
	"log/slog"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
)

// ModelPlazaHandler 处理「模型广场」查询。
//
// 广场路由挂 OptionalJWT 中间件：匿名可访问（除非 require_auth 开启），带 token 则
// 识别用户。可见性规则（橱窗语义，与「可用渠道」的可绑定语义不同）：
//   - 匿名：仅非专属分组（订阅型照常展示）；
//   - 登录：非专属分组 + user_allowed_groups 授权或持有有效订阅的专属分组；
//     若该用户开启了公开分组限制，则公开分组同样需要落在授权集合内。
type ModelPlazaHandler struct {
	plazaService   *service.ModelPlazaService
	apiKeyService  *service.APIKeyService
	settingService *service.SettingService
}

// NewModelPlazaHandler 创建模型广场 handler。
func NewModelPlazaHandler(
	plazaService *service.ModelPlazaService,
	apiKeyService *service.APIKeyService,
	settingService *service.SettingService,
) *ModelPlazaHandler {
	return &ModelPlazaHandler{
		plazaService:   plazaService,
		apiKeyService:  apiKeyService,
		settingService: settingService,
	}
}

// modelPlazaOfficialPricing 官方参考价（USD per token，与计费目录同源）。
type modelPlazaOfficialPricing struct {
	InputPrice         *float64 `json:"input_price"`
	OutputPrice        *float64 `json:"output_price"`
	CacheWritePrice    *float64 `json:"cache_write_price"`
	CacheWrite1hPrice  *float64 `json:"cache_write_1h_price,omitempty"`
	CacheWrite12hPrice *float64 `json:"cache_write_12h_price,omitempty"`
	CacheReadPrice     *float64 `json:"cache_read_price"`
	// Intervals 官方长上下文阶梯，仅多档模型给出。
	Intervals []userPricingIntervalDTO `json:"intervals,omitempty"`
}

// modelPlazaTimePricingPeriod 分时倍率时段（配置时区当天 [start, end)）。
type modelPlazaTimePricingPeriod struct {
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Multiplier float64 `json:"multiplier"`
}

// modelPlazaTimePricing 计费会生效的分时倍率（仅倍率 ≠ 1 的时段）。
// WeekdaysOnly 为 true 时时段仅周一至周五生效，周末整天按标准价计费。
type modelPlazaTimePricing struct {
	Timezone     string                        `json:"timezone"`
	WeekdaysOnly bool                          `json:"weekdays_only,omitempty"`
	Periods      []modelPlazaTimePricingPeriod `json:"periods"`
}

// modelPlazaModel 广场模型条目：实收口径展示定价（白名单形态）+ 官方参考价 + 元数据与脱敏指标。
type modelPlazaModel struct {
	Name            string                     `json:"name"`
	Platform        string                     `json:"platform"`
	Pricing         *userSupportedModelPricing `json:"pricing"`
	OfficialPricing *modelPlazaOfficialPricing `json:"official_pricing"`
	// LongContextBasis 多档时的计价基准："whole_request"（整单按档）| "marginal"（仅超出部分）。
	LongContextBasis string `json:"long_context_basis,omitempty"`
	// TimePricing 分时倍率时段，落在时段内的请求整单乘倍率；无分时省略。
	TimePricing      *modelPlazaTimePricing      `json:"time_pricing,omitempty"`
	Metadata         *service.PlazaModelMetadata `json:"metadata,omitempty"`
	Metrics          *service.PlazaModelMetrics  `json:"metrics,omitempty"`
	Note             string                      `json:"note,omitempty"`
	BareAliasForHigh bool                        `json:"bare_alias_for_high,omitempty"`
}

// modelPlazaGroupComparison 用户可见的全部分组横向比价项（严禁暴露分组数字 ID）。
type modelPlazaGroupComparison struct {
	GroupName        string                     `json:"group_name"`
	Platform         string                     `json:"platform"`
	SubscriptionType string                     `json:"subscription_type"`
	IsExclusive      bool                       `json:"is_exclusive"`
	EffectiveRate    float64                    `json:"effective_rate"`
	Pricing          *userSupportedModelPricing `json:"pricing"`
	OfficialPricing  *modelPlazaOfficialPricing `json:"official_pricing"`
	Metrics          *service.PlazaModelMetrics `json:"metrics"`
}

// modelPlazaDetailResponse 单模型详情响应（按需懒加载，不含数字分组 ID）。
type modelPlazaDetailResponse struct {
	Model            string                          `json:"model"`
	Platform         string                          `json:"platform"`
	GroupName        string                          `json:"group_name"`
	Metadata         service.PlazaModelMetadata      `json:"metadata"`
	Pricing          *userSupportedModelPricing      `json:"pricing"`
	OfficialPricing  *modelPlazaOfficialPricing      `json:"official_pricing"`
	LongContextBasis string                          `json:"long_context_basis,omitempty"`
	TimePricing      *modelPlazaTimePricing          `json:"time_pricing,omitempty"`
	Note             string                          `json:"note,omitempty"`
	BareAliasForHigh bool                            `json:"bare_alias_for_high,omitempty"`
	IsAuthorized     bool                            `json:"is_authorized"`
	ShowcaseBadge    string                          `json:"showcase_badge,omitempty"`
	Windows          service.PlazaDetailWindows      `json:"windows"`
	Heartbeat72      []service.PlazaHeartbeatSlot    `json:"heartbeat_72"`
	Trend24h         []service.PlazaDetailTrendPoint `json:"trend_24h"`
	GroupComparisons []modelPlazaGroupComparison     `json:"group_comparisons"`
	Protection       service.PlazaDetailProtection   `json:"protection"`
	UpdatedAt        *string                         `json:"updated_at"`
}

// modelPlazaGroup 广场分组条目（白名单字段）。
type modelPlazaGroup struct {
	ID                 int64                         `json:"id"`
	Name               string                        `json:"name"`
	Description        string                        `json:"description"`
	Platform           string                        `json:"platform"`
	SubscriptionType   string                        `json:"subscription_type"`
	RateMultiplier     float64                       `json:"rate_multiplier"`
	UserRateMultiplier *float64                      `json:"user_rate_multiplier,omitempty"`
	PeakRateEnabled    bool                          `json:"peak_rate_enabled"`
	PeakStart          string                        `json:"peak_start"`
	PeakEnd            string                        `json:"peak_end"`
	PeakRateMultiplier float64                       `json:"peak_rate_multiplier"`
	IsExclusive        bool                          `json:"is_exclusive"`
	IsAuthorized       bool                          `json:"is_authorized"`
	ShowcaseBadge      string                        `json:"showcase_badge,omitempty"`
	PoolStatus         *service.PlazaGroupPoolStatus `json:"pool_status,omitempty"`
	// 生图独立倍率：为 true 时图片计费模型的实付倍率取 ImageRateMultiplier，
	// 不取分组/用户专属倍率。
	ImageRateIndependent bool    `json:"image_rate_independent"`
	ImageRateMultiplier  float64 `json:"image_rate_multiplier"`
	VideoRateIndependent bool    `json:"video_rate_independent"`
	VideoRateMultiplier  float64 `json:"video_rate_multiplier"`
	// 分组是否启用长上下文阶梯计费；关闭时模型实付列只展示最低档/基础价。
	LongContextPricingEnabled bool              `json:"long_context_pricing_enabled"`
	Models                    []modelPlazaModel `json:"models"`
}

// modelPlazaResponse 广场页响应。
type modelPlazaResponse struct {
	Description string                    `json:"description"`
	Groups      []modelPlazaGroup         `json:"groups"`
	SiteSummary *service.PlazaSiteSummary `json:"site_summary,omitempty"`
}

// Get 返回模型广场数据。
// GET /api/v1/model-plaza
func (h *ModelPlazaHandler) Get(c *gin.Context) {
	rt, visible, userRates, ok := h.loadVisiblePlazaContext(c)
	if !ok {
		return
	}

	acceptLanguage := c.GetHeader("Accept-Language")
	enriched, siteSummary := h.plazaService.EnrichVisibleGroups(c.Request.Context(), visible, acceptLanguage)

	// 广场可匿名访问，分组文案与模型简介随请求语言变化，提示中间缓存按语言区分。
	// 追加而非覆盖：CORS 中间件已为具体来源写入 Vary: Origin。
	c.Writer.Header().Add("Vary", "Accept-Language")
	out := toLocalizedModelPlazaGroups(enriched, userRates, acceptLanguage)
	response.Success(c, modelPlazaResponse{
		Description: rt.Description,
		Groups:      out,
		SiteSummary: siteSummary,
	})
}

// Detail 返回单模型全维详情（24h/7d/30d 三档 SLA、72 格心跳、趋势点、全可见分组比价、保护说明）。
// GET /api/v1/model-plaza/detail?model=<模型名>&group=<分组名(可选)>
func (h *ModelPlazaHandler) Detail(c *gin.Context) {
	if h.settingService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !rt.Enabled {
		response.NotFound(c, "Model plaza is not enabled")
		return
	}

	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if rt.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return
	}

	modelQuery := strings.TrimSpace(c.Query("model"))
	if modelQuery == "" {
		response.BadRequest(c, "model parameter is required")
		return
	}
	groupQuery := strings.TrimSpace(c.Query("group"))

	visible, userRates, ok := h.loadVisibleGroupsForSubject(c, subject, authed)
	if !ok {
		return
	}

	acceptLanguage := c.GetHeader("Accept-Language")
	detail, found := h.plazaService.BuildModelDetail(
		c.Request.Context(),
		visible,
		userRates,
		modelQuery,
		groupQuery,
		acceptLanguage,
	)
	if !found || detail == nil {
		response.NotFound(c, "Model not found")
		return
	}

	c.Writer.Header().Add("Vary", "Accept-Language")
	response.Success(c, toModelPlazaDetailDTO(detail))
}

func (h *ModelPlazaHandler) loadVisiblePlazaContext(c *gin.Context) (service.ModelPlazaRuntime, []service.PlazaGroup, map[int64]float64, bool) {
	if h.settingService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return service.ModelPlazaRuntime{}, nil, nil, false
	}
	rt := h.settingService.GetModelPlazaRuntime(c.Request.Context())
	if !rt.Enabled {
		response.NotFound(c, "Model plaza is not enabled")
		return service.ModelPlazaRuntime{}, nil, nil, false
	}

	subject, authed := middleware.GetAuthSubjectFromContext(c)
	if rt.RequireAuth && !authed {
		response.Unauthorized(c, "Authentication required")
		return service.ModelPlazaRuntime{}, nil, nil, false
	}

	visible, userRates, ok := h.loadVisibleGroupsForSubject(c, subject, authed)
	if !ok {
		return service.ModelPlazaRuntime{}, nil, nil, false
	}
	return rt, visible, userRates, true
}

func (h *ModelPlazaHandler) loadVisibleGroupsForSubject(
	c *gin.Context,
	subject middleware.AuthSubject,
	authed bool,
) ([]service.PlazaGroup, map[int64]float64, bool) {
	if h.plazaService == nil {
		response.NotFound(c, "Model plaza is not enabled")
		return nil, nil, false
	}
	groups, err := h.plazaService.ListGroups(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return nil, nil, false
	}

	// allowedGroups == nil 表示匿名；登录用户恒为非 nil（可能为空集合）。
	var allowedGroups map[int64]struct{}
	var restrictPublicGroups bool
	var userRates map[int64]float64
	if authed {
		allowedGroups, restrictPublicGroups, err = h.apiKeyService.GetUserGroupVisibility(c.Request.Context(), subject.UserID)
		if err != nil {
			// 可见性数据拿不到时不能静默降级成匿名视图（会错漏专属分组），直接报错。
			response.ErrorFrom(c, err)
			return nil, nil, false
		}
		userRates, err = h.apiKeyService.GetUserGroupRates(c.Request.Context(), subject.UserID)
		if err != nil {
			// 专属倍率仅是展示增强，失败降级为分组默认倍率。
			slog.Warn("model_plaza_user_rates_failed", "error", err, "user_id", subject.UserID)
			userRates = nil
		}
	}

	visible := filterPlazaVisibleGroups(groups, allowedGroups, restrictPublicGroups)
	return visible, userRates, true
}

// filterPlazaVisibleGroups 按登录态裁剪分组可见性并标注 IsAuthorized。
//   - 非专属分组（且未命中 restrictPublicGroups 限制）：可见，IsAuthorized = true；
//   - 专属分组（或受限公开分组）：若落在 allowedGroups 内则可见且 IsAuthorized = true；
//     若未获授权（含匿名访问）但配置了 ShowcasePublic = true，则保留在可见列表中并标记
//     IsAuthorized = false（供前端展示内测徽章与联系管理员开通指引）；否则隐藏。
func filterPlazaVisibleGroups(
	groups []service.PlazaGroup,
	allowedGroups map[int64]struct{},
	restrictPublicGroups bool,
) []service.PlazaGroup {
	visible := make([]service.PlazaGroup, 0, len(groups))
	for _, g := range groups {
		requiresGrant := g.IsExclusive || (restrictPublicGroups && allowedGroups != nil)
		if !requiresGrant {
			g.IsAuthorized = true
			visible = append(visible, g)
			continue
		}
		if allowedGroups != nil {
			if _, ok := allowedGroups[g.ID]; ok {
				g.IsAuthorized = true
				visible = append(visible, g)
				continue
			}
		}
		if g.ShowcasePublic {
			g.IsAuthorized = false
			visible = append(visible, g)
		}
	}
	return visible
}

// toLocalizedModelPlazaGroups 转换可见分组，并把名称与描述换成请求首选语言的版本。
func toLocalizedModelPlazaGroups(visible []service.PlazaGroup, userRates map[int64]float64, acceptLanguage string) []modelPlazaGroup {
	out := make([]modelPlazaGroup, 0, len(visible))
	for i := range visible {
		group := toModelPlazaGroupDTO(&visible[i], userRates)
		group.Name, group.Description = visible[i].I18n.Localize(acceptLanguage, group.Name, group.Description)
		out = append(out, group)
	}
	return out
}

// toModelPlazaGroupDTO 将 service 层广场分组映射为白名单 DTO,并合并用户专属倍率。
func toModelPlazaGroupDTO(g *service.PlazaGroup, userRates map[int64]float64) modelPlazaGroup {
	models := make([]modelPlazaModel, 0, len(g.Models))
	for i := range g.Models {
		m := &g.Models[i]
		models = append(models, modelPlazaModel{
			Name:             m.Name,
			Platform:         m.Platform,
			Pricing:          toUserPricing(m.Pricing),
			OfficialPricing:  toModelPlazaOfficialPricing(m.OfficialPricing),
			LongContextBasis: string(m.LongContextBasis),
			TimePricing:      toModelPlazaTimePricing(m.TimePricing),
			Metadata:         m.Metadata,
			Metrics:          m.Metrics,
			Note:             m.Note,
			BareAliasForHigh: m.BareAliasForHigh,
		})
	}
	dto := modelPlazaGroup{
		ID:                        g.ID,
		Name:                      g.Name,
		Description:               g.Description,
		Platform:                  g.Platform,
		SubscriptionType:          g.SubscriptionType,
		RateMultiplier:            g.RateMultiplier,
		PeakRateEnabled:           g.PeakRateEnabled,
		PeakStart:                 g.PeakStart,
		PeakEnd:                   g.PeakEnd,
		PeakRateMultiplier:        g.PeakRateMultiplier,
		IsExclusive:               g.IsExclusive,
		IsAuthorized:              g.IsAuthorized,
		ShowcaseBadge:             g.ShowcaseBadge,
		PoolStatus:                g.PoolStatus,
		ImageRateIndependent:      g.ImageRateIndependent,
		ImageRateMultiplier:       g.ImageRateMultiplier,
		VideoRateIndependent:      g.VideoRateIndependent,
		VideoRateMultiplier:       g.VideoRateMultiplier,
		LongContextPricingEnabled: g.LongContextPricingEnabled,
		Models:                    models,
	}
	if !g.ShowcasePublic || g.IsAuthorized {
		if rate, ok := userRates[g.ID]; ok {
			dto.UserRateMultiplier = &rate
		}
	}
	return dto
}

// toModelPlazaTimePricing 转换分时倍率；nil 透传（JSON 省略）。
func toModelPlazaTimePricing(p *service.TimePricingSchedule) *modelPlazaTimePricing {
	if p == nil || len(p.Periods) == 0 {
		return nil
	}
	periods := make([]modelPlazaTimePricingPeriod, 0, len(p.Periods))
	for _, period := range p.Periods {
		periods = append(periods, modelPlazaTimePricingPeriod{
			StartTime:  period.StartTime,
			EndTime:    period.EndTime,
			Multiplier: period.Multiplier,
		})
	}
	return &modelPlazaTimePricing{Timezone: p.Timezone, WeekdaysOnly: p.WeekdaysOnly, Periods: periods}
}

// toModelPlazaOfficialPricing 转换官方参考价；nil 透传（前端显示 "-"）。
func toModelPlazaOfficialPricing(p *service.PlazaOfficialPricing) *modelPlazaOfficialPricing {
	if p == nil {
		return nil
	}
	return &modelPlazaOfficialPricing{
		InputPrice:         p.InputPrice,
		OutputPrice:        p.OutputPrice,
		CacheWritePrice:    p.CacheWritePrice,
		CacheWrite1hPrice:  p.CacheWrite1hPrice,
		CacheWrite12hPrice: p.CacheWrite12hPrice,
		CacheReadPrice:     p.CacheReadPrice,
		Intervals:          toUserPricingIntervals(p.Intervals),
	}
}

// toModelPlazaDetailDTO 将 service 层单模型详情映射为白名单 DTO（不含任何分组数字 ID）。
func toModelPlazaDetailDTO(d *service.PlazaModelDetail) modelPlazaDetailResponse {
	comparisons := make([]modelPlazaGroupComparison, 0, len(d.GroupComparisons))
	for i := range d.GroupComparisons {
		cmp := &d.GroupComparisons[i]
		comparisons = append(comparisons, modelPlazaGroupComparison{
			GroupName:        cmp.GroupName,
			Platform:         cmp.Platform,
			SubscriptionType: cmp.SubscriptionType,
			IsExclusive:      cmp.IsExclusive,
			EffectiveRate:    cmp.EffectiveRate,
			Pricing:          toUserPricing(cmp.Pricing),
			OfficialPricing:  toModelPlazaOfficialPricing(cmp.OfficialPricing),
			Metrics:          cmp.Metrics,
		})
	}
	heartbeat := d.Heartbeat72
	if heartbeat == nil {
		heartbeat = []service.PlazaHeartbeatSlot{}
	}
	trend := d.Trend24h
	if trend == nil {
		trend = []service.PlazaDetailTrendPoint{}
	}
	meta := d.Metadata
	if meta.Capabilities == nil {
		meta.Capabilities = []string{}
	}
	return modelPlazaDetailResponse{
		Model:            d.Model,
		Platform:         d.Platform,
		GroupName:        d.GroupName,
		Metadata:         meta,
		Pricing:          toUserPricing(d.Pricing),
		OfficialPricing:  toModelPlazaOfficialPricing(d.OfficialPricing),
		LongContextBasis: string(d.LongContextBasis),
		TimePricing:      toModelPlazaTimePricing(d.TimePricing),
		Note:             d.Note,
		BareAliasForHigh: d.BareAliasForHigh,
		IsAuthorized:     d.IsAuthorized,
		ShowcaseBadge:    d.ShowcaseBadge,
		Windows:          d.Windows,
		Heartbeat72:      heartbeat,
		Trend24h:         trend,
		GroupComparisons: comparisons,
		Protection:       d.Protection,
		UpdatedAt:        d.UpdatedAt,
	}
}
