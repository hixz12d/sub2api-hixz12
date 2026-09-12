import type { UserErrorRequestDetail } from '@/types'

export function userErrorAdvice(detail: UserErrorRequestDetail): string {
  // Use machine-readable codes only; arbitrary upstream prose is not evidence
  // that the user's key, quota, or configuration is wrong.
  let code = ''
  try {
    const body = JSON.parse(detail.error_body || '{}')
    code = body?.error?.code || body?.error?.type || ''
  } catch { /* A non-JSON response retains its coarse category. */ }
  if (detail.category === 'auth') return 'auth'
  if (detail.category === 'quota') return 'quota'
  if (code === 'gateway_concurrency_limit' || code === 'gateway_queue_full') return 'concurrency'
  if (code === 'first_output_timeout') return 'firstOutput'
  if (['upstream_timeout', 'stream_timeout', 'timeout_error'].includes(code)) return 'timeout'
  if (['rate_limit', 'invalid_request', 'service_unavailable', 'upstream', 'internal', 'cyber'].includes(detail.category)) {
    return detail.category
  }
  return 'other'
}

function reportText(value?: string): string {
  return (value || '')
    .replace(/sk-[\w-]{8,}/gi, '[redacted]')
    .replace(/Bearer\s+[^\s,;]+/gi, 'Bearer [redacted]')
    .replace(/\p{Cc}/gu, ' ')
    .slice(0, 300)
}

// Explicit allowlist: never copy raw upstream bodies/messages, IP addresses,
// internal account details, or arbitrary fields returned by a future API.
export function buildUserErrorReport(detail: UserErrorRequestDetail): string {
  return JSON.stringify({
    error_id: detail.id,
    time: detail.created_at,
    key_name: reportText(detail.key_name),
    model: reportText(detail.model),
    endpoint: reportText(detail.inbound_endpoint?.split(/[?#]/)[0]),
    status: detail.status_code,
    upstream_status: detail.upstream_status_code,
    category: detail.category,
    platform: reportText(detail.platform),
    client: reportText(detail.user_agent)
  }, null, 2)
}
