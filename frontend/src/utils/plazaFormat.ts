/**
 * Shared pure formatters, vendor/icon helpers, and tone utilities for the Model Plaza views.
 * Model Plaza pricing is strictly denominated in RMB (¥) at 1R = 1 baseline × group multiplier,
 * with trailing zeros stripped and zero FX conversion.
 */

import type {
  PlazaConsistencyState,
  PlazaMetricSampleState,
  PlazaModelMetadata,
  PlazaOutputTpsState
} from '@/api/modelPlaza'
import { formatScaled } from '@/utils/pricing'

export const PLAZA_EMPTY_TEXT = '—'
export const PLAZA_CURRENCY_SYMBOL = '¥'

export type PlazaMetricTone = 'ok' | 'warn' | 'bad' | 'neutral' | 'muted'

export interface PlazaFormattedMetric {
  text: string
  tone: PlazaMetricTone
  valid: boolean
}

export type PlazaTranslateFn = (key: string, params?: Record<string, unknown>) => string

export const KNOWN_CAPABILITY_KEYS: ReadonlySet<string> = new Set([
  'reasoning',
  'tools',
  'vision',
  'audio',
  'image',
  'video',
  'prompt_caching',
  'web_search',
  'long_context',
  'structured_outputs',
  'code',
  'embedding',
  'realtime'
])

export const KNOWN_VENDOR_KEYS: ReadonlySet<string> = new Set([
  'all',
  'openai',
  'anthropic',
  'google',
  'xai',
  'deepseek',
  'qwen',
  'zhipu',
  'meta',
  'mistral',
  'moonshot',
  'minimax',
  'doubao',
  'cohere',
  'other'
])

/**
 * Infers a normalized vendor key from explicit metadata vendor, model name, or platform.
 */
export function inferVendorKey(
  modelName: string,
  platform?: string | null,
  vendor?: string | null
): string {
  const metaVendor = vendor?.trim().toLowerCase()
  if (metaVendor) return metaVendor

  const name = (modelName || '').trim().toLowerCase()
  if (name.includes('claude')) return 'anthropic'
  if (
    name.startsWith('gpt') ||
    name.startsWith('o1') ||
    name.startsWith('o3') ||
    name.startsWith('o4') ||
    name.startsWith('codex') ||
    name.includes('chatgpt') ||
    name.includes('dall-e') ||
    name.includes('whisper') ||
    name.includes('tts-') ||
    name.includes('text-embedding') ||
    name.includes('text-moderation')
  ) {
    return 'openai'
  }
  if (
    name.includes('gemini') ||
    name.includes('imagen') ||
    name.includes('veo') ||
    name.includes('gemma')
  ) {
    return 'google'
  }
  if (name.includes('grok')) return 'xai'
  if (name.includes('deepseek')) return 'deepseek'
  if (name.includes('qwen') || name.includes('qwq')) return 'qwen'
  if (name.includes('glm')) return 'zhipu'
  if (name.includes('llama')) return 'meta'
  if (name.includes('mistral') || name.includes('mixtral') || name.includes('codestral')) {
    return 'mistral'
  }
  if (name.includes('kimi') || name.includes('moonshot')) return 'moonshot'
  if (name.includes('minimax') || name.includes('abab')) return 'minimax'
  if (name.includes('doubao')) return 'doubao'
  if (name.includes('command')) return 'cohere'

  const normPlatform = platform?.trim().toLowerCase()
  switch (normPlatform) {
    case 'anthropic':
      return 'anthropic'
    case 'openai':
      return 'openai'
    case 'gemini':
    case 'antigravity':
      return 'google'
    case 'grok':
      return 'xai'
    case 'deepseek':
      return 'deepseek'
    case 'qwen':
      return 'qwen'
    case 'zhipu':
      return 'zhipu'
    case 'kimi':
    case 'moonshot':
      return 'moonshot'
    case 'minimax':
      return 'minimax'
    default:
      return 'other'
  }
}

/**
 * Returns true for monochrome dark SVG model icons that require `dark:invert` in dark mode.
 */
