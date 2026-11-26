<template>
  <div>
    <!-- Loading skeleton -->
    <div v-if="loading" class="tile-grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="i in 6"
        :key="i"
        class="h-[246px] animate-pulse rounded-none bg-surface-tile p-[18px]"
      >
        <div class="h-5 w-2/3 bg-surface-tile-2" />
        <div class="mt-3 h-4 w-3/4 bg-surface-tile-2" />
        <div class="mt-3 h-[58px] w-full bg-surface-tile-2" />
        <div class="mt-3 h-9 w-full bg-surface-tile-2" />
        <div class="mt-4 h-4 w-1/2 bg-surface-tile-2" />
      </div>
    </div>

    <!-- Empty state -->
    <EmptyState
      v-else-if="models.length === 0 && !hideEmptyState"
      :title="t('modelPlaza.card.emptyTitle')"
      :description="t('modelPlaza.card.emptyDescription')"
    />

    <!-- 5-segment strictly equal-height card grid -->
    <div
      v-else-if="models.length > 0"
      class="tile-grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3"
    >
      <article
        v-for="row in cardRows"
        :key="`${row.model.platform}:${row.model.name}`"
        role="button"
        tabindex="0"
        class="group flex h-full cursor-pointer flex-col rounded-none bg-surface-tile p-[18px] text-ink transition-colors duration-150 hover:bg-surface-tile-2"
        @click="emit('select-model', row.model)"
        @keydown.enter.self="emit('select-model', row.model)"
        @keydown.space.prevent.self="emit('select-model', row.model)"
      >
        <!-- Segment 1: Header row (fixed single-line height) -->
        <header class="flex h-7 shrink-0 items-center justify-between gap-2">
          <div class="flex min-w-0 flex-1 items-center gap-2">
            <ModelIcon
              :model="row.model.name"
              size="20px"
              :class="row.darkInvert ? 'dark:invert' : ''"
              aria-hidden="true"
            />
            <span
              class="min-w-0 truncate font-mono text-[14px] font-semibold text-ink"
              :title="row.model.name"
            >
              {{ row.model.name }}
            </span>
            <span
              v-if="row.consistencyBadge"
              class="inline-flex shrink-0 cursor-help items-center"
              :class="row.consistencyBadge.toneClass"
              @mouseenter="showTooltip($event, row.consistencyBadge.tooltip)"
              @mouseleave="hideTooltip"
              @click.stop
            >
              <Icon :name="row.consistencyBadge.icon" size="sm" class="h-4 w-4" aria-hidden="true" />
            </span>
          </div>

          <div class="flex shrink-0 items-center gap-1">
            <span
              class="inline-flex items-center gap-0.5 px-1.5 py-1 text-xs font-medium text-ink-2 transition-colors duration-150 group-hover:text-ink"
            >
              <span>{{ t('modelPlaza.card.details') }}</span>
              <Icon name="chevronRight" size="xs" class="h-3.5 w-3.5" aria-hidden="true" />
            </span>
            <button
              type="button"
              class="inline-flex h-7 items-center gap-1 rounded-none px-2 text-xs font-medium transition-colors duration-150 active:scale-[0.985]"
              :class="
                copiedModelName === row.model.name
                  ? 'bg-ok text-white'
                  : 'bg-surface-tile-2 text-ink-2 hover:bg-surface hover:text-ink'
              "
              :title="t('modelPlaza.table.copyModelId', { model: row.model.name })"
              :aria-label="t('modelPlaza.table.copyModelId', { model: row.model.name })"
              @click.stop="handleCopy(row.model.name)"
            >
              <Icon
                :name="copiedModelName === row.model.name ? 'check' : 'copy'"
                size="xs"
                class="h-3.5 w-3.5"
                aria-hidden="true"
              />
              <span>
                {{
                  copiedModelName === row.model.name
                    ? t('modelPlaza.card.copied')
                    : t('modelPlaza.card.copyId')
                }}
              </span>
            </button>
          </div>
        </header>

        <!-- Segment 2: Single-line pricing row -->
        <div class="mt-2.5 flex min-h-[24px] shrink-0 items-center justify-between gap-2 text-xs tabular-nums">
          <div class="flex min-w-0 items-center gap-x-2 overflow-hidden whitespace-nowrap">
            <template v-if="row.pricing.isToken">
              <template v-if="row.pricing.hasPrice">
                <span class="inline-flex items-baseline gap-1">
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceInput') }}</span>
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.inputText }}</span>
                </span>
                <span class="text-ink-3" aria-hidden="true">/</span>
                <span class="inline-flex items-baseline gap-1">
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceOutput') }}</span>
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.outputText }}</span>
                </span>
                <template v-if="row.pricing.cacheReadText">
                  <span class="text-ink-3" aria-hidden="true">·</span>
                  <span class="inline-flex items-baseline gap-1 truncate">
                    <span class="text-ink-3">{{ t('modelPlaza.card.priceCacheRead') }}</span>
                    <span class="font-mono font-medium text-ink-2">{{ row.pricing.cacheReadText }}</span>
                  </span>
                </template>
              </template>
              <span v-else class="text-ink-3">{{ t('modelPlaza.detail.noPricing') }}</span>
            </template>

            <template v-else>
              <template v-if="row.pricing.requestPriceText">
                <span class="font-mono font-semibold text-ink">{{ row.pricing.requestPriceText }}</span>
                <span class="text-ink-3">{{ row.pricing.requestUnitSuffix }}</span>
              </template>
              <span v-else class="text-ink-3">{{ t('modelPlaza.detail.noPricing') }}</span>
            </template>
          </div>

          <div class="flex shrink-0 items-center gap-1">
            <span
              v-if="row.pricing.tierCount > 1"
              class="rounded-none bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
            >
              {{ t('modelPlaza.card.tiersBadge', { count: row.pricing.tierCount }) }}
            </span>
            <span
              v-if="row.pricing.hasTimePricing"
              class="rounded-none bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
            >
              {{ t('modelPlaza.card.timePricingBadge') }}
            </span>
            <span
              v-else-if="row.pricing.hasPeakRate && row.pricing.tierCount <= 1"
              class="rounded-none bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
            >
              {{ t('modelPlaza.card.peakBadge') }}
            </span>
            <span
              v-if="row.pricing.savingsPercent != null"
              class="rounded-none bg-ok/10 px-1.5 py-0.5 text-[11px] font-semibold text-ok"
            >
              {{ t('modelPlaza.card.savingsBadge', { percent: row.pricing.savingsPercent }) }}
            </span>
          </div>
        </div>

        <!-- Segment 3: Embedded bg-surface-tile-2 3-column live metrics strip -->
        <div
          class="mt-3 grid shrink-0 grid-cols-3 divide-x divide-rule border border-transparent bg-surface-tile-2 py-2.5 transition-colors duration-150 group-hover:border-rule"
        >
          <!-- Col 1: SLA (24h) + 12-slot compact heartbeat -->
          <div class="min-w-0 px-2.5">
            <div class="mb-1.5 flex items-center justify-between gap-1 text-[11px] leading-4">
              <span class="truncate text-ink-3">{{ t('modelPlaza.card.metricSla') }}</span>
              <span
                class="shrink-0 font-semibold tabular-nums"
                :class="plazaToneTextClass(row.sla.tone)"
                @mouseenter="row.slaTooltip && showTooltip($event, row.slaTooltip)"
                @mouseleave="hideTooltip"
              >
                {{ row.sla.text }}
              </span>
            </div>
            <div @click.stop>
              <PlazaHeartbeatBar
                :slots="row.model.metrics?.heartbeat_12 ?? []"
                variant="compact"
                class="!h-[14px]"
              />
            </div>
          </div>

          <!-- Col 2: Cache Hit Rate -->
          <div class="min-w-0 px-2.5">
            <div class="mb-1 text-[11px] leading-4 text-ink-3">
              {{ t('modelPlaza.card.metricCache') }}
            </div>
            <div
              class="truncate text-[16px] font-semibold leading-5 tabular-nums"
              :class="[plazaToneTextClass(row.cache.tone), row.cacheTooltip ? 'cursor-help' : '']"
              @mouseenter="row.cacheTooltip && showTooltip($event, row.cacheTooltip)"
              @mouseleave="hideTooltip"
            >
              {{ row.cache.text }}
            </div>
          </div>

          <!-- Col 3: 4-token Reference Multiplier -->
          <div class="min-w-0 px-2.5">
            <div class="mb-1 text-[11px] leading-4 text-ink-3">
              <span
                class="cursor-help border-b border-dashed border-ink-3/70"
                @mouseenter="showTooltip($event, t('modelPlaza.card.refMultiplierTooltip'))"
                @mouseleave="hideTooltip"
                @click.stop
              >
                {{ t('modelPlaza.card.metricRefMultiplier') }}
              </span>
            </div>
            <div
              class="truncate text-[16px] font-semibold leading-5 tabular-nums"
              :class="[plazaToneTextClass(row.refMult.tone), row.refMultTooltip ? 'cursor-help' : '']"
              @mouseenter="row.refMultTooltip && showTooltip($event, row.refMultTooltip)"
              @mouseleave="hideTooltip"
            >
              {{ row.refMult.text }}
            </div>
          </div>
        </div>

        <!-- Segment 4: Locked 2-line summary -->
        <p class="mt-3 line-clamp-2 min-h-[2.6rem] text-[13px] leading-[1.3rem] text-ink-2">
          {{ row.summary }}
        </p>

        <!-- Segment 5: Footer (group & billing + TTFT/TPS, then 4+N capabilities) -->
        <footer class="mt-auto pt-3">
          <div class="space-y-2 border-t border-rule pt-2.5">
            <!-- Top line: Group & billing nature | TTFT + TPS -->
            <div class="flex items-center justify-between gap-2 text-xs">
              <div class="flex min-w-0 items-center gap-1.5 text-ink-3">
                <span class="truncate text-ink-2">{{ group.name }}</span>
                <span aria-hidden="true">·</span>
                <span class="shrink-0">{{ row.billingNature }}</span>
              </div>

              <div class="flex shrink-0 items-center gap-1 tabular-nums">
                <span
                  :class="[
                    row.ttft.valid
                      ? row.ttft.tone === 'ok'
                        ? 'font-semibold text-ok'
                        : 'text-ink-2'
                      : 'text-ink-3',
                    'cursor-help'
                  ]"
                  @mouseenter="showTooltip($event, row.ttftTooltip)"
                  @mouseleave="hideTooltip"
                >
                  {{ t('modelPlaza.card.ttftLabel', { value: row.ttft.text }) }}
                </span>
                <template v-if="row.tpsText">
                  <span class="text-ink-3" aria-hidden="true">·</span>
                  <span
                    class="cursor-help text-ink-2"
                    @mouseenter="showTooltip($event, t('modelPlaza.card.tpsTooltip'))"
                    @mouseleave="hideTooltip"
                  >
                    {{ row.tpsText }}
                  </span>
                </template>
              </div>
            </div>

            <!-- Bottom line: single-row capabilities (max 4 + "+N") -->
            <div class="flex h-[22px] items-center gap-1.5 overflow-hidden">
              <template v-if="row.visibleCapabilities.length > 0">
                <span
                  v-for="cap in row.visibleCapabilities"
                  :key="cap.key"
                  class="inline-flex shrink-0 items-center rounded-none bg-surface-tile-2 px-1.5 py-0.5 text-[11px] leading-4 text-ink-2"
                >
                  {{ cap.label }}
                </span>
                <span
                  v-if="row.overflowCapabilities.length > 0"
                  class="inline-flex shrink-0 cursor-help items-center rounded-none bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium leading-4 text-ink-3 tabular-nums"
                  @mouseenter="showTooltip($event, row.overflowCapabilities.join(' · '))"
                  @mouseleave="hideTooltip"
                  @click.stop
                >
                  {{ t('modelPlaza.card.moreCapabilities', { count: row.overflowCapabilities.length }) }}
                </span>
              </template>
              <span v-else class="text-[11px] leading-4 text-ink-3">
                {{ t('modelPlaza.drawer.noCapabilities') }}
              </span>
            </div>
          </div>
        </footer>
      </article>
    </div>

    <!-- Flat inverted Teleport tooltip (DESIGN.md:330, immune to scroll offset) -->
    <Teleport to="body">
      <div
        v-if="activeTooltip"
        role="tooltip"
        class="pointer-events-none fixed z-[9999] max-w-[260px] -translate-x-1/2 -translate-y-full rounded-none bg-gray-950 px-2.5 py-2 text-xs leading-relaxed text-white dark:bg-white dark:text-black"
        :style="{ left: `${activeTooltip.left}px`, top: `${activeTooltip.top}px` }"
      >
        {{ activeTooltip.text }}
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import PlazaHeartbeatBar from './PlazaHeartbeatBar.vue'
import { useClipboard } from '@/composables/useClipboard'
import { formatScaled, resolveIntervalPrices } from '@/utils/pricing'
import { hasPeakRate } from '@/utils/peak-rate'
import {
  BILLING_MODE_IMAGE,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import type {
  ModelPlazaGroup,
  PlazaMetricSampleState,
  PlazaModel
} from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import {
  buildFallbackSummary,
  computeOfficialSavingsPercent,
  formatCacheRate,
  formatConsistencyPercent,
  formatRefMultiplier,
  formatSlaPercent,
  formatTps,
  formatTtft,
  plazaToneTextClass
} from '@/utils/plazaFormat'

interface Props {
  group: ModelPlazaGroup
  models: PlazaModel[]
  loading?: boolean
  hideEmptyState?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  hideEmptyState: false
})

