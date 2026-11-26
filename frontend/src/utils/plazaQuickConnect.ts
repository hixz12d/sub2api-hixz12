/**
 * Non-invasive CLI / SDK quick-connect snippet generator for Model Plaza.
 * Pure functions that accept platform, modelName, baseUrl, and apiKey and produce
 * ready-to-paste configurations without touching UseKeyModal.vue.
 */

import { buildCodexApiKeyAuthConfig } from '@/utils/codexAuthConfig'

export const PLACEHOLDER_API_KEY = '<YOUR_API_KEY>'

export type QuickConnectClientId =
  | 'codex'
  | 'claude-code'
  | 'opencode'
  | 'gemini-cli'
  | 'openai-sdk'
  | 'curl'

export type QuickConnectLabelKey =
  | 'modelPlaza.quickConnect.clientCodex'
  | 'modelPlaza.quickConnect.clientClaudeCode'
  | 'modelPlaza.quickConnect.clientOpenCode'
  | 'modelPlaza.quickConnect.clientGeminiCli'
  | 'modelPlaza.quickConnect.clientOpenAISdk'
  | 'modelPlaza.quickConnect.clientCurl'

export interface QuickConnectSnippet {
  id: QuickConnectClientId
  labelKey: QuickConnectLabelKey
  filename: string
  language: 'toml' | 'bash' | 'json' | 'typescript'
  code: string
}

export interface QuickConnectInput {
  platform: string
  modelName: string
  baseUrl: string
  apiKey?: string | null
}

function normalizeBaseRoot(rawUrl: string): string {
  const trimmed = rawUrl.trim().replace(/\/+$/, '')
  if (!trimmed) {
    return 'https://api.example.com'
  }
  return trimmed.replace(/\/(v1beta|v1)$/i, '').replace(/\/+$/, '')
}

function escapeTomlString(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}

function escapeShellDoubleQuote(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\$/g, '\\$').replace(/`/g, '\\`')
}

