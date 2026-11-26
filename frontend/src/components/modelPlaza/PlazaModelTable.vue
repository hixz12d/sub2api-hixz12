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
      v-else-if="models.length === 0 && !hideEmptyState"
      :title="t('modelPlaza.card.emptyTitle')"
      :description="t('modelPlaza.card.emptyDescription')"
    />

    <!-- 8-column zero-horizontal-scroll flat tile table -->
    <div v-else-if="models.length > 0" class="w-full">
      <table class="w-full table-fixed border-separate border-spacing-y-1 text-sm tabular-nums">
        <colgroup>
          <col class="w-[24%]" />
          <col class="w-[17%]" />
          <col class="w-[9%]" />
          <col class="w-[11%]" />
          <col class="w-[12%]" />
          <col class="w-[10%]" />
          <col class="w-[8%]" />
          <col class="w-[9%]" />
        </colgroup>
        <thead>
          <tr class="text-[13px] text-ink-3">
            <!-- 1. Model (sortable) -->
            <th class="px-3 pb-1.5 text-left font-normal">
              <button
                type="button"
                class="group/th inline-flex items-center gap-1 transition-colors duration-150 hover:text-ink"
                :class="activeColumn === 'model' ? 'font-medium text-money-text' : ''"
                @click="handleColumnSort('model')"
              >
                <span>{{ t('modelPlaza.table.model') }}</span>
                <Icon
                  :name="sortIconFor('model')"
                  size="xs"
                  class="h-3.5 w-3.5"
                  :class="activeColumn === 'model' ? 'text-money-text' : 'opacity-40 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>

            <!-- 2. Price (In / Out) (sortable) -->
            <th class="px-2.5 pb-1.5 text-right font-normal">
              <button
                type="button"
                class="group/th inline-flex items-center justify-end gap-1 transition-colors duration-150 hover:text-ink"
                :class="activeColumn === 'price' ? 'font-medium text-money-text' : ''"
                @click="handleColumnSort('price')"
              >
                <span class="truncate">{{ t('modelPlaza.table.priceHeader') }}</span>
                <Icon
                  :name="sortIconFor('price')"
                  size="xs"
                  class="h-3.5 w-3.5 shrink-0"
                  :class="activeColumn === 'price' ? 'text-money-text' : 'opacity-40 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>

            <!-- 3. Cache Read Price -->
            <th class="px-2.5 pb-1.5 text-right font-normal">
              <span class="truncate">{{ t('modelPlaza.table.cacheReadHeader') }}</span>
            </th>

            <!-- 4. SLA (24h) (sortable) -->
            <th class="px-2.5 pb-1.5 text-right font-normal">
              <button
                type="button"
                class="group/th inline-flex items-center justify-end gap-1 transition-colors duration-150 hover:text-ink"
                :class="activeColumn === 'sla' ? 'font-medium text-money-text' : ''"
                @click="handleColumnSort('sla')"
                @mouseenter="showTooltip($event, t('modelPlaza.table.slaTooltip'))"
                @mouseleave="hideTooltip"
              >
                <span class="border-b border-dashed border-ink-3/60">
                  {{ t('modelPlaza.table.sla24hHeader') }}
                </span>
                <Icon
                  :name="sortIconFor('sla')"
                  size="xs"
                  class="h-3.5 w-3.5 shrink-0"
                  :class="activeColumn === 'sla' ? 'text-money-text' : 'opacity-40 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>

            <!-- 5. TTFT / Speed (sortable) -->
            <th class="px-2.5 pb-1.5 text-right font-normal">
              <button
                type="button"
                class="group/th inline-flex items-center justify-end gap-1 transition-colors duration-150 hover:text-ink"
                :class="activeColumn === 'ttft' ? 'font-medium text-money-text' : ''"
                @click="handleColumnSort('ttft')"
                @mouseenter="showTooltip($event, t('modelPlaza.table.ttftSpeedTooltip'))"
                @mouseleave="hideTooltip"
              >
                <span class="border-b border-dashed border-ink-3/60">
                  {{ t('modelPlaza.table.ttftSpeedHeader') }}
                </span>
                <Icon
                  :name="sortIconFor('ttft')"
                  size="xs"
                  class="h-3.5 w-3.5 shrink-0"
                  :class="activeColumn === 'ttft' ? 'text-money-text' : 'opacity-40 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>

            <!-- 6. Model Consistency -->
            <th class="px-2.5 pb-1.5 text-right font-normal">
              <span
                class="cursor-help border-b border-dashed border-ink-3/60"
                @mouseenter="showTooltip($event, t('modelPlaza.table.consistencyTooltip'))"
                @mouseleave="hideTooltip"
              >
                {{ t('modelPlaza.table.consistencyHeader') }}
              </span>
            </th>

            <!-- 7. Cache Rate (sortable) -->
            <th class="px-2.5 pb-1.5 text-right font-normal">
              <button
                type="button"
                class="group/th inline-flex items-center justify-end gap-1 transition-colors duration-150 hover:text-ink"
                :class="activeColumn === 'cache' ? 'font-medium text-money-text' : ''"
                @click="handleColumnSort('cache')"
                @mouseenter="showTooltip($event, t('modelPlaza.table.cacheRateTooltip'))"
                @mouseleave="hideTooltip"
              >
                <span class="border-b border-dashed border-ink-3/60">
                  {{ t('modelPlaza.table.cacheRateHeader') }}
                </span>
                <Icon
                  :name="sortIconFor('cache')"
                  size="xs"
                  class="h-3.5 w-3.5 shrink-0"
                  :class="activeColumn === 'cache' ? 'text-money-text' : 'opacity-40 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>

            <!-- 8. Reference Multiplier (sortable + dashed Tooltip) -->
            <th class="px-3 pb-1.5 text-right font-normal">
              <button
                type="button"
                class="group/th inline-flex items-center justify-end gap-1 transition-colors duration-150 hover:text-ink"
                :class="activeColumn === 'ref_multiplier' ? 'font-medium text-money-text' : ''"
                @click="handleColumnSort('ref_multiplier')"
                @mouseenter="showTooltip($event, t('modelPlaza.table.refMultiplierTooltip'))"
                @mouseleave="hideTooltip"
              >
                <span class="border-b border-dashed border-ink-3/70">
                  {{ t('modelPlaza.table.refMultiplierHeader') }}
                </span>
                <Icon
                  :name="sortIconFor('ref_multiplier')"
                  size="xs"
                  class="h-3.5 w-3.5 shrink-0"
                  :class="activeColumn === 'ref_multiplier' ? 'text-money-text' : 'opacity-40 group-hover/th:opacity-100'"
                  aria-hidden="true"
                />
              </button>
            </th>
          </tr>
        </thead>

        <tbody>
          <tr
            v-for="row in tableRows"
            :key="`${row.model.platform}:${row.model.name}`"
            tabindex="0"
            class="group cursor-pointer"
            :title="t('modelPlaza.table.clickForDetails')"
            @click="emit('select-model', row.model)"
            @keydown.enter.self="emit('select-model', row.model)"
            @keydown.space.prevent.self="emit('select-model', row.model)"
          >
            <!-- 1. Model -->
            <td class="bg-surface-tile px-3 py-3 align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <div class="flex min-w-0 items-center gap-2">
                <ModelIcon
                  :model="row.model.name"
                  size="18px"
                  :class="row.darkInvert ? 'dark:invert' : ''"
                  aria-hidden="true"
                />
                <span
                  class="min-w-0 flex-1 truncate font-mono text-[13.5px] font-semibold text-ink"
                  :title="row.model.name"
                >
                  {{ row.model.name }}
                </span>
                <button
                  type="button"
                  class="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-none transition-colors duration-150"
                  :class="
                    copiedModelName === row.model.name
                      ? 'bg-ok text-white'
                      : 'text-ink-3 opacity-60 hover:bg-surface hover:text-ink group-hover:opacity-100'
                  "
                  :title="
                    copiedModelName === row.model.name
                      ? t('modelPlaza.card.copied')
                      : t('modelPlaza.table.copyModelId', { model: row.model.name })
                  "
                  :aria-label="t('modelPlaza.table.copyModelId', { model: row.model.name })"
                  @click.stop="handleCopy(row.model.name)"
                >
                  <Icon
                    :name="copiedModelName === row.model.name ? 'check' : 'copy'"
                    size="xs"
                    class="h-3.5 w-3.5"
                    aria-hidden="true"
                  />
                </button>
              </div>
            </td>

            <!-- 2. Price (In / Out) -->
            <td class="bg-surface-tile px-2.5 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <template v-if="row.pricing.isToken">
                <template v-if="row.pricing.hasPrice">
                  <div class="truncate font-mono text-[13px] font-semibold text-ink">
                    {{ row.pricing.inputText }}
                    <span class="font-sans font-normal text-ink-3">/</span>
                    {{ row.pricing.outputText }}
                  </div>
                  <div class="mt-0.5 flex items-center justify-end gap-1.5 text-[11px] leading-4 text-ink-3">
                    <span>{{ t('modelPlaza.table.perMillionShort') }}</span>
                    <span v-if="row.pricing.tierCount > 1" aria-hidden="true">·</span>
                    <span v-if="row.pricing.tierCount > 1" class="text-ink-2">
                      {{ t('modelPlaza.card.tiersBadge', { count: row.pricing.tierCount }) }}
                    </span>
                    <span v-else-if="row.pricing.hasTimePricing" aria-hidden="true">·</span>
                    <span v-if="row.pricing.tierCount <= 1 && row.pricing.hasTimePricing" class="text-ink-2">
                      {{ t('modelPlaza.card.timePricingBadge') }}
                    </span>
                    <span
                      v-if="row.pricing.savingsPercent != null"
                      class="font-semibold text-ok"
                    >
                      {{ t('modelPlaza.card.savingsBadge', { percent: row.pricing.savingsPercent }) }}
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

            <!-- 3. Cache Read Price -->
            <td class="bg-surface-tile px-2.5 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
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

            <!-- 4. SLA (24h) -->
            <td class="bg-surface-tile px-2.5 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <span
                v-if="row.sla.valid"
                class="inline-flex items-center justify-end gap-1 font-semibold"
                :class="plazaToneTextClass(row.sla.tone)"
              >
                <Icon :name="row.slaIcon" size="xs" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
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

            <!-- 5. TTFT / Speed -->
            <td class="bg-surface-tile px-2.5 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <template v-if="row.ttft.valid">
                <div
                  class="whitespace-nowrap text-[13px]"
                  :class="row.ttft.tone === 'ok' ? 'font-semibold text-ok' : 'text-ink'"
                >
                  <span class="mr-1 text-xs font-normal text-ink-3">{{ t('modelPlaza.table.ttftPrefix') }}</span>
                  <span>{{ row.ttft.text }}</span>
                </div>
                <div v-if="row.tpsText" class="mt-0.5 text-[11px] leading-4 text-ink-3">
                  {{ row.tpsText }}
                </div>
              </template>
              <span
                v-else
                class="cursor-help text-ink-3"
                @mouseenter="showTooltip($event, row.ttftTooltip)"
                @mouseleave="hideTooltip"
              >
                {{ PLAZA_EMPTY_TEXT }}
              </span>
            </td>

            <!-- 6. Model Consistency -->
            <td class="bg-surface-tile px-2.5 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <span
                v-if="row.consistency.icon"
                class="inline-flex cursor-help items-center justify-end gap-1 text-xs font-semibold"
                :class="row.consistency.toneClass"
                @mouseenter="showTooltip($event, row.consistency.tooltip)"
                @mouseleave="hideTooltip"
              >
                <Icon :name="row.consistency.icon" size="xs" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                <span>{{ row.consistency.label }}</span>
              </span>
              <span
                v-else
                class="cursor-help text-ink-3"
                @mouseenter="showTooltip($event, row.consistency.tooltip)"
                @mouseleave="hideTooltip"
              >
                {{ PLAZA_EMPTY_TEXT }}
              </span>
            </td>

            <!-- 7. Cache Rate -->
            <td class="bg-surface-tile px-2.5 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <span
                v-if="row.cache.valid"
                class="font-semibold"
                :class="plazaToneTextClass(row.cache.tone)"
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

            <!-- 8. Reference Multiplier -->
            <td class="bg-surface-tile px-3 py-3 text-right align-middle transition-colors duration-150 group-hover:bg-surface-tile-2">
              <span
                v-if="row.refMult.valid"
                class="font-mono font-semibold"
                :class="plazaToneTextClass(row.refMult.tone)"
              >
                {{ row.refMult.text }}
              </span>
              <span
                v-else
                class="cursor-help text-ink-3"
                @mouseenter="showTooltip($event, row.refMultTooltip)"
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
import { formatScaled, resolveIntervalPrices } from '@/utils/pricing'
import {
  BILLING_MODE_IMAGE,
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
  computeOfficialSavingsPercent,
  formatCacheRate,
  formatConsistencyPercent,
  formatRefMultiplier,
  formatSlaPercent,
  formatTps,
  formatTtft,
  plazaToneTextClass
} from '@/utils/plazaFormat'

