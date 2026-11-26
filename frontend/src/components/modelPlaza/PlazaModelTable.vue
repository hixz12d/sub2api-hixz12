<template>
  <div>
    <!-- Loading skeleton -->
    <div v-if="loading" class="space-y-1">
      <div
        v-for="i in 6"
        :key="i"
        class="h-[54px] animate-pulse rounded-none bg-surface-tile"
      />
    </div>

    <!-- Empty state -->
    <EmptyState
      v-else-if="entries.length === 0 && !hideEmptyState"
      :title="t('modelPlaza.card.emptyTitle')"
      :description="t('modelPlaza.card.emptyDescription')"
    />

    <!-- Fixed 6-column left-aligned table (pure RMB ¥ pricing, effort pills, light/dark adaptive) -->
    <div v-else-if="entries.length > 0" class="w-full border border-rule bg-surface">
      <table class="w-full table-fixed border-collapse text-left text-sm tabular-nums">
        <colgroup>
          <col class="w-[34%]" />
          <col class="w-[9%]" />
          <col class="w-[19%]" />
          <col class="w-[12%]" />
          <col class="w-[14%]" />
          <col class="w-[12%]" />
        </colgroup>
        <thead>
          <tr class="border-b border-rule bg-surface-tile text-xs text-ink-3">
            <!-- 1. Model (only column with sort arrows) -->
            <th class="px-4 py-3 text-left font-medium">
              <button
                type="button"
                class="group/th inline-flex items-center gap-1 transition-colors duration-150 hover:text-ink"
                :class="modelSortDir ? 'font-semibold text-money-text' : ''"
                @click="toggleModelSort"
              >
                <span>{{ t('modelPlaza.table.model') }}</span>
                <Icon
                  :name="modelSortIcon"
                  size="xs"
                  class="h-3.5 w-3.5"
                  :class="modelSortDir ? 'text-money-text' : 'opacity-50 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>

            <!-- 2. Type -->
            <th class="px-3 py-3 text-left font-medium">
              <span>{{ t('modelPlaza.table.typeHeader') }}</span>
            </th>

            <!-- 3. Price -->
            <th class="px-3 py-3 text-left font-medium">
              <span>{{ t('modelPlaza.table.priceColumnHeader') }}</span>
            </th>

            <!-- 4. Cache -->
            <th class="px-3 py-3 text-left font-medium">
              <span>{{ t('modelPlaza.table.cacheColumnHeader') }}</span>
            </th>

            <!-- 5. Status -->
            <th class="px-3 py-3 text-left font-medium">
              <span
                class="cursor-help"
                @mouseenter="showTooltip($event, t('modelPlaza.table.slaTooltip'))"
                @mouseleave="hideTooltip"
              >
                {{ t('modelPlaza.table.statusColumnHeader') }}
              </span>
            </th>

            <!-- 6. Coding Cache -->
            <th class="py-3 pl-3 pr-4 text-left font-medium">
              <span>{{ t('modelPlaza.table.codingCacheHeader') }}</span>
            </th>
          </tr>
        </thead>

        <tbody class="divide-y divide-rule">
          <tr
            v-for="row in tableRows"
            :key="row.entry.key"
            tabindex="0"
            class="group cursor-pointer bg-surface transition-colors duration-150 hover:bg-surface-tile"
            :title="t('modelPlaza.table.clickForDetails')"
            @click="emit('select-model', row.activeModel)"
            @keydown.enter.self="emit('select-model', row.activeModel)"
            @keydown.space.prevent.self="emit('select-model', row.activeModel)"
          >
            <!-- 1. Model: icon + mono ID + inline consistency badge + copy button + optional folded effort pills -->
            <td class="px-4 py-3 align-middle">
              <div class="flex max-w-full items-center gap-2">
                <ModelIcon
                  :model="row.activeModel.name"
                  size="18px"
                  :class="row.darkInvert ? 'dark:invert' : ''"
                  aria-hidden="true"
                />
                <div class="inline-flex min-w-0 max-w-full items-center gap-1.5">
                  <span
                    class="min-w-0 truncate font-mono text-[13.5px] font-semibold text-ink"
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
                  <button
                    type="button"
                    class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-none transition-colors duration-150"
                    :class="
                      copiedModelName === row.activeModel.name
                        ? 'bg-ok text-white'
                        : 'text-ink-3 opacity-65 hover:bg-surface-tile-2 hover:text-ink group-hover:opacity-100'
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
              </div>

              <!-- Folded effort pill selector -->
              <div
                v-if="row.entry.isFolded"
                class="mt-1.5 flex flex-wrap items-center gap-1 pl-[26px]"
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
                  class="rounded-none px-1.5 py-0.5 font-mono text-[11px] leading-4 transition-colors duration-150"
                  :class="
                    variant.effort === row.activeEffort
                      ? 'bg-money font-semibold text-money-ink'
                      : 'border border-rule bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'
                  "
                  @click.stop="emit('select-effort', row.entry.key, variant.effort)"
                >
                  {{ variant.label }}
                </button>
              </div>

              <!-- Optional model note -->
              <div
                v-if="row.modelNote"
                class="mt-1 truncate pl-[26px] text-[11px] text-warn"
                :title="row.modelNote"
                @mouseenter="showTooltip($event, row.modelNote)"
                @mouseleave="hideTooltip"
                @click.stop
              >
                {{ row.modelNote }}
              </div>
            </td>

            <!-- 2. Type (blue semantic text) -->
            <td class="px-3 py-3 align-middle">
              <span class="whitespace-nowrap text-xs font-medium text-series-2">
                {{ row.typeLabel }}
              </span>
            </td>

            <!-- 3. Price (pure RMB ¥, no $, no savings %) -->
            <td class="px-3 py-3 align-middle">
              <template v-if="row.pricing.isToken">
                <template v-if="row.pricing.hasPrice">
                  <div class="truncate font-mono text-[13px] font-semibold text-ink">
                    {{ row.pricing.inputText }}
                    <span class="font-sans font-normal text-ink-3">/</span>
                    {{ row.pricing.outputText }}
                  </div>
                  <div class="mt-0.5 truncate text-[11px] leading-4 text-ink-3">
                    <span>{{ t('modelPlaza.table.perMillionTokens') }}</span>
                    <span v-if="row.pricing.hasTimePricing" class="text-ink-2">
                      · {{ t('modelPlaza.card.timePricingBadge') }}
                    </span>
                  </div>
                </template>
                <span v-else class="text-xs text-ink-3">{{ t('modelPlaza.detail.noPricing') }}</span>
              </template>

              <template v-else>
                <template v-if="row.pricing.requestPriceText">
                  <div class="truncate font-mono text-[13px] font-semibold text-ink">
                    {{ row.pricing.requestPriceText }}
                    <span class="font-sans text-xs font-normal text-ink-3">
                      {{ row.pricing.requestUnitSuffix }}
                    </span>
                  </div>
                  <div
                    v-if="row.pricing.tierCount > 1"
                    class="mt-0.5 text-[11px] leading-4 text-ink-2"
                  >
                    {{ t('modelPlaza.card.tiersBadge', { count: row.pricing.tierCount }) }}
                  </div>
                </template>
                <span v-else class="text-xs text-ink-3">{{ t('modelPlaza.detail.noPricing') }}</span>
              </template>
            </td>

            <!-- 4. Cache Read Price -->
            <td class="px-3 py-3 align-middle">
              <template v-if="row.pricing.cacheReadText">
                <div class="font-mono text-[13px] font-medium text-ink">
                  {{ row.pricing.cacheReadText }}
                </div>
                <div class="mt-0.5 text-[11px] leading-4 text-ink-3">
                  {{ t('modelPlaza.table.perMillionShort') }}
                </div>
              </template>
              <span v-else class="text-ink-3">{{ PLAZA_EMPTY_TEXT }}</span>
            </td>

            <!-- 5. Status (solid dot + 2-decimal SLA, or green dot + Available (Pool)) -->
            <td class="px-3 py-3 align-middle">
              <span
                v-if="row.isGroupPoolFallback"
                class="inline-flex cursor-help items-center gap-1.5 whitespace-nowrap text-xs font-medium text-ok"
                @mouseenter="showTooltip($event, t('modelPlaza.card.availableGroupPoolTooltip'))"
                @mouseleave="hideTooltip"
                @click.stop
              >
                <span class="h-2 w-2 shrink-0 rounded-full bg-ok" aria-hidden="true" />
                <span>{{ t('modelPlaza.card.availableGroupPool') }}</span>
              </span>
              <span
                v-else-if="row.sla.valid"
                class="inline-flex items-center gap-1.5 whitespace-nowrap font-semibold"
                :class="plazaToneTextClass(row.sla.tone)"
              >
                <span
                  class="h-2 w-2 shrink-0 rounded-full"
                  :class="row.slaDotClass"
                  aria-hidden="true"
                />
                <span>{{ row.sla.text }}</span>
              </span>
              <span
                v-else
                class="cursor-help text-ink-3"
                @mouseenter="showTooltip($event, row.slaTooltip)"
                @mouseleave="hideTooltip"
              >
                {{ PLAZA_EMPTY_TEXT }}
              </span>
            </td>

            <!-- 6. Coding Cache (2 decimals, neutral text) -->
            <td class="py-3 pl-3 pr-4 align-middle">
              <span
                v-if="row.cache.valid"
                class="font-medium text-ink"
              >
                {{ row.cache.text }}
              </span>
              <span
                v-else
                class="cursor-help text-ink-3"
                @mouseenter="showTooltip($event, row.cacheTooltip)"
                @mouseleave="hideTooltip"
              >
                {{ PLAZA_EMPTY_TEXT }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Flat inverted Teleport tooltip (DESIGN.md:330) -->
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
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { useClipboard } from '@/composables/useClipboard'
import { resolveIntervalPrices } from '@/utils/pricing'
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
import type { PlazaSortBy } from './PlazaToolbar.vue'
import {
  PLAZA_EMPTY_TEXT,
  formatCacheRate,
  formatConsistencyPercent,
  formatPlazaPrice,
  formatSlaPercent,
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
  sortBy?: PlazaSortBy
  loading?: boolean
  hideEmptyState?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  selectedEfforts: () => ({}),
  sortBy: 'default',
  loading: false,
  hideEmptyState: false
})

