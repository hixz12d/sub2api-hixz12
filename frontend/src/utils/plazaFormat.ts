/**
 * Shared pure formatters and tone helpers for the Model Plaza views.
 * Follows DESIGN.md semantic tones (ok / warn / bad / neutral / muted)
 * and honest low-sample fallbacks (always "—", never fabricated green values).
 */

import type {
  PlazaConsistencyState,
  PlazaMetricSampleState,
  PlazaModelMetadata,
  PlazaOutputTpsState
} from '@/api/modelPlaza'

export const PLAZA_EMPTY_TEXT = '—'

export type PlazaMetricTone = 'ok' | 'warn' | 'bad' | 'neutral' | 'muted'

export interface PlazaFormattedMetric {
  text: string
  tone: PlazaMetricTone
  valid: boolean
}

export type PlazaTranslateFn = (key: string, params?: Record<string, unknown>) => string

const KNOWN_CAPABILITY_KEYS = new Set([
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

/**
 * Formats an SLA ratio (0~1) with status tone:
 * - >= 0.995 -> 'ok'
 * - 0.95 ~ 0.995 -> 'warn'
 * - < 0.95 -> 'bad'
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

  let tone: PlazaMetricTone = 'warn'
  if (clamped >= 0.995) {
    tone = 'ok'
  } else if (clamped < 0.95) {
    tone = 'bad'
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
 * Formats cache hit ratio (0~1) strictly following DESIGN.md:
 * - >= 0.70 -> 'ok'
 * - 0.40 ~ 0.70 -> 'neutral' (ink-2)
 * - < 0.40 -> 'warn'
 * - insufficient sample -> '—'
 */
export function formatCacheRate(
  ratio: number | null | undefined,
  state?: PlazaMetricSampleState | null
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || ratio == null || !Number.isFinite(ratio)) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const clamped = Math.max(0, Math.min(1, ratio))
  const pct = clamped * 100
  const text = pct >= 100 ? '100%' : `${pct.toFixed(1)}%`

  let tone: PlazaMetricTone = 'warn'
  if (clamped >= 0.7) {
    tone = 'ok'
  } else if (clamped >= 0.4) {
    tone = 'neutral'
  }
  return { text, tone, valid: true }
}

/**
 * Formats 4-token-weighted effective reference multiplier (e.g. "0.172×").
 * Values strictly below 0.5 receive 'ok' tone to highlight meaningful cache savings.
 */
export function formatRefMultiplier(
  val: number | null | undefined,
  state?: PlazaMetricSampleState | null
): PlazaFormattedMetric {
  if ((state != null && state !== 'valid') || val == null || !Number.isFinite(val) || val <= 0) {
    return { text: PLAZA_EMPTY_TEXT, tone: 'muted', valid: false }
  }
  const rounded = Math.round(val * 1000) / 1000
  const formatted = rounded.toFixed(3).replace(/(\.\d{2})0$/, '$1')
  const tone: PlazaMetricTone = val < 0.5 ? 'ok' : 'neutral'
  return { text: `${formatted}×`, tone, valid: true }
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
 * Computes official savings percentage (integer).
 * Returns a positive integer only when `paid` is strictly lower than `official` by at least 5%.
 * Never returns negative or misleading values.
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