function escapeShellSingleQuote(value: string): string {
  return value.replace(/'/g, "'\\''")
}

type ResolvedProtocolFamily = 'openai' | 'anthropic' | 'gemini' | 'openai-compatible'

function resolveProtocolFamily(platform: string, modelName: string): ResolvedProtocolFamily {
  const normalizedPlatform = platform.trim().toLowerCase()
  const normalizedModel = modelName.trim().toLowerCase()

  if (normalizedPlatform === 'openai') {
    return 'openai'
  }
  if (normalizedPlatform === 'anthropic') {
    return 'anthropic'
  }
  if (normalizedPlatform === 'gemini') {
    return 'gemini'
  }
  if (normalizedPlatform === 'antigravity') {
    return normalizedModel.includes('gemini') ? 'gemini' : 'anthropic'
  }
  if (normalizedPlatform === 'composite') {
    if (normalizedModel.includes('claude')) return 'anthropic'
    if (normalizedModel.includes('gemini')) return 'gemini'
    if (
      normalizedModel.startsWith('gpt') ||
      normalizedModel.startsWith('o1') ||
      normalizedModel.startsWith('o3') ||
      normalizedModel.startsWith('o4') ||
      normalizedModel.includes('codex')
    ) {
      return 'openai'
    }
  }
  return 'openai-compatible'
}

function resolvePlatformRoot(baseRoot: string, platform: string): string {
  return platform.trim().toLowerCase() === 'antigravity' ? `${baseRoot}/antigravity` : baseRoot
}

function buildCodexSnippet(apiBase: string, modelName: string, apiKey: string): QuickConnectSnippet {
  const escapedModel = escapeTomlString(modelName)
  const escapedBase = escapeTomlString(apiBase)
  const authBlock = buildCodexApiKeyAuthConfig(apiKey)

  const code = `model_provider = "OpenAI"
model = "${escapedModel}"
review_model = "${escapedModel}"
disable_response_storage = true

[model_providers.OpenAI]
name = "OpenAI"
base_url = "${escapedBase}"
wire_api = "responses"
${authBlock}`

  return {
    id: 'codex',
    labelKey: 'modelPlaza.quickConnect.clientCodex',
    filename: '~/.codex/config.toml',
    language: 'toml',
    code
  }
}

function buildClaudeCodeSnippet(anthropicRoot: string, modelName: string, apiKey: string): QuickConnectSnippet {
  const code = `export ANTHROPIC_BASE_URL="${escapeShellDoubleQuote(anthropicRoot)}"
export ANTHROPIC_AUTH_TOKEN="${escapeShellDoubleQuote(apiKey)}"
export ANTHROPIC_MODEL="${escapeShellDoubleQuote(modelName)}"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`

  return {
    id: 'claude-code',
    labelKey: 'modelPlaza.quickConnect.clientClaudeCode',
    filename: 'Terminal',
    language: 'bash',
    code
  }
}

function buildGeminiCliSnippet(geminiRoot: string, modelName: string, apiKey: string): QuickConnectSnippet {
  const code = `export GOOGLE_GEMINI_BASE_URL="${escapeShellDoubleQuote(geminiRoot)}"
export GEMINI_API_KEY="${escapeShellDoubleQuote(apiKey)}"
export GEMINI_MODEL="${escapeShellDoubleQuote(modelName)}"`

  return {
    id: 'gemini-cli',
    labelKey: 'modelPlaza.quickConnect.clientGeminiCli',
    filename: 'Terminal',
    language: 'bash',
    code
  }
}

function buildOpenCodeSnippet(
  family: ResolvedProtocolFamily,
  platformRoot: string,
  modelName: string,
  apiKey: string
): QuickConnectSnippet {
  let providerKey = 'openai'
  let npmPackage: string | undefined
  let providerName: string | undefined
  let baseURL = `${platformRoot}/v1`

  if (family === 'anthropic') {
    providerKey = 'anthropic'
    npmPackage = '@ai-sdk/anthropic'
    baseURL = `${platformRoot}/v1`
  } else if (family === 'gemini') {
    providerKey = 'google'
    npmPackage = '@ai-sdk/google'
    baseURL = `${platformRoot}/v1beta`
  } else if (family === 'openai-compatible') {
    providerKey = 'sub2api'
    npmPackage = '@ai-sdk/openai-compatible'
    providerName = 'Sub2API'
    baseURL = `${platformRoot}/v1`
  }

  const providerConfig: Record<string, unknown> = {
    ...(npmPackage ? { npm: npmPackage } : {}),
    ...(providerName ? { name: providerName } : {}),
    options: {
      baseURL,
      apiKey
    },
    models: {
      [modelName]: {
        name: modelName
      }
    }
  }

  const payload: Record<string, unknown> = {
    $schema: 'https://opencode.ai/config.json',
    provider: {
      [providerKey]: providerConfig
    },
    ...(family === 'openai'
      ? {
          agent: {
            build: { options: { store: false } },
            plan: { options: { store: false } }
          }
        }
      : {})
  }

  return {
    id: 'opencode',
    labelKey: 'modelPlaza.quickConnect.clientOpenCode',
    filename: 'opencode.json',
    language: 'json',
    code: JSON.stringify(payload, null, 2)
  }
}

function buildOpenAISdkSnippet(apiBase: string, modelName: string, apiKey: string): QuickConnectSnippet {
  const code = `import OpenAI from 'openai'

const client = new OpenAI({
  baseURL: ${JSON.stringify(apiBase)},
  apiKey: ${JSON.stringify(apiKey)}
})

const response = await client.chat.completions.create({
  model: ${JSON.stringify(modelName)},
  messages: [{ role: 'user', content: 'Hello' }]
})

console.log(response.choices[0]?.message?.content)`

  return {
    id: 'openai-sdk',
    labelKey: 'modelPlaza.quickConnect.clientOpenAISdk',
    filename: 'example.ts',
    language: 'typescript',
    code
  }
}

function buildCurlSnippet(
  family: ResolvedProtocolFamily,
  platformRoot: string,
  modelName: string,
  apiKey: string
): QuickConnectSnippet {
  if (family === 'anthropic') {
    const endpoint = `${platformRoot}/v1/messages`
    const payload = JSON.stringify(
      {
        model: modelName,
        max_tokens: 1024,
        messages: [{ role: 'user', content: 'Hello' }]
      },
      null,
      2
    )
    const code = `curl -X POST "${escapeShellDoubleQuote(endpoint)}" \\
  -H "x-api-key: ${escapeShellDoubleQuote(apiKey)}" \\
  -H "anthropic-version: 2023-06-01" \\
  -H "content-type: application/json" \\
  -d '${escapeShellSingleQuote(payload)}'`
    return {
      id: 'curl',
      labelKey: 'modelPlaza.quickConnect.clientCurl',
      filename: 'cURL (/v1/messages)',
      language: 'bash',
      code
    }
  }

  if (family === 'gemini') {
    const endpoint = `${platformRoot}/v1beta/models/${encodeURIComponent(modelName)}:generateContent`
    const payload = JSON.stringify(
      {
        contents: [{ parts: [{ text: 'Hello' }] }]
      },
      null,
      2
    )
    const code = `curl -X POST "${escapeShellDoubleQuote(endpoint)}" \\
  -H "x-goog-api-key: ${escapeShellDoubleQuote(apiKey)}" \\
  -H "Content-Type: application/json" \\
  -d '${escapeShellSingleQuote(payload)}'`
    return {
      id: 'curl',
      labelKey: 'modelPlaza.quickConnect.clientCurl',
      filename: 'cURL (:generateContent)',
      language: 'bash',
      code
    }
  }

  if (family === 'openai') {
    const endpoint = `${platformRoot}/v1/responses`
    const payload = JSON.stringify(
      {
        model: modelName,
        input: 'Hello'
      },
      null,
      2
    )
    const code = `curl -X POST "${escapeShellDoubleQuote(endpoint)}" \\
  -H "Authorization: Bearer ${escapeShellDoubleQuote(apiKey)}" \\
  -H "Content-Type: application/json" \\
  -d '${escapeShellSingleQuote(payload)}'`
    return {
      id: 'curl',
      labelKey: 'modelPlaza.quickConnect.clientCurl',
      filename: 'cURL (/v1/responses)',
      language: 'bash',
      code
    }
  }

  const endpoint = `${platformRoot}/v1/chat/completions`
  const payload = JSON.stringify(
    {
      model: modelName,
      messages: [{ role: 'user', content: 'Hello' }]
    },
    null,
    2
  )
  const code = `curl -X POST "${escapeShellDoubleQuote(endpoint)}" \\
  -H "Authorization: Bearer ${escapeShellDoubleQuote(apiKey)}" \\
  -H "Content-Type: application/json" \\
  -d '${escapeShellSingleQuote(payload)}'`
  return {
    id: 'curl',
    labelKey: 'modelPlaza.quickConnect.clientCurl',
    filename: 'cURL (/v1/chat/completions)',
    language: 'bash',
    code
  }
}

/**
 * Generates ordered client configuration snippets tailored to the model's platform.
 */
export function buildQuickConnectSnippets(input: QuickConnectInput): QuickConnectSnippet[] {
  const modelName = input.modelName.trim()
  const apiKey = input.apiKey?.trim() || PLACEHOLDER_API_KEY
  const baseRoot = normalizeBaseRoot(input.baseUrl)
  const platformRoot = resolvePlatformRoot(baseRoot, input.platform)
  const apiBase = `${platformRoot}/v1`
  const family = resolveProtocolFamily(input.platform, modelName)

  if (family === 'openai') {
    return [
      buildCodexSnippet(apiBase, modelName, apiKey),
      buildOpenCodeSnippet('openai', platformRoot, modelName, apiKey),
      buildCurlSnippet('openai', platformRoot, modelName, apiKey)
    ]
  }

  if (family === 'anthropic') {
    return [
      buildClaudeCodeSnippet(platformRoot, modelName, apiKey),
      buildOpenCodeSnippet('anthropic', platformRoot, modelName, apiKey),
      buildCurlSnippet('anthropic', platformRoot, modelName, apiKey)
    ]
  }

  if (family === 'gemini') {
    return [
      buildGeminiCliSnippet(platformRoot, modelName, apiKey),
      buildOpenCodeSnippet('gemini', platformRoot, modelName, apiKey),
      buildCurlSnippet('gemini', platformRoot, modelName, apiKey)
    ]
  }

  return [
    buildOpenAISdkSnippet(apiBase, modelName, apiKey),
    buildOpenCodeSnippet('openai-compatible', platformRoot, modelName, apiKey),
    buildCurlSnippet('openai-compatible', platformRoot, modelName, apiKey)
  ]
}
