import { apiClient } from '../client'
export interface TaskPlan { id: string; configuration_hash: string; planned_requests: number; expires_at: string; estimate_status: string }
export interface TaskStatus { reports: { verdict: string; contract_status: string; reasons: string[]; benchmark: { id: string; version: string } }[]; id: string; state: string; planned_requests: number; dispatched: number; completed: number }
export const detectorTaskAPI = {
  async plan(input: { site_api_key_id: number; channel: string; request_model: string; claimed_model: string; tier: string }, signal?: AbortSignal) {
    return (await apiClient.post<TaskPlan>('/admin/detector/plans', input, { signal, timeout: 70000 })).data
  },
  async create(plan: TaskPlan, idempotency_key: string) {
    return (await apiClient.post<{ id: string }>('/admin/detector/jobs', { plan_id: plan.id, configuration_hash: plan.configuration_hash, idempotency_key })).data
  },
  async job(id: string, signal?: AbortSignal) { return (await apiClient.get<TaskStatus>(`/admin/detector/jobs/${encodeURIComponent(id)}`, { signal })).data },
  async cancel(id: string) { await apiClient.post(`/admin/detector/jobs/${encodeURIComponent(id)}/cancel`, {}) }
}
