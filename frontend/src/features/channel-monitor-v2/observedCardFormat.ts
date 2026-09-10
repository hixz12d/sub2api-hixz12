import type { MonitorCardWindow } from '@/api/channelMonitorV2'

export function observedCardValue(window: Partial<MonitorCardWindow>, kind: 'tps' | 'ttft' | 'cache'): string {
  if (kind === 'cache') {
    const ratio = window.observed_cache_read_ratio
    if (window.observed_cache_reason || ratio == null || !Number.isFinite(ratio) || ratio < 0 || ratio > 1) return '-'
    return `${(ratio * 100).toFixed(1)}%`
  }
  if (window.output_tps_reason === 'partial_coverage') return '-'
  if (kind === 'tps' && window.output_tps_reason) return '-'
  const value = kind === 'tps' ? window.output_tps_p50_milli : window.observed_ttft_p90_ms
  if (value == null || !Number.isSafeInteger(value) || value < 0) return '-'
  return kind === 'tps' ? `~ ${(value / 1000).toFixed(2)} tok/s` : `~ ${value} ms`
}
