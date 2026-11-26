package service

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// PlazaMinimumSample 复用 ChannelMonitorV2 默认最低样本门槛（50 次），用于缓存/倍率/TTFT/TPS/一致性。
	PlazaMinimumSample int64 = 50
	// PlazaModelSLAMinimumSample 单模型 24h 真实流量独立展示 SLA 的最低样本门槛（20 次）；
	// 低于该门槛时自动回退展示同分组服务池可用性（sla_source = "group_pool"）。
	PlazaModelSLAMinimumSample int64 = 20
	// PlazaConsistencyVerifiedThreshold 原模型一致率达标阈值（>= 99.5% 或 0 不一致）。
	PlazaConsistencyVerifiedThreshold = 0.995
	// PlazaHealthySLAThreshold 健康模型 24h 可用率判定阈值（与前端 < 90% 红线对齐）。
	PlazaHealthySLAThreshold = 0.90
	// PlazaHeartbeatNormalMaxErrorRate 模型广场单格心跳绿灯最大错误率（<= 3%，即成功率 >= 97%）。
	PlazaHeartbeatNormalMaxErrorRate = 0.03
	// PlazaHeartbeatPartialMaxErrorRate 模型广场单格心跳橙黄最大错误率（3% < errRate <= 10%，即成功率 90% ~ < 97%；> 10% 为红）。
	PlazaHeartbeatPartialMaxErrorRate = 0.10
	// PlazaHeartbeatSlotMinCriticalSample 模型广场单格心跳标红的最低样本门槛（总请求 < 3 时出错最多标 partial_failure）。
	PlazaHeartbeatSlotMinCriticalSample int64 = 3
	// PlazaConsistencyMethodPassiveAudit 一致性核验方法标识。
	PlazaConsistencyMethodPassiveAudit = "passive_response_model_audit"
	// PlazaCapabilityProbeReserved 能力探针预留状态标识。
	PlazaCapabilityProbeReserved = "reserved"

	plazaListSnapshotTTL    = 90 * time.Second
	plazaDetailSnapshotTTL  = 60 * time.Second
	plazaErrorCacheTTL      = 10 * time.Second
	plazaQueryTimeout       = 10 * time.Second
	plazaDetailCacheMaxSize = 512
)

// PlazaMetricSampleState 单项指标样本状态。
type PlazaMetricSampleState string

const (
	PlazaMetricStateValid     PlazaMetricSampleState = "valid"
	PlazaMetricStateLowSample PlazaMetricSampleState = "low_sample"
	PlazaMetricStateNoData    PlazaMetricSampleState = "no_data"
)

// PlazaOutputTPSState 输出速度状态（非 OpenAI 平台一期固定返回 unsupported）。
type PlazaOutputTPSState string

const (
	PlazaOutputTPSValid       PlazaOutputTPSState = "valid"
	PlazaOutputTPSLowSample   PlazaOutputTPSState = "low_sample"
	PlazaOutputTPSNoData      PlazaOutputTPSState = "no_data"
	PlazaOutputTPSUnsupported PlazaOutputTPSState = "unsupported"
)

// PlazaHeartbeatSlotState 心跳槽状态。
type PlazaHeartbeatSlotState string

const (
	PlazaHeartbeatNormal         PlazaHeartbeatSlotState = "normal"
	PlazaHeartbeatPartialFailure PlazaHeartbeatSlotState = "partial_failure"
	PlazaHeartbeatManyFailures   PlazaHeartbeatSlotState = "many_failures"
	PlazaHeartbeatLowSample      PlazaHeartbeatSlotState = "low_sample"
	PlazaHeartbeatNoData         PlazaHeartbeatSlotState = "no_data"
	PlazaHeartbeatIdle           PlazaHeartbeatSlotState = "idle"
)

// PlazaSLASource 模型 SLA 数据来源：单模型真实流量、同分组服务池回退、或无数据。
type PlazaSLASource string

const (
	PlazaSLASourceModel     PlazaSLASource = "model"
	PlazaSLASourceGroupPool PlazaSLASource = "group_pool"
	PlazaSLASourceNone      PlazaSLASource = "none"
)

// PlazaGroupPoolStatus 分组级服务池近 24h 可用性状态与心跳。
// 仅汇总已上架模型白名单内的真实流量与 availability_probe 探针流量，剔除未上架模型 4xx 噪点。
type PlazaGroupPoolStatus struct {
	SLA24h      *float64               `json:"sla_24h"`
	SLAState    PlazaMetricSampleState `json:"sla_state"`
	Heartbeat12 []PlazaHeartbeatSlot   `json:"heartbeat_12"`
	LastProbeAt *string                `json:"last_probe_at,omitempty"`
}

// PlazaConsistencyState 原模型一致核验状态。
type PlazaConsistencyState string

const (
	PlazaConsistencyVerified         PlazaConsistencyState = "verified"
	PlazaConsistencyMismatchDetected PlazaConsistencyState = "mismatch_detected"
	PlazaConsistencyLowSample        PlazaConsistencyState = "low_sample"
	PlazaConsistencyNoData           PlazaConsistencyState = "no_data"
	PlazaConsistencyUnsupported      PlazaConsistencyState = "unsupported"
)

// PlazaSiteSummary 第 1 层：全站实力招牌条脱敏摘要（严禁包含绝对请求量与 Token 量）。
type PlazaSiteSummary struct {
	SLA30d            *float64 `json:"sla_30d"`
	SLA7d             *float64 `json:"sla_7d"`
	SLA24h            *float64 `json:"sla_24h"`
	TTFTP50Ms         *int64   `json:"ttft_p50_ms"`
	CacheReadRatio    *float64 `json:"cache_read_ratio"`
	ConsistencyRate   *float64 `json:"consistency_rate"`
	ActiveModelCount  int      `json:"active_model_count"`
	HealthyModelCount int      `json:"healthy_model_count"`
	UpdatedAt         *string  `json:"updated_at"`
	AggregationReady  bool     `json:"aggregation_ready"`
}

// PlazaHeartbeatSlot 单格心跳槽数据。
type PlazaHeartbeatSlot struct {
	Start       string                  `json:"start"`
	End         string                  `json:"end"`
	SuccessRate *float64                `json:"success_rate"`
	State       PlazaHeartbeatSlotState `json:"state"`
}

// PlazaModelMetrics 模型近 24h 脱敏实测指标。
type PlazaModelMetrics struct {
	SLA24h                   *float64               `json:"sla_24h"`
	SLAState                 PlazaMetricSampleState `json:"sla_state"`
	SLASource                PlazaSLASource         `json:"sla_source,omitempty"`
	Heartbeat12              []PlazaHeartbeatSlot   `json:"heartbeat_12"`
	TTFTP50Ms                *int64                 `json:"ttft_p50_ms"`
	TTFTP90Ms                *int64                 `json:"ttft_p90_ms"`
	TTFTState                PlazaMetricSampleState `json:"ttft_state"`
	OutputTPSP50             *float64               `json:"output_tps_p50"`
	OutputTPSState           PlazaOutputTPSState    `json:"output_tps_state"`
	CacheReadRatio           *float64               `json:"cache_read_ratio"`
	CacheState               PlazaMetricSampleState `json:"cache_state"`
	ReferenceMultiplier      *float64               `json:"reference_multiplier"`
	ReferenceMultiplierState PlazaMetricSampleState `json:"reference_multiplier_state"`
	ConsistencyState         PlazaConsistencyState  `json:"consistency_state"`
	ConsistencyRate          *float64               `json:"consistency_rate"`
	AutoPauseProtected       bool                   `json:"auto_pause_protected"`
	Capability               *struct{}              `json:"capability"`
}

