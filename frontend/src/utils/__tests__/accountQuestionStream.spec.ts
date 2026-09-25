import { afterEach, describe, expect, it, vi } from 'vitest'
import { streamAccountQuestion } from '../accountQuestionStream'

vi.mock('@/api/client', () => ({ buildApiUrl: (path: string) => `/api/v1${path}` }))
afterEach(() => vi.unstubAllGlobals())
function responseFromChunks(chunks: Uint8Array[]) {
  return new Response(new ReadableStream({ start(controller) { for (const chunk of chunks) controller.enqueue(chunk); controller.close() } }))
}
describe('account question SSE stream', () => {
  it('preserves split UTF-8, CRLF, multiline SSE data and the final unsuffixed record event', async () => {
    const raw = ': keepalive\r\ndata:{"type":"content",\r\ndata: "text":"你好"}\r\n\r\ndata: {"type":"test_complete","success":true}\n\ndata: {"type":"question_record","saved":true,"record_id":"q-1"}'
    const bytes = new TextEncoder().encode(raw)
    const fetcher = vi.fn().mockResolvedValue(responseFromChunks(Array.from(bytes, byte => new Uint8Array([byte]))))
    vi.stubGlobal('fetch', fetcher)
    const events: unknown[] = []
    await streamAccountQuestion(5, 'gpt-5.4', 'same question', new AbortController().signal, event => events.push(event))
    expect(events).toEqual([{ type: 'content', text: '你好' }, { type: 'test_complete', success: true }, { type: 'question_record', saved: true, record_id: 'q-1' }])
    expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({ model_id: 'gpt-5.4', prompt: 'same question', mode: 'question' })
  })
  it('sends reasoning effort only when one is selected', async () => {
    const done = () => Promise.resolve(responseFromChunks([new TextEncoder().encode('data: {"type":"test_complete","success":true}\n\n')]))
    const fetcher = vi.fn().mockImplementation(done)
    vi.stubGlobal('fetch', fetcher)
    await streamAccountQuestion(1, 'gpt-5.4', 'q', new AbortController().signal, vi.fn(), 'xhigh')
    await streamAccountQuestion(1, 'gpt-5.4', 'q', new AbortController().signal, vi.fn(), '')
    expect(JSON.parse(fetcher.mock.calls[0][1].body)).toEqual({ model_id: 'gpt-5.4', prompt: 'q', mode: 'question', reasoning_effort: 'xhigh' })
    expect(JSON.parse(fetcher.mock.calls[1][1].body)).toEqual({ model_id: 'gpt-5.4', prompt: 'q', mode: 'question' })
  })
  it('rejects a dropped stream instead of marking a partial answer successful', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(responseFromChunks([new TextEncoder().encode('data: {"type":"content","text":"partial"}\n\n')])))
    const events = vi.fn()
    await expect(streamAccountQuestion(1, 'gpt-5.4', 'q', new AbortController().signal, events)).rejects.toThrow('ACCOUNT_TEST_INCOMPLETE')
    expect(events).toHaveBeenCalledWith({ type: 'content', text: 'partial' })
  })
  it('cancels a pending reader on abort and accepts no late events', async () => {
    let streamController!: ReadableStreamDefaultController<Uint8Array>
    const cancelled = vi.fn()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(new ReadableStream({ start(c) { streamController = c }, cancel: cancelled }))))
    const controller = new AbortController()
    const events = vi.fn()
    const pending = streamAccountQuestion(1, 'gpt-5.4', 'q', controller.signal, events)
    await Promise.resolve()
    controller.abort()
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' })
    expect(cancelled).toHaveBeenCalledOnce()
    expect(() => streamController.enqueue(new TextEncoder().encode('late'))).toThrow()
    expect(events).not.toHaveBeenCalled()
  })
  it('reports authorization errors without treating them as SSE', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({ message: 'Question review unavailable' }), { status: 403 })))
    await expect(streamAccountQuestion(1, 'gpt-5.4', 'q', new AbortController().signal, vi.fn())).rejects.toThrow('Question review unavailable')
  })
})
