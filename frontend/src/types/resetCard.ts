/**
 * 重置卡（消费返还卡）类型定义。
 *
 * 字段名与 docs/plans/2026-11-14-reset-cards/part-0-contracts.md 的"接口约定"一字不差，
 * 后端（Part 1）、用户端（Part 2）、管理端（Part 3）共用。
 *
 * 约定：金额是数字（单位与余额一致，前端显示为 ¥）；percent 是 0–100 的数字；时间是 RFC3339 字符串。
 */

import type { BasePaginationResponse } from './index'

// ==================== 公共 ====================

/** 卡状态：expired 由服务端按 expires_at 现场算出，数据库不存。 */
export type ResetCardStatus = 'unused' | 'used' | 'expired' | 'revoked'

/** 限时额度状态：active = 生效中，settled = 已到期结算。 */
export type ResetCardCreditStatus = 'active' | 'settled'

/** 发卡对象：all = 全部用户，users = 指定用户。 */
export type ResetCardTargetMode = 'all' | 'users'

/** 卡适用的分组。分组已删除时 name 为空字符串，前端显示"已删除分组"。 */
export interface ResetCardGroup {
  id: number
  name: string
}

/** 业务错误码，出现在错误响应的 reason 字段。 */
export type ResetCardErrorReason =
  | 'RESET_CARD_NOTHING_TO_REFUND' // 409：可返还为 0，卡不会被消耗
  | 'RESET_CARD_EXPIRED'
  | 'RESET_CARD_NOT_AVAILABLE' // 已用、已作废或不是本人的卡
  | 'RESET_CARD_DISABLED'

// ==================== 用户端 ====================

/** 可用卡的实时预估（按"现在使用"计算）。 */
export interface ResetCardEstimate {
  /** 预计返还额（已按比例和上限计算） */
  amount: number
  /** 统计到的可统计消费（返还前的基数） */
  base_spend: number
  /** 从旧到新 7 个连续 24 小时段内的可统计消费，长度固定为 7 */
  daily: number[]
  /** 是否触顶（返还额被单张上限截断） */
  capped: boolean
}

/** 用卡后产生的限时额度。 */
export interface ResetCardCredit {
  amount: number
  /** 期间在适用分组已花掉的部分（生效中时为实时值） */
  used_amount: number
  /** 剩余可用部分 = amount − used_amount，不小于 0 */
  remaining_amount: number
  starts_at: string
  expires_at: string
  status: ResetCardCreditStatus
  /** 到期回收额；生效中（active）时为 null */
  clawback_amount: number | null
}

export interface ResetCard {
  id: number
  batch_name: string
  groups: ResetCardGroup[]
  percent: number
  max_amount: number
  /** 批次设定的返还额度有效天数 */
  credit_valid_days: number
  status: ResetCardStatus
  expires_at: string
  created_at: string
  /** 未使用时为 null */
  used_at: string | null
  /** 已使用时为实际返还额，否则为 null */
  refund_amount: number | null
  /** 只有可用（status = unused）的卡才有，否则为 null */
  estimate: ResetCardEstimate | null
  /** 只有已使用（status = used）的卡才有，否则为 null */
  credit: ResetCardCredit | null
}

/** GET /reset-cards */
export interface ResetCardListResponse {
  /** 可用在前，然后按创建时间倒序 */
  cards: ResetCard[]
  /** 可用（未用且未过期）的张数 */
  unused_count: number
  /** 所有生效中限时额度的剩余之和 */
  limited_credit_remaining: number
  /** 待弹窗的卡：可用且 popup_seen_at 为空 */
  popup_card_ids: number[]
}

/** GET /reset-cards/:id/preview 直接返回 estimate 对象 */
export type ResetCardPreviewResponse = ResetCardEstimate

/** POST /reset-cards/:id/use */
export interface ResetCardUseResponse {
  refund_amount: number
  credit_expires_at: string
  /** 使用后的总余额 */
  balance: number
}

/** POST /reset-cards/popup-seen 的 body */
export interface ResetCardPopupSeenRequest {
  card_ids: number[]
}

// ==================== 管理端 ====================

export interface ResetCardBatchStats {
  issued: number
  unused: number
  used: number
  expired: number
  revoked: number
  refunded_total: number
  clawback_total: number
}

export interface ResetCardBatch {
  id: number
  name: string
  groups: ResetCardGroup[]
  percent: number
  max_amount: number
  card_valid_days: number
  credit_valid_days: number
  target_mode: ResetCardTargetMode
  only_active: boolean
  created_at: string
  stats: ResetCardBatchStats
}

/** POST /admin/reset-cards/batches/preview 的 body */
export interface ResetCardBatchPreviewRequest {
  group_ids: number[]
  percent: number
  max_amount: number
  target_mode: ResetCardTargetMode
  only_active: boolean
  /** target_mode = users 时必填，最多 2000 个 */
  user_ids?: number[]
}

/** POST /admin/reset-cards/batches/preview 的返回 */
export interface ResetCardBatchPreviewResponse {
  recipient_count: number
  estimated_max_cost: number
}

/** POST /admin/reset-cards/batches 的 body */
export interface CreateResetCardBatchRequest {
  name: string
  group_ids: number[]
  percent: number
  max_amount: number
  card_valid_days: number
  credit_valid_days: number
  target_mode: ResetCardTargetMode
  only_active: boolean
  /** target_mode = users 时必填，最多 2000 个 */
  user_ids?: number[]
}

export interface AdminResetCardUser {
  id: number
  email: string
  username: string
}

/** 管理端卡列表里限时额度的简要信息 */
export interface AdminResetCardCredit {
  amount: number
  used_amount: number
  remaining_amount: number
  expires_at: string
  status: ResetCardCreditStatus
  /** 生效中（active）时为 null */
  clawback_amount: number | null
}

export interface AdminResetCard {
  id: number
  user: AdminResetCardUser
  status: ResetCardStatus
  expires_at: string
  created_at: string
  used_at: string | null
  refund_amount: number | null
  credit: AdminResetCardCredit | null
}

/** GET /admin/reset-cards/batches/:id/cards 的筛选参数 */
export interface AdminResetCardListFilters {
  status?: ResetCardStatus
  /** 按邮箱或用户名查 */
  search?: string
}

/** POST /admin/reset-cards/batches/:id/revoke 和 /cards/:id/revoke 的返回 */
export interface ResetCardRevokeResponse {
  revoked: number
}

export type ResetCardBatchListResponse = BasePaginationResponse<ResetCardBatch>
export type AdminResetCardListResponse = BasePaginationResponse<AdminResetCard>
