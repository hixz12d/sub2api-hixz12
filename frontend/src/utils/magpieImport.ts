import type { GroupPlatform } from '@/types'

export interface MagpieImportLinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  providerName: string
  apiKey: string
}

/** Magpie 名称上限 80 字符。 */
const MAGPIE_NAME_MAX_LENGTH = 80
const MAGPIE_ID_MAX_LENGTH = 32
const MAGPIE_ID_FALLBACK = 'sub2api'

function normalizeRootUrl(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '').replace(/\/v1$/, '')
}

function slugifyMagpieId(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, MAGPIE_ID_MAX_LENGTH)
    .replace(/-+$/, '')
}

/** 取域名里有辨识度的一段：sub2api.example.com → sub2api，api.acme.com → acme，IP 整段保留。 */
function hostLabelForId(root: string): string {
  let host: string
  try {
    host = new URL(root).hostname
  } catch {
    return ''
  }
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(host)) return host
  const labels = host.split('.').filter(Boolean)
  while (labels.length > 2 && /^(www|api)$/i.test(labels[0])) labels.shift()
  return labels[0] || ''
}

/**
 * Magpie 的 id（agent 里写成 `id/model`）不传时由名称推出，只认英文字母和数字；
 * 名称全是中文（如“小破站”）时推不出来，导入会被拒绝。所以总是显式给出 id：
 * 名称是纯英文时按名称生成（与 Magpie 默认一致），否则用 API 地址的域名生成。
 */
function buildMagpieId(name: string, root: string): string {
  const fromName = /^[\x20-\x7e]+$/.test(name) ? slugifyMagpieId(name) : ''
  return fromName || slugifyMagpieId(hostLabelForId(root)) || MAGPIE_ID_FALLBACK
}

/**
 * 生成 Magpie 导入链接（https://usemagpie.ai/import# 形式）。
 * 参数放在 # 后面，浏览器不会把 Key 发给 usemagpie.ai；没装 Magpie 时该页会提示下载。
 *
 * 只填该分组平台真正支持的入口，其余格式由 Magpie 本地转换：
 * - gemini：/v1/chat/completions、/v1/messages、/v1beta（/v1/responses 仅多协议直通账号可用，不填）
 * - antigravity：只有 /antigravity/v1/messages 与 /antigravity/v1beta
 * - 其他（anthropic、openai、grok、composite 等）：chat、responses、anthropic 三种都支持
 * 不传 models，Magpie 会读 /v1/models（按分组返回可用模型）。
 */
export function buildMagpieImportLink(input: MagpieImportLinkInput): string {
  const root = normalizeRootUrl(input.baseUrl)
  const platform = input.platform || 'anthropic'
  const name = (input.providerName.trim() || 'sub2api').slice(0, MAGPIE_NAME_MAX_LENGTH)

  const entries: [string, string][] = [
    ['name', name],
    ['id', buildMagpieId(name, root)]
  ]

  switch (platform) {
    case 'antigravity':
      entries.push(['anthropic', `${root}/antigravity`])
      entries.push(['gemini', `${root}/antigravity/v1beta`])
      break
    case 'gemini':
      entries.push(['chat', `${root}/v1`])
      entries.push(['anthropic', root])
      entries.push(['gemini', `${root}/v1beta`])
      break
    default:
      entries.push(['chat', `${root}/v1`])
      entries.push(['responses', `${root}/v1`])
      entries.push(['anthropic', root])
      break
  }

  entries.push(['key', input.apiKey])
  entries.push(['website', root])
  entries.push(['balance', `${root}/v1/usage`])
  entries.push(['balance_path', 'remaining'])

  return `https://usemagpie.ai/import#${new URLSearchParams(entries).toString()}`
}
