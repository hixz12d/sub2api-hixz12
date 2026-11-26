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
      v-else-if="entries.length === 0 && !hideEmptyState"
      :title="t('modelPlaza.card.emptyTitle')"
      :description="t('modelPlaza.card.emptyDescription')"
    />

    <!-- 5-segment aligned card grid (pure RMB ¥ pricing, effort folding, light/dark adaptive) -->
    <div
      v-else-if="entries.length > 0"
      class="tile-grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3"
    >
      <article
        v-for="row in cardRows"
        :key="row.entry.key"
        role="button"
        tabindex="0"
        class="group flex h-full cursor-pointer flex-col rounded-none bg-surface-tile p-[18px] text-ink transition-colors duration-150 hover:bg-surface-tile-2"
        @click="emit('select-model', row.activeModel)"
        @keydown.enter.self="emit('select-model', row.activeModel)"
        @keydown.space.prevent.self="emit('select-model', row.activeModel)"
      >
        <!-- Segment 1: Header row (icon frame + mono model ID + outlined Details button + icon-only Copy button) -->
        <header class="flex h-8 shrink-0 items-center justify-between gap-2">
          <div class="flex min-w-0 flex-1 items-center gap-2.5">
            <span
              class="flex h-8 w-8 shrink-0 items-center justify-center rounded-none border border-rule/80 bg-surface transition-colors duration-150"
              aria-hidden="true"
            >
              <ModelIcon
                :model="row.activeModel.name"
                size="18px"
                :class="row.darkInvert ? 'dark:invert' : ''"
              />
            </span>
            <span
              class="min-w-0 truncate font-mono text-[14px] font-semibold text-ink"
              :title="row.activeModel.name"
            >
              {{ row.activeModel.name }}
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

          <div class="flex shrink-0 items-center gap-1.5">
            <span
              class="inline-flex h-7 items-center gap-0.5 rounded-none border border-rule bg-surface px-2 text-xs font-medium text-ink-2 transition-colors duration-150 group-hover:border-ink-3/50 group-hover:text-ink"
            >
              <span>{{ t('modelPlaza.card.details') }}</span>
              <Icon name="chevronRight" size="xs" class="h-3.5 w-3.5" aria-hidden="true" />
            </span>
            <button
              type="button"
              class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-none border transition-colors duration-150 active:scale-[0.985]"
              :class="
                copiedModelName === row.activeModel.name
                  ? 'border-ok bg-ok text-white'
                  : 'border-rule bg-surface text-ink-2 hover:border-ink-3/50 hover:text-ink'
              "
              :title="
                copiedModelName === row.activeModel.name
                  ? t('modelPlaza.card.copied')
                  : t('modelPlaza.table.copyModelId', { model: row.activeModel.name })
              "
              :aria-label="t('modelPlaza.table.copyModelId', { model: row.activeModel.name })"
              @click.stop="handleCopy(row.activeModel.name)"
            >
              <Icon
                :name="copiedModelName === row.activeModel.name ? 'check' : 'copy'"
                size="xs"
                class="h-3.5 w-3.5"
                aria-hidden="true"
              />
            </button>
          </div>
        </header>

        <!-- Optional folded thinking-effort pill switcher (only rendered for folded families) -->
        <div
          v-if="row.entry.isFolded"
          class="mt-2.5 flex flex-wrap items-center gap-1.5"
          role="radiogroup"
          :aria-label="t('modelPlaza.card.effortSwitchAria', { model: row.entry.baseName })"
          @click.stop
        >
          <button
            v-for="variant in row.entry.variants"
            :key="variant.effort"
            type="button"
            role="radio"
            :aria-checked="variant.effort === row.activeEffort"
            class="rounded-none px-2 py-0.5 font-mono text-[11px] leading-4 transition-colors duration-150"
            :class="
              variant.effort === row.activeEffort
                ? 'bg-money font-semibold text-money-ink'
                : 'border border-rule bg-surface text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
            "
            @click.stop="emit('select-effort', row.entry.key, variant.effort)"
          >
            {{ variant.label }}
          </button>
          <span
            v-if="row.entry.bareAliasForHigh"
            class="ml-auto truncate text-[11px] text-ink-3"
          >
            {{ t('modelPlaza.card.bareAliasShort') }}
          </span>
        </div>

        <!-- Segment 2: Pure RMB ¥ pricing row (space-separated, each with /1M, zero $ / savings %) -->
        <div class="mt-2.5 flex min-h-[22px] shrink-0 items-center justify-between gap-2 text-xs tabular-nums">
          <div class="flex min-w-0 flex-wrap items-baseline gap-x-3 gap-y-0.5">
            <template v-if="row.pricing.isToken">
              <template v-if="row.pricing.hasPrice">
                <span class="inline-flex items-baseline gap-1 whitespace-nowrap">
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceInput') }}</span>
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.inputDisplay }}</span>
                </span>
                <span class="inline-flex items-baseline gap-1 whitespace-nowrap">
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceOutput') }}</span>
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.outputDisplay }}</span>
                </span>
                <span
                  v-if="row.pricing.cacheReadDisplay"
                  class="inline-flex items-baseline gap-1 whitespace-nowrap"
                >
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceCacheRead') }}</span>
                  <span class="font-mono font-medium text-ink-2">{{ row.pricing.cacheReadDisplay }}</span>
                </span>
              </template>
              <span v-else class="text-ink-3">{{ t('modelPlaza.detail.noPricing') }}</span>
            </template>

            <template v-else>
              <template v-if="row.pricing.requestPriceText">
                <span class="inline-flex items-baseline gap-1 whitespace-nowrap">
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.requestPriceText }}</span>
                  <span class="text-ink-3">{{ row.pricing.requestUnitSuffix }}</span>
                </span>
              </template>
              <span v-else class="text-ink-3">{{ t('modelPlaza.detail.noPricing') }}</span>
            </template>
          </div>

          <div
            v-if="row.pricing.tierCount > 1 || row.pricing.hasTimePricing || row.pricing.hasPeakRate"
            class="flex shrink-0 items-center gap-1"
          >
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
          </div>
        </div>

        <!-- Segment 3: Light-tint 3-column live metrics strip (no vertical dividers) -->
        <div
          class="mt-3 grid shrink-0 grid-cols-[1.35fr_1fr_0.9fr] gap-2 bg-surface/80 px-3 py-2.5 transition-colors duration-150 dark:bg-dark-900/60"
        >
          <!-- Col 1: Status (12-slot heartbeat + 2-decimal SLA inline, or group_pool fallback) -->
          <div class="min-w-0">
            <div class="mb-1 text-[11px] leading-4 text-ink-3">
              {{ t('modelPlaza.card.metricStatus') }}
            </div>
            <div
              v-if="row.isGroupPoolFallback"
              class="flex items-center gap-1.5"
              @click.stop
            >
              <div class="w-[48px] shrink-0 sm:w-[54px]">
                <PlazaHeartbeatBar
                  :slots="row.heartbeatSlots"
                  variant="compact"
                  class="!h-[12px]"
                />
              </div>
              <span
                class="cursor-help truncate text-xs font-semibold text-ok"
                @mouseenter="showTooltip($event, t('modelPlaza.card.availableGroupPoolTooltip'))"
                @mouseleave="hideTooltip"
              >
                {{ t('modelPlaza.card.availableGroupPool') }}
              </span>
            </div>
            <div
              v-else-if="row.sla.valid"
              class="flex items-center gap-1.5"
              @click.stop
            >
              <div class="w-[50px] shrink-0 sm:w-[56px]">
                <PlazaHeartbeatBar
                  :slots="row.heartbeatSlots"
                  variant="compact"
                  class="!h-[12px]"
                />
              </div>
              <span
                class="shrink-0 text-[13px] font-semibold leading-5 tabular-nums"
                :class="plazaToneTextClass(row.sla.tone)"
              >
                {{ row.sla.text }}
              </span>
            </div>
            <div
              v-else
              class="cursor-help text-[14px] font-semibold leading-5 text-ink-3 tabular-nums"
              @mouseenter="showTooltip($event, row.slaTooltip)"
              @mouseleave="hideTooltip"
            >
              {{ PLAZA_EMPTY_TEXT }}
            </div>
          </div>

          <!-- Col 2: Coding Cache (2 decimals, neutral bold ink) -->
          <div class="min-w-0">
            <div class="mb-1 truncate text-[11px] leading-4 text-ink-3">
              {{ t('modelPlaza.card.metricCodingCache') }}
            </div>
            <div
              class="truncate text-[14px] font-semibold leading-5 tabular-nums"
              :class="[row.cache.valid ? 'text-ink' : 'text-ink-3', row.cacheTooltip ? 'cursor-help' : '']"
              @mouseenter="row.cacheTooltip && showTooltip($event, row.cacheTooltip)"
              @mouseleave="hideTooltip"
            >
              {{ row.cache.text }}
            </div>
          </div>

          <!-- Col 3: Reference Multiplier (fixed 3 decimals, neutral bold mono ink) -->
          <div class="min-w-0">
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
              class="truncate font-mono text-[14px] font-semibold leading-5 tabular-nums"
              :class="[row.refMult.valid ? 'text-ink' : 'text-ink-3', row.refMultTooltip ? 'cursor-help' : '']"
              @mouseenter="row.refMultTooltip && showTooltip($event, row.refMultTooltip)"
              @mouseleave="hideTooltip"
            >
              {{ row.refMult.text }}
            </div>
          </div>
        </div>

        <!-- Segment 4: Locked 2-line summary -->
        <p class="mt-3 line-clamp-2 min-h-[2.5rem] text-[13px] leading-[1.25rem] text-ink-2">
          {{ row.summary }}
        </p>

        <!-- Segment 5: Footer (no top divider line) -->
        <footer class="mt-auto pt-3">
          <div class="flex items-end justify-between gap-3">
            <!-- Left stacked metadata: Group / Billing nature / Model note -->
            <div class="min-w-0 flex-1 space-y-0.5 text-xs">
              <div class="flex min-w-0 items-center gap-1.5 text-ink-3">
                <span class="truncate text-ink-2">{{ group.name }}</span>
                <span aria-hidden="true">·</span>
                <span class="shrink-0">{{ row.billingNature }}</span>
              </div>
              <div
                v-if="row.modelNote"
                class="truncate text-[11px] text-warn"
                :title="row.modelNote"
                @mouseenter="showTooltip($event, row.modelNote)"
                @mouseleave="hideTooltip"
                @click.stop
              >
                {{ row.modelNote }}
              </div>
            </div>

            <!-- Right compact vertical mini metrics: Latency + Throughput (hidden for non-OpenAI / unsupported) -->
            <div class="flex shrink-0 items-end gap-4 text-right tabular-nums">
              <div
                class="cursor-help"
                @mouseenter="showTooltip($event, row.latencyTooltip)"
                @mouseleave="hideTooltip"
              >
                <div class="text-[11px] leading-4 text-ink-3">
                  {{ t('modelPlaza.card.metricLatency') }}
                </div>
                <div
                  class="font-mono text-xs font-semibold leading-4"
                  :class="row.latency.valid ? 'text-ink' : 'text-ink-3'"
                >
                  {{ row.latency.text }}
                </div>
              </div>

              <div
                v-if="row.showThroughput && row.compactTpsText"
                class="cursor-help"
                @mouseenter="showTooltip($event, t('modelPlaza.card.tpsTooltip'))"
                @mouseleave="hideTooltip"
              >
                <div class="text-[11px] leading-4 text-ink-3">
                  {{ t('modelPlaza.card.metricThroughput') }}
                </div>
                <div class="font-mono text-xs font-semibold leading-4 text-ink">
                  {{ row.compactTpsText }}
                </div>
              </div>
            </div>
          </div>

          <!-- Bottom plain muted tag line (no pill backgrounds): Vendor  Capabilities  ContextWindow  +N -->
          <div class="mt-2.5 flex items-center gap-x-2.5 overflow-hidden whitespace-nowrap text-[11px] leading-4 text-ink-3">
            <span class="shrink-0 font-medium text-ink-2">{{ row.vendorText }}</span>
            <span
              v-for="cap in row.visibleCapabilities"
              :key="cap.key"
              class="shrink-0"
            >
              {{ cap.label }}
            </span>
            <span v-if="row.contextText" class="shrink-0 font-mono tabular-nums">
              {{ row.contextText }}
            </span>
            <span
              v-if="row.overflowCapabilities.length > 0"
              class="shrink-0 cursor-help tabular-nums text-ink-3 hover:text-ink-2"
              @mouseenter="showTooltip($event, row.overflowCapabilities.join(' · '))"
              @mouseleave="hideTooltip"
              @click.stop
            >
              {{ t('modelPlaza.card.moreCapabilities', { count: row.overflowCapabilities.length }) }}
            </span>
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
import { resolveIntervalPrices } from '@/utils/pricing'
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
  KNOWN_CAPABILITY_KEYS,
  KNOWN_VENDOR_KEYS,
  PLAZA_EMPTY_TEXT,
  buildFallbackSummary,
  formatCacheRate,
  formatCompactLatency,
  formatCompactTps,
  formatConsistencyPercent,
  formatContextWindow,
  formatPlazaPrice,
  formatRefMultiplier,
  formatSlaPercent,
  inferVendorKey,
  needsDarkInvert,
  plazaToneTextClass
} from '@/utils/plazaFormat'
import {
  resolveVariantForEntry,
  type PlazaEffortLevel,
  type PlazaFoldedEntry
} from '@/utils/plazaFolding'

