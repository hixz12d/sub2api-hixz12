<template>
  <div class="space-y-3">
    <!-- 3-window SLA scorecard tiles (24h / 7d / 30d) -->
    <div v-if="loading && !windows" class="grid grid-cols-1 gap-tile-gap sm:grid-cols-3">
      <div
        v-for="idx in 3"
        :key="idx"
        class="tile p-3.5"
      >
        <div class="skeleton h-3.5 w-16" />
        <div class="skeleton mt-2.5 h-7 w-24" />
        <div class="skeleton mt-3 h-3 w-full" />
        <div class="skeleton mt-1.5 h-3 w-4/5" />
      </div>
    </div>

    <div v-else class="grid grid-cols-1 gap-tile-gap sm:grid-cols-3">
      <div
        v-for="card in windowCards"
        :key="card.key"
        class="tile p-3.5"
      >
        <div class="text-xs text-ink-3">
          {{ card.label }}
        </div>

        <!-- Primary SLA metric with dual color + icon encoding -->
        <div
          class="mt-1.5 flex flex-wrap items-center gap-1.5"
          :title="card.slaTooltip"
        >
          <Icon
            v-if="card.slaIcon"
            :name="card.slaIcon"
            size="sm"
            class="shrink-0"
            :class="plazaToneTextClass(card.sla.tone)"
          />
          <span
            class="text-xl font-normal leading-none tracking-tight tabular-nums"
            :class="plazaToneTextClass(card.sla.tone)"
          >
            {{ card.sla.text }}
          </span>
          <span
            v-if="card.slaFromGroupPool"
            class="badge badge-gray text-[11px] font-normal"
            :title="card.slaTooltip"
          >
            {{ t('modelPlaza.drawer.groupPoolFallbackBadge') }}
          </span>
        </div>

        <!-- Secondary metrics: TTFT P50, Cache Hit, optional TPS -->
        <div class="mt-3 space-y-1 border-t border-rule pt-2 text-xs">
          <div class="flex items-center justify-between gap-2" :title="card.ttftTooltip">
            <span class="text-ink-3">{{ t('modelPlaza.drawer.windowTtft') }}</span>
            <span class="font-mono tabular-nums" :class="plazaToneTextClass(card.ttft.tone)">
              {{ card.ttft.text }}
            </span>
          </div>
          <div class="flex items-center justify-between gap-2" :title="card.cacheTooltip">
            <span class="text-ink-3">{{ t('modelPlaza.drawer.windowCache') }}</span>
            <span class="font-mono tabular-nums" :class="plazaToneTextClass(card.cache.tone)">
              {{ card.cache.text }}
            </span>
          </div>
          <div
            v-if="card.tpsText"
            class="flex items-center justify-between gap-2"
          >
            <span class="text-ink-3">{{ t('modelPlaza.drawer.windowTps') }}</span>
            <span class="font-mono tabular-nums text-ink-2">
              {{ card.tpsText }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <div
      v-if="hasGroupPoolFallback"
      class="flex items-start gap-2 bg-surface-tile px-3.5 py-2 text-[11px] leading-relaxed text-ink-3"
    >
      <Icon name="infoCircle" size="xs" class="mt-0.5 shrink-0 text-ink-3" />
      <span>{{ t('modelPlaza.drawer.groupPoolFallbackHint') }}</span>
    </div>

    <!-- 72-slot 24h heartbeat strip -->
    <div class="tile p-4">
      <div class="mb-2.5 flex items-center justify-between gap-2">
        <span class="text-xs font-medium text-ink-2">
          {{ t('modelPlaza.drawer.heartbeatTitle') }}
        </span>
      </div>

      <div v-if="loading && heartbeat72.length === 0" class="skeleton h-[28px] w-full" />
      <PlazaHeartbeatBar v-else :slots="heartbeat72" variant="full" />

      <div class="mt-2 flex flex-wrap items-center justify-between gap-2 text-[11px] text-ink-3">
        <div class="flex items-center gap-3 tabular-nums">
          <span>{{ t('modelPlaza.heartbeat.range24hStart') }}</span>
          <span>—</span>
          <span>{{ t('modelPlaza.heartbeat.rangeNow') }}</span>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <span class="inline-flex items-center gap-1">
            <i class="inline-block h-2 w-2 bg-ok" />
            {{ t('modelPlaza.heartbeat.normal') }}
          </span>
          <span class="inline-flex items-center gap-1">
            <i class="inline-block h-2 w-2 bg-warn" />
            {{ t('modelPlaza.heartbeat.partialFailure') }}
          </span>
          <span class="inline-flex items-center gap-1">
            <i class="inline-block h-2 w-2 bg-bad" />
            {{ t('modelPlaza.heartbeat.manyFailures') }}
          </span>
          <span
            class="inline-flex items-center gap-1"
            :title="t('modelPlaza.heartbeat.idleTooltip')"
          >
            <i class="inline-block h-2 w-2 bg-ink-3/20 dark:bg-ink-3/25" />
            {{ t('modelPlaza.heartbeat.idle') }}
          </span>
          <span class="inline-flex items-center gap-1">
            <i class="inline-block h-2 w-2 border border-rule bg-surface-tile-2" />
            {{ t('modelPlaza.heartbeat.lowSample') }}
          </span>
        </div>
      </div>
    </div>

    <!-- 24h trend chart (Chart.js, DESIGN.md flat straight line) -->
    <div class="tile p-4">
      <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
        <span class="text-xs font-medium text-ink-2">
          {{ t('modelPlaza.drawer.trendTitle') }}
        </span>
        <div class="tabs" role="tablist">
          <button
            v-for="tab in metricTabs"
            :key="tab.id"
            type="button"
            role="tab"
            :aria-selected="activeMetric === tab.id"
            class="tab h-7 px-2.5 py-0 text-xs"
            :class="{ 'tab-active': activeMetric === tab.id }"
            @click="activeMetric = tab.id"
          >
            {{ tab.label }}
          </button>
        </div>
      </div>

      <div v-if="loading && trend24h.length === 0" class="skeleton h-[180px] w-full" />

      <div
        v-else-if="!hasValidPoints"
        class="flex items-center justify-center gap-2 bg-surface-tile-2 px-3 py-5 text-xs text-ink-3"
      >
        <Icon name="chart" size="sm" class="shrink-0 text-ink-3" />
        <span>{{ t('modelPlaza.drawer.trendEmpty') }}</span>
      </div>

      <div v-else class="relative h-[180px]">
        <Line :key="`${themeKey}-${activeMetric}`" :data="chartData" :options="chartOptions" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Line } from 'vue-chartjs'