const emit = defineEmits<{
  'select-model': [model: PlazaModel]
  'copy-model': [modelName: string]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2
const MAX_VISIBLE_CAPABILITIES = 4

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

const copiedModelName = ref<string | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null

interface TooltipState {
  left: number
  top: number
  text: string
}

const activeTooltip = ref<TooltipState | null>(null)

function showTooltip(event: MouseEvent, text: string) {
  if (!text) return
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const rect = target.getBoundingClientRect()
  const rawLeft = rect.left + rect.width / 2
  const viewportWidth = typeof window !== 'undefined' ? window.innerWidth : 1024
  const clampedLeft = Math.max(136, Math.min(viewportWidth - 136, rawLeft))
  activeTooltip.value = {
    left: clampedLeft,
    top: Math.max(48, rect.top - 8),
    text
  }
}

function hideTooltip() {
  activeTooltip.value = null
}

async function handleCopy(modelName: string) {
  const ok = await copyToClipboard(modelName)
  if (ok) {
    copiedModelName.value = modelName
    emit('copy-model', modelName)
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      if (copiedModelName.value === modelName) {
        copiedModelName.value = null
      }
    }, 2000)
  }
}

function needsDarkInvert(modelName: string): boolean {
  const lower = modelName.toLowerCase()
  return (
    lower.startsWith('gpt') ||
    lower.startsWith('o1') ||
    lower.startsWith('o3') ||
    lower.startsWith('o4') ||
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

function capabilityLabel(cap: string): string {
  const norm = cap.trim().toLowerCase()
  if (KNOWN_CAPABILITY_KEYS.has(norm)) {
    return t(`modelPlaza.capabilities.${norm}`)
  }
  return cap
}

function sortByContext(intervals: UserPricingInterval[]): UserPricingInterval[] {
  return [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)
}

function sampleStateTooltip(state: PlazaMetricSampleState | null | undefined): string {
  if (state === 'low_sample') return t('modelPlaza.common.lowSampleTooltip')
  return t('modelPlaza.common.noDataTooltip')
}

const effectiveRate = computed(
  () => props.group.user_rate_multiplier ?? props.group.rate_multiplier
)

const groupHasPeakRate = computed(() => hasPeakRate(props.group))

function computeModelPricing(model: PlazaModel) {
  const mode = (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
  const isToken = mode === BILLING_MODE_TOKEN
  const hasTimePricing = (model.time_pricing?.periods?.length ?? 0) > 0

  if (isToken) {
    const intervals = model.pricing?.intervals?.length
      ? sortByContext(model.pricing.intervals).map((iv) => resolveIntervalPrices(iv, model.pricing!))
      : []
    const base = intervals[0]
    const rawIn = base ? base.input_price : (model.pricing?.input_price ?? null)
    const rawOut = base ? base.output_price : (model.pricing?.output_price ?? null)
    const rawCacheRead = base ? base.cache_read_price : (model.pricing?.cache_read_price ?? null)
    const rate = effectiveRate.value

    const paidInNum = rawIn != null ? rawIn * rate : null
    const paidOutNum = rawOut != null ? rawOut * rate : null
    const hasPrice = paidInNum != null || paidOutNum != null

    const officialIntervals = sortByContext(model.official_pricing?.intervals ?? [])
    const officialIn = model.official_pricing?.input_price ?? officialIntervals[0]?.input_price ?? null
    const officialOut = model.official_pricing?.output_price ?? officialIntervals[0]?.output_price ?? null
    const savingsPercent =
      computeOfficialSavingsPercent(paidInNum, officialIn) ??
      computeOfficialSavingsPercent(paidOutNum, officialOut)

    return {
      mode,
      isToken: true,
      hasPrice,
      inputText: paidInNum != null ? formatScaled(paidInNum, PER_MILLION, MIN_DECIMALS) : '—',
      outputText: paidOutNum != null ? formatScaled(paidOutNum, PER_MILLION, MIN_DECIMALS) : '—',
      cacheReadText:
        rawCacheRead != null ? formatScaled(rawCacheRead * rate, PER_MILLION, MIN_DECIMALS) : null,
      requestPriceText: null as string | null,
      requestUnitSuffix: '',
      tierCount: intervals.length,
      hasTimePricing,
      hasPeakRate: groupHasPeakRate.value,
      savingsPercent
    }
  }

  let reqRate = effectiveRate.value
  if (mode === BILLING_MODE_VIDEO && props.group.video_rate_independent) {
    reqRate = Math.max(0, props.group.video_rate_multiplier ?? 1)
  } else if (mode === BILLING_MODE_IMAGE && props.group.image_rate_independent) {
    reqRate = props.group.image_rate_multiplier ?? 1
  }

  const reqIntervals = (model.pricing?.intervals ?? []).filter((iv) => iv.per_request_price != null)
  const rawReqPrice =
    reqIntervals.length > 0 ? reqIntervals[0].per_request_price : (model.pricing?.per_request_price ?? null)
  const requestUnitSuffix =
    mode === BILLING_MODE_IMAGE
      ? t('modelPlaza.table.perUnitImage')
      : t('modelPlaza.table.perUnitRequest')

  return {
    mode,
    isToken: false,
    hasPrice: rawReqPrice != null,
    inputText: '—',
    outputText: '—',
    cacheReadText: null as string | null,
    requestPriceText: rawReqPrice != null ? formatScaled(rawReqPrice * reqRate, 1, MIN_DECIMALS) : null,
    requestUnitSuffix,
    tierCount: reqIntervals.length,
    hasTimePricing,
    hasPeakRate: groupHasPeakRate.value,
    savingsPercent: null as number | null
  }
}

function billingNatureLabel(mode: BillingMode, tierCount: number, hasTimePricing: boolean): string {
  if (mode === BILLING_MODE_IMAGE) return t('modelPlaza.card.billingPerImage')
  if (mode === BILLING_MODE_VIDEO) return t('modelPlaza.card.billingPerVideo')
  if (mode === BILLING_MODE_PER_REQUEST) return t('modelPlaza.card.billingPerRequest')
  if (hasTimePricing) return t('modelPlaza.card.billingTimePrice')
  if (tierCount > 1) return t('modelPlaza.card.billingTieredPrice')
  return t('modelPlaza.card.billingGroupPrice')
}

function buildConsistencyBadge(model: PlazaModel) {
  const m = model.metrics
  if (!m) return null
  if (m.consistency_state === 'verified') {
    const rate = formatConsistencyPercent(m.consistency_rate, m.consistency_state, 2)
    const header = rate.valid
      ? `${t('modelPlaza.consistency.verifiedShort')} (${rate.text})`
      : t('modelPlaza.consistency.verifiedShort')
    const extra = m.auto_pause_protected ? ` · ${t('modelPlaza.consistency.autoPauseActive')}` : ''
    return {
      icon: 'shield' as const,
      toneClass: 'text-ok',
      tooltip: `${header}${extra} — ${t('modelPlaza.consistency.methodDesc')}`
    }
  }
  if (m.consistency_state === 'mismatch_detected') {
    const rate = formatConsistencyPercent(m.consistency_rate, m.consistency_state, 2)
    const header = rate.valid
      ? `${t('modelPlaza.consistency.mismatchShort')} (${rate.text})`
      : t('modelPlaza.consistency.mismatchShort')
    return {
      icon: 'exclamationTriangle' as const,
      toneClass: rate.tone === 'bad' ? 'text-bad' : 'text-warn',
      tooltip: `${header} — ${t('modelPlaza.consistency.autoPauseDesc')}`
    }
  }
  return null
}

const cardRows = computed(() =>
  props.models.map((model) => {
    const pricing = computeModelPricing(model)
    const metrics = model.metrics
    const sla = formatSlaPercent(metrics?.sla_24h, metrics?.sla_state, 1)
    const cache = formatCacheRate(metrics?.cache_read_ratio, metrics?.cache_state)
    const refMult = formatRefMultiplier(
      metrics?.reference_multiplier,
      metrics?.reference_multiplier_state
    )
    const ttft = formatTtft(metrics?.ttft_p50_ms, metrics?.ttft_state)
    const tpsText = formatTps(metrics?.output_tps_p50, metrics?.output_tps_state)

    const allCaps = (model.metadata?.capabilities ?? [])
      .map((c) => c.trim())
      .filter(Boolean)
      .map((key) => ({ key, label: capabilityLabel(key) }))

    return {
      model,
      darkInvert: needsDarkInvert(model.name),
      pricing,
      billingNature: billingNatureLabel(pricing.mode, pricing.tierCount, pricing.hasTimePricing),
      consistencyBadge: buildConsistencyBadge(model),
      sla,
      slaTooltip: sla.valid ? '' : sampleStateTooltip(metrics?.sla_state),
      cache,
      cacheTooltip: cache.valid ? '' : sampleStateTooltip(metrics?.cache_state),
      refMult,
      refMultTooltip: refMult.valid ? '' : sampleStateTooltip(metrics?.reference_multiplier_state),
      ttft,
      ttftTooltip: ttft.valid
        ? t('modelPlaza.card.ttftTooltip')
        : sampleStateTooltip(metrics?.ttft_state),
      tpsText,
      summary: buildFallbackSummary(model.metadata, t),
      visibleCapabilities: allCaps.slice(0, MAX_VISIBLE_CAPABILITIES),
      overflowCapabilities: allCaps.slice(MAX_VISIBLE_CAPABILITIES).map((c) => c.label)
    }
  })
)

onMounted(() => {
  window.addEventListener('scroll', hideTooltip, true)
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', hideTooltip, true)
  if (copyTimer) clearTimeout(copyTimer)
})
</script>
