import { describe, expect, it } from 'vitest'
import type { UserErrorRequestDetail } from '@/types'
import { buildUserErrorReport, userErrorAdvice } from '../userErrorAdvice'

const detail: UserErrorRequestDetail = {
  id: 12, created_at: '2026-09-12T00:00:00Z', model: 'gpt-5', inbound_endpoint: '/v1/responses?api_key=secret',
  status_code: 503, category: 'upstream', platform: 'openai', message: 'Bearer sensitive-token',
  key_name: 'sk-secret-123456789', key_deleted: false, client_ip: '127.0.0.1',
  user_agent: 'Codex/1.2 Bearer another-token', error_body: '{"error":{"message":"secret upstream details"}}'
}
describe('user error guidance', () => {
  it('does not mistake upstream authentication failure for a broken user key', () => {
    expect(userErrorAdvice({ ...detail, upstream_status_code: 401 })).toBe('upstream')
    expect(userErrorAdvice({ ...detail, category: 'auth' })).toBe('auth')
    expect(userErrorAdvice({ ...detail, error_body: '{"error":{"type":"first_output_timeout"}}' })).toBe('firstOutput')
    expect(userErrorAdvice({ ...detail, error_body: '{"error":{"code":"gateway_concurrency_limit"}}' })).toBe('concurrency')
    expect(userErrorAdvice({ ...detail, error_body: 'not json' })).toBe('upstream')
  })
  it('copies only safe troubleshooting fields', () => {
    const report = buildUserErrorReport(detail)
    expect(JSON.parse(report).endpoint).toBe('/v1/responses')
    expect(JSON.parse(report).error_id).toBe(12)
    for (const value of ['sk-secret', 'sensitive-token', 'another-token', 'secret upstream', 'client_ip', 'error_body', 'api_key=']) expect(report).not.toContain(value)
    expect(report).toContain('[redacted]')
  })
})