import {
  CategoryScale,
  Chart as ChartJS,
  LineController,
  LineElement,
  LinearScale,
  PointElement,
  Tooltip,
  type ChartData,
  type ChartOptions
} from 'chart.js'
import Icon from '@/components/icons/Icon.vue'
import PlazaHeartbeatBar from './PlazaHeartbeatBar.vue'
import {
  formatCacheRate,
  formatSlaPercent,
  formatTps,
  formatTtft,
  plazaToneTextClass,
  type PlazaFormattedMetric,
  type PlazaMetricTone
} from '@/utils/plazaFormat'
import type {
  ModelPlazaDetailResponse,
  PlazaDetailTrendPoint,
  PlazaDetailWindow,
  PlazaHeartbeatSlot,
  PlazaMetricSampleState
} from '@/api/modelPlaza'

ChartJS.register(CategoryScale, LinearScale, LineController, LineElement, PointElement, Tooltip)

const props = withDefaults(
  defineProps<{
    windows: ModelPlazaDetailResponse['windows'] | null
    heartbeat72: PlazaHeartbeatSlot[]
    trend24h: PlazaDetailTrendPoint[]
    loading?: boolean
  }>(),
  {
    loading: false
  }
)

const { t } = useI18n()

type TrendMetricKey = 'ttft' | 'cache' | 'sla'
const activeMetric = ref<TrendMetricKey>('ttft')

