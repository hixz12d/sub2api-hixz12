<template>
  <section
    class="rounded-none bg-surface-tile px-4 py-2.5 text-ink md:px-5"
    :aria-label="t('modelPlaza.strengthBar.title')"
  >
    <!-- Loading skeleton -->
    <div v-if="loading" class="flex min-h-[28px] items-center justify-between gap-4">
      <div class="grid flex-1 grid-cols-2 gap-3 md:flex md:items-center md:gap-6">
        <div v-for="i in 4" :key="i" class="h-5 w-28 animate-pulse bg-surface-tile-2" />
      </div>
      <div class="hidden h-5 w-24 animate-pulse bg-surface-tile-2 md:block" />
    </div>

    <!-- Content -->
    <div v-else class="flex flex-col md:flex-row md:items-center md:justify-between md:gap-4">
      <!-- 4 core site-wide live metrics: 2x2 grid on mobile, single row on md+ -->
      <div class="grid grid-cols-2 gap-x-4 gap-y-2 md:flex md:min-w-0 md:flex-wrap md:items-center md:gap-x-6 md:gap-y-1">
        <!-- 1. 30d SLA -->
        <HelpTooltip class="!ml-0" width-class="w-60">
          <template #trigger>
            <div class="inline-flex cursor-help items-center gap-1.5 text-xs md:text-[13px]">
              <Icon
                :name="slaIcon"
                size="xs"
                class="h-3.5 w-3.5 shrink-0"
                :class="plazaToneTextClass(sla30d.tone)"
                aria-hidden="true"
              />
              <span class="whitespace-nowrap text-ink-3">{{ t('modelPlaza.strengthBar.sla30d') }}</span>
              <span
                class="font-semibold tabular-nums"
                :class="sla30d.valid ? plazaToneTextClass(sla30d.tone) : 'text-ink-3'"
              >
                {{ sla30d.text }}
              </span>
            </div>
          </template>
          <div class="space-y-1.5">
            <div class="font-semibold">{{ t('modelPlaza.strengthBar.slaTooltipTitle') }}</div>
            <div v-if="!isReady" class="opacity-80">
              {{ t('modelPlaza.strengthBar.notReady') }}
            </div>
            <div v-else class="space-y-1 tabular-nums">
              <div class="flex items-center justify-between gap-4">
                <span class="opacity-80">{{ t('modelPlaza.strengthBar.sla24h') }}</span>
                <span class="font-mono font-semibold">{{ sla24h.text }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="opacity-80">{{ t('modelPlaza.strengthBar.sla7d') }}</span>
                <span class="font-mono font-semibold">{{ sla7d.text }}</span>
              </div>
              <div class="flex items-center justify-between gap-4">
                <span class="opacity-80">{{ t('modelPlaza.strengthBar.sla30dShort') }}</span>
                <span class="font-mono font-semibold">{{ sla30d.text }}</span>
              </div>
            </div>
          </div>
        </HelpTooltip>

        <!-- 2. Median TTFT -->
        <HelpTooltip class="!ml-0" width-class="w-64">
          <template #trigger>
            <div class="inline-flex cursor-help items-center gap-1.5 text-xs md:text-[13px]">
              <Icon
                name="clock"
                size="xs"
                class="h-3.5 w-3.5 shrink-0"
                :class="ttftMetric.tone === 'ok' ? 'text-ok' : 'text-ink-3'"
                aria-hidden="true"
              />
              <span class="whitespace-nowrap text-ink-3">{{ t('modelPlaza.strengthBar.ttft') }}</span>
              <span
                class="font-semibold tabular-nums"
                :class="ttftMetric.valid ? (ttftMetric.tone === 'ok' ? 'text-ok' : 'text-ink') : 'text-ink-3'"
              >
                {{ ttftMetric.text }}
              </span>
            </div>
          </template>
          <div class="space-y-1">
            <div>{{ t('modelPlaza.strengthBar.ttftTooltip') }}</div>
            <div v-if="!isReady" class="opacity-75">{{ t('modelPlaza.strengthBar.notReady') }}</div>
          </div>
        </HelpTooltip>

        <!-- 3. Average Cache Hit -->
        <HelpTooltip class="!ml-0" width-class="w-64">
          <template #trigger>
            <div class="inline-flex cursor-help items-center gap-1.5 text-xs md:text-[13px]">
              <Icon
                name="database"
                size="xs"
                class="h-3.5 w-3.5 shrink-0"
                :class="cacheMetric.valid ? plazaToneTextClass(cacheMetric.tone) : 'text-ink-3'"
                aria-hidden="true"
              />
              <span class="whitespace-nowrap text-ink-3">{{ t('modelPlaza.strengthBar.cacheRate') }}</span>
              <span
                class="font-semibold tabular-nums"
                :class="cacheMetric.valid ? plazaToneTextClass(cacheMetric.tone) : 'text-ink-3'"
              >
                {{ cacheMetric.text }}
              </span>
            </div>
          </template>
          <div class="space-y-1">
            <div>{{ t('modelPlaza.strengthBar.cacheRateTooltip') }}</div>
            <div v-if="!isReady" class="opacity-75">{{ t('modelPlaza.strengthBar.notReady') }}</div>
          </div>
        </HelpTooltip>

        <!-- 4. Model Consistency Rate -->
        <HelpTooltip class="!ml-0" width-class="w-72">
          <template #trigger>
            <div class="inline-flex cursor-help items-center gap-1.5 text-xs md:text-[13px]">
              <Icon
                :name="consistencyIcon"
                size="xs"
                class="h-3.5 w-3.5 shrink-0"
                :class="consistencyMetric.valid ? plazaToneTextClass(consistencyMetric.tone) : 'text-ink-3'"
                aria-hidden="true"
              />
              <span class="whitespace-nowrap text-ink-3">{{ t('modelPlaza.strengthBar.consistencyRate') }}</span>
              <span
                class="font-semibold tabular-nums"
                :class="consistencyMetric.valid ? plazaToneTextClass(consistencyMetric.tone) : 'text-ink-3'"
              >
                {{ consistencyMetric.text }}
              </span>
            </div>
          </template>
          <div class="space-y-1">
            <div>{{ t('modelPlaza.strengthBar.consistencyTooltip') }}</div>
            <div v-if="!isReady" class="opacity-75">{{ t('modelPlaza.strengthBar.notReady') }}</div>
          </div>
        </HelpTooltip>
      </div>

      <!-- Right side: updated relative time / warmup hint + monitor link -->
      <div
        v-if="updatedLabel || !isReady || canOpenMonitor"
        class="mt-2 flex shrink-0 items-center justify-between gap-4 border-t border-rule pt-2 text-xs md:mt-0 md:justify-end md:border-t-0 md:pt-0"
      >
        <span v-if="updatedLabel" class="whitespace-nowrap text-ink-3 tabular-nums">
          {{ updatedLabel }}
        </span>
        <span v-else-if="!isReady" class="whitespace-nowrap text-ink-3">
          {{ t('modelPlaza.strengthBar.notReady') }}
        </span>
        <RouterLink
          v-if="canOpenMonitor"
          :to="monitorTo"
          class="inline-flex items-center gap-1 whitespace-nowrap font-medium text-money-text transition-colors duration-150 hover:underline"
        >
          <span>{{ t('modelPlaza.strengthBar.openMonitor') }}</span>
          <Icon name="arrowRight" size="xs" class="h-3.5 w-3.5" aria-hidden="true" />
        </RouterLink>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import type { PlazaSiteSummary } from '@/api/modelPlaza'
import {
  PLAZA_EMPTY_TEXT,
  formatCacheRate,
  formatConsistencyPercent,
  formatRelativeUpdatedAt,
  formatSlaPercent,
  formatTtft,
  plazaToneTextClass,
  type PlazaFormattedMetric
} from '@/utils/plazaFormat'

interface Props {
  summary?: PlazaSiteSummary | null
  canOpenMonitor?: boolean
  monitorTo?: string
  loading?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  summary: null,
  canOpenMonitor: false,
  monitorTo: '/monitor',
  loading: false
})

