<template>
  <div>
    <!-- Loading skeleton -->
    <div v-if="loading" class="tile-grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3">
      <div
        v-for="i in 6"
        :key="i"
        class="h-[144px] animate-pulse rounded-none bg-surface-tile p-[18px]"
      >
        <div class="h-5 w-2/3 bg-surface-tile-2" />
        <div class="mt-3 h-4 w-3/4 bg-surface-tile-2" />
        <div class="mt-3 h-9 w-full bg-surface-tile-2" />
      </div>
    </div>

    <!-- Empty state -->
    <EmptyState
      v-else-if="entries.length === 0 && !hideEmptyState"
      :title="t('modelPlaza.card.emptyTitle')"
      :description="t('modelPlaza.card.emptyDescription')"
    />

    <!-- Compact 4-segment card grid (pure RMB ¥ pricing, effort folding, light/dark adaptive) -->
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
        <!-- Row 1: Header (icon frame + mono model ID + inline consistency badge + 36x36 Copy button) -->
        <header class="flex min-h-9 shrink-0 items-center justify-between gap-2">
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
              class="inline-flex shrink-0 items-center gap-0.5 rounded-sm px-1.5 py-0.5 text-[11px] font-medium leading-4"
              :class="row.consistencyBadge.toneClass"
            >
              <Icon
                v-if="row.consistencyBadge.state === 'mismatch_detected'"
                name="exclamationTriangle"
                size="xs"
                class="h-3 w-3 shrink-0"
                aria-hidden="true"
              />
              <span>{{ row.consistencyBadge.label }}</span>
            </span>
          </div>

          <button
            type="button"
            class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-none border transition-colors duration-150 active:scale-[0.985]"
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
              size="sm"
              class="h-4 w-4"
              aria-hidden="true"
            />
          </button>
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

        <!-- Row 2: Compact single-line RMB ¥ pricing row (/1M once at the end) -->
        <div class="mt-2.5 flex min-h-[22px] shrink-0 items-center justify-between gap-2 text-xs tabular-nums">
          <div class="flex min-w-0 flex-wrap items-baseline gap-x-1.5 gap-y-0.5">
            <template v-if="row.pricing.isToken">
              <template v-if="row.pricing.hasPrice">
                <span class="inline-flex items-baseline gap-1 whitespace-nowrap">
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceInput') }}</span>
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.inputDisplay }}</span>
                </span>
                <span class="text-ink-3" aria-hidden="true">·</span>
                <span class="inline-flex items-baseline gap-1 whitespace-nowrap">
                  <span class="text-ink-3">{{ t('modelPlaza.card.priceOutput') }}</span>
                  <span class="font-mono font-semibold text-ink">{{ row.pricing.outputDisplay }}</span>
                </span>
                <template v-if="row.pricing.cacheReadDisplay">
                  <span class="text-ink-3" aria-hidden="true">·</span>
                  <span class="inline-flex items-baseline gap-1 whitespace-nowrap">
                    <span class="text-ink-3">{{ t('modelPlaza.card.priceCacheShort') }}</span>
                    <span class="font-mono font-medium text-ink-2">{{ row.pricing.cacheReadDisplay }}</span>
                  </span>
                </template>
                <span class="whitespace-nowrap text-[11px] text-ink-3">
                  {{ t('modelPlaza.card.perMillionUnit') }}
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
            v-if="row.pricing.showTierBadge || row.pricing.hasTimePricing || row.pricing.hasPeakRate"
            class="flex shrink-0 items-center gap-1"
          >
            <span
              v-if="row.pricing.showTierBadge"
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
              v-if="row.pricing.hasPeakRate"
              class="rounded-none bg-surface-tile-2 px-1.5 py-0.5 text-[11px] font-medium text-ink-2"
            >
              {{ t('modelPlaza.card.peakBadge') }}
            </span>
          </div>
        </div>

        <!-- Row 3: Compact single-line status + valid metrics strip (omitted when no valid data) -->
        <div
          v-if="row.hasStatusStrip"
          class="mt-2.5 flex flex-wrap items-center justify-between gap-x-3 gap-y-1.5 bg-surface/80 px-3 py-2 text-xs tabular-nums dark:bg-dark-900/60"
        >
          <!-- Left: 12-slot non-interactive heartbeat + SLA% or group pool fallback -->
          <div
            v-if="row.isGroupPoolFallback"
            class="flex min-w-0 items-center gap-1.5"
          >
            <div class="w-[48px] shrink-0 sm:w-[54px]">
              <PlazaHeartbeatBar
                :slots="row.heartbeatSlots"
                variant="compact"
                :interactive="false"
                class="!h-[12px]"
              />
            </div>
            <span class="truncate font-semibold text-ok">
              {{ t('modelPlaza.card.availableGroupPool') }}
            </span>
          </div>
          <div
            v-else-if="row.sla.valid"
            class="flex min-w-0 items-center gap-1.5"
          >
            <div class="w-[50px] shrink-0 sm:w-[56px]">
              <PlazaHeartbeatBar
                :slots="row.heartbeatSlots"
                variant="compact"
                :interactive="false"
                class="!h-[12px]"
              />
            </div>
            <span
              class="shrink-0 font-semibold tabular-nums"
              :class="plazaToneTextClass(row.sla.tone)"
            >
              {{ row.sla.text }}
            </span>
          </div>

          <!-- Right: valid secondary metrics only (never render empty dashes) -->
          <div
            v-if="row.hasRightMetrics"
            class="flex flex-wrap items-center gap-x-2.5 gap-y-0.5 text-[11px] text-ink-3 tabular-nums"
          >
            <span v-if="row.cache.valid" class="whitespace-nowrap">
              {{ t('modelPlaza.card.metricCacheShort') }}
              <span class="font-mono font-semibold text-ink">{{ row.cache.text }}</span>
            </span>
            <span v-if="row.latency.valid" class="whitespace-nowrap">
              {{ t('modelPlaza.card.metricLatency') }}
              <span class="font-mono font-semibold text-ink">{{ row.latency.text }}</span>
            </span>
            <span
              v-if="row.showThroughput && row.compactTpsText"
              class="whitespace-nowrap font-mono font-semibold text-ink"
            >
              {{ row.compactTpsText }}
            </span>
          </div>
        </div>

        <!-- Optional Row 4: Admin single-model orange note (2-line clamp, clickable to open drawer) -->
        <p
          v-if="row.modelNote"
          class="mt-2 line-clamp-2 text-xs leading-relaxed text-warn"
        >
          {{ row.modelNote }}
        </p>
      </article>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
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
  BILLING_MODE_TOKEN,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
