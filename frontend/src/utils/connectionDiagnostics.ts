export class ConnectionDiagnosticError extends Error {
  constructor(public readonly code: string, public readonly status?: number) {
    super(code)
  }
}

export function diagnosticBaseUrl(value: string): string {
  let url: URL
  try { url = new URL(value.trim()) } catch { throw new ConnectionDiagnosticError('invalidUrl') }
  if (!['https:', 'http:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) {
    throw new ConnectionDiagnosticError('invalidUrl')
  }
  const path = url.pathname.replace(/\/+$/, '')
  if (/\/(responses|models|messages|chat\/completions)$/i.test(path)) throw new ConnectionDiagnosticError('endpointUrl')
  url.pathname = /\/v1$/i.test(path) ? path : `${path}/v1`
  return url.toString().replace(/\/$/, '')
}

async function diagnosticResponse(base: string, key: string, endpoint: string, signal: AbortSignal, body?: object): Promise<Response> {
  const response = await fetch(`${diagnosticBaseUrl(base)}/${endpoint}`, {
    method: body ? 'POST' : 'GET',
    headers: { Authorization: `Bearer ${key}`, Accept: body ? 'text/event-stream' : 'application/json', ...(body ? { 'Content-Type': 'application/json' } : {}) },
    ...(body ? { body: JSON.stringify(body) } : {}),
    signal, cache: 'no-store', credentials: 'omit', redirect: 'error'
  })
  if (!response.ok) {
    await response.body?.cancel()
    throw new ConnectionDiagnosticError(response.status === 401 ? 'auth' : response.status === 429 ? 'rateLimit' : 'httpError', response.status)
  }
  return response
}

export async function probeDiagnosticModels(base: string, key: string, signal: AbortSignal): Promise<string[]> {
  const response = await diagnosticResponse(base, key, 'models', signal)
  if (!response.body) throw new ConnectionDiagnosticError('invalidModels')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let text = '', bytes = 0
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) break
      bytes += value.byteLength
      if (bytes > 2 * 1024 * 1024) throw new ConnectionDiagnosticError('responseTooLarge')
      text += decoder.decode(value, { stream: true })
    }
    text += decoder.decode()
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
  let payload
  try { payload = JSON.parse(text) } catch { throw new ConnectionDiagnosticError('invalidModels') }
  const items = payload?.data ?? payload?.models
  if (!Array.isArray(items)) throw new ConnectionDiagnosticError('invalidModels')
  return items.map(item => item?.id ?? item?.slug).filter((id): id is string => typeof id === 'string')
}

// Success requires a Responses terminal event, not just HTTP 200 or a heartbeat.
export async function probeDiagnosticStream(base: string, key: string, model: string, signal: AbortSignal): Promise<void> {
  const response = await diagnosticResponse(base, key, 'responses', signal, {
    model, input: 'Reply with OK.', stream: true, max_output_tokens: 16
  })
  if (!response.headers.get('content-type')?.toLowerCase().includes('text/event-stream') || !response.body) {
    await response.body?.cancel()
    throw new ConnectionDiagnosticError('notStream')
  }
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = '', bytes = 0
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) throw new ConnectionDiagnosticError('incompleteStream')
      bytes += value.byteLength
      if (bytes > 256 * 1024) throw new ConnectionDiagnosticError('responseTooLarge')
      buffer += decoder.decode(value, { stream: true }).replace(/\r/g, '')
      let boundary: number
      while ((boundary = buffer.indexOf('\n\n')) >= 0) {
        const frame = buffer.slice(0, boundary)
        buffer = buffer.slice(boundary + 2)
        const data = frame.split('\n').filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
        if (!data || data === '[DONE]') continue
        let event
        try { event = JSON.parse(data) } catch { throw new ConnectionDiagnosticError('invalidStream') }
        if (['error', 'response.failed', 'response.incomplete', 'response.cancelled', 'response.canceled'].includes(event.type)) {
          throw new ConnectionDiagnosticError('upstreamRejected')
        }
        if (event.type === 'response.completed' || event.type === 'response.done') return
      }
    }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
}

export function diagnosticFailure(error: unknown): { code: string; status?: number } {
  if (error instanceof ConnectionDiagnosticError) return { code: error.code, status: error.status }
  if (error instanceof Error && (error.name === 'AbortError' || error.name === 'TimeoutError')) return { code: 'timeout' }
  return { code: 'browserNetwork' }
}
