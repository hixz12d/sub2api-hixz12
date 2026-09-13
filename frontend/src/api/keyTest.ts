import { buildGatewayUrl } from './url'

export class KeyTestError extends Error {
  constructor(public readonly code: 'http' | 'invalidResponse' | 'incomplete' | 'empty' | 'tooLong', message = '') {
    super(message)
  }
}

function headers(apiKey: string) {
  return { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json' }
}

async function checkResponse(response: Response) {
  if (response.ok) return
  let message = `HTTP ${response.status}`
  try {
    const payload = await response.json()
    if (typeof payload?.error?.message === 'string') message += `: ${payload.error.message}`
  } catch { /* A proxy may return a non-JSON error page. */ }
  throw new KeyTestError('http', message)
}

export async function listKeyTestModels(apiKey: string, signal: AbortSignal): Promise<string[]> {
  const response = await fetch(buildGatewayUrl('/v1/models'), {
    headers: headers(apiKey), signal, cache: 'no-store', credentials: 'omit', redirect: 'error'
  })
  await checkResponse(response)
  const payload = await response.json()
  if (!Array.isArray(payload?.data)) throw new KeyTestError('invalidResponse')
  const models = payload.data
    .map((model: { id?: unknown }) => model?.id)
    .filter((id: unknown): id is string => typeof id === 'string' && id.length > 0)
    .filter((id: string) => !/(?:image|embedding|whisper|tts|realtime|audio|video|sora|dall-e)/i.test(id))
  return [...new Set<string>(models)]
}

// Use the public gateway with the user's own key so routing, billing and limits
// are identical to a normal API call. No administrator test endpoint is involved.
export async function runKeyQuestion(
  apiKey: string,
  model: string,
  prompt: string,
  signal: AbortSignal,
  onText: (text: string) => void
): Promise<{ truncated: boolean }> {
  const response = await fetch(buildGatewayUrl('/v1/chat/completions'), {
    method: 'POST',
    headers: headers(apiKey),
    credentials: 'omit',
    redirect: 'error',
    signal,
    body: JSON.stringify({
      model,
      messages: [{ role: 'user', content: prompt }],
      tools: [],
      stream: true
    })
  })
  await checkResponse(response)
  if (!response.headers.get('content-type')?.includes('text/event-stream')) {
    throw new KeyTestError('invalidResponse')
  }
  const reader = response.body?.getReader()
  if (!reader) throw new KeyTestError('invalidResponse')
  const decoder = new TextDecoder()
  let buffer = ''
  let data: string[] = []
  let finished = false
  let doneMarker = false
  let truncated = false
  let textLength = 0

  const consumeEvent = () => {
    if (!data.length) return
    const raw = data.join('\n')
    data = []
    if (raw.trim() === '[DONE]') {
      doneMarker = true
      return
    }
    let event
    try { event = JSON.parse(raw) } catch { throw new KeyTestError('invalidResponse') }
    if (event.error) {
      throw new KeyTestError('http', typeof event.error.message === 'string' ? event.error.message : '')
    }
    const choice = event.choices?.[0]
    if (typeof choice?.delta?.content === 'string') {
      textLength += choice.delta.content.length
      if (textLength > 200_000) throw new KeyTestError('tooLong')
      onText(choice.delta.content)
    }
    if (choice?.finish_reason) {
      if (!['stop', 'length'].includes(choice.finish_reason)) throw new KeyTestError('incomplete')
      finished = true
      truncated = choice.finish_reason === 'length'
    }
  }
  const consumeLine = (line: string) => {
    line = line.replace(/\r$/, '')
    if (!line) consumeEvent()
    else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
  }
  try {
    while (!doneMarker) {
      const { done, value } = await reader.read()
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true })
      let newline: number
      while ((newline = buffer.indexOf('\n')) >= 0) {
        consumeLine(buffer.slice(0, newline))
        buffer = buffer.slice(newline + 1)
      }
      // Bound malformed frames even when the server never sends a newline.
      if (buffer.length + data.reduce((size, line) => size + line.length, 0) > 1_000_000) {
        throw new KeyTestError('invalidResponse')
      }
      if (done) {
        if (buffer) consumeLine(buffer)
        consumeEvent()
        break
      }
    }
    if (!finished && !doneMarker) throw new KeyTestError('incomplete')
    if (!textLength) throw new KeyTestError('empty')
    return { truncated }
  } finally {
    await reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}