// PlazaDetailWindow 详情页单时间窗指标汇总（24h / 7d / 30d）。
type PlazaDetailWindow struct {
	SLA                      *float64               `json:"sla"`
	SLAState                 PlazaMetricSampleState `json:"sla_state"`
	SLASource                PlazaSLASource         `json:"sla_source,omitempty"`
	TTFTP50Ms                *int64                 `json:"ttft_p50_ms"`
	TTFTP90Ms                *int64                 `json:"ttft_p90_ms"`
	TTFTState                PlazaMetricSampleState `json:"ttft_state"`
	OutputTPSP50             *float64               `json:"output_tps_p50"`
	OutputTPSState           PlazaOutputTPSState    `json:"output_tps_state"`
	CacheReadRatio           *float64               `json:"cache_read_ratio"`
	CacheState               PlazaMetricSampleState `json:"cache_state"`
	ReferenceMultiplier      *float64               `json:"reference_multiplier"`
	ReferenceMultiplierState PlazaMetricSampleState `json:"reference_multiplier_state"`
	ConsistencyRate          *float64               `json:"consistency_rate"`
	ConsistencyState         PlazaConsistencyState  `json:"consistency_state"`
}

// PlazaDetailWindows 详情页三档时间窗。
type PlazaDetailWindows struct {
	H24 PlazaDetailWindow `json:"h24"`
	D7  PlazaDetailWindow `json:"d7"`
	D30 PlazaDetailWindow `json:"d30"`
}

// PlazaDetailTrendPoint 详情页近 24h 小时级趋势点。
type PlazaDetailTrendPoint struct {
	BucketStart    string   `json:"bucket_start"`
	TTFTP50Ms      *int64   `json:"ttft_p50_ms"`
	CacheReadRatio *float64 `json:"cache_read_ratio"`
	SuccessRate    *float64 `json:"success_rate"`
}

// PlazaDetailProtection 详情页一致性核验与保护说明。
type PlazaDetailProtection struct {
	ConsistencyMethod     string `json:"consistency_method"`
	AutoPauseEnabled      bool   `json:"auto_pause_enabled"`
	CapabilityProbeStatus string `json:"capability_probe_status"`
}

// PlazaRawBucketCounts 预聚合表读出的内部原始计数，严禁直接下发到对外接口。
type PlazaRawBucketCounts struct {
	SuccessRequests       int64
	ErrorRequests         int64
	InputTokens           int64
	OutputTokens          int64
	CacheCreationTokens   int64
	CacheReadTokens       int64
	ModelObservedRequests int64
	ModelMismatchRequests int64
}

// Add 将另一组原始计数累加到当前聚合中。
func (c *PlazaRawBucketCounts) Add(other PlazaRawBucketCounts) {
	c.SuccessRequests += other.SuccessRequests
	c.ErrorRequests += other.ErrorRequests
	c.InputTokens += other.InputTokens
	c.OutputTokens += other.OutputTokens
	c.CacheCreationTokens += other.CacheCreationTokens
	c.CacheReadTokens += other.CacheReadTokens
	c.ModelObservedRequests += other.ModelObservedRequests
	c.ModelMismatchRequests += other.ModelMismatchRequests
}

// PlazaModelKey 内部按 (platform, group_id, model) 索引的聚合键。
type PlazaModelKey struct {
	Platform string
	GroupID  int64
	Model    string
}

// NewPlazaModelKey 构造规范化后的内部索引键。
func NewPlazaModelKey(platform string, groupID int64, model string) PlazaModelKey {
	return PlazaModelKey{
		Platform: strings.ToLower(strings.TrimSpace(platform)),
		GroupID:  groupID,
		Model:    strings.TrimSpace(model),
	}
}

// PlazaListModelRaw 列表快照中单个 (platform, group_id, model) 的原始 24h 数据与 30d 一致性汇总。
type PlazaListModelRaw struct {
	Total24h               PlazaRawBucketCounts
	Slots12                [12]PlazaRawBucketCounts
	TTFTHist               map[int64]int64
	TPSCounts              []int64
	Consistency30dObserved int64
	Consistency30dMismatch int64
}

// PlazaProbeModelRaw 近 24h 单模型 availability_probe 探针原始计数（与真实流量隔离）。
type PlazaProbeModelRaw struct {
	Total24h    PlazaRawBucketCounts
	Slots12     [12]PlazaRawBucketCounts
	LastProbeAt *time.Time
}

// PlazaGroupProbeRaw 近 24h 分组级 availability_probe 探针原始计数与最近探测时间。
type PlazaGroupProbeRaw struct {
	Total24h    PlazaRawBucketCounts
	Slots12     [12]PlazaRawBucketCounts
	LastProbeAt *time.Time
}

// PlazaListMetricsRawSnapshot 模型广场列表快照的内部原始聚合数据。
type PlazaListMetricsRawSnapshot struct {
	AggregationReady bool
	UpdatedAt        *time.Time
	Window24hStart   time.Time
	Window24hEnd     time.Time
	Group30d         map[int64]PlazaRawBucketCounts
	Group7d          map[int64]PlazaRawBucketCounts
	Models           map[PlazaModelKey]*PlazaListModelRaw
	ProbeModels      map[PlazaModelKey]*PlazaProbeModelRaw
	ProbeGroups      map[int64]*PlazaGroupProbeRaw
}

// PlazaDetailWindowRaw 详情页单个时间窗（24h / 7d / 30d）的原始数据。
type PlazaDetailWindowRaw struct {
	Counts    PlazaRawBucketCounts
	TTFTHist  map[int64]int64
	TPSCounts []int64
}

// PlazaDetailHourRaw 详情页 24h 小时级趋势点的原始数据。
type PlazaDetailHourRaw struct {
	BucketStart time.Time
	Counts      PlazaRawBucketCounts
	TTFTHist    map[int64]int64
}

// PlazaModelDetailRawSnapshot 单模型详情的内部原始聚合数据。
type PlazaModelDetailRawSnapshot struct {
	AggregationReady    bool
	UpdatedAt           *time.Time
	H24                 PlazaDetailWindowRaw
	D7                  PlazaDetailWindowRaw
	D30                 PlazaDetailWindowRaw
	HeartbeatStart      time.Time
	Slots72             [72]PlazaRawBucketCounts
	GroupSlots72ByModel map[PlazaModelKey]*[72]PlazaRawBucketCounts
	ModelProbe24h       PlazaRawBucketCounts
	ModelProbeSlots72   [72]PlazaRawBucketCounts
	GroupProbe24h       PlazaRawBucketCounts
	GroupProbeSlots72   [72]PlazaRawBucketCounts
	LastProbeAt         *time.Time
	Trend24h            []PlazaDetailHourRaw
}

