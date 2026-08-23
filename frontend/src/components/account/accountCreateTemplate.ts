import type { AccountPlatform, AccountType, AdminGroup, Proxy } from '@/types'
import {
  OPENAI_WS_MODE_OFF,
  type OpenAIWSMode,
} from '@/utils/openaiWsMode'
import type { OpenAICompactMode } from '@/types'

export const ACCOUNT_CREATE_TEMPLATE_NONE = ''

export type CodexFingerprintMode = 'off' | 'device' | 'session' | 'window' | 'window40' | 'full'

export interface AccountCreateTemplateValues {
  proxy_id: number | null
  concurrency: number
  load_factor: number | null
  priority: number
  rate_multiplier: number
  group_ids: number[]
  quota_limit: number | null
  quota_daily_limit: number | null
  quota_weekly_limit: number | null
  auto_pause_on_expired: boolean
  intercept_warmup: boolean
  openai_passthrough: boolean
  openai_flatten_namespaces: boolean
  openai_long_context_billing: boolean
  openai_ws_mode: OpenAIWSMode
  openai_compact_mode: OpenAICompactMode
  codex_cli_only: boolean
  codex_cli_only_app_server: boolean
  codex_fingerprint_mode: CodexFingerprintMode
  tls_fingerprint_enabled: boolean
  tls_fingerprint_profile_id: number | null
}

export interface AccountCreateTemplate {
  id: string
  name: string
  platform: AccountPlatform
  type: AccountType
  is_default: boolean
  include_groups: boolean
  values: AccountCreateTemplateValues
}

export interface AccountCreateTemplateWritePayload {
  name: string
  platform: AccountPlatform
  type: AccountType
  is_default: boolean
  include_groups: boolean
  values: AccountCreateTemplateValues
}

const WS_MODES = new Set<OpenAIWSMode>(['off', 'ctx_pool', 'passthrough', 'http_bridge'])
const FINGERPRINT_MODES = new Set<CodexFingerprintMode>(['off', 'device', 'session', 'window', 'window40', 'full'])
const COMPACT_MODES = new Set<OpenAICompactMode>(['auto', 'force_on', 'force_off'])

export function emptyAccountCreateTemplateValues(): AccountCreateTemplateValues {
  return {
    proxy_id: null,
    concurrency: 10,
    load_factor: null,
    priority: 1,
    rate_multiplier: 1,
    group_ids: [],
    quota_limit: null,
    quota_daily_limit: null,
    quota_weekly_limit: null,
    auto_pause_on_expired: true,
    intercept_warmup: false,
    openai_passthrough: false,
    openai_flatten_namespaces: false,
    openai_long_context_billing: false,
    openai_ws_mode: OPENAI_WS_MODE_OFF,
    openai_compact_mode: 'auto',
    codex_cli_only: false,
    codex_cli_only_app_server: false,
    codex_fingerprint_mode: 'off',
    tls_fingerprint_enabled: false,
    tls_fingerprint_profile_id: null,
  }
}

export function normalizeAccountCreateTemplateValues(
  input?: Partial<AccountCreateTemplateValues> | null,
): AccountCreateTemplateValues {
  const base = emptyAccountCreateTemplateValues()
  if (!input) return base
  const concurrency = toPositiveInt(input.concurrency, base.concurrency)
  const priority = toPositiveInt(input.priority, base.priority)
  const wsMode = typeof input.openai_ws_mode === 'string' && WS_MODES.has(input.openai_ws_mode)
    ? input.openai_ws_mode
    : base.openai_ws_mode
  const compactMode = typeof input.openai_compact_mode === 'string' && COMPACT_MODES.has(input.openai_compact_mode)
    ? input.openai_compact_mode
    : base.openai_compact_mode
  const fingerprintMode = typeof input.codex_fingerprint_mode === 'string' && FINGERPRINT_MODES.has(input.codex_fingerprint_mode)
    ? input.codex_fingerprint_mode
    : base.codex_fingerprint_mode
  return {
    proxy_id: toOptionalPositiveId(input.proxy_id),
    concurrency,
    load_factor: toOptionalPositiveId(input.load_factor),
    priority,
    rate_multiplier: toNonNegativeNumber(input.rate_multiplier, base.rate_multiplier),
    group_ids: uniquePositiveIds(input.group_ids),
    quota_limit: toOptionalPositiveNumber(input.quota_limit),
    quota_daily_limit: toOptionalPositiveNumber(input.quota_daily_limit),
    quota_weekly_limit: toOptionalPositiveNumber(input.quota_weekly_limit),
    auto_pause_on_expired: input.auto_pause_on_expired !== false,
    intercept_warmup: input.intercept_warmup === true,
    openai_passthrough: input.openai_passthrough === true,
    openai_flatten_namespaces: input.openai_flatten_namespaces === true,
    openai_long_context_billing: input.openai_long_context_billing === true,
    openai_ws_mode: wsMode,
    openai_compact_mode: compactMode,
    codex_cli_only: input.codex_cli_only === true,
    codex_cli_only_app_server: input.codex_cli_only_app_server === true,
    codex_fingerprint_mode: fingerprintMode,
    tls_fingerprint_enabled: input.tls_fingerprint_enabled === true,
    tls_fingerprint_profile_id: normalizeTlsProfileId(input.tls_fingerprint_profile_id, input.tls_fingerprint_enabled === true),
  }
}

