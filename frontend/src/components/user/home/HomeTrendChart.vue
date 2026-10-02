<template>
  <div class="bg-surface-tile px-3 pb-2 pt-4">
    <div class="flex gap-[18px] px-3 pb-2.5 text-[13px] text-ink-2">
      <span class="inline-flex items-center gap-1.5"><i class="inline-block h-2.5 w-2.5 bg-chart-bar"></i>{{ t('userHome.trend.legendTokens') }}</span>
      <span class="inline-flex items-center gap-1.5"><i class="inline-block h-[3px] w-2.5 bg-chart-line"></i>{{ t('userHome.trend.legendCost') }}</span>
    </div>
    <div class="relative h-[200px] lg:h-[240px]">
      <div v-if="loading" class="absolute inset-0 animate-pulse bg-surface-tile-2"></div>
      <div v-else-if="isEmpty" class="flex h-full items-center justify-center text-sm text-ink-3">{{ t('userHome.trend.empty') }}</div>
      <Bar v-else :key="themeKey" :data="chartData" :options="chartOptions" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Bar } from 'vue-chartjs'
import {
  BarController,
  BarElement,
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
import { formatHomeMoney, formatHomeTokens, themeColor } from './homeFormat'

ChartJS.register(BarController, BarElement, LineController, LineElement, PointElement, CategoryScale, LinearScale, Tooltip)

export interface HomeTrendBucket {
  label: string
  /** null：时间还没到（今天之后的小时） */
  tokens: number | null
  cost: number | null
}

const props = defineProps<{
  buckets: HomeTrendBucket[]
  loading: boolean
}>()

const { t } = useI18n()

// 主题切换时重新取色（颜色来自 CSS 变量）
const themeKey = ref(0)
let observer: MutationObserver | null = null
onMounted(() => {
  observer = new MutationObserver(() => themeKey.value++)
  observer.observe(document.documentElement, { attributes: true, attributeFilter: ['class'] })
})
onBeforeUnmount(() => observer?.disconnect())

const isEmpty = computed(() => props.buckets.every((b) => !b.tokens && !b.cost))

const chartData = computed(() => {
  void themeKey.value
  return {
    labels: props.buckets.map((b) => b.label),
    datasets: [
      {
        type: 'bar',
        label: t('userHome.trend.legendTokens'),
        data: props.buckets.map((b) => b.tokens),
        backgroundColor: themeColor('chart-bar'),
        borderRadius: 0,
        categoryPercentage: 0.7,
        barPercentage: 1,
        yAxisID: 'tokens',
        order: 2
      },
      {
        type: 'line',
        label: t('userHome.trend.legendCost'),
        data: props.buckets.map((b) => b.cost),
        borderColor: themeColor('chart-line'),
        backgroundColor: themeColor('chart-line'),
        borderWidth: 3,
        pointRadius: 0,
        pointHoverRadius: 4,
        tension: 0,
        yAxisID: 'cost',
        order: 1
      }
    ]
  } as unknown as ChartData<'bar'>
})

const chartOptions = computed((): ChartOptions<'bar'> => {
  void themeKey.value
  const tick = themeColor('ink-3')
  const grid = themeColor('rule')
  const reduce = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  return {
    responsive: true,
    maintainAspectRatio: false,
    animation: reduce ? false : { duration: 500, easing: 'easeOutQuart' },
    interaction: { mode: 'index', intersect: false },
    plugins: {
      legend: { display: false },
      tooltip: {
        backgroundColor: themeColor('ink'),
        titleColor: themeColor('bg'),
        bodyColor: themeColor('bg'),
        cornerRadius: 0,
        padding: 10,
        displayColors: false,
        callbacks: {
          label: (ctx) =>
            ctx.dataset.yAxisID === 'cost'
              ? `${t('userHome.trend.legendCost')}  ${formatHomeMoney(Number(ctx.parsed.y))}`
              : `${t('userHome.trend.legendTokens')}  ${formatHomeTokens(Number(ctx.parsed.y))}`
        }
      }
    },
    scales: {
      x: {
        grid: { display: false },
        border: { color: grid },
        ticks: { color: tick, font: { size: 11 }, maxRotation: 0, autoSkip: true, maxTicksLimit: 8 }
      },
      tokens: {
        position: 'left',
        beginAtZero: true,
        grid: { color: grid },
        border: { display: false },
        ticks: { color: tick, font: { size: 11 }, maxTicksLimit: 5, callback: (v) => formatHomeTokens(Number(v)) }
      },
      cost: {
        position: 'right',
        beginAtZero: true,
        grid: { display: false },
        border: { display: false },
        ticks: { color: tick, font: { size: 11 }, maxTicksLimit: 5, callback: (v) => formatHomeMoney(Number(v)) }
      }
    }
  }
})
</script>