// ModelPlazaMetricsRepository 模型广场专用只读质量聚合仓库。
// 独立于 ChannelMonitorV2Repository，避免修改既有监控接口与测试 Stub。
type ModelPlazaMetricsRepository interface {
	LoadPlazaListSnapshot(ctx context.Context, now time.Time) (*PlazaListMetricsRawSnapshot, error)
	LoadPlazaModelDetail(ctx context.Context, platform string, groupID int64, model string, now time.Time) (*PlazaModelDetailRawSnapshot, error)
}

type plazaListCacheEntry struct {
	snapshot  *PlazaListMetricsRawSnapshot
	expiresAt time.Time
}

type plazaDetailCacheEntry struct {
	snapshot  *PlazaModelDetailRawSnapshot
	expiresAt time.Time
}

// ModelPlazaMetricsCache 对模型广场脱敏实测指标做内存快照缓存与 singleflight 防击穿保护。
type ModelPlazaMetricsCache struct {
	repo      ModelPlazaMetricsRepository
	listTTL   time.Duration
	detailTTL time.Duration
	nowFn     func() time.Time

	mu          sync.RWMutex
	listEntry   *plazaListCacheEntry
	detailCache map[string]plazaDetailCacheEntry

	listSF   singleflight.Group
	detailSF singleflight.Group
}

// NewModelPlazaMetricsCache 创建模型广场质量快照缓存器（列表 90s TTL，详情 60s TTL）。
func NewModelPlazaMetricsCache(repo ModelPlazaMetricsRepository) *ModelPlazaMetricsCache {
	return &ModelPlazaMetricsCache{
		repo:        repo,
		listTTL:     plazaListSnapshotTTL,
		detailTTL:   plazaDetailSnapshotTTL,
		nowFn:       func() time.Time { return time.Now().UTC() },
		detailCache: make(map[string]plazaDetailCacheEntry),
	}
}

// GetListSnapshot 获取列表全量原始快照；发生降级错误时返回 AggregationReady=false，绝不阻断价格表。
func (c *ModelPlazaMetricsCache) GetListSnapshot(ctx context.Context) *PlazaListMetricsRawSnapshot {
	if c == nil || c.repo == nil {
		return &PlazaListMetricsRawSnapshot{AggregationReady: false}
	}
	now := c.nowFn()
	c.mu.RLock()
	if c.listEntry != nil && now.Before(c.listEntry.expiresAt) {
		snap := c.listEntry.snapshot
		c.mu.RUnlock()
		return snap
	}
	c.mu.RUnlock()

	val, _, _ := c.listSF.Do("list", func() (any, error) {
		// double-check in case another caller just populated the cache
		checkNow := c.nowFn()
		c.mu.RLock()
		if c.listEntry != nil && checkNow.Before(c.listEntry.expiresAt) {
			snap := c.listEntry.snapshot
			c.mu.RUnlock()
			return snap, nil
		}
		c.mu.RUnlock()

		qCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), plazaQueryTimeout)
		defer cancel()

		snap, err := c.repo.LoadPlazaListSnapshot(qCtx, checkNow)
		ttl := c.listTTL
		if err != nil {
			slog.Warn("model_plaza_list_metrics_load_failed", "error", err)
			snap = &PlazaListMetricsRawSnapshot{AggregationReady: false}
			ttl = plazaErrorCacheTTL
		} else if snap == nil {
			snap = &PlazaListMetricsRawSnapshot{AggregationReady: false}
		}
		c.mu.Lock()
		c.listEntry = &plazaListCacheEntry{
			snapshot:  snap,
			expiresAt: c.nowFn().Add(ttl),
		}
		c.mu.Unlock()
		return snap, nil
	})
	if snap, ok := val.(*PlazaListMetricsRawSnapshot); ok && snap != nil {
		return snap
	}
	return &PlazaListMetricsRawSnapshot{AggregationReady: false}
}

// GetDetailSnapshot 获取单模型单分组详情原始快照；出错优雅降级为 AggregationReady=false。
func (c *ModelPlazaMetricsCache) GetDetailSnapshot(ctx context.Context, platform string, groupID int64, model string) *PlazaModelDetailRawSnapshot {
	if c == nil || c.repo == nil || groupID <= 0 || strings.TrimSpace(model) == "" {
		return &PlazaModelDetailRawSnapshot{AggregationReady: false}
	}
	normPlatform := strings.ToLower(strings.TrimSpace(platform))
	normModel := strings.TrimSpace(model)
	cacheKey := normPlatform + "\x00" + strconv.FormatInt(groupID, 10) + "\x00" + normModel

	now := c.nowFn()
	c.mu.RLock()
	if entry, ok := c.detailCache[cacheKey]; ok && now.Before(entry.expiresAt) {
		snap := entry.snapshot
		c.mu.RUnlock()
		return snap
	}
	c.mu.RUnlock()

	val, _, _ := c.detailSF.Do(cacheKey, func() (any, error) {
		checkNow := c.nowFn()
		c.mu.RLock()
		if entry, ok := c.detailCache[cacheKey]; ok && checkNow.Before(entry.expiresAt) {
			snap := entry.snapshot
			c.mu.RUnlock()
			return snap, nil
		}
		c.mu.RUnlock()

		qCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), plazaQueryTimeout)
		defer cancel()

		snap, err := c.repo.LoadPlazaModelDetail(qCtx, normPlatform, groupID, normModel, checkNow)
		ttl := c.detailTTL
		if err != nil {
			slog.Warn("model_plaza_detail_metrics_load_failed",
				"platform", normPlatform, "group_id", groupID, "model", normModel, "error", err)
			snap = &PlazaModelDetailRawSnapshot{AggregationReady: false}
			ttl = plazaErrorCacheTTL
		} else if snap == nil {
			snap = &PlazaModelDetailRawSnapshot{AggregationReady: false}
		}

		c.mu.Lock()
		nowStore := c.nowFn()
		if len(c.detailCache) >= plazaDetailCacheMaxSize {
			c.evictDetailCacheLocked(nowStore)
		}
		c.detailCache[cacheKey] = plazaDetailCacheEntry{
			snapshot:  snap,
			expiresAt: nowStore.Add(ttl),
		}
		c.mu.Unlock()
		return snap, nil
	})
	if snap, ok := val.(*PlazaModelDetailRawSnapshot); ok && snap != nil {
		return snap
	}
	return &PlazaModelDetailRawSnapshot{AggregationReady: false}
}

func (c *ModelPlazaMetricsCache) evictDetailCacheLocked(now time.Time) {
	for k, entry := range c.detailCache {
		if !now.Before(entry.expiresAt) {
			delete(c.detailCache, k)
		}
	}
	if len(c.detailCache) < plazaDetailCacheMaxSize {
		return
	}
	type keyExpiry struct {
		key       string
		expiresAt time.Time
	}
	items := make([]keyExpiry, 0, len(c.detailCache))
	for k, entry := range c.detailCache {
		items = append(items, keyExpiry{key: k, expiresAt: entry.expiresAt})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].expiresAt.Before(items[j].expiresAt)
	})
	removeCount := len(items) / 2
	for i := 0; i < removeCount; i++ {
		delete(c.detailCache, items[i].key)
	}
}

type plazaRefTokenPrices struct {
	input      float64
	output     float64
	cacheWrite float64
	cacheRead  float64
}

