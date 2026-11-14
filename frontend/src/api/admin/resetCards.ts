/**
 * 重置卡（消费返还卡）管理端接口，不受功能开关限制。
 */

import { apiClient } from '../client'
import type {
  ResetCardBatch,
  ResetCardBatchListResponse,
  ResetCardBatchPreviewRequest,
  ResetCardBatchPreviewResponse,
  CreateResetCardBatchRequest,
  AdminResetCardListResponse,
  AdminResetCardListFilters,
  ResetCardRevokeResponse
} from '@/types/resetCard'

const BASE = '/admin/reset-cards'

/** 分页返回批次（带 stats） */
export async function listBatches(
  page: number = 1,
  pageSize: number = 20,
  options?: { signal?: AbortSignal }
): Promise<ResetCardBatchListResponse> {
  const { data } = await apiClient.get<ResetCardBatchListResponse>(`${BASE}/batches`, {
    params: { page, page_size: pageSize },
    signal: options?.signal
  })
  return data
}

/** 发卡前预估收卡人数和最大成本 */
export async function previewBatch(
  request: ResetCardBatchPreviewRequest
): Promise<ResetCardBatchPreviewResponse> {
  const { data } = await apiClient.post<ResetCardBatchPreviewResponse>(
    `${BASE}/batches/preview`,
    request
  )
  return data
}

/** 创建批次并发卡，返回批次对象（带 stats） */
export async function createBatch(request: CreateResetCardBatchRequest): Promise<ResetCardBatch> {
  const { data } = await apiClient.post<ResetCardBatch>(`${BASE}/batches`, request)
  return data
}

/** 单个批次（含 stats） */
export async function getBatch(id: number): Promise<ResetCardBatch> {
  const { data } = await apiClient.get<ResetCardBatch>(`${BASE}/batches/${id}`)
  return data
}

/** 批次里的卡；search 按邮箱或用户名查 */
export async function listCards(
  batchId: number,
  page: number = 1,
  pageSize: number = 20,
  filters?: AdminResetCardListFilters,
  options?: { signal?: AbortSignal }
): Promise<AdminResetCardListResponse> {
  const { data } = await apiClient.get<AdminResetCardListResponse>(
    `${BASE}/batches/${batchId}/cards`,
    {
      params: { page, page_size: pageSize, ...filters },
      signal: options?.signal
    }
  )
  return data
}

/** 作废该批次里所有还没用的卡 */
export async function revokeBatch(id: number): Promise<ResetCardRevokeResponse> {
  const { data } = await apiClient.post<ResetCardRevokeResponse>(`${BASE}/batches/${id}/revoke`)
  return data
}

/** 作废单张没用的卡；已用过的卡返回错误 */
export async function revokeCard(id: number): Promise<ResetCardRevokeResponse> {
  const { data } = await apiClient.post<ResetCardRevokeResponse>(`${BASE}/cards/${id}/revoke`)
  return data
}

const resetCardsAPI = {
  listBatches,
  previewBatch,
  createBatch,
  getBatch,
  listCards,
  revokeBatch,
  revokeCard
}

export default resetCardsAPI
