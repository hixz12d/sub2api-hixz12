/**
 * Model Plaza API（公开端点，可匿名访问）
 * 以分组为中心的模型价目：分组信息 + 模型渠道定价 + LiteLLM 官方参考价。
 * 带 token 请求时后端会额外返回专属分组与用户专属倍率。
 */

import { apiClient } from './client'
import type { UserPricingInterval, UserSupportedModelPricing } from './channels'

/** 官方参考价（USD per token，与计费目录同源；字段缺失 = 目录未覆盖）。 */
export interface PlazaOfficialPricing {
  input_price: number | null
  output_price: number | null
  /** 5m 缓存写入（= LiteLLM cache_creation）。 */
  cache_write_price: number | null
  /** 1h 缓存写入（LiteLLM cache_creation_above_1hr），多数模型缺失。 */
  cache_write_1h_price?: number | null
  cache_write_12h_price?: number | null
  cache_read_price: number | null
  /** 官方长上下文阶梯（多档模型才有），不受分组开关影响。 */
  intervals?: UserPricingInterval[]
}

/**
 * 多档时的计价基准：
 * - whole_request：整单按所在档单价计价（目录阶梯、渠道区间）；
 * - marginal：仅超出阈值的部分按该档单价计价（平台旧规则）。
 */
export type PlazaLongContextBasis = 'whole_request' | 'marginal'

/** 分时倍率时段：配置时区当天 [start_time, end_time) 内整单实付乘 multiplier。 */
export interface PlazaTimePricingPeriod {
  start_time: string
  end_time: string
  multiplier: number
}

/** 计费会生效的分时倍率（仅倍率 ≠ 1 的时段，已按开始时间升序）。 */
export interface PlazaTimePricing {
  /** IANA 时区名，如 Asia/Shanghai。 */
  timezone: string
  /** true 时时段仅周一至周五生效，周末整天按标准价计费。 */
  weekdays_only?: boolean
  periods: PlazaTimePricingPeriod[]
}

/** 单项指标样本状态（复用 V2 最低 50 样本门槛）。 */
export type PlazaMetricSampleState = 'valid' | 'low_sample' | 'no_data'

/** 输出速度状态（非 OpenAI 平台一期固定返回 unsupported）。 */
export type PlazaOutputTpsState = 'valid' | 'low_sample' | 'no_data' | 'unsupported'

/** 心跳槽状态。 */
export type PlazaHeartbeatSlotState =
  | 'normal'
  | 'partial_failure'
  | 'many_failures'
  | 'low_sample'
  | 'no_data'

/** 原模型一致核验状态。 */
export type PlazaConsistencyState = 'verified' | 'mismatch_detected' | 'low_sample' | 'no_data'

/** 第 1 层：全站实力招牌条脱敏摘要。 */
export interface PlazaSiteSummary {
  sla_30d: number | null
  sla_7d: number | null
  sla_24h: number | null
  ttft_p50_ms: number | null
  cache_read_ratio: number | null
  consistency_rate: number | null
  active_model_count: number
  healthy_model_count: number
  updated_at: string | null
  aggregation_ready: boolean
}

/** 模型元数据目录信息。 */
export interface PlazaModelMetadata {
  display_name: string
  description: string
  /** 规范化厂商键，如 openai / anthropic / google / xai / deepseek / other。 */
  vendor: string
  context_window: number | null
  max_output_tokens: number | null
  /** 标准化能力标签键，如 reasoning / tools / vision / audio / image / video / prompt_caching / web_search。 */
  capabilities: string[]
}

/** 单格心跳槽数据。 */
export interface PlazaHeartbeatSlot {
  start: string
  end: string
  success_rate: number | null
  state: PlazaHeartbeatSlotState
}

/** 模型近 24h 脱敏实测指标。 */
export interface PlazaModelMetrics {
  sla_24h: number | null
  sla_state: PlazaMetricSampleState
  heartbeat_12: PlazaHeartbeatSlot[]
  ttft_p50_ms: number | null
  ttft_p90_ms: number | null
  ttft_state: PlazaMetricSampleState
  output_tps_p50: number | null
  output_tps_state: PlazaOutputTpsState
  cache_read_ratio: number | null
  cache_state: PlazaMetricSampleState
  reference_multiplier: number | null
  reference_multiplier_state: PlazaMetricSampleState
  consistency_state: PlazaConsistencyState
  consistency_rate: number | null
  auto_pause_protected: boolean
  /** 预留未来满血能力探针升级槽位，一期固定为 null。 */
  capability: null
}

/** 详情页单时间窗指标汇总（24h / 7d / 30d）。 */
export interface PlazaDetailWindow {
  sla: number | null
  sla_state: PlazaMetricSampleState
  ttft_p50_ms: number | null
  ttft_p90_ms: number | null
  ttft_state: PlazaMetricSampleState
  output_tps_p50: number | null
  output_tps_state: PlazaOutputTpsState
  cache_read_ratio: number | null
  cache_state: PlazaMetricSampleState
  reference_multiplier: number | null
  reference_multiplier_state: PlazaMetricSampleState
  consistency_rate: number | null
  consistency_state: PlazaConsistencyState
}

