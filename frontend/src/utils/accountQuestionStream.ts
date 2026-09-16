import { buildApiUrl } from '@/api/client'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'

export interface AccountQuestionEvent {
  type: string
  text?: string
  model?: string
  success?: boolean
  error?: string
  saved?: boolean
  record_id?: string
}

/** Consume the existing admin question-test SSE endpoint, including its final record event. */
export async function streamAccountQuestion(
  accountId: number,
  model: string,
  prompt: string,
  signal: AbortSignal,
  onEvent: (event: AccountQuestionEvent) => void,
): Promise<void> {
  const response = await fetch(buildApiUrl(`/admin/accounts/${accountId}/test`), {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${localStorage.getItem('auth_token')}`,
      'Content-Type': 'application/json',
      [ADMIN_UI_REQUEST_HEADER]: '1',
    },
    body: JSON.stringify({ model_id: model, prompt, mode: 'question' }),
    signal,
  })
  if (!response.ok) {
    const body = await response.json().catch(() => null)
    throw new Error(body?.message || `HTTP ${response.status}`)
  }
  const reader = response.body?.getReader()
  if (!reader) throw new Error('ACCOUNT_TEST_INCOMPLETE')
  const cancelReader = () => { void reader.cancel().catch(() => undefined) }
  signal.addEventListener('abort', cancelReader, { once: true })
  const decoder = new TextDecoder()
  let buffer = ''
  let data: string[] = []
  let terminal = false
  const dispatch = () => {
    if (signal.aborted || !data.length) { data = []; return }
    const raw = data.join('\n')
    data = []
    if (raw === '[DONE]') return
    const event: AccountQuestionEvent = JSON.parse(raw)
    if (event.type === 'test_complete' || event.type === 'error') terminal = true
    onEvent(event)
  }
  const line = (value: string) => {
    const text = value.replace(/\r$/, '')
    if (!text) dispatch()
    else if (text.startsWith('data:')) data.push(text.slice(5).replace(/^ /, ''))
  }
  try {
    while (!signal.aborted) {
      const { done, value } = await reader.read()
      if (signal.aborted) break
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true })
      if (buffer.length > 1024 * 1024) throw new Error('Account test event too large')
      let newline: number
      while ((newline = buffer.indexOf('\n')) >= 0) {
        line(buffer.slice(0, newline))
        buffer = buffer.slice(newline + 1)
      }
      if (done) {
        if (buffer) line(buffer)
        dispatch()
        break
      }
    }
    if (signal.aborted) throw new DOMException('Aborted', 'AbortError')
    if (!terminal) throw new Error('ACCOUNT_TEST_INCOMPLETE')
  } finally {
    signal.removeEventListener('abort', cancelReader)
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