const emit = defineEmits<{
  'select-model': [model: PlazaModel]
  'select-effort': [entryKey: string, effort: PlazaEffortLevel]
  'update:sortBy': [value: PlazaSortBy]
  'copy-model': [modelName: string]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const PER_MILLION = 1_000_000

const modelSortDir = ref<'asc' | 'desc' | null>(null)
const copiedModelName = ref<string | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.sortBy,
  () => {
    modelSortDir.value = null
  }
)

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

const modelSortIcon = computed(() => {
  if (!modelSortDir.value) return 'sort' as const
  return modelSortDir.value === 'asc' ? ('arrowUp' as const) : ('arrowDown' as const)
})

function toggleModelSort() {
  if (modelSortDir.value === 'asc') {
    modelSortDir.value = 'desc'
  } else if (modelSortDir.value === 'desc') {
    modelSortDir.value = null
  } else {
    modelSortDir.value = 'asc'
  }
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

function billingTypeLabel(mode: BillingMode): string {
  switch (mode) {
    case BILLING_MODE_PER_REQUEST:
      return t('modelPlaza.table.typePerRequest')
    case BILLING_MODE_IMAGE:
      return t('modelPlaza.table.typePerImage')
    case BILLING_MODE_VIDEO:
      return t('modelPlaza.table.typePerVideo')
    case BILLING_MODE_TOKEN:
    default:
      return t('modelPlaza.table.typeToken')
  }
}

function computePricing(model: PlazaModel) {
  const mode = (model.pricing?.billing_mode || BILLING_MODE_TOKEN) as BillingMode
  const hasTimePricing = (model.time_pricing?.periods?.length ?? 0) > 0

  if (mode === BILLING_MODE_TOKEN) {
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

    return {
      mode,
      isToken: true,
      hasPrice,
      inputText: paidInNum != null ? formatPlazaPrice(paidInNum, PER_MILLION) : PLAZA_EMPTY_TEXT,
      outputText: paidOutNum != null ? formatPlazaPrice(paidOutNum, PER_MILLION) : PLAZA_EMPTY_TEXT,
      cacheReadText:
        rawCacheRead != null ? formatPlazaPrice(rawCacheRead * rate, PER_MILLION) : null,
      requestPriceText: null as string | null,
      requestUnitSuffix: '',
      tierCount: intervals.length,
      hasTimePricing
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
    inputText: PLAZA_EMPTY_TEXT,
    outputText: PLAZA_EMPTY_TEXT,
    cacheReadText: null as string | null,
    requestPriceText: rawReqPrice != null ? formatPlazaPrice(rawReqPrice * reqRate, 1) : null,
    requestUnitSuffix,
    tierCount: reqIntervals.length,
    hasTimePricing
  }
}

const orderedEntries = computed<PlazaFoldedEntry[]>(() => {
  const dir = modelSortDir.value
  if (!dir) return props.entries
  const list = [...props.entries]
  list.sort((a, b) => {
    const modelA = resolveVariantForEntry(a, props.selectedEfforts[a.key] ?? null)
    const modelB = resolveVariantForEntry(b, props.selectedEfforts[b.key] ?? null)
    return dir === 'asc'
      ? modelA.name.localeCompare(modelB.name)
      : modelB.name.localeCompare(modelA.name)
  })
  return list
})

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

const tableRows = computed(() =>
  orderedEntries.value.map((entry) => {
    const activeEffort = entry.isFolded
      ? (props.selectedEfforts[entry.key] ?? entry.defaultEffort)
      : null
    const activeModel = resolveVariantForEntry(entry, activeEffort)
    const pricing = computePricing(activeModel)
    const metrics = activeModel.metrics
    const isGroupPoolFallback = metrics?.sla_source === 'group_pool'
    const sla = formatSlaPercent(metrics?.sla_24h, metrics?.sla_state, 2)
    const cache = formatCacheRate(metrics?.cache_read_ratio, metrics?.cache_state, 2)

    const slaDotClass =
      sla.tone === 'ok' ? 'bg-ok' : sla.tone === 'warn' ? 'bg-warn' : 'bg-bad'

    return {
      entry,
      activeEffort,
      activeModel,
      darkInvert: needsDarkInvert(activeModel.name),
      typeLabel: billingTypeLabel(pricing.mode),
      modelNote: activeModel.note?.trim() || '',
      consistencyBadge: buildConsistencyBadge(activeModel),
      pricing,
      isGroupPoolFallback,
      sla,
      slaDotClass,
      slaTooltip: sampleStateTooltip(metrics?.sla_state),
      cache,
      cacheTooltip: sampleStateTooltip(metrics?.cache_state)
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