const metricTabs = computed(() => [
  { id: 'ttft' as const, label: t('modelPlaza.drawer.trendMetricTtft') },
  { id: 'cache' as const, label: t('modelPlaza.drawer.trendMetricCache') },
  { id: 'sla' as const, label: t('modelPlaza.drawer.trendMetricSla') }
])

function sampleStateTooltip(state?: PlazaMetricSampleState | null): string | undefined {
  if (state === 'low_sample') return t('modelPlaza.common.lowSampleTooltip')
  if (state === 'no_data') return t('modelPlaza.common.noDataTooltip')
  return undefined
}

function slaIconForTone(
  tone: PlazaMetricTone,
  valid: boolean
): 'checkCircle' | 'exclamationTriangle' | 'xCircle' | null {
  if (!valid) return null
  if (tone === 'ok') return 'checkCircle'
  if (tone === 'warn') return 'exclamationTriangle'
  if (tone === 'bad') return 'xCircle'
  return null
}

interface WindowCardModel {
  key: 'h24' | 'd7' | 'd30'
  label: string
  sla: PlazaFormattedMetric
  slaIcon: 'checkCircle' | 'exclamationTriangle' | 'xCircle' | null
  slaFromGroupPool: boolean
  slaTooltip?: string
  ttft: PlazaFormattedMetric
  ttftTooltip?: string
  cache: PlazaFormattedMetric
  cacheTooltip?: string
  tpsText: string | null
}

function buildWindowCard(
  key: 'h24' | 'd7' | 'd30',
  label: string,
  win?: PlazaDetailWindow | null
): WindowCardModel {
  const sla = formatSlaPercent(win?.sla, win?.sla_state, 2)
  const slaFromGroupPool = sla.valid && win?.sla_source === 'group_pool'
  const ttft = formatTtft(win?.ttft_p50_ms, win?.ttft_state)
  const cache = formatCacheRate(win?.cache_read_ratio, win?.cache_state, 2)
  const tpsText = formatTps(win?.output_tps_p50, win?.output_tps_state)

  return {
    key,
    label,
    sla,
    slaIcon: slaIconForTone(sla.tone, sla.valid),
    slaFromGroupPool,
    slaTooltip: slaFromGroupPool
      ? t('modelPlaza.drawer.groupPoolFallbackHint')
      : sla.valid
        ? undefined
        : sampleStateTooltip(win?.sla_state),
    ttft,
    ttftTooltip: ttft.valid ? undefined : sampleStateTooltip(win?.ttft_state),
    cache,
    cacheTooltip: cache.valid ? undefined : sampleStateTooltip(win?.cache_state),
    tpsText
  }
}

const windowCards = computed<WindowCardModel[]>(() => [
  buildWindowCard('h24', t('modelPlaza.drawer.window24h'), props.windows?.h24),
  buildWindowCard('d7', t('modelPlaza.drawer.window7d'), props.windows?.d7),
  buildWindowCard('d30', t('modelPlaza.drawer.window30d'), props.windows?.d30)
])

const hasGroupPoolFallback = computed(() =>
  windowCards.value.some((card) => card.slaFromGroupPool)
)

// Theme observer: re-read CSS variables whenever light/dark or UI style switches
const themeKey = ref(0)
let observer: MutationObserver | null = null

onMounted(() => {
  if (typeof MutationObserver === 'undefined' || typeof document === 'undefined') return
  observer = new MutationObserver(() => {
    themeKey.value++
  })
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class', 'data-ui']
  })
})

onBeforeUnmount(() => {
  observer?.disconnect()
})