// extractPlazaRefTokenPrices 提取用于 4 项 Token 加权参考倍率计算的单价。
// 仅按量 Token 模型且具备基础输入/输出/缓存读单价时返回非 nil；无 cache_write 时回落 input 单价。
func extractPlazaRefTokenPrices(pricing *ChannelModelPricing, official *PlazaOfficialPricing) *plazaRefTokenPrices {
	if pricing != nil && pricing.BillingMode != "" && pricing.BillingMode != BillingModeToken {
		return nil
	}
	if pricing != nil && pricing.InputPrice != nil && *pricing.InputPrice > 0 &&
		pricing.OutputPrice != nil && *pricing.OutputPrice > 0 &&
		pricing.CacheReadPrice != nil && *pricing.CacheReadPrice >= 0 {
		cw := *pricing.InputPrice
		if pricing.CacheWritePrice != nil && *pricing.CacheWritePrice > 0 {
			cw = *pricing.CacheWritePrice
		}
		return &plazaRefTokenPrices{
			input:      *pricing.InputPrice,
			output:     *pricing.OutputPrice,
			cacheWrite: cw,
			cacheRead:  *pricing.CacheReadPrice,
		}
	}
	if official != nil && official.InputPrice != nil && *official.InputPrice > 0 &&
		official.OutputPrice != nil && *official.OutputPrice > 0 &&
		official.CacheReadPrice != nil && *official.CacheReadPrice >= 0 {
		cw := *official.InputPrice
		if official.CacheWritePrice != nil && *official.CacheWritePrice > 0 {
			cw = *official.CacheWritePrice
		}
		return &plazaRefTokenPrices{
			input:      *official.InputPrice,
			output:     *official.OutputPrice,
			cacheWrite: cw,
			cacheRead:  *official.CacheReadPrice,
		}
	}
	return nil
}

func isPlazaNonTextMediaModel(platform, model string, pricing *ChannelModelPricing) bool {
	if pricing != nil && (pricing.BillingMode == BillingModeImage || pricing.BillingMode == BillingModeVideo) {
		return true
	}
	return IsImageGenerationIntentForPlatform("", model, nil, platform)
}

// isPlazaConsistencySupported 判断模型是否适用模型广场原模型一致性核验。
// 仅对 OpenAI 平台下的 GPT/推理文本对话模型生效，排除非 OpenAI 平台与非文本多媒体/按次计费模型。
func isPlazaConsistencySupported(platform, model string, pricing *ChannelModelPricing) bool {
	if !strings.EqualFold(strings.TrimSpace(platform), PlatformOpenAI) {
		return false
	}
	if isPlazaNonTextMediaModel(platform, model, pricing) {
		return false
	}
	if pricing != nil && pricing.BillingMode != "" && pricing.BillingMode != BillingModeToken {
		return false
	}
	normalized := normalizePlazaMetadataModelKey(model)
	if normalized == "" || inferPlazaModelVendor(normalized, platform) != "openai" {
		return false
	}
	for _, prefix := range []string{
		"dall-e",
		"whisper",
		"tts-",
		"text-embedding",
		"text-moderation",
		"sora",
		"gpt-image",
		"chatgpt-image",
	} {
		if strings.HasPrefix(normalized, prefix) {
			return false
		}
	}
	return true
}

func plazaClampRatio(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func plazaEvalSLA(success, errs int64) (*float64, PlazaMetricSampleState) {
	total := success + errs
	if total <= 0 {
		return nil, PlazaMetricStateNoData
	}
	if total < PlazaMinimumSample {
		return nil, PlazaMetricStateLowSample
	}
	rate := plazaClampRatio(float64(success) / float64(total))
	return &rate, PlazaMetricStateValid
}

func plazaEvalHeartbeatSlot(start, end time.Time, counts PlazaRawBucketCounts) PlazaHeartbeatSlot {
	slot := PlazaHeartbeatSlot{
		Start: start.UTC().Format(time.RFC3339),
		End:   end.UTC().Format(time.RFC3339),
		State: PlazaHeartbeatIdle,
	}
	total := counts.SuccessRequests + counts.ErrorRequests
	if total <= 0 {
		return slot
	}
	errRate := plazaClampRatio(float64(counts.ErrorRequests) / float64(total))
	successRate := plazaClampRatio(float64(counts.SuccessRequests) / float64(total))
	slot.SuccessRate = &successRate
	switch {
	case errRate > PlazaHeartbeatPartialMaxErrorRate:
		if total < PlazaHeartbeatSlotMinCriticalSample {
			slot.State = PlazaHeartbeatPartialFailure
		} else {
			slot.State = PlazaHeartbeatManyFailures
		}
	case errRate > PlazaHeartbeatNormalMaxErrorRate:
		slot.State = PlazaHeartbeatPartialFailure
	default:
		slot.State = PlazaHeartbeatNormal
	}
	return slot
}

func plazaAddRequestCounts(a, b PlazaRawBucketCounts) PlazaRawBucketCounts {
	return PlazaRawBucketCounts{
		SuccessRequests: a.SuccessRequests + b.SuccessRequests,
		ErrorRequests:   a.ErrorRequests + b.ErrorRequests,
	}
}

type plazaGroupWhitelist struct {
	isComposite    bool
	groupPlatform  string
	modelNames     map[string]struct{}
	platformModels map[string]struct{}
	bareAliases    map[string]struct{}
}

func buildPlazaGroupWhitelist(g PlazaGroup) plazaGroupWhitelist {
	w := plazaGroupWhitelist{
		isComposite:    strings.EqualFold(strings.TrimSpace(g.Platform), PlatformComposite),
		groupPlatform:  strings.ToLower(strings.TrimSpace(g.Platform)),
		modelNames:     make(map[string]struct{}, len(g.Models)+len(g.BareAliases)),
		platformModels: make(map[string]struct{}, len(g.Models)),
		bareAliases:    make(map[string]struct{}, len(g.BareAliases)),
	}
	for i := range g.Models {
		name := strings.ToLower(strings.TrimSpace(g.Models[i].Name))
		if name == "" {
			continue
		}
		w.modelNames[name] = struct{}{}
		plat := strings.ToLower(strings.TrimSpace(g.Models[i].Platform))
		if plat != "" {
			w.platformModels[plat+"\x00"+name] = struct{}{}
		}
	}
	for _, alias := range g.BareAliases {
		if norm := strings.ToLower(strings.TrimSpace(alias)); norm != "" {
			w.modelNames[norm] = struct{}{}
			w.bareAliases[norm] = struct{}{}
		}
	}
	return w
}

func (w plazaGroupWhitelist) matches(platform, model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return false
	}
	if w.isComposite {
		plat := strings.ToLower(strings.TrimSpace(platform))
		if _, ok := w.platformModels[plat+"\x00"+name]; ok {
			return true
		}
		_, ok := w.bareAliases[name]
		return ok
	}
	_, ok := w.modelNames[name]
	return ok
}

