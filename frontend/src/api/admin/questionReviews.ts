import { apiClient } from '../client'

export interface QuestionRecord {
  id: string
  account_id: number
  request_model: string
  prompt: string
  answer: string
  transport_state: 'completed' | 'failed' | 'incomplete'
  created_at: string
}
export type QuestionVerdict = 'normal' | 'degraded' | 'unlabeled'
export interface QuestionReview {
  id: string
  record_id: string
  reviewed_by: number
  verdict: QuestionVerdict
  reason: string
  revision: number
  created_at: string
}
interface Page<T> { items: T[]; page: number; has_more: boolean }
export const questionAPI = {
  async list(account: number, page = 1, signal?: AbortSignal) {
    return (await apiClient.get<Page<QuestionRecord>>(`/admin/accounts/${account}/questions`, { params: { page }, signal })).data
  },
  async history(record: string, page = 1, signal?: AbortSignal) {
    return (await apiClient.get<Page<QuestionReview>>(`/admin/accounts/questions/${encodeURIComponent(record)}/reviews`, { params: { page }, signal })).data
  },
  async review(record: string, verdict: QuestionVerdict, reason: string, expected_revision: number) {
    return (await apiClient.post<QuestionReview>(`/admin/accounts/questions/${encodeURIComponent(record)}/reviews`, { verdict, reason, expected_revision })).data
  }
}
