import { afterEach, describe, expect, it, vi } from 'vitest'
import { diagnosticBaseUrl, probeDiagnosticModels, probeDiagnosticStream } from '../connectionDiagnostics'

afterEach(() => vi.unstubAllGlobals())
const signal = () => new AbortController().signal

describe('connection diagnostics', () => {
  it('normalizes base URLs and rejects credential-bearing or endpoint URLs before sending a key', () => {
    expect(diagnosticBaseUrl(' https://example.com/ ')).toBe('https://example.com/v1')
    expect(diagnosticBaseUrl('https://example.com/v1/')).toBe('https://example.com/v1')
    for (const url of ['https://user:pass@example.com', 'https://example.com?key=secret', 'https://example.com/v1/responses', 'https://example.com/v1/models', 'javascript:alert(1)']) expect(() => diagnosticBaseUrl(url)).toThrow()
  })
  it('checks actual model IDs with bearer authentication and refuses redirects', async () => {
    const fetch = vi.fn().mockResolvedValue(new Response(JSON.stringify({ data: [{ id: 'gpt-5' }] })))
    vi.stubGlobal('fetch', fetch)
    expect(await probeDiagnosticModels('https://example.com', 'test-key', signal())).toEqual(['gpt-5'])
    expect(fetch).toHaveBeenCalledWith('https://example.com/v1/models', expect.objectContaining({ redirect: 'error', credentials: 'omit', headers: expect.objectContaining({ Authorization: 'Bearer test-key' }) }))
  })
  it('accepts a terminal SSE event across chunks and cancels further reading', async () => {
    const cancel = vi.fn()
    const body = new ReadableStream({ start(controller) {
      controller.enqueue(new TextEncoder().encode(': heartbeat\r\n\r\ndata: {"type":"response.com'))
      controller.enqueue(new TextEncoder().encode('pleted"}\r\n\r\n'))
    }, cancel })
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { headers: { 'Content-Type': 'text/event-stream' } })))
    await probeDiagnosticStream('https://example.com', 'test-key', 'gpt-5', signal())
    expect(cancel).toHaveBeenCalled()
  })
  it.each([
    ['text/html', 'OK', 'notStream'],
    ['text/event-stream', ': heartbeat\n\ndata: [DONE]\n\n', 'incompleteStream'],
    ['text/event-stream', 'data: {"type":"response.failed"}\n\n', 'upstreamRejected']
  ])('does not treat %s response as a successful model request', async (contentType, body, code) => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(body, { headers: { 'Content-Type': contentType } })))
    await expect(probeDiagnosticStream('https://example.com', 'test-key', 'gpt-5', signal())).rejects.toMatchObject({ code })
  })
})