type TableColumnKey = 'model' | 'price' | 'sla' | 'ttft' | 'cache' | 'ref_multiplier'
type LocalSortOverride =
  | { column: TableColumnKey; dir: 'asc' | 'desc' }
  | null

interface Props {
  group: ModelPlazaGroup
  models: PlazaModel[]
  sortBy?: PlazaSortBy
  loading?: boolean
  hideEmptyState?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  sortBy: 'default',
  loading: false,
  hideEmptyState: false
})

const emit = defineEmits<{
  'select-model': [model: PlazaModel]
  'update:sortBy': [value: PlazaSortBy]
  'copy-model': [modelName: string]
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const PER_MILLION = 1_000_000
const MIN_DECIMALS = 2

const localSort = ref<LocalSortOverride>(null)
const copiedModelName = ref<string | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | null = null

watch(
  () => props.sortBy,
  () => {
    localSort.value = null
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

const activeSortDescriptor = computed<{ column: TableColumnKey; dir: 'asc' | 'desc' } | null>(() => {
  if (localSort.value) return localSort.value
  switch (props.sortBy) {
    case 'price_asc':
      return { column: 'price', dir: 'asc' }
    case 'price_desc':
      return { column: 'price', dir: 'desc' }
    case 'sla_desc':
      return { column: 'sla', dir: 'desc' }
    case 'ttft_asc':
      return { column: 'ttft', dir: 'asc' }
    case 'cache_desc':
      return { column: 'cache', dir: 'desc' }
    case 'ref_multiplier_asc':
      return { column: 'ref_multiplier', dir: 'asc' }
    default:
      return null
  }
})

const activeColumn = computed(() => activeSortDescriptor.value?.column ?? null)

function sortIconFor(column: TableColumnKey) {
  const active = activeSortDescriptor.value
  if (!active || active.column !== column) return 'sort' as const
  return active.dir === 'asc' ? ('arrowUp' as const) : ('arrowDown' as const)
}

function handleColumnSort(column: TableColumnKey) {
  const current = activeSortDescriptor.value
  if (column === 'model') {
    if (localSort.value?.column === 'model') {
      localSort.value =
        localSort.value.dir === 'asc' ? { column: 'model', dir: 'desc' } : null
    } else {
      localSort.value = { column: 'model', dir: 'asc' }
    }
    return
  }

  if (column === 'price') {
    localSort.value = null
    emit('update:sortBy', current?.column === 'price' && current.dir === 'asc' ? 'price_desc' : 'price_asc')
    return
  }

  if (column === 'sla') {
    if (current?.column === 'sla' && current.dir === 'desc') {
      localSort.value = { column: 'sla', dir: 'asc' }
    } else {
      localSort.value = null
      emit('update:sortBy', 'sla_desc')
    }
    return
  }

  if (column === 'ttft') {
    if (current?.column === 'ttft' && current.dir === 'asc') {
      localSort.value = { column: 'ttft', dir: 'desc' }
    } else {
      localSort.value = null
      emit('update:sortBy', 'ttft_asc')
    }
    return
  }

  if (column === 'cache') {
    if (current?.column === 'cache' && current.dir === 'desc') {
      localSort.value = { column: 'cache', dir: 'asc' }
    } else {
      localSort.value = null
      emit('update:sortBy', 'cache_desc')
    }
    return
  }

  if (column === 'ref_multiplier') {
    if (current?.column === 'ref_multiplier' && current.dir === 'asc') {
      localSort.value = { column: 'ref_multiplier', dir: 'desc' }
    } else {
      localSort.value = null
      emit('update:sortBy', 'ref_multiplier_asc')
    }
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

    const officialIntervals = sortByContext(model.official_pricing?.intervals ?? [])
    const officialIn = model.official_pricing?.input_price ?? officialIntervals[0]?.input_price ?? null
    const officialOut = model.official_pricing?.output_price ?? officialIntervals[0]?.output_price ?? null
    const savingsPercent =
      computeOfficialSavingsPercent(paidInNum, officialIn) ??
      computeOfficialSavingsPercent(paidOutNum, officialOut)

    return {
      isToken: true,
      hasPrice,
      sortPrice: paidInNum ?? paidOutNum,
      inputText: paidInNum != null ? formatScaled(paidInNum, PER_MILLION, MIN_DECIMALS) : PLAZA_EMPTY_TEXT,
      outputText: paidOutNum != null ? formatScaled(paidOutNum, PER_MILLION, MIN_DECIMALS) : PLAZA_EMPTY_TEXT,
      cacheReadText:
        rawCacheRead != null ? formatScaled(rawCacheRead * rate, PER_MILLION, MIN_DECIMALS) : null,
      requestPriceText: null as string | null,
      requestUnitSuffix: '',
      tierCount: intervals.length,
      hasTimePricing,
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
    isToken: false,
    hasPrice: rawReqPrice != null,
    sortPrice: rawReqPrice != null ? rawReqPrice * reqRate : null,
    inputText: PLAZA_EMPTY_TEXT,
    outputText: PLAZA_EMPTY_TEXT,
    cacheReadText: null as string | null,
    requestPriceText: rawReqPrice != null ? formatScaled(rawReqPrice * reqRate, 1, MIN_DECIMALS) : null,
    requestUnitSuffix,
    tierCount: reqIntervals.length,
    hasTimePricing,
    savingsPercent: null as number | null
  }
}

function buildConsistencyCell(model: PlazaModel) {
  const m = model.metrics
  if (!m || m.consistency_state === 'no_data') {
    return {
      icon: null,
      label: PLAZA_EMPTY_TEXT,
      toneClass: 'text-ink-3',
      tooltip: t('modelPlaza.common.noDataTooltip')
    }
  }
  if (m.consistency_state === 'low_sample') {
    return {
      icon: null,
      label: PLAZA_EMPTY_TEXT,
      toneClass: 'text-ink-3',
      tooltip: t('modelPlaza.consistency.lowSampleDesc')
    }
  }
  if (m.consistency_state === 'verified') {
    const pct = formatConsistencyPercent(m.consistency_rate, m.consistency_state, 2)
    const header = pct.valid
      ? `${t('modelPlaza.consistency.verified')} (${pct.text})`
      : t('modelPlaza.consistency.verified')
    const extra = m.auto_pause_protected ? ` · ${t('modelPlaza.consistency.autoPauseActive')}` : ''
    return {
      icon: 'shield' as const,
      label: t('modelPlaza.consistency.verified'),
      toneClass: 'text-ok',
      tooltip: `${header}${extra} — ${t('modelPlaza.consistency.methodDesc')}`
    }
  }
  const pct = formatConsistencyPercent(m.consistency_rate, m.consistency_state, 2)
  const header = pct.valid
    ? `${t('modelPlaza.consistency.mismatchShort')} (${pct.text})`
    : t('modelPlaza.consistency.mismatchShort')
  return {
    icon: 'exclamationTriangle' as const,
    label: t('modelPlaza.consistency.mismatchShort'),
    toneClass: pct.tone === 'bad' ? 'text-bad' : 'text-warn',
    tooltip: `${header} — ${t('modelPlaza.consistency.autoPauseDesc')}`
  }
}

function validMetricValue(
  val: number | null | undefined,
  state: PlazaMetricSampleState | null | undefined
): number | null {
  if (state != null && state !== 'valid') return null
  if (val == null || !Number.isFinite(val)) return null
  return val
}

function compareNullable(a: number | null, b: number | null, dir: 'asc' | 'desc'): number {
  // Missing / insufficient-sample values ALWAYS sort to the bottom
  if (a == null && b == null) return 0
  if (a == null) return 1
  if (b == null) return -1
  return dir === 'asc' ? a - b : b - a
}

const orderedModels = computed<PlazaModel[]>(() => {
  const override = localSort.value
  if (!override) return props.models

  const list = [...props.models]
  const { column, dir } = override
  list.sort((a, b) => {
    if (column === 'model') {
      return dir === 'asc' ? a.name.localeCompare(b.name) : b.name.localeCompare(a.name)
    }
    if (column === 'sla') {
      return (
        compareNullable(
          validMetricValue(a.metrics?.sla_24h, a.metrics?.sla_state),
          validMetricValue(b.metrics?.sla_24h, b.metrics?.sla_state),
          dir
        ) || a.name.localeCompare(b.name)
      )
    }
    if (column === 'ttft') {
      return (
        compareNullable(
          validMetricValue(a.metrics?.ttft_p50_ms, a.metrics?.ttft_state),
          validMetricValue(b.metrics?.ttft_p50_ms, b.metrics?.ttft_state),
          dir
        ) || a.name.localeCompare(b.name)
      )
    }
    if (column === 'cache') {
      return (
        compareNullable(
          validMetricValue(a.metrics?.cache_read_ratio, a.metrics?.cache_state),
          validMetricValue(b.metrics?.cache_read_ratio, b.metrics?.cache_state),
          dir
        ) || a.name.localeCompare(b.name)
      )
    }
    if (column === 'ref_multiplier') {
      return (
        compareNullable(
          validMetricValue(
            a.metrics?.reference_multiplier,
            a.metrics?.reference_multiplier_state
          ),
          validMetricValue(
            b.metrics?.reference_multiplier,
            b.metrics?.reference_multiplier_state
          ),
          dir
        ) || a.name.localeCompare(b.name)
      )
    }
    return 0
  })
  return list
})

const tableRows = computed(() =>
  orderedModels.value.map((model) => {
    const pricing = computePricing(model)
    const metrics = model.metrics
    const sla = formatSlaPercent(metrics?.sla_24h, metrics?.sla_state, 1)
    const ttft = formatTtft(metrics?.ttft_p50_ms, metrics?.ttft_state)
    const tpsText = formatTps(metrics?.output_tps_p50, metrics?.output_tps_state)
    const cache = formatCacheRate(metrics?.cache_read_ratio, metrics?.cache_state)
    const refMult = formatRefMultiplier(
      metrics?.reference_multiplier,
      metrics?.reference_multiplier_state
    )

    const slaIcon =
      sla.tone === 'ok'
        ? ('checkCircle' as const)
        : sla.tone === 'warn'
          ? ('exclamationTriangle' as const)
          : ('xCircle' as const)

    return {
      model,
      darkInvert: needsDarkInvert(model.name),
      pricing,
      sla,
      slaIcon,
      slaTooltip: sampleStateTooltip(metrics?.sla_state),
      ttft,
      ttftTooltip: sampleStateTooltip(metrics?.ttft_state),
      tpsText,
      consistency: buildConsistencyCell(model),
      cache,
      cacheTooltip: sampleStateTooltip(metrics?.cache_state),
      refMult,
      refMultTooltip: sampleStateTooltip(metrics?.reference_multiplier_state)
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
