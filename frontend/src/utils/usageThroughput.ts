import type { UsageLog } from '@/types'

export type ThroughputRow = Partial<Pick<UsageLog,
  'output_tokens' | 'duration_ms' | 'first_token_ms' | 'image_count' | 'image_output_tokens' | 'billing_mode'
>>

/** Output tokens per second; includes reasoning tokens reported by the upstream. */
export function usageTokensPerSecond(row: ThroughputRow): number | null {
  if ((row.image_count ?? 0) > 0 || (row.image_output_tokens ?? 0) > 0 || row.billing_mode === 'image') return null
  const tokens = row.output_tokens
  const duration = row.duration_ms
  if (tokens == null || !Number.isFinite(tokens) || tokens <= 0 ||
      duration == null || !Number.isFinite(duration) || duration <= 0) return null

  const first = row.first_token_ms
  if (first != null && (!Number.isFinite(first) || first < 0 || first >= duration)) return null
  const seconds = (duration - (first ?? 0)) / 1000
  const tps = tokens / seconds
  return Number.isFinite(tps) ? tps : null
}

export function formatUsageTokensPerSecond(row: ThroughputRow): string {
  const value = usageTokensPerSecond(row)
  return value == null ? '—' : `${value.toFixed(1)} tok/s`
}
