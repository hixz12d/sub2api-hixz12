import type { UsageLog } from '@/types'

export type ThroughputRow = Partial<Pick<UsageLog,
  'output_tokens' | 'duration_ms' | 'first_token_ms' | 'image_count' | 'image_output_tokens' | 'billing_mode' |
  'stream' | 'request_type' | 'monitor_output_tps_milli'
>>

const MIN_GENERATION_MS = 500
const MIN_OUTPUT_TOKENS = 20

function isStreamRow(row: ThroughputRow): boolean {
  if (row.request_type === 'stream' || row.request_type === 'ws_v2') return true
  if (row.request_type === 'sync') return false
  return row.stream === true
}

/**
 * Output tokens per second. Prefers the gateway-measured visible stream rate;
 * otherwise only streaming rows with enough generation time and tokens qualify,
 * because short or non-streaming requests produce meaningless spikes.
 */
export function usageTokensPerSecond(row: ThroughputRow): number | null {
  if ((row.image_count ?? 0) > 0 || (row.image_output_tokens ?? 0) > 0 || row.billing_mode === 'image') return null

  const monitored = row.monitor_output_tps_milli
  if (monitored != null && Number.isFinite(monitored) && monitored >= 0) return monitored / 1000

  if (!isStreamRow(row)) return null
  const tokens = row.output_tokens
  const duration = row.duration_ms
  const first = row.first_token_ms
  if (tokens == null || !Number.isFinite(tokens) || tokens < MIN_OUTPUT_TOKENS ||
      duration == null || !Number.isFinite(duration) ||
      first == null || !Number.isFinite(first) || first < 0) return null

  const generationMs = duration - first
  if (generationMs < MIN_GENERATION_MS) return null
  const tps = tokens / (generationMs / 1000)
  return Number.isFinite(tps) ? tps : null
}

export function formatUsageTokensPerSecond(row: ThroughputRow): string {
  const value = usageTokensPerSecond(row)
  return value == null ? '—' : `${value.toFixed(1)} tok/s`
}