const { t } = useI18n()

const EMPTY_METRIC: PlazaFormattedMetric = {
  text: PLAZA_EMPTY_TEXT,
  tone: 'muted',
  valid: false
}

const isReady = computed(() => Boolean(props.summary?.aggregation_ready))

const sla30d = computed(() =>
  isReady.value ? formatSlaPercent(props.summary?.sla_30d, null, 2) : EMPTY_METRIC
)
const sla7d = computed(() =>
  isReady.value ? formatSlaPercent(props.summary?.sla_7d, null, 2) : EMPTY_METRIC
)
const sla24h = computed(() =>
  isReady.value ? formatSlaPercent(props.summary?.sla_24h, null, 2) : EMPTY_METRIC
)

const slaIcon = computed(() => {
  if (!sla30d.value.valid) return 'checkCircle' as const
  if (sla30d.value.tone === 'ok') return 'checkCircle' as const
  if (sla30d.value.tone === 'warn') return 'exclamationTriangle' as const
  return 'xCircle' as const
})

const ttftMetric = computed(() =>
  isReady.value ? formatTtft(props.summary?.ttft_p50_ms, null) : EMPTY_METRIC
)

const cacheMetric = computed(() =>
  isReady.value ? formatCacheRate(props.summary?.cache_read_ratio, null, 2) : EMPTY_METRIC
)

const consistencyMetric = computed(() =>
  isReady.value ? formatConsistencyPercent(props.summary?.consistency_rate, null, 2) : EMPTY_METRIC
)

const consistencyIcon = computed(() => {
  if (consistencyMetric.value.valid && (consistencyMetric.value.tone === 'warn' || consistencyMetric.value.tone === 'bad')) {
    return 'exclamationTriangle' as const
  }
  return 'shield' as const
})

const updatedLabel = computed(() => {
  const relative = formatRelativeUpdatedAt(props.summary?.updated_at, t)
  return relative ? t('modelPlaza.strengthBar.updatedPrefix', { time: relative }) : ''
})
</script>
