import type { GroupPlatform } from '@/types'

export interface MagpieImportLinkInput {
  baseUrl: string
  platform?: GroupPlatform | null
  providerName: string
  apiKey: string
}

/** Magpie 名称上限 80 字符。 */
const MAGPIE_NAME_MAX_LENGTH = 80

function normalizeRootUrl(baseUrl: string): string {
  return baseUrl.replace(/\/+$/, '').replace(/\/v1$/, '')
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

  const entries: [string, string][] = [['name', name]]

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
