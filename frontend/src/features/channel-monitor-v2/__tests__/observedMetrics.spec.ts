import { describe, expect, it } from 'vitest'
import { formatMonitorObservedSuccessRate, formatMonitorCacheReadRatio } from '../monitorFormat'

const evidence = {
  state: 'valid' as const,
  has_requests: true,
  has_ttft: false,
  has_duration: false,
  has_cache_measurement: false,
}

describe('observed monitor metrics', () => {
  it('uses observed success despite ignored errors and redacted counts', () => {
    const metric = { success_rate: .9, request_count: 0, error_rate: .05, measurement: evidence }
    expect(formatMonitorObservedSuccessRate(metric)).toBe('90.0%')
  })

  it('keeps all-failed zero distinct from no data', () => {
    expect(formatMonitorObservedSuccessRate({ success_rate: 0, measurement: evidence })).toBe('0.00%')
    expect(formatMonitorObservedSuccessRate({ success_rate: 0, measurement: { ...evidence, state: 'no_data', has_requests: false } })).toBe('-')
  })

  it('fails closed for legacy redacted or malformed values', () => {
    expect(formatMonitorObservedSuccessRate({ success_rate: .9, request_count: 0 })).toBe('-')
    expect(formatMonitorObservedSuccessRate({ success_rate: .9, request_count: 100 })).toBe('90.0%')
    for (const value of [undefined, NaN, Infinity, -1, 2]) {
      expect(formatMonitorObservedSuccessRate({ success_rate: value, measurement: evidence })).toBe('-')
    }
  })

  it('distinguishes unavailable cache measurement from measured zero', () => {
    expect(formatMonitorCacheReadRatio({ cache_rate: 0, measurement: evidence })).toBe('-')
    expect(formatMonitorCacheReadRatio({ cache_rate: 0, measurement: { ...evidence, has_cache_measurement: true } })).toBe('0.00%')
  })
})