export function needsDarkInvert(modelName: string): boolean {
  const lower = (modelName || '').trim().toLowerCase()
  return (
    lower.startsWith('gpt') ||
    lower.startsWith('o1') ||
    lower.startsWith('o3') ||
    lower.startsWith('o4') ||
    lower.startsWith('codex') ||
    lower.includes('chatgpt') ||
    lower.includes('dall-e') ||
    lower.includes('whisper') ||
    lower.includes('tts-1') ||
    lower.includes('text-embedding-3') ||
    lower.includes('text-moderation') ||
    lower.includes('grok') ||
    lower.includes('moonshot') ||
    lower.includes('kimi') ||
    lower.includes('midjourney') ||
    lower.includes('mj_') ||
    lower.includes('jina') ||
    lower.includes('suno') ||
    lower.includes('ollama')
  )
}

/**
 * Formats a Model Plaza price strictly in RMB (¥) without trailing zeros (minFractionDigits = 0).
 *   formatPlazaPrice(4e-7, 1_000_000)   -> "¥0.4"
 *   formatPlazaPrice(2e-6, 1_000_000)   -> "¥2"
 *   formatPlazaPrice(1.5e-8, 1_000_000) -> "¥0.015"
 *   formatPlazaPrice(null, 1_000_000)   -> "—"
 */
export function formatPlazaPrice(
  value: number | null | undefined,
  scale = 1_000_000
): string {
  if (value == null || !Number.isFinite(value)) {
    return PLAZA_EMPTY_TEXT
  }
  return formatScaled(value, scale, 0, PLAZA_CURRENCY_SYMBOL)
}

/** Maps a semantic tone to its DESIGN.md Tailwind text color class. */
export function plazaToneTextClass(tone: PlazaMetricTone): string {
  switch (tone) {
    case 'ok':
      return 'text-ok'
    case 'warn':
      return 'text-warn'
    case 'bad':
      return 'text-bad'
    case 'neutral':
      return 'text-ink-2'
    case 'muted':
    default:
      return 'text-ink-3'
  }
}

export const PLAZA_SLA_OK_THRESHOLD = 0.97
export const PLAZA_SLA_WARN_THRESHOLD = 0.90

/**
 * Formats an SLA ratio (0~1) with 2 decimals by default (e.g. "99.85%") and status tone:
 * - >= 0.97 (PLAZA_SLA_OK_THRESHOLD) -> 'ok'
 * - 0.90 ~ <0.97 (PLAZA_SLA_WARN_THRESHOLD) -> 'warn'
 * - < 0.90 -> 'bad'
 * - low_sample / no_data / null -> '—' ('muted')
 */
export function formatSlaPercent(
  val: number | null | undefined,
  state?: PlazaMetricSampleState | null,
  decimals = 2
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || val == null || !Number.isFinite(val)) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const clamped = Math.max(0, Math.min(1, val))
  const pct = clamped * 100
  const text = pct >= 100 ? '100%' : `${pct.toFixed(decimals)}%`

  let tone: PlazaMetricTone = 'bad'
  if (clamped >= PLAZA_SLA_OK_THRESHOLD) {
    tone = 'ok'
  } else if (clamped >= PLAZA_SLA_WARN_THRESHOLD) {
    tone = 'warn'
  }
  return { text, tone, valid: true }
}

/**
 * Formats Time To First Token (milliseconds):
 * - < 1000ms -> e.g. "820ms"; >= 1000ms -> e.g. "1.2s"
 * - < 1500ms -> 'ok' highlight
 * - 1500ms ~ 15000ms -> 'neutral' (avoids mislabeling reasoning models as degraded)
 * - > 15000ms -> 'warn'
 */
export function formatTtft(
  ms: number | null | undefined,
  state?: PlazaMetricSampleState | null
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || ms == null || !Number.isFinite(ms) || ms < 0) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const text = ms < 1000 ? `${Math.round(ms)}ms` : `${(ms / 1000).toFixed(1)}s`

  let tone: PlazaMetricTone = 'neutral'
  if (ms < 1500) {
    tone = 'ok'
  } else if (ms > 15000) {
    tone = 'warn'
  }
  return { text, tone, valid: true }
}

/**
 * Formats compact latency for Model Plaza cards:
 * - < 1000ms -> e.g. "820ms"
 * - >= 1000ms -> rounded whole seconds, e.g. "9s"
 */
