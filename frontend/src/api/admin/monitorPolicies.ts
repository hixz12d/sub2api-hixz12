import { apiClient } from '../client'
import type { ChannelMonitorGroupPolicy } from '../channelMonitorGroups'
export interface PlatformReport { job_id: string; account_id: number; request_model: string; created_at: string; evidence: { verdict: string; contract_status: string; reasons: string[]; benchmark: { id: string; version: string } } }
export const monitorPolicyAPI = {
  async reports(id: number, page = 1, signal?: AbortSignal) { return (await apiClient.get<{ items: PlatformReport[]; has_more: boolean }>(`/admin/monitor-policies/${id}/reports`, { params: { page }, signal })).data },
  async list(page = 1, signal?: AbortSignal) { return (await apiClient.get<{ items: ChannelMonitorGroupPolicy[]; has_more: boolean }>('/admin/monitor-policies', { params: { page }, signal })).data },
  async save(policy: ChannelMonitorGroupPolicy) {
    const { group_id, display_name, primary_model, extra_models, enabled, probe_config, capability_config } = policy
    const body = { group_id, display_name, primary_model, extra_models, enabled, probe_config, capability_config, expected_revision: policy.version }
    return (await (policy.id ? apiClient.put<ChannelMonitorGroupPolicy>(`/admin/monitor-policies/${policy.id}`, body) : apiClient.post<ChannelMonitorGroupPolicy>('/admin/monitor-policies', body))).data
  },
  async remove(policy: ChannelMonitorGroupPolicy) { await apiClient.delete(`/admin/monitor-policies/${policy.id}`, { data: { expected_revision: policy.version } }) }
}
