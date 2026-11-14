/**
 * 重置卡（消费返还卡）用户端接口。
 * 开关 reset_card_enabled 关闭时这些接口返回 404。
 */

import { apiClient } from './client'
import type {
  ResetCardListResponse,
  ResetCardPreviewResponse,
  ResetCardUseResponse,
  ResetCardPopupSeenRequest
} from '@/types/resetCard'

/** 列出我的卡（可用在前，然后按创建时间倒序） */
export async function list(): Promise<ResetCardListResponse> {
  const { data } = await apiClient.get<ResetCardListResponse>('/reset-cards')
  return data
}

/** 实时重算一张可用卡的预估返还 */
export async function preview(id: number): Promise<ResetCardPreviewResponse> {
  const { data } = await apiClient.get<ResetCardPreviewResponse>(`/reset-cards/${id}/preview`)
  return data
}

/**
 * 使用卡。可返还为 0 时返回 409，reason 为 RESET_CARD_NOTHING_TO_REFUND，卡不会被消耗。
 */
export async function use(id: number): Promise<ResetCardUseResponse> {
  const { data } = await apiClient.post<ResetCardUseResponse>(`/reset-cards/${id}/use`)
  return data
}

/** 把这些卡标记为已弹过窗（只处理本人的卡） */
export async function markPopupSeen(cardIds: number[]): Promise<void> {
  const payload: ResetCardPopupSeenRequest = { card_ids: cardIds }
  await apiClient.post('/reset-cards/popup-seen', payload)
}

export const resetCardsAPI = {
  list,
  preview,
  use,
  markPopupSeen
}

export default resetCardsAPI
