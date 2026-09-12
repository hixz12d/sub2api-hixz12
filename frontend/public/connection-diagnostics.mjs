// Node.js 20+. No dependencies. Run on the same computer/network as your client.
// Default: model-list authentication + WebSocket handshake (no inference).
// Add --stream only if you accept the cost of one small Responses request.
import http from 'node:http'
import https from 'node:https'
import { createHash, randomBytes } from 'node:crypto'
import { pathToFileURL } from 'node:url'

function failure(code, status) { return Object.assign(new Error(code), { code, status }) }
function normalizeBase(value) {
  let url
  try { url = new URL(value) } catch { throw failure('invalid_url') }
  if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password || url.search || url.hash) throw failure('invalid_url')
  const path = url.pathname.replace(/\/+$/, '')
  if (/\/(responses|models|messages|chat\/completions)$/i.test(path)) throw failure('use_base_url_not_endpoint')
  url.pathname = /\/v1$/i.test(path) ? path : `${path}/v1`
  return url.toString().replace(/\/$/, '')
}
async function boundedText(response, limit) {
  if (!response.body) throw failure('empty_response')
  const reader = response.body.getReader(), decoder = new TextDecoder()
  let bytes = 0, text = ''
  try {
    while (true) {
      const { value, done } = await reader.read()
      if (done) return text + decoder.decode()
      bytes += value.byteLength
      if (bytes > limit) throw failure('response_too_large')
      text += decoder.decode(value, { stream: true })
    }
  } finally { await reader.cancel().catch(() => {}); reader.releaseLock() }
}
async function request(base, key, endpoint, body, timeout) {
  const response = await fetch(`${base}/${endpoint}`, {
    method: body ? 'POST' : 'GET', redirect: 'error', signal: AbortSignal.timeout(timeout),
    headers: { Authorization: `Bearer ${key}`, ...(body ? { 'Content-Type': 'application/json', Accept: 'text/event-stream' } : { Accept: 'application/json' }) },
    ...(body ? { body: JSON.stringify(body) } : {})
  })
  if (!response.ok) { await response.body?.cancel(); throw failure('http_error', response.status) }
  return response
}
async function checkStream(base, key, model, timeout) {
  const response = await request(base, key, 'responses', { model, input: 'Reply with OK.', stream: true, max_output_tokens: 16 }, timeout)
  if (!response.headers.get('content-type')?.toLowerCase().includes('text/event-stream')) { await response.body?.cancel(); throw failure('not_sse') }
  // The test requests at most 16 output tokens. A bounded full read also verifies
  // that the stream finishes, rather than mistaking heartbeats for completion.
  const text = await boundedText(response, 256 * 1024)
  let complete = false
  for (const frame of text.replace(/\r/g, '').split('\n\n')) {
    const data = frame.split('\n').filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
    if (!data || data === '[DONE]') continue
    let event
    try { event = JSON.parse(data) } catch { throw failure('invalid_sse') }
    if (['error', 'response.failed', 'response.incomplete', 'response.cancelled', 'response.canceled'].includes(event.type)) throw failure('upstream_rejected')
    if (['response.completed', 'response.done'].includes(event.type)) complete = true
  }
  if (!complete) throw failure('incomplete_stream')
}
export function checkWebSocket(base, key, timeout = 15000) {
  return new Promise((resolve, reject) => {
    const url = new URL(`${base}/responses`)
    const nonce = randomBytes(16).toString('base64')
    const expected = createHash('sha1').update(nonce + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64')
    const req = (url.protocol === 'https:' ? https : http).request(url, {
      headers: { Authorization: `Bearer ${key}`, Connection: 'Upgrade', Upgrade: 'websocket', 'Sec-WebSocket-Version': '13', 'Sec-WebSocket-Key': nonce }
    })
    const timer = setTimeout(() => req.destroy(failure('timeout')), timeout)
    req.once('error', error => { clearTimeout(timer); reject(error) })
    req.once('response', response => { clearTimeout(timer); response.destroy(); reject(failure('handshake_http_error', response.statusCode)) })
    req.once('upgrade', (response, socket) => {
      clearTimeout(timer)
      if (response.statusCode !== 101 || response.headers['sec-websocket-accept'] !== expected || String(response.headers.upgrade).toLowerCase() !== 'websocket') {
        socket.destroy(); reject(failure('invalid_handshake')); return
      }
      // A masked normal Close frame ends the probe without sending response.create.
      const mask = randomBytes(4), payload = Buffer.from([0x03, 0xe8])
      payload[0] ^= mask[0]; payload[1] ^= mask[1]
      socket.on('error', () => {})
      socket.end(Buffer.concat([Buffer.from([0x88, 0x82]), mask, payload]))
      setTimeout(() => socket.destroy(), 1000).unref()
      resolve()
    })
    req.end()
  })
}

export async function runDiagnostics({ baseUrl, key, model, stream = false, timeout = 15000 }) {
  const report = { time: new Date().toISOString(), node: process.versions.node, checks: [] }
  async function check(name, work) {
    const start = Date.now()
    try { await work(); report.checks.push({ name, status: 'passed', elapsed_ms: Date.now() - start }); return true }
    catch (error) {
      report.checks.push({ name, status: 'failed', code: error?.name === 'TimeoutError' ? 'timeout' : (typeof error?.code === 'string' ? error.code : 'connection_failed'), http_status: error?.status, elapsed_ms: Date.now() - start })
      return false
    }
  }
  let base
  if (!await check('configuration', async () => { base = normalizeBase(baseUrl); if (!key?.trim()) throw failure('missing_key'); if (!model?.trim()) throw failure('missing_model') })) return report
  report.base_url = base
  report.model = model
  let models = []
  const listed = await check('models_authentication', async () => {
    const payload = JSON.parse(await boundedText(await request(base, key, 'models', null, timeout), 2 * 1024 * 1024))
    const items = payload?.data ?? payload?.models
    if (!Array.isArray(items)) throw failure('invalid_models')
    models = items.map(item => item?.id ?? item?.slug)
  })
  if (listed) await check('model_available', async () => { if (!models.includes(model)) throw failure('model_not_listed') })
  await check('websocket_handshake_only', () => checkWebSocket(base, key, timeout))
  if (stream && listed && models.includes(model)) await check('http_stream', () => checkStream(base, key, model, Math.max(timeout, 45000)))
  else report.checks.push({ name: 'http_stream', status: 'not_tested', reason: stream ? 'model_check_failed' : 'add_--stream_to_authorize_a_billable_request' })
  return report
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const args = process.argv.slice(2)
  const option = name => args[args.indexOf(name) + 1]
  const key = process.env.SUB2API_API_KEY || ''
  const report = await runDiagnostics({ baseUrl: args.includes('--base-url') ? option('--base-url') : '', model: args.includes('--model') ? option('--model') : '', key, stream: args.includes('--stream') })
  let output = JSON.stringify(report, null, 2)
  if (key) output = output.split(key).join('[redacted]')
  console.log(output.replace(/sk-[\w-]{8,}/gi, '[redacted]'))
  process.exitCode = report.checks.some(check => check.status === 'failed') ? 1 : 0
}
