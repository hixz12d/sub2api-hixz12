import { apiClient } from '../client'

export interface ProxyGroup {
  id: number
  name: string
  max_accounts_per_proxy: number
  proxy_ids: number[]
  available_proxy_ids: number[]
}
export type ProxyGroupInput = Pick<ProxyGroup, 'name' | 'max_accounts_per_proxy' | 'proxy_ids'>

export async function listProxyGroups(): Promise<ProxyGroup[]> {
  const { data } = await apiClient.get<ProxyGroup[]>('/admin/proxy-groups')
  return data
}
export async function saveProxyGroup(id: number | null, input: ProxyGroupInput): Promise<ProxyGroup> {
  const { data } = id === null
    ? await apiClient.post<ProxyGroup>('/admin/proxy-groups', input)
    : await apiClient.put<ProxyGroup>(`/admin/proxy-groups/${id}`, input)
  return data
}
export async function deleteProxyGroup(id: number): Promise<void> {
  await apiClient.delete(`/admin/proxy-groups/${id}`)
}
