import { afterEach, describe, expect, it, vi } from 'vitest'
import { KeyTestError, listKeyTestModels, runKeyQuestion } from '../keyTest'

const signal = () => new AbortController().signal
const frame = (content: string, finish: string | null = null) =>
  `data: ${JSON.stringify({ choices: [{ delta: { content }, finish_reason: finish }] })}\r\n\r\n`

function mockStream(content: string, chunkSize = 7) {
  const encoded = new TextEncoder().encode(content)
  const cancel = vi.fn()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (let i = 0; i < encoded.length; i += chunkSize) controller.enqueue(encoded.slice(i, i + chunkSize))
      controller.close()
    }, cancel
  })
  const fetchMock = vi.fn().mockResolvedValue(new Response(body, { headers: { 'content-type': 'text/event-stream' } }))
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

afterEach(() => vi.unstubAllGlobals())

describe('API key manual question transport', () => {
  it('discovers future text models using only the selected key', async () => {
    const fetchMock = vi.fn().mockResolvedValue(Response.json({ data: [
      { id: 'gpt-future' }, { id: 'claude-future' }, { id: 'gpt-image-2' },
      { id: 'gpt-future' }, {}, null
    ] }))
    vi.stubGlobal('fetch', fetchMock)
    expect(await listKeyTestModels('sk-selected-user', signal())).toEqual(['gpt-future', 'claude-future'])
    expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/v1\/models$/), expect.objectContaining({
      headers: expect.objectContaining({ Authorization: 'Bearer sk-selected-user' }),
      credentials: 'omit', redirect: 'error'
    }))
  })

  it('sends a single unmodified question and decodes split UTF-8 SSE frames', async () => {
    const fetchMock = mockStream(': keepalive\r\n\r\n' + frame('中文🙂') + frame('\nAnswer', 'stop') + 'data: [DONE]\n\n', 1)
    let answer = ''
    const result = await runKeyQuestion('sk-user', 'gpt-future', '  question\n  ', signal(), text => { answer += text })
    expect(answer).toBe('中文🙂\nAnswer')
    expect(result.truncated).toBe(false)
    const [url, options] = fetchMock.mock.calls[0]
    expect(url).toMatch(/\/v1\/chat\/completions$/)
    expect(options.headers.Authorization).toBe('Bearer sk-user')
    expect(JSON.parse(options.body)).toEqual({ model: 'gpt-future', messages: [{ role: 'user', content: '  question\n  ' }], tools: [], stream: true })
  })

  it('accepts a final event without a trailing newline and reports output limits', async () => {
    mockStream(frame('partial', 'length').trimEnd())
    expect(await runKeyQuestion('k', 'm', 'q', signal(), vi.fn())).toEqual({ truncated: true })
  })

  it('rejects an interrupted stream and preserves the received text', async () => {
    mockStream(frame('partial'))
    const onText = vi.fn()
    await expect(runKeyQuestion('k', 'm', 'q', signal(), onText)).rejects.toMatchObject({ code: 'incomplete' })
    expect(onText).toHaveBeenCalledWith('partial')
  })

  it.each([
    ['data: {broken}\n\n', 'invalidResponse'],
    ['data: {"error":{"message":"Quota exceeded"}}\n\n', 'http'],
    ['data: [DONE]\n\n', 'empty'],
    [frame('', 'stop'), 'empty'],
    [frame('partial', 'content_filter'), 'incomplete'],
  ])('handles invalid/error/empty streams: %s', async (content, code) => {
    mockStream(content)
    await expect(runKeyQuestion('k', 'm', 'q', signal(), vi.fn())).rejects.toMatchObject({ code })
  })

  it('returns normal gateway permission and quota errors', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ error: { message: 'Key quota exhausted' } }, { status: 403 })))
    await expect(runKeyQuestion('k', 'm', 'q', signal(), vi.fn())).rejects.toThrow('HTTP 403: Key quota exhausted')
  })

  it('does not treat HTML success pages as answers', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('<html>login</html>')))
    await expect(runKeyQuestion('k', 'm', 'q', signal(), vi.fn())).rejects.toBeInstanceOf(KeyTestError)
  })

  it('passes cancellation to fetch', async () => {
    const controller = new AbortController()
    vi.stubGlobal('fetch', vi.fn((_url, options) => new Promise((_resolve, reject) => {
      options.signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })))
    const request = runKeyQuestion('k', 'm', 'q', controller.signal, vi.fn())
    controller.abort()
    await expect(request).rejects.toMatchObject({ name: 'AbortError' })
  })
})