/** 详情页近 24h 小时级趋势点。 */
export interface PlazaDetailTrendPoint {
  bucket_start: string
  ttft_p50_ms: number | null
  cache_read_ratio: number | null
  success_rate: number | null
}

/** 用户可见的全部分组横向比价项（不含分组数字 ID）。 */
export interface PlazaGroupComparison {
  group_name: string
  platform: string
  subscription_type: string
  is_exclusive: boolean
  effective_rate: number
  pricing: UserSupportedModelPricing | null
  official_pricing: PlazaOfficialPricing | null
  metrics: PlazaModelMetrics | null
}

/** 详情页一致性核验与保护说明。 */
export interface PlazaDetailProtection {
  consistency_method: string
  auto_pause_enabled: boolean
  capability_probe_status: 'reserved'
}

export interface PlazaModel {
  name: string
  platform: string
  /** 实收口径的展示定价：档位可提供绝对单价或相对基础价倍率；均为标准时段价。 */
  pricing: UserSupportedModelPricing | null
  official_pricing: PlazaOfficialPricing | null
  /** 仅多档模型返回。 */
  long_context_basis?: PlazaLongContextBasis
  /** 仅配置了分时倍率的模型返回。 */
  time_pricing?: PlazaTimePricing
  /** 模型元数据（展示名、简介、厂商、上下文窗口、能力标签）。 */
  metadata?: PlazaModelMetadata | null
  /** 近 24h 脱敏实测指标。 */
  metrics?: PlazaModelMetrics | null
}

export interface ModelPlazaGroup {
  id: number
  name: string
  description: string
  platform: string
  /** 'standard' | 'subscription' */
  subscription_type: string
  rate_multiplier: number
  /** 登录且管理员为该用户配了专属倍率时返回；生效倍率 = user_rate ?? rate_multiplier。 */
  user_rate_multiplier?: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  is_exclusive: boolean
  /** 生图独立倍率：true 时图片计费模型的实付倍率取 image_rate_multiplier，不取分组/专属倍率。 */
  image_rate_independent: boolean
  image_rate_multiplier: number
  /** 视频独立倍率开启时，覆盖视频模型的分组/用户专属倍率。 */
  video_rate_independent: boolean
  video_rate_multiplier: number
  /** 分组是否启用长上下文阶梯计费；false 时实付列只展示最低档，官方阶梯仅供参考。 */
  long_context_pricing_enabled: boolean
  models: PlazaModel[]
}

export interface ModelPlazaResponse {
  /** 管理员配置的全局价格说明（Markdown）。 */
  description: string
  groups: ModelPlazaGroup[]
  /** 全站实力招牌条脱敏摘要（聚合未就绪时可能为 null 或 aggregation_ready=false）。 */
  site_summary?: PlazaSiteSummary | null
}

/** 模型广场单模型详情响应（按需懒加载，不含数字分组 ID）。 */
export interface ModelPlazaDetailResponse {
  model: string
  platform: string
  group_name: string
  metadata: PlazaModelMetadata
  pricing: UserSupportedModelPricing | null
  official_pricing: PlazaOfficialPricing | null
  long_context_basis?: PlazaLongContextBasis
  time_pricing?: PlazaTimePricing
  windows: {
    h24: PlazaDetailWindow
    d7: PlazaDetailWindow
    d30: PlazaDetailWindow
  }
  heartbeat_72: PlazaHeartbeatSlot[]
  trend_24h: PlazaDetailTrendPoint[]
  group_comparisons: PlazaGroupComparison[]
  protection: PlazaDetailProtection
  updated_at: string | null
}

export interface GetModelPlazaDetailParams {
  model: string
  /** 分组名称（可选，绝不传数字分组 ID）。 */
  group?: string
}

/** 获取模型广场数据。开关未启用时后端返回 404。 */
export async function getModelPlaza(options?: { signal?: AbortSignal }): Promise<ModelPlazaResponse> {
  const { data } = await apiClient.get<ModelPlazaResponse>('/model-plaza', {
    signal: options?.signal
  })
  return data
}

/** 获取单模型全维详情（三档 SLA、72 格心跳、趋势、全分组横评）。 */
export async function getModelPlazaDetail(
  params: GetModelPlazaDetailParams,
  options?: { signal?: AbortSignal }
): Promise<ModelPlazaDetailResponse> {
  const query: Record<string, string> = { model: params.model }
  if (params.group) {
    query.group = params.group
  }
  const { data } = await apiClient.get<ModelPlazaDetailResponse>('/model-plaza/detail', {
    params: query,
    signal: options?.signal
  })
  return data
}

export const modelPlazaAPI = { getModelPlaza, getModelPlazaDetail }

export default modelPlazaAPI