import type { ModelPlazaGroup, PlazaModel } from '@/api/modelPlaza'
import type { UserPricingInterval } from '@/api/channels'
import {
  PLAZA_EMPTY_TEXT,
  formatCacheRate,
  formatCompactLatency,
  formatCompactTps,
  formatConsistencyPercent,
  formatPlazaPrice,
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

const copiedModelName = ref<string | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null

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

function sortByContext(intervals: UserPricingInterval[]): UserPricingInterval[] {
  return [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)
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
    const paidCacheNum = rawCacheRead != null ? rawCacheRead * rate : null
    const hasPrice = paidInNum != null || paidOutNum != null

    return {
      mode,
      isToken: true,
      hasPrice,
      inputDisplay:
        paidInNum != null ? formatPlazaPrice(paidInNum, PER_MILLION) : PLAZA_EMPTY_TEXT,
      outputDisplay:
        paidOutNum != null ? formatPlazaPrice(paidOutNum, PER_MILLION) : PLAZA_EMPTY_TEXT,
      cacheReadDisplay:
        paidCacheNum != null ? formatPlazaPrice(paidCacheNum, PER_MILLION) : null,
      requestPriceText: null as string | null,
      requestUnitSuffix: '',
      tierCount: intervals.length,
      showTierBadge: false,
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
    showTierBadge: reqIntervals.length > 1,
    hasTimePricing,
    hasPeakRate: groupHasPeakRate.value
  }
}

function buildConsistencyBadge(model: PlazaModel) {
  const m = model.metrics
  if (!m) return null
  if (m.consistency_state === 'verified') {
    return {
      state: 'verified' as const,
      label: t('modelPlaza.consistency.verifiedBadge'),
      toneClass: 'bg-ok/12 text-ok'
    }
  }
  if (m.consistency_state === 'mismatch_detected') {
    const rate = formatConsistencyPercent(m.consistency_rate, m.consistency_state, 2)
    return {
      state: 'mismatch_detected' as const,
      label: t('modelPlaza.consistency.mismatchBadge'),
      toneClass: rate.tone === 'bad' ? 'bg-bad/12 text-bad' : 'bg-warn/12 text-warn'
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

    const hasRightMetrics = cache.valid || latency.valid || (showThroughput && Boolean(compactTpsText))
    const hasStatusStrip = isGroupPoolFallback || sla.valid || hasRightMetrics

    return {
      entry,
      activeEffort,
      activeModel,
      darkInvert: needsDarkInvert(activeModel.name),
      pricing,
      modelNote: activeModel.note?.trim() || '',
      consistencyBadge: buildConsistencyBadge(activeModel),
      isGroupPoolFallback,
      heartbeatSlots,
      sla,
      cache,
      latency,
      compactTpsText,
      showThroughput,
      hasRightMetrics,
      hasStatusStrip
    }
  })
)

onBeforeUnmount(() => {
  if (copyTimer) clearTimeout(copyTimer)
})
</script>
