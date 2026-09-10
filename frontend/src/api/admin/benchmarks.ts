import { apiClient } from '../client'

export interface BenchmarkRelease {
  id: string
  benchmark_id: string
  version: string
  sha256: string
  mode: string
  engine_lock_sha256: string
  state: 'candidate' | 'approved' | 'withdrawn'
}
export interface BenchmarkChannel { name: string; release_id: string; revision: number; state: string }
export interface BenchmarkPage { items: BenchmarkRelease[]; channels: BenchmarkChannel[]; page: number; has_more: boolean }
const base = '/admin/monitor-benchmarks'
export const benchmarkAPI = {
  async list(page: number, signal?: AbortSignal) {
    return (await apiClient.get<BenchmarkPage>(base, { params: { page }, signal })).data
  },
  async stage(file: File, signal?: AbortSignal) {
    if (file.size === 0 || file.size > 32 * 1024 * 1024) throw new Error('Invalid benchmark file size')
    const bytes = await file.arrayBuffer()
    const parsed = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes))
    if (!parsed || !['id', 'version', 'mode', 'content_sha256'].every(key => typeof parsed[key] === 'string')) throw new Error('Invalid benchmark package')
    const digest = await crypto.subtle.digest('SHA-256', bytes)
    const sha256 = Array.from(new Uint8Array(digest), b => b.toString(16).padStart(2, '0')).join('')
    const payload = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader()
      reader.onerror = () => reject(new Error('File read failed'))
      reader.onload = () => resolve(String(reader.result).split(',')[1])
      reader.readAsDataURL(new Blob([bytes]))
    })
    return (await apiClient.post<{ id: string }>(base, {
      benchmark_id: parsed.id, version: parsed.version, mode: parsed.mode, content_sha256: parsed.content_sha256, sha256, payload
    }, { signal, timeout: 70000 })).data
  },
  async approve(id: string, signal?: AbortSignal) {
    await apiClient.post(`${base}/${encodeURIComponent(id)}/approve`, {}, { signal, timeout: 70000 })
  },
  async activate(id: string, channel: string, expected_revision: number, signal?: AbortSignal) {
    return (await apiClient.post<{ revision: number }>(`${base}/${encodeURIComponent(id)}/activate`, { channel, expected_revision }, { signal })).data
  },
  async withdraw(id: string, reason: string, signal?: AbortSignal) {
    await apiClient.post(`${base}/${encodeURIComponent(id)}/withdraw`, { reason }, { signal })
  }
}