export function formatCompactLatency(
  ms: number | null | undefined,
  state?: PlazaMetricSampleState | null
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || ms == null || !Number.isFinite(ms) || ms < 0) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const text = ms < 1000 ? `${Math.round(ms)}ms` : `${Math.round(ms / 1000)}s`

  let tone: PlazaMetricTone = 'neutral'
  if (ms < 1500) {
    tone = 'ok'
  } else if (ms > 15000) {
    tone = 'warn'
  }
  return { text, tone, valid: true }
}

/**
 * Formats visible output speed (tokens/sec).
 * Returns `null` when unsupported, low-sample, or unavailable so callers can omit TPS cleanly
 * instead of rendering placeholder "— t/s".
 */
export function formatTps(
  tps: number | null | undefined,
  state?: PlazaOutputTpsState | null
): string | null {
  if (
    state === 'unsupported' ||
    (state != null && state !== 'valid') ||
    tps == null ||
    !Number.isFinite(tps) ||
    tps <= 0
  ) {
    return null
  }
  return tps >= 10 ? `${Math.round(tps)} t/s` : `${tps.toFixed(1)} t/s`
}

/**
 * Formats compact throughput for Model Plaza cards (e.g. "105t").
 * Returns `null` when unsupported, low-sample, or unavailable so cards hide the column cleanly.
 */
export function formatCompactTps(
  tps: number | null | undefined,
  state?: PlazaOutputTpsState | null
): string | null {
  if (
    state === 'unsupported' ||
    (state != null && state !== 'valid') ||
    tps == null ||
    !Number.isFinite(tps) ||
    tps <= 0
  ) {
    return null
  }
  return tps >= 1 ? `${Math.round(tps)}t` : `${Number(tps.toFixed(1))}t`
}

/**
 * Formats cache hit ratio (0~1) with 2 decimals by default (e.g. "92.46%"):
 * - >= 0.70 -> 'ok'
 * - 0.40 ~ 0.70 -> 'neutral' (ink-2)
 * - < 0.40 -> 'warn'
 * - insufficient sample -> '—'
 */
export function formatCacheRate(
  ratio: number | null | undefined,
  state?: PlazaMetricSampleState | null,
  decimals = 2
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || ratio == null || !Number.isFinite(ratio)) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const clamped = Math.max(0, Math.min(1, ratio))
  const pct = clamped * 100
  const text = pct >= 100 ? '100%' : `${pct.toFixed(decimals)}%`

  let tone: PlazaMetricTone = 'warn'
  if (clamped >= 0.7) {
    tone = 'ok'
  } else if (clamped >= 0.4) {
    tone = 'neutral'
  }
  return { text, tone, valid: true }
}

/**
 * Formats 4-token-weighted effective reference multiplier with fixed 3 decimals (e.g. "0.780×").
 * Values strictly below 0.5 receive 'ok' tone to highlight meaningful cache savings.
 */
export function formatRefMultiplier(
  val: number | null | undefined,
  state?: PlazaMetricSampleState | null,
  decimals = 3
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || val == null || !Number.isFinite(val) || val <= 0) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const factor = 10 ** decimals
  const rounded = Math.round(val * factor) / factor
  const formatted = rounded.toFixed(decimals)
  const tone: PlazaMetricTone = val < 0.5 ? 'ok' : 'neutral'
  return { text: `${formatted}×`, tone, valid: true }
}

/**
 * Converts a 0 < multiplier < 1 effective ratio into Chinese discount fold (e.g. 0.088 -> "0.9", 0.35 -> "3.5", 0.5 -> "5").
 * Returns null when multiplier is invalid or >= 1.
 */
export function formatDiscountFold(multiplier: number | null | undefined): string | null {
  if (multiplier == null || !Number.isFinite(multiplier) || multiplier <= 0 || multiplier >= 1) {
    return null
  }
  const fold = Math.max(0.1, Math.round(multiplier * 100) / 10)
  return String(fold)
}

/**
 * Formats response model consistency rate (0~1) with status tone.
 */