export function filterAccountCreateTemplates(
  items: AccountCreateTemplate[],
  platform: AccountPlatform,
  accountType: AccountType,
): AccountCreateTemplate[] {
  return items.filter((item) => item.platform === platform && item.type === accountType)
}

export function findDefaultAccountCreateTemplate(
  items: AccountCreateTemplate[],
  platform: AccountPlatform,
  accountType: AccountType,
): AccountCreateTemplate | null {
  return filterAccountCreateTemplates(items, platform, accountType).find((item) => item.is_default) ?? null
}

export function applyAccountCreateTemplateGroups(
  currentGroupIds: number[],
  values: AccountCreateTemplateValues,
  includeGroups: boolean,
): number[] {
  if (!includeGroups) return [...currentGroupIds]
  return [...values.group_ids]
}

export function buildAccountCreateTemplatePreview(
  values: AccountCreateTemplateValues,
  options: {
    includeGroups: boolean
    proxies?: Array<Pick<Proxy, 'id' | 'name'>>
    groups?: Array<Pick<AdminGroup, 'id' | 'name'>>
  } = {},
): string {
  const parts = [
    `代理=${resolveProxyLabel(values.proxy_id, options.proxies)}`,
    `WS=${values.openai_ws_mode}`,
    `指纹=${values.codex_fingerprint_mode}`,
    `并发=${values.concurrency}`,
  ]
  if (options.includeGroups) {
    parts.push(`分组=${resolveGroupLabel(values.group_ids, options.groups)}`)
  }
  return parts.join(' · ')
}

function resolveProxyLabel(
  proxyId: number | null,
  proxies?: Array<Pick<Proxy, 'id' | 'name'>>,
): string {
  if (proxyId == null) return '直连'
  const found = proxies?.find((item) => item.id === proxyId)
  return found?.name || `#${proxyId}`
}

function resolveGroupLabel(
  groupIds: number[],
  groups?: Array<Pick<AdminGroup, 'id' | 'name'>>,
): string {
  if (groupIds.length === 0) return '无'
  return groupIds
    .map((id) => groups?.find((item) => item.id === id)?.name || `#${id}`)
    .join(',')
}

function toPositiveInt(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 1
    ? Math.trunc(value)
    : fallback
}

function toOptionalPositiveId(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value >= 1
    ? Math.trunc(value)
    : null
}

function toOptionalPositiveNumber(value: unknown): number | null {
  return typeof value === 'number' && Number.isFinite(value) && value > 0 ? value : null
}

function toNonNegativeNumber(value: unknown, fallback: number): number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : fallback
}

function uniquePositiveIds(value: unknown): number[] {
  if (!Array.isArray(value)) return []
  const seen = new Set<number>()
  const out: number[] = []
  for (const item of value) {
    if (typeof item !== 'number' || !Number.isFinite(item) || item < 1) continue
    const id = Math.trunc(item)
    if (seen.has(id)) continue
    seen.add(id)
    out.push(id)
  }
  return out
}

function normalizeTlsProfileId(value: unknown, enabled: boolean): number | null {
  if (!enabled) return null
  if (typeof value !== 'number' || !Number.isFinite(value)) return null
  const id = Math.trunc(value)
  if (id === -1 || id > 0) return id
  return null
}
