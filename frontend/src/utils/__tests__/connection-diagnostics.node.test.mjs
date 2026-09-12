import { test } from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'
import { createHash } from 'node:crypto'
import { runDiagnostics } from '../../../public/connection-diagnostics.mjs'

async function fixture(t) {
  const sockets = new Set(), requests = [], frames = []
  const server = http.createServer((req, res) => {
    requests.push({ method: req.method, url: req.url, auth: req.headers.authorization })
    if (req.url === '/v1/models') {
      res.setHeader('Content-Type', 'application/json'); res.end(JSON.stringify({ data: [{ id: 'gpt-test' }] })); return
    }
    res.setHeader('Content-Type', 'text/event-stream')
    res.end('data: {"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1}}}\n\n')
  })
  server.on('connection', socket => { sockets.add(socket); socket.on('close', () => sockets.delete(socket)) })
  server.on('upgrade', (req, socket) => {
    requests.push({ method: 'UPGRADE', url: req.url, auth: req.headers.authorization })
    const accept = createHash('sha1').update(req.headers['sec-websocket-key'] + '258EAFA5-E914-47DA-95CA-C5AB0DC85B11').digest('base64')
    socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`)
    socket.on('data', data => { frames.push(data); socket.end() })
    socket.on('error', () => {})
  })
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve))
  t.after(async () => { for (const socket of sockets) socket.destroy(); await new Promise(resolve => server.close(resolve)) })
  return { baseUrl: `http://127.0.0.1:${server.address().port}`, requests, frames }
}

test('default checks authenticate and handshake without inference', async t => {
  const f = await fixture(t)
  const report = await runDiagnostics({ baseUrl: f.baseUrl, model: 'gpt-test', key: 'test-key' })
  assert.equal(report.checks.filter(check => check.status === 'failed').length, 0)
  assert.equal(report.checks.find(check => check.name === 'http_stream').status, 'not_tested')
  assert.equal(f.requests.filter(req => req.method === 'POST').length, 0)
  assert.ok(f.requests.every(req => req.auth === 'Bearer test-key'))
  assert.equal(report.checks.find(check => check.name === 'websocket_handshake_only').status, 'passed')
})

test('explicit stream option sends exactly one model request', async t => {
  const f = await fixture(t)
  const report = await runDiagnostics({ baseUrl: f.baseUrl, model: 'gpt-test', key: 'test-key', stream: true })
  assert.equal(report.checks.find(check => check.name === 'http_stream').status, 'passed')
  assert.equal(f.requests.filter(req => req.method === 'POST').length, 1)
})

test('a missing model prevents a billable test even with the stream flag', async t => {
  const f = await fixture(t)
  const report = await runDiagnostics({ baseUrl: f.baseUrl, model: 'missing', key: 'test-key', stream: true })
  assert.equal(report.checks.find(check => check.name === 'model_available').status, 'failed')
  assert.equal(f.requests.filter(req => req.method === 'POST').length, 0)
})