export function formatConsistencyPercent(
  rate: number | null | undefined,
  state?: PlazaConsistencyState | null,
  decimals = 2
): PlazaFormattedMetric {
  if (
    state === 'unsupported' ||
    state === 'low_sample' ||
    state === 'no_data' ||
    rate == null ||
    !Number.isFinite(rate)
  ) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const clamped = Math.max(0, Math.min(1, rate))
  const pct = clamped * 100
  const text = pct >= 100 ? '100%' : `${pct.toFixed(decimals)}%`

  let tone: PlazaMetricTone = 'ok'
  if (state === 'mismatch_detected' || clamped < 0.995) {
    tone = clamped < 0.95 ? 'bad' : 'warn'
  }
  return { text, tone, valid: true }
}

/**
 * Computes baseline vs paid savings percentage (integer).
 * Kept for backward compatibility during incremental component migration.
 */
export function computeOfficialSavingsPercent(
  paid: number | null | undefined,
  official: number | null | undefined
): number | null {
  if (
    paid == null ||
    official == null ||
    !Number.isFinite(paid) ||
    !Number.isFinite(official) ||
    official <= 0 ||
    paid < 0 ||
    paid >= official
  ) {
    return null
  }
  const ratio = (official - paid) / official
  if (ratio < 0.05) {
    return null
  }
  const percent = Math.round(ratio * 100)
  if (percent < 5) {
    return null
  }
  return paid === 0 ? 100 : Math.min(percent, 99)
}

/**
 * Formats token context window / max output count (e.g. 1_000_000 -> "1M", 200_000 -> "200K").
 */
export function formatContextWindow(tokens: number | null | undefined): string | null {
  if (tokens == null || !Number.isFinite(tokens) || tokens <= 0) {
    return null
  }
  if (tokens >= 1_048_576 && tokens % 1_048_576 === 0) {
    return `${tokens / 1_048_576}M`
  }
  if (tokens >= 1_000_000) {
    const val = Number((tokens / 1_000_000).toFixed(1))
    return `${val}M`
  }
  if (tokens >= 4096 && tokens % 1024 === 0 && tokens % 1000 !== 0) {
    return `${tokens / 1024}K`
  }
  if (tokens >= 1_000) {
    const k = tokens / 1_000
    const val = k >= 100 ? Math.round(k) : Number(k.toFixed(1))
    return `${val}K`
  }
  return String(Math.round(tokens))
}

/**
 * Builds a concise fallback summary when a model has no custom description,
 * ensuring card description regions keep a consistent height.
 */
export function buildFallbackSummary(
  metadata: PlazaModelMetadata | null | undefined,
  t: PlazaTranslateFn
): string {
  const existing = metadata?.description?.trim()
  if (existing) {
    return existing
  }
  const context = formatContextWindow(metadata?.context_window)
  const capLabels = (metadata?.capabilities ?? [])
    .slice(0, 3)
    .map((cap) => {
      const normalized = cap.trim().toLowerCase()
      if (KNOWN_CAPABILITY_KEYS.has(normalized)) {
        return t(`modelPlaza.capabilities.${normalized}`)
      }
      return cap
    })
    .filter(Boolean)
  const capabilities = capLabels.join(' / ')

  if (context && capabilities) {
    return t('modelPlaza.card.fallbackSummaryWithContextAndCaps', { context, capabilities })
  }
  if (context) {
    return t('modelPlaza.card.fallbackSummaryWithContext', { context })
  }
  if (capabilities) {
    return t('modelPlaza.card.fallbackSummaryWithCaps', { capabilities })
  }
  return t('modelPlaza.card.fallbackSummaryGeneric')
}

/**
 * Formats an ISO timestamp into a short relative updated-at label.
 */
export function formatRelativeUpdatedAt(
  iso: string | null | undefined,
  t: PlazaTranslateFn,
  nowMs = Date.now()
): string {
  if (!iso) return ''
  const parsed = Date.parse(iso)
  if (!Number.isFinite(parsed)) return ''
  const diffSec = Math.max(0, Math.floor((nowMs - parsed) / 1000))
  if (diffSec < 60) {
    return t('modelPlaza.common.updatedJustNow')
  }
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) {
    return t('modelPlaza.common.updatedMinutesAgo', { count: diffMin })
  }
  const diffHours = Math.floor(diffMin / 60)
  if (diffHours < 24) {
    return t('modelPlaza.common.updatedHoursAgo', { count: diffHours })
  }
  const diffDays = Math.floor(diffHours / 24)
  return t('modelPlaza.common.updatedDaysAgo', { count: diffDays })
}