// plazaBuildGroupPoolStatus 汇总分组级服务池近 24h 可用性状态与 12 格心跳。
// 仅统计该分组在广场已上架模型白名单（含 bare_aliases）内的真实流量 + 该分组 availability_probe 探针流量，
// 剔除未上架模型触发的 4xx 噪点。总请求 >= 1 即产出有效 sla_24h。
func plazaBuildGroupPoolStatus(g PlazaGroup, snap *PlazaListMetricsRawSnapshot) *PlazaGroupPoolStatus {
	if snap == nil {
		return nil
	}
	var gp *PlazaGroupProbeRaw
	if snap.ProbeGroups != nil {
		gp = snap.ProbeGroups[g.ID]
	}
	if !snap.AggregationReady && gp == nil {
		return nil
	}

	whitelist := buildPlazaGroupWhitelist(g)
	var totalCounts PlazaRawBucketCounts
	var slots12 [12]PlazaRawBucketCounts

	if snap.AggregationReady {
		for key, raw := range snap.Models {
			if raw == nil || key.GroupID != g.ID {
				continue
			}
			if !whitelist.matches(key.Platform, key.Model) {
				continue
			}
			totalCounts = plazaAddRequestCounts(totalCounts, raw.Total24h)
			for i := 0; i < 12; i++ {
				slots12[i] = plazaAddRequestCounts(slots12[i], raw.Slots12[i])
			}
		}
	}

	var lastProbeAt *string
	if gp != nil {
		totalCounts = plazaAddRequestCounts(totalCounts, gp.Total24h)
		for i := 0; i < 12; i++ {
			slots12[i] = plazaAddRequestCounts(slots12[i], gp.Slots12[i])
		}
		if gp.LastProbeAt != nil && !gp.LastProbeAt.IsZero() {
			ts := gp.LastProbeAt.UTC().Format(time.RFC3339)
			lastProbeAt = &ts
		}
	}

	var sla *float64
	slaState := PlazaMetricStateNoData
	if total := totalCounts.SuccessRequests + totalCounts.ErrorRequests; total >= 1 {
		rate := plazaClampRatio(float64(totalCounts.SuccessRequests) / float64(total))
		sla = &rate
		slaState = PlazaMetricStateValid
	}

	hb12 := make([]PlazaHeartbeatSlot, 0)
	if !snap.Window24hStart.IsZero() {
		hb12 = make([]PlazaHeartbeatSlot, 12)
		for i := 0; i < 12; i++ {
			slotStart := snap.Window24hStart.Add(time.Duration(i) * 2 * time.Hour)
			hb12[i] = plazaEvalHeartbeatSlot(slotStart, slotStart.Add(2*time.Hour), slots12[i])
		}
	}

	return &PlazaGroupPoolStatus{
		SLA24h:      sla,
		SLAState:    slaState,
		Heartbeat12: hb12,
		LastProbeAt: lastProbeAt,
	}
}

func plazaEvalModelSLAWithFallback(
	realSuccess, realErrs int64,
	aggregationReady bool,
	groupPool *PlazaGroupPoolStatus,
	fallbackProbe *PlazaRawBucketCounts,
) (*float64, PlazaMetricSampleState, PlazaSLASource) {
	realTotal := realSuccess + realErrs
	if aggregationReady && realTotal >= PlazaModelSLAMinimumSample {
		rate := plazaClampRatio(float64(realSuccess) / float64(realTotal))
		return &rate, PlazaMetricStateValid, PlazaSLASourceModel
	}
	if groupPool != nil && groupPool.SLAState == PlazaMetricStateValid && groupPool.SLA24h != nil {
		rate := *groupPool.SLA24h
		return &rate, PlazaMetricStateValid, PlazaSLASourceGroupPool
	}
	if fallbackProbe != nil {
		if total := fallbackProbe.SuccessRequests + fallbackProbe.ErrorRequests; total >= 1 {
			rate := plazaClampRatio(float64(fallbackProbe.SuccessRequests) / float64(total))
			return &rate, PlazaMetricStateValid, PlazaSLASourceGroupPool
		}
	}
	return nil, PlazaMetricStateNoData, PlazaSLASourceNone
}

func clonePlazaHeartbeatSlots(slots []PlazaHeartbeatSlot) []PlazaHeartbeatSlot {
	if len(slots) == 0 {
		return []PlazaHeartbeatSlot{}
	}
	out := make([]PlazaHeartbeatSlot, len(slots))
	copy(out, slots)
	return out
}

func plazaBuildDetailHeartbeat72(
	group PlazaGroup,
	snap *PlazaModelDetailRawSnapshot,
	slaSource PlazaSLASource,
) []PlazaHeartbeatSlot {
	if snap == nil || snap.HeartbeatStart.IsZero() {
		return []PlazaHeartbeatSlot{}
	}
	if !snap.AggregationReady &&
		(snap.GroupProbe24h.SuccessRequests+snap.GroupProbe24h.ErrorRequests) <= 0 &&
		(snap.ModelProbe24h.SuccessRequests+snap.ModelProbe24h.ErrorRequests) <= 0 {
		return []PlazaHeartbeatSlot{}
	}

	var slots [72]PlazaRawBucketCounts
	if slaSource == PlazaSLASourceGroupPool {
		whitelist := buildPlazaGroupWhitelist(group)
		if snap.AggregationReady {
			for key, modelSlots := range snap.GroupSlots72ByModel {
				if modelSlots == nil || key.GroupID != group.ID {
					continue
				}
				if !whitelist.matches(key.Platform, key.Model) {
					continue
				}
				for i := 0; i < 72; i++ {
					slots[i] = plazaAddRequestCounts(slots[i], modelSlots[i])
				}
			}
		}
		for i := 0; i < 72; i++ {
			slots[i] = plazaAddRequestCounts(slots[i], snap.GroupProbeSlots72[i])
		}
	} else if slaSource == PlazaSLASourceModel {
		if snap.AggregationReady {
			for i := 0; i < 72; i++ {
				slots[i] = snap.Slots72[i]
			}
		}
	} else {
		for i := 0; i < 72; i++ {
			if snap.AggregationReady {
				slots[i] = plazaAddRequestCounts(slots[i], snap.Slots72[i])
			}
			slots[i] = plazaAddRequestCounts(slots[i], snap.ModelProbeSlots72[i])
		}
	}

	out := make([]PlazaHeartbeatSlot, 72)
	for i := 0; i < 72; i++ {
		slotStart := snap.HeartbeatStart.Add(time.Duration(i) * 20 * time.Minute)
		out[i] = plazaEvalHeartbeatSlot(slotStart, slotStart.Add(20*time.Minute), slots[i])
	}
	return out
}

func plazaEvalCacheAndRefMultiplier(
	counts PlazaRawBucketCounts,
	nonTextMedia bool,
	prices *plazaRefTokenPrices,
) (*float64, PlazaMetricSampleState, *float64, PlazaMetricSampleState) {
	if nonTextMedia {
		return nil, PlazaMetricStateNoData, nil, PlazaMetricStateNoData
	}
	denom := counts.InputTokens + counts.CacheCreationTokens + counts.CacheReadTokens
	if counts.SuccessRequests <= 0 || denom <= 0 {
		return nil, PlazaMetricStateNoData, nil, PlazaMetricStateNoData
	}
	if counts.SuccessRequests < PlazaMinimumSample {
		if prices == nil {
			return nil, PlazaMetricStateLowSample, nil, PlazaMetricStateNoData
		}
		return nil, PlazaMetricStateLowSample, nil, PlazaMetricStateLowSample
	}
	ratio := plazaClampRatio(float64(counts.CacheReadTokens) / float64(denom))
	cacheState := PlazaMetricStateValid

	if prices == nil {
		return &ratio, cacheState, nil, PlazaMetricStateNoData
	}
	actualWeighted := float64(counts.InputTokens)*prices.input +
		float64(counts.CacheCreationTokens)*prices.cacheWrite +
		float64(counts.CacheReadTokens)*prices.cacheRead +
		float64(counts.OutputTokens)*prices.output
	baseWeighted := float64(denom)*prices.input + float64(counts.OutputTokens)*prices.output
	if baseWeighted <= 0 || math.IsNaN(actualWeighted) || math.IsInf(actualWeighted, 0) {
		return &ratio, cacheState, nil, PlazaMetricStateNoData
	}
	mult := math.Round((actualWeighted/baseWeighted)*1000) / 1000
	if mult <= 0 || math.IsNaN(mult) || math.IsInf(mult, 0) {
		return &ratio, cacheState, nil, PlazaMetricStateNoData
	}
	return &ratio, cacheState, &mult, PlazaMetricStateValid
}