interface Props {
  group: ModelPlazaGroup
  entries: PlazaFoldedEntry[]
  selectedEfforts?: Record<string, PlazaEffortLevel>
  loading?: boolean
  hideEmptyState?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  selectedEfforts: () => ({}),
  loading: false,
  hideEmptyState: false
})

const emit = defineEmits<{
  'select-model': [model: PlazaModel]
  'select-effort': [entryKey: string, effort: PlazaEffortLevel]
  'copy-model': [modelName: string]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const PER_MILLION = 1_000_000
const MAX_VISIBLE_CAPABILITIES = 3

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

function capabilityLabel(cap: string): string {
  const norm = cap.trim().toLowerCase()
  if (KNOWN_CAPABILITY_KEYS.has(norm)) {
    return t(`modelPlaza.capabilities.${norm}`)
  }
  return cap
}

function vendorLabel(vendorKey: string): string {
  const norm = vendorKey.trim().toLowerCase()
  if (KNOWN_VENDOR_KEYS.has(norm)) {
    return t(`modelPlaza.vendors.${norm}`)
  }
  return vendorKey
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
  const perMillionUnit = t('modelPlaza.card.perMillionUnit')

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
    const paidCacheNum = rawCacheRead != null ? rawCacheRead * rate : null
    const hasPrice = paidInNum != null || paidOutNum != null

    return {
      mode,
      isToken: true,
      hasPrice,
      inputDisplay:
        paidInNum != null ? `${formatPlazaPrice(paidInNum, PER_MILLION)}${perMillionUnit}` : PLAZA_EMPTY_TEXT,
      outputDisplay:
        paidOutNum != null ? `${formatPlazaPrice(paidOutNum, PER_MILLION)}${perMillionUnit}` : PLAZA_EMPTY_TEXT,
      cacheReadDisplay:
        paidCacheNum != null ? `${formatPlazaPrice(paidCacheNum, PER_MILLION)}${perMillionUnit}` : null,
      requestPriceText: null as string | null,
      requestUnitSuffix: '',
      tierCount: intervals.length,
      hasTimePricing,
      hasPeakRate: groupHasPeakRate.value
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
      : mode === BILLING_MODE_VIDEO
        ? t('modelPlaza.table.perUnitVideo')
        : t('modelPlaza.table.perUnitRequest')

  return {
    mode,
    isToken: false,
    hasPrice: rawReqPrice != null,
    inputDisplay: PLAZA_EMPTY_TEXT,
    outputDisplay: PLAZA_EMPTY_TEXT,
    cacheReadDisplay: null as string | null,
    requestPriceText: rawReqPrice != null ? formatPlazaPrice(rawReqPrice * reqRate, 1) : null,
    requestUnitSuffix,
    tierCount: reqIntervals.length,
    hasTimePricing,
    hasPeakRate: groupHasPeakRate.value
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
  props.entries.map((entry) => {
    const activeEffort = entry.isFolded
      ? (props.selectedEfforts[entry.key] ?? entry.defaultEffort)
      : null
    const activeModel = resolveVariantForEntry(entry, activeEffort)
    const pricing = computeModelPricing(activeModel)
    const metrics = activeModel.metrics
    const isGroupPoolFallback = metrics?.sla_source === 'group_pool'
    const sla = formatSlaPercent(metrics?.sla_24h, metrics?.sla_state, 2)
    const cache = formatCacheRate(metrics?.cache_read_ratio, metrics?.cache_state, 2)
    const refMult = formatRefMultiplier(
      metrics?.reference_multiplier,
      metrics?.reference_multiplier_state,
      3
    )
    const latency = formatCompactLatency(metrics?.ttft_p50_ms, metrics?.ttft_state)
    const compactTpsText = formatCompactTps(metrics?.output_tps_p50, metrics?.output_tps_state)

    const vendorKey = inferVendorKey(
      activeModel.name,
      activeModel.platform,
      activeModel.metadata?.vendor
    )
    const showThroughput =
      vendorKey === 'openai' &&
      metrics?.output_tps_state !== 'unsupported' &&
      compactTpsText !== null

    const heartbeatSlots =
      (metrics?.heartbeat_12?.length ?? 0) > 0
        ? metrics!.heartbeat_12
        : isGroupPoolFallback
          ? (props.group.pool_status?.heartbeat_12 ?? [])
          : []

    const allCaps = (activeModel.metadata?.capabilities ?? [])
      .map((c) => c.trim())
      .filter(Boolean)
      .map((key) => ({ key, label: capabilityLabel(key) }))

    return {
      entry,
      activeEffort,
      activeModel,
      darkInvert: needsDarkInvert(activeModel.name),
      pricing,
      billingNature: billingNatureLabel(pricing.mode, pricing.tierCount, pricing.hasTimePricing),
      modelNote: activeModel.note?.trim() || '',
      consistencyBadge: buildConsistencyBadge(activeModel),
      isGroupPoolFallback,
      heartbeatSlots,
      sla,
      slaTooltip: sla.valid ? '' : sampleStateTooltip(metrics?.sla_state),
      cache,
      cacheTooltip: cache.valid ? '' : sampleStateTooltip(metrics?.cache_state),
      refMult,
      refMultTooltip: refMult.valid ? '' : sampleStateTooltip(metrics?.reference_multiplier_state),
      latency,
      latencyTooltip: latency.valid
        ? t('modelPlaza.card.ttftTooltip')
        : sampleStateTooltip(metrics?.ttft_state),
      compactTpsText,
      showThroughput,
      summary: buildFallbackSummary(activeModel.metadata, t),
      vendorText: vendorLabel(vendorKey),
      contextText: formatContextWindow(activeModel.metadata?.context_window),
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
