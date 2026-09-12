import { describe, expect, it } from 'vitest'
import { formatUsageTokensPerSecond, usageTokensPerSecond } from '../usageThroughput'

describe('usage throughput', () => {
  it('excludes first-token latency from generation time', () => {
    expect(formatUsageTokensPerSecond({ output_tokens: 542, duration_ms: 8240, first_token_ms: 2820 })).toBe('100.0 tok/s')
  })

  it('falls back to total time when first-token latency is missing', () => {
    expect(formatUsageTokensPerSecond({ output_tokens: 200, duration_ms: 4000 })).toBe('50.0 tok/s')
    expect(formatUsageTokensPerSecond({ output_tokens: 200, duration_ms: 4000, first_token_ms: 0 })).toBe('50.0 tok/s')
  })

  it.each([
    {}, { output_tokens: 0 }, { output_tokens: -1 }, { output_tokens: Number.NaN },
    { duration_ms: null }, { duration_ms: 0 }, { duration_ms: -1 }, { duration_ms: Infinity },
    { first_token_ms: -1 }, { first_token_ms: 2000 }, { first_token_ms: 3000 }, { first_token_ms: Number.NaN },
    { image_count: 1 }, { image_output_tokens: 50 }, { billing_mode: 'image' as const },
  ])('omits unavailable or invalid measurements: %j', (override) => {
    const row = Object.keys(override).length ? { output_tokens: 100, duration_ms: 2000, ...override } : {}
    expect(usageTokensPerSecond(row)).toBeNull()
    expect(formatUsageTokensPerSecond(row)).toBe('—')
  })
})