function readThemeColor(name: string, alpha = 1): string {
  if (typeof window === 'undefined' || typeof document === 'undefined') {
    return `rgba(128, 128, 128, ${alpha})`
  }
  const raw = getComputedStyle(document.documentElement).getPropertyValue(`--c-${name}`).trim()
  const parts = raw.split(/\s+/).map(Number)
  if (parts.length !== 3 || parts.some((n) => !Number.isFinite(n))) {
    return `rgba(128, 128, 128, ${alpha})`
  }
  return `rgba(${parts[0]}, ${parts[1]}, ${parts[2]}, ${alpha})`
}

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function formatBucketLabel(iso: string): string {
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}`
}

const activeSeriesValues = computed<Array<number | null>>(() =>
  props.trend24h.map((pt) => {
    if (activeMetric.value === 'ttft') {
      return pt.ttft_p50_ms != null && Number.isFinite(pt.ttft_p50_ms) ? pt.ttft_p50_ms : null
    }
    if (activeMetric.value === 'cache') {
      return pt.cache_read_ratio != null && Number.isFinite(pt.cache_read_ratio)
        ? Math.max(0, Math.min(100, pt.cache_read_ratio * 100))
        : null
    }
    return pt.success_rate != null && Number.isFinite(pt.success_rate)
      ? Math.max(0, Math.min(100, pt.success_rate * 100))
      : null
  })
)

const validPointCount = computed(
  () => activeSeriesValues.value.filter((v): v is number => v != null && Number.isFinite(v)).length
)

const hasValidPoints = computed(() => validPointCount.value > 0)

function formatTtftAxis(ms: number): string {
  if (!Number.isFinite(ms)) return ''
  return ms < 1000 ? `${Math.round(ms)}ms` : `${(ms / 1000).toFixed(1)}s`
}

const chartData = computed<ChartData<'line'>>(() => {
  void themeKey.value
  const lineToken =
    activeMetric.value === 'ttft'
      ? 'series-2'
      : activeMetric.value === 'cache'
        ? 'chart-line'
        : 'ok'
  const lineColor = readThemeColor(lineToken)
  const activeLabel =
    metricTabs.value.find((tab) => tab.id === activeMetric.value)?.label ?? ''

  return {
    labels: props.trend24h.map((pt) => formatBucketLabel(pt.bucket_start)),
    datasets: [
      {
        label: activeLabel,
        data: activeSeriesValues.value,
        borderColor: lineColor,
        backgroundColor: lineColor,
        borderWidth: 2.5,
        pointRadius: validPointCount.value <= 1 ? 3 : 0,
        pointHoverRadius: 4,
        tension: 0,
        fill: false,
        spanGaps: false
      }
    ]
  }
})

const chartOptions = computed<ChartOptions<'line'>>(() => {
  void themeKey.value
  const tickColor = readThemeColor('ink-3')
  const gridColor = readThemeColor('rule')
  const tooltipBg = readThemeColor('ink')
  const tooltipInk = readThemeColor('bg')
  const reduceMotion =
    typeof window !== 'undefined' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches

  const metric = activeMetric.value
  const isPercent = metric === 'cache' || metric === 'sla'

  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: reduceMotion ? false : { duration: 500, easing: 'easeOutQuart' },
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: tooltipBg,
        titleColor: tooltipInk,
        bodyColor: tooltipInk,
        cornerRadius: 0,
        padding: 10,
        displayColors: false,
        callbacks: {
          label: (ctx) => {
            const val = ctx.parsed.y
            if (val == null || !Number.isFinite(val)) return ''
            const formatted =
              metric === 'ttft' ? formatTtftAxis(val) : `${val.toFixed(2)}%`
            return `${ctx.dataset.label ?? ''}  ${formatted}`
          }
        }
      }
    },
    scales: {
      x: {
        grid: { display: false },
        border: { color: gridColor },
        ticks: {
          color: tickColor,
          font: { size: 11 },
          maxRotation: 0,
          autoSkip: true,
          maxTicksLimit: 6
        }
      },
      y: {
        beginAtZero: metric !== 'sla',
        suggestedMin: metric === 'sla' ? 90 : undefined,
        max: isPercent ? 100 : undefined,
        grid: { color: gridColor },
        border: { display: false },
        ticks: {
          color: tickColor,
          font: { size: 11 },
          maxTicksLimit: 4,
          callback: (value) => {
            const num = Number(value)
            return metric === 'ttft' ? formatTtftAxis(num) : `${Math.round(num)}%`
          }
        }
      }
    }
  }
})
</script>