func plazaHistTotal(hist map[int64]int64) int64 {
	var total int64
	for _, count := range hist {
		if count > 0 {
			total += count
		}
	}
	return total
}

func plazaHistPercentile(hist map[int64]int64, p float64) *int64 {
	var total int64
	keys := make([]int64, 0, len(hist))
	for bound, count := range hist {
		if count <= 0 {
			continue
		}
		keys = append(keys, bound)
		total += count
	}
	if total <= 0 {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	target := int64(float64(total)*p + 0.999999)
	if target < 1 {
		target = 1
	}
	var cumulative int64
	for _, bound := range keys {
		cumulative += hist[bound]
		if cumulative >= target {
			v := bound
			return &v
		}
	}
	v := keys[len(keys)-1]
	return &v
}

func plazaEvalTTFT(hist map[int64]int64, nonTextMedia bool) (*int64, *int64, PlazaMetricSampleState) {
	if nonTextMedia {
		return nil, nil, PlazaMetricStateNoData
	}
	total := plazaHistTotal(hist)
	if total <= 0 {
		return nil, nil, PlazaMetricStateNoData
	}
	if total < PlazaMinimumSample {
		return nil, nil, PlazaMetricStateLowSample
	}
	p50 := plazaHistPercentile(hist, 0.50)
	p90 := plazaHistPercentile(hist, 0.90)
	if p50 == nil {
		return nil, nil, PlazaMetricStateNoData
	}
	return p50, p90, PlazaMetricStateValid
}

func plazaEvalOutputTPS(platform string, counts []int64, nonTextMedia bool) (*float64, PlazaOutputTPSState) {
	if !strings.EqualFold(strings.TrimSpace(platform), PlatformOpenAI) {
		return nil, PlazaOutputTPSUnsupported
	}
	if nonTextMedia {
		return nil, PlazaOutputTPSNoData
	}
	expectedLen := len(MonitorTPSBoundsV1()) + 1
	if len(counts) != expectedLen {
		return nil, PlazaOutputTPSNoData
	}
	var total int64
	for _, n := range counts {
		if n > 0 {
			total += n
		}
	}
	if total <= 0 {
		return nil, PlazaOutputTPSNoData
	}
	if total < PlazaMinimumSample {
		return nil, PlazaOutputTPSLowSample
	}
	milli, reason, err := MonitorTPSPercentileV1(counts, 50)
	if err != nil || reason != "" || milli == nil || *milli <= 0 {
		return nil, PlazaOutputTPSNoData
	}
	tps := math.Round(float64(*milli)/100.0) / 10.0
	if tps <= 0 {
		return nil, PlazaOutputTPSNoData
	}
	return &tps, PlazaOutputTPSValid
}

func plazaEvalConsistency(observed, mismatch int64) (*float64, PlazaConsistencyState) {
	if observed <= 0 {
		return nil, PlazaConsistencyNoData
	}
	if observed < PlazaMinimumSample {
		return nil, PlazaConsistencyLowSample
	}
	if mismatch < 0 {
		mismatch = 0
	}
	if mismatch > observed {
		mismatch = observed
	}
	rate := plazaClampRatio(float64(observed-mismatch) / float64(observed))
	if mismatch == 0 || rate >= PlazaConsistencyVerifiedThreshold {
		return &rate, PlazaConsistencyVerified
	}
	return &rate, PlazaConsistencyMismatchDetected
}

func plazaClampMismatch(observed, mismatch int64) int64 {
	if observed <= 0 || mismatch <= 0 {
		return 0
	}
	if mismatch > observed {
		return observed
	}
	return mismatch
}

func plazaEvalModelConsistencyWithFallback(
	real24hObs, real24hMismatch int64,
	obs30d, mismatch30d int64,
	probe24hObs, probe24hMismatch int64,
) (*float64, PlazaConsistencyState) {
	if real24hObs < 0 {
		real24hObs = 0
	}
	real24hMismatch = plazaClampMismatch(real24hObs, real24hMismatch)
	if obs30d < 0 {
		obs30d = 0
	}
	mismatch30d = plazaClampMismatch(obs30d, mismatch30d)
	if probe24hObs < 0 {
		probe24hObs = 0
	}
	probe24hMismatch = plazaClampMismatch(probe24hObs, probe24hMismatch)

	if real24hObs >= PlazaMinimumSample {
		return plazaEvalConsistency(real24hObs, real24hMismatch)
	}
	if real24hObs >= 5 && real24hMismatch > 0 {
		rate24h := plazaClampRatio(float64(real24hObs-real24hMismatch) / float64(real24hObs))
		if rate24h < PlazaConsistencyVerifiedThreshold {
			return &rate24h, PlazaConsistencyMismatchDetected
		}
	}

	baseObs := obs30d
	if real24hObs > baseObs {
		baseObs = real24hObs
	}
	baseMismatch := mismatch30d
	if real24hMismatch > baseMismatch {
		baseMismatch = real24hMismatch
	}
	baseMismatch = plazaClampMismatch(baseObs, baseMismatch)

	combinedObs := baseObs + probe24hObs
	combinedMismatch := plazaClampMismatch(combinedObs, baseMismatch+probe24hMismatch)
	if combinedObs <= 0 {
		return nil, PlazaConsistencyNoData
	}
	if combinedObs >= 5 && combinedMismatch == 0 {
		rate := 1.0
		return &rate, PlazaConsistencyVerified
	}
	if combinedObs >= 20 {
		combinedRate := plazaClampRatio(float64(combinedObs-combinedMismatch) / float64(combinedObs))
		if combinedRate >= PlazaConsistencyVerifiedThreshold {
			return &combinedRate, PlazaConsistencyVerified
		}
		return &combinedRate, PlazaConsistencyMismatchDetected
	}
	return nil, PlazaConsistencyLowSample
}

func plazaAutoPauseProtected(platform, model string) bool {
	return strings.EqualFold(strings.TrimSpace(platform), PlatformOpenAI) &&
		strings.EqualFold(strings.TrimSpace(model), "gpt-6-astra")
}

func plazaBuildModelMetrics(
	platform string,
	model string,
	pricing *ChannelModelPricing,
	official *PlazaOfficialPricing,
	raw *PlazaListModelRaw,
	modelProbeRaw *PlazaProbeModelRaw,
	groupPool *PlazaGroupPoolStatus,
	windowStart time.Time,
	aggregationReady bool,
) *PlazaModelMetrics {
	nonTextMedia := isPlazaNonTextMediaModel(platform, model, pricing)
	prices := extractPlazaRefTokenPrices(pricing, official)
	autoPause := plazaAutoPauseProtected(platform, model)

	var realCounts PlazaRawBucketCounts
	var ttftHist map[int64]int64
	var tpsCounts []int64
	var obs30d, mismatch30d int64
	if aggregationReady && raw != nil {
		realCounts = raw.Total24h
		ttftHist = raw.TTFTHist
		tpsCounts = raw.TPSCounts
		obs30d = raw.Consistency30dObserved
		mismatch30d = raw.Consistency30dMismatch
	}

	var modelProbeCounts *PlazaRawBucketCounts
	var probeObs, probeMismatch int64
	if modelProbeRaw != nil {
		c := modelProbeRaw.Total24h
		modelProbeCounts = &c
		probeObs = modelProbeRaw.Total24h.ModelObservedRequests
		probeMismatch = modelProbeRaw.Total24h.ModelMismatchRequests
	}

	sla, slaState, slaSource := plazaEvalModelSLAWithFallback(
		realCounts.SuccessRequests,
		realCounts.ErrorRequests,
		aggregationReady,
		groupPool,
		modelProbeCounts,
	)

	// 严格指标隔离：TTFT / TPS / Coding 缓存 / 参考倍率 100% 仅消费真实流量并维持 50 样本门槛。
	ttftP50, ttftP90, ttftState := (*int64)(nil), (*int64)(nil), PlazaMetricStateNoData
	tpsP50, tpsState := (*float64)(nil), PlazaOutputTPSNoData
	if !strings.EqualFold(strings.TrimSpace(platform), PlatformOpenAI) {
		tpsState = PlazaOutputTPSUnsupported
	}
	cacheRatio, cacheState := (*float64)(nil), PlazaMetricStateNoData
	refMult, refMultState := (*float64)(nil), PlazaMetricStateNoData
	if aggregationReady && raw != nil {
		ttftP50, ttftP90, ttftState = plazaEvalTTFT(ttftHist, nonTextMedia)
		tpsP50, tpsState = plazaEvalOutputTPS(platform, tpsCounts, nonTextMedia)
		cacheRatio, cacheState, refMult, refMultState = plazaEvalCacheAndRefMultiplier(realCounts, nonTextMedia, prices)
	}
	var consRate *float64
	consState := PlazaConsistencyUnsupported
	if isPlazaConsistencySupported(platform, model, pricing) {
		consRate, consState = plazaEvalModelConsistencyWithFallback(
			realCounts.ModelObservedRequests,
			realCounts.ModelMismatchRequests,
			obs30d,
			mismatch30d,
			probeObs,
			probeMismatch,
		)
	}

	hb := make([]PlazaHeartbeatSlot, 0)
	if slaSource == PlazaSLASourceGroupPool && groupPool != nil && len(groupPool.Heartbeat12) > 0 {
		hb = clonePlazaHeartbeatSlots(groupPool.Heartbeat12)
	} else if slaSource == PlazaSLASourceModel {
		if !windowStart.IsZero() && aggregationReady {
			hb = make([]PlazaHeartbeatSlot, 12)
			for i := 0; i < 12; i++ {
				var slotCounts PlazaRawBucketCounts
				if raw != nil {
					slotCounts = raw.Slots12[i]
				}
				slotStart := windowStart.Add(time.Duration(i) * 2 * time.Hour)
				hb[i] = plazaEvalHeartbeatSlot(slotStart, slotStart.Add(2*time.Hour), slotCounts)
			}
		}
	} else if !windowStart.IsZero() && (aggregationReady || modelProbeRaw != nil) {
		hb = make([]PlazaHeartbeatSlot, 12)
		for i := 0; i < 12; i++ {
			var slotCounts PlazaRawBucketCounts
			if aggregationReady && raw != nil {
				slotCounts = plazaAddRequestCounts(slotCounts, raw.Slots12[i])
			}
			if modelProbeRaw != nil {
				slotCounts = plazaAddRequestCounts(slotCounts, modelProbeRaw.Slots12[i])
			}
			slotStart := windowStart.Add(time.Duration(i) * 2 * time.Hour)
			hb[i] = plazaEvalHeartbeatSlot(slotStart, slotStart.Add(2*time.Hour), slotCounts)
		}
	}

	return &PlazaModelMetrics{
		SLA24h:                   sla,
		SLAState:                 slaState,
		SLASource:                slaSource,
		Heartbeat12:              hb,
		TTFTP50Ms:                ttftP50,
		TTFTP90Ms:                ttftP90,
		TTFTState:                ttftState,
		OutputTPSP50:             tpsP50,
		OutputTPSState:           tpsState,
		CacheReadRatio:           cacheRatio,
		CacheState:               cacheState,
		ReferenceMultiplier:      refMult,
		ReferenceMultiplierState: refMultState,
		ConsistencyState:         consState,
		ConsistencyRate:          consRate,
		AutoPauseProtected:       autoPause,
		Capability:               nil,
	}
}

func plazaBuildDetailWindow(
	platform string,
	model string,
	pricing *ChannelModelPricing,
	official *PlazaOfficialPricing,
	raw PlazaDetailWindowRaw,
	groupPool *PlazaGroupPoolStatus,
	fallbackProbe *PlazaRawBucketCounts,
	aggregationReady bool,
) PlazaDetailWindow {
	nonTextMedia := isPlazaNonTextMediaModel(platform, model, pricing)
	consistencySupported := isPlazaConsistencySupported(platform, model, pricing)
	prices := extractPlazaRefTokenPrices(pricing, official)

	sla, slaState, slaSource := plazaEvalModelSLAWithFallback(
		raw.Counts.SuccessRequests,
		raw.Counts.ErrorRequests,
		aggregationReady,
		groupPool,
		fallbackProbe,
	)

	if !aggregationReady {
		tpsState := PlazaOutputTPSNoData
		if !strings.EqualFold(strings.TrimSpace(platform), PlatformOpenAI) {
			tpsState = PlazaOutputTPSUnsupported
		}
		consState := PlazaConsistencyNoData
		if !consistencySupported {
			consState = PlazaConsistencyUnsupported
		}
		return PlazaDetailWindow{
			SLA:                      sla,
			SLAState:                 slaState,
			SLASource:                slaSource,
			TTFTState:                PlazaMetricStateNoData,
			OutputTPSState:           tpsState,
			CacheState:               PlazaMetricStateNoData,
			ReferenceMultiplierState: PlazaMetricStateNoData,
			ConsistencyState:         consState,
		}
	}

	ttftP50, ttftP90, ttftState := plazaEvalTTFT(raw.TTFTHist, nonTextMedia)
	tpsP50, tpsState := plazaEvalOutputTPS(platform, raw.TPSCounts, nonTextMedia)
	cacheRatio, cacheState, refMult, refMultState := plazaEvalCacheAndRefMultiplier(raw.Counts, nonTextMedia, prices)
	var consRate *float64
	consState := PlazaConsistencyUnsupported
	if consistencySupported {
		consRate, consState = plazaEvalConsistency(raw.Counts.ModelObservedRequests, raw.Counts.ModelMismatchRequests)
	}

	return PlazaDetailWindow{
		SLA:                      sla,
		SLAState:                 slaState,
		SLASource:                slaSource,
		TTFTP50Ms:                ttftP50,
		TTFTP90Ms:                ttftP90,
		TTFTState:                ttftState,
		OutputTPSP50:             tpsP50,
		OutputTPSState:           tpsState,
		CacheReadRatio:           cacheRatio,
		CacheState:               cacheState,
		ReferenceMultiplier:      refMult,
		ReferenceMultiplierState: refMultState,
		ConsistencyRate:          consRate,
		ConsistencyState:         consState,
	}
}

func plazaBuildTrend24h(
	platform string,
	model string,
	pricing *ChannelModelPricing,
	rawHours []PlazaDetailHourRaw,
	aggregationReady bool,
) []PlazaDetailTrendPoint {
	if !aggregationReady || len(rawHours) == 0 {
		return []PlazaDetailTrendPoint{}
	}
	nonTextMedia := isPlazaNonTextMediaModel(platform, model, pricing)
	points := make([]PlazaDetailTrendPoint, 0, len(rawHours))
	hasAnyData := false
	for _, h := range rawHours {
		pt := PlazaDetailTrendPoint{
			BucketStart: h.BucketStart.UTC().Format(time.RFC3339),
		}
		total := h.Counts.SuccessRequests + h.Counts.ErrorRequests
		if total > 0 {
			rate := plazaClampRatio(float64(h.Counts.SuccessRequests) / float64(total))
			pt.SuccessRate = &rate
			hasAnyData = true
		}
		if !nonTextMedia {
			denom := h.Counts.InputTokens + h.Counts.CacheCreationTokens + h.Counts.CacheReadTokens
			if h.Counts.SuccessRequests > 0 && denom > 0 {
				ratio := plazaClampRatio(float64(h.Counts.CacheReadTokens) / float64(denom))
				pt.CacheReadRatio = &ratio
				hasAnyData = true
			}
			if p50 := plazaHistPercentile(h.TTFTHist, 0.50); p50 != nil {
				pt.TTFTP50Ms = p50
				hasAnyData = true
			}
		}
		points = append(points, pt)
	}
	if !hasAnyData {
		return []PlazaDetailTrendPoint{}
	}
	return points
}

func plazaBuildSiteSummary(visible []PlazaGroup, snap *PlazaListMetricsRawSnapshot) *PlazaSiteSummary {
	type distinctModelKey struct {
		platform string
		model    string
	}
	visibleGroupSet := make(map[int64]plazaGroupWhitelist, len(visible))
	distinctModels := make(map[distinctModelKey]struct{})
	listedPricing := make(map[distinctModelKey]*ChannelModelPricing)
	groupModelPricing := make(map[PlazaModelKey]*ChannelModelPricing)
	for i := range visible {
		g := &visible[i]
		visibleGroupSet[g.ID] = buildPlazaGroupWhitelist(*g)
		for j := range g.Models {
			m := &g.Models[j]
			dmKey := distinctModelKey{
				platform: strings.ToLower(strings.TrimSpace(m.Platform)),
				model:    strings.TrimSpace(m.Name),
			}
			distinctModels[dmKey] = struct{}{}
			if m.Pricing != nil {
				groupModelPricing[NewPlazaModelKey(m.Platform, g.ID, m.Name)] = m.Pricing
				if listedPricing[dmKey] == nil {
					listedPricing[dmKey] = m.Pricing
				}
			}
		}
	}

	summary := &PlazaSiteSummary{
		ActiveModelCount: len(distinctModels),
		AggregationReady: snap != nil && snap.AggregationReady,
	}
	if snap == nil || !snap.AggregationReady {
		return summary
	}
	if snap.UpdatedAt != nil && !snap.UpdatedAt.IsZero() {
		ts := snap.UpdatedAt.UTC().Format(time.RFC3339)
		summary.UpdatedAt = &ts
	}

	var total30d, total7d, total24h PlazaRawBucketCounts
	var cache24hSuccess, cache24hRead, cache24hDenom int64
	var consistency24hObserved, consistency24hMismatch int64
	mergedTTFT := make(map[int64]int64, 16)

	for gid := range visibleGroupSet {
		if c30, ok := snap.Group30d[gid]; ok {
			total30d.Add(c30)
		}
		if c7, ok := snap.Group7d[gid]; ok {
			total7d.Add(c7)
		}
	}

	model24hCounts := make(map[distinctModelKey][2]int64, len(distinctModels))
	for key, raw := range snap.Models {
		whitelist, ok := visibleGroupSet[key.GroupID]
		if !ok || raw == nil {
			continue
		}
		if !whitelist.matches(key.Platform, key.Model) {
			continue
		}
		total24h.Add(raw.Total24h)

		isImg := IsImageGenerationIntentForPlatform("", key.Model, nil, key.Platform)
		if !isImg {
			denom := raw.Total24h.InputTokens + raw.Total24h.CacheCreationTokens + raw.Total24h.CacheReadTokens
			if raw.Total24h.SuccessRequests > 0 && denom > 0 {
				cache24hSuccess += raw.Total24h.SuccessRequests
				cache24hRead += raw.Total24h.CacheReadTokens
				cache24hDenom += denom
			}
			for bound, cnt := range raw.TTFTHist {
				if cnt > 0 {
					mergedTTFT[bound] += cnt
				}
			}
		}

		dmKey := distinctModelKey{platform: key.Platform, model: key.Model}
		pricing := groupModelPricing[key]
		if pricing == nil {
			pricing = listedPricing[dmKey]
		}
		if isPlazaConsistencySupported(key.Platform, key.Model, pricing) {
			consistency24hObserved += raw.Total24h.ModelObservedRequests
			consistency24hMismatch += raw.Total24h.ModelMismatchRequests
		}
		if _, listed := distinctModels[dmKey]; listed {
			pair := model24hCounts[dmKey]
			pair[0] += raw.Total24h.SuccessRequests
			pair[1] += raw.Total24h.ErrorRequests
			model24hCounts[dmKey] = pair
		}
	}

	summary.SLA30d, _ = plazaEvalSLA(total30d.SuccessRequests, total30d.ErrorRequests)
	summary.SLA7d, _ = plazaEvalSLA(total7d.SuccessRequests, total7d.ErrorRequests)
	summary.SLA24h, _ = plazaEvalSLA(total24h.SuccessRequests, total24h.ErrorRequests)

	if ttftP50, _, state := plazaEvalTTFT(mergedTTFT, false); state == PlazaMetricStateValid {
		summary.TTFTP50Ms = ttftP50
	}
	if cache24hSuccess >= PlazaMinimumSample && cache24hDenom > 0 {
		ratio := plazaClampRatio(float64(cache24hRead) / float64(cache24hDenom))
		summary.CacheReadRatio = &ratio
	}
	if consRate, consState := plazaEvalConsistency(consistency24hObserved, consistency24hMismatch); consState == PlazaConsistencyVerified || consState == PlazaConsistencyMismatchDetected {
		summary.ConsistencyRate = consRate
	}

	healthy := 0
	for _, pair := range model24hCounts {
		total := pair[0] + pair[1]
		if total >= PlazaMinimumSample && float64(pair[0])/float64(total) >= PlazaHealthySLAThreshold {
			healthy++
		}
	}
	summary.HealthyModelCount = healthy

	return summary
}
