<template>
  <section class="observations" :aria-label="text.title" :aria-busy="loading">
    <header class="observations-header">
      <h2>{{ text.title }}</h2>
      <div class="window-tabs" role="tablist" :aria-label="text.window">
        <button v-for="item in windows" :key="item" type="button" role="tab" :aria-selected="windowKey === item" @click="windowKey = item">{{ item === 'h24' ? '24h' : '7d' }}</button>
      </div>
      <button type="button" :disabled="loading" :title="text.refresh" :aria-label="text.refresh" @click="load(1, true)"><Icon name="refresh" size="sm" /></button>
    </header>
    <p v-if="error" role="alert">{{ text.error }}</p>
    <p v-else-if="loading && !data">{{ text.loading }}</p>
    <p v-else-if="!data?.items.length">{{ text.empty }}</p>
    <template v-else>
      <p v-if="loading" role="status">{{ text.updating }}</p>
      <p v-else-if="data.stale" role="status">{{ text.stale }}</p>
      <div class="observations-grid">
        <article v-for="card in data.items" :key="JSON.stringify(card.identity)" class="observation-card">
          <h3>{{ card.display.group_label }}</h3>
          <p class="model-label">{{ card.display.platform_label }} · {{ card.display.model_label }}</p>
          <dl>
            <div><dt>{{ text.tps }}</dt><dd>{{ value(card, 'tps') }}<span class="sample-state">{{ sampleState(card, 'tps') }}</span></dd></div>
            <div><dt>{{ text.ttft }}</dt><dd>{{ value(card, 'ttft') }}<span class="sample-state">{{ sampleState(card, 'visible_ttft') }}</span></dd></div>
            <div class="cache-metric"><dt>{{ text.cache }}</dt><dd>{{ value(card, 'cache') }}<span class="sample-state">{{ sampleState(card, 'cache') }}</span></dd></div>
          </dl>
          <p class="observation-state">{{ reason(card) }}</p>
          <HumanAssessmentLabel v-if="admin" group :summary="humanAssessments[card.identity.group_id]" :failed="humanAssessmentsFailed" />
        </article>
      </div>
      <footer>
        <button type="button" :disabled="page <= 1 || loading" :title="text.previous" :aria-label="text.previous" @click="load(page - 1)"><Icon name="chevronLeft" size="sm" /></button>
        <span>{{ page }}</span>
        <button type="button" :disabled="!data.has_more || loading" :title="text.next" :aria-label="text.next" @click="load(page + 1)"><Icon name="chevronRight" size="sm" /></button>
      </footer>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { getCards, type MonitorCardsResponse, type MonitorFilter, type MonitorStatusCard } from '@/api/channelMonitorV2'
import { observedCardValue } from './observedCardFormat'
import HumanAssessmentLabel from '@/components/admin/account/HumanAssessmentLabel.vue'
import { useHumanAssessments } from '@/composables/useHumanAssessments'

const props = defineProps<{ filter: MonitorFilter; admin?: boolean; refreshKey?: number }>()
const { locale } = useI18n()
const text = computed(() => locale.value.startsWith('zh') ? {
  lowSample: '样本不足', sufficient: '样本达阈值', unavailable: '证据未提供',
  title: '文本输出观测', window: '观测窗口', refresh: '刷新', previous: '上一页', next: '下一页',
  cache: '观测缓存读取率', tps: '输出速度 P50', ttft: '可见首字 P90', loading: '加载中', updating: '更新中', empty: '暂无观测记录',
  error: '观测暂不可用，请重试', stale: '聚合数据延迟', partial: '采集覆盖不足',
  noData: '无可测样本', overflow: '输出速度超出范围', measured: '观测值 · 近似分位',
} : {
  lowSample: 'Low sample', sufficient: 'Sample threshold met', unavailable: 'Evidence unavailable',
  title: 'Text output observations', window: 'Observation window', refresh: 'Refresh', previous: 'Previous page', next: 'Next page',
  cache: 'Observed cache read ratio', tps: 'Output speed P50', ttft: 'Visible first text P90', loading: 'Loading', updating: 'Updating', empty: 'No observations',
  error: 'Observations unavailable. Please retry.', stale: 'Aggregation delayed', partial: 'Incomplete collection coverage',
  noData: 'No measurable samples', overflow: 'Output speed out of range', measured: 'Observed · Approximate percentile',
})
const windows = ['h24', 'd7'] as const
const windowKey = ref<'h24' | 'd7'>('h24')
const data = ref<MonitorCardsResponse | null>(null)
const { summaries: humanAssessments, failed: humanAssessmentsFailed } = useHumanAssessments('group', computed(() => props.admin ? (data.value?.items.map(card => card.identity.group_id) || []) : []))
const page = ref(1)
const loading = ref(false)
const error = ref(false)
let controller: AbortController | undefined
let sequence = 0

async function load(target = 1, resetSnapshot = false) {
  controller?.abort()
  controller = new AbortController()
  const current = ++sequence
  const asOf = resetSnapshot ? undefined : data.value?.as_of ?? undefined
  loading.value = true
  error.value = false
  try {
    const response = await getCards(props.filter, { page: target, page_size: 12, as_of: asOf }, props.admin, controller.signal)
    if (current !== sequence) return
    data.value = response
    page.value = target
  } catch {
    if (current !== sequence) return
    error.value = true
    data.value = null
  } finally {
    if (current === sequence) loading.value = false
  }
}
function value(card: MonitorStatusCard, kind: 'tps' | 'ttft' | 'cache') {
  return observedCardValue(card.windows[windowKey.value], kind)
}
function sampleState(card: MonitorStatusCard, metric: 'tps' | 'visible_ttft' | 'cache') {
  const window = card.windows[windowKey.value]
  const state = window.observation_evidence?.[metric] ?? (metric === 'cache' ? window.observed_cache_reason : window.output_tps_reason === 'partial_coverage' ? 'partial_coverage' : undefined)
  switch (state) {
    case 'valid': return text.value.sufficient
    case 'low_sample': return text.value.lowSample
    case 'no_data': return text.value.noData
    case 'partial_coverage': return text.value.partial
    case 'out_of_range': return text.value.overflow
    default: return text.value.unavailable
  }
}
function reason(card: MonitorStatusCard) {
  const why = card.windows[windowKey.value].output_tps_reason
  if (why === 'partial_coverage') return text.value.partial
  if (why === 'out_of_range') return text.value.overflow
  if (value(card, 'tps') === '-' && value(card, 'ttft') === '-' && value(card, 'cache') === '-') return text.value.noData
  return text.value.measured
}
// Snapshot the scope: deep watchers otherwise retain the same mutated object.
const requestScope = computed(() => JSON.stringify([props.filter, Boolean(props.admin)]))
watch([requestScope, () => props.refreshKey], ([scope], previous) => {
  if (!previous || scope !== previous[0]) {
    data.value = null
    page.value = 1
  }
  void load(1, true)
}, { immediate: true })
onBeforeUnmount(() => { sequence++; controller?.abort() })
</script>

<style scoped>
.observations { @apply border-gray-200 text-gray-900 dark:border-dark-700 dark:text-gray-100; min-width: 0; border-top-width: 1px; padding: 1rem 0; letter-spacing: 0; }
.observations-header { display: flex; align-items: center; gap: .75rem; flex-wrap: wrap; margin-bottom: 1rem; }
h2 { font-size: 1rem; font-weight: 600; margin-right: auto; }
button { @apply border-gray-200 bg-white text-gray-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300; display: inline-flex; align-items: center; justify-content: center; min-width: 36px; height: 36px; border-width: 1px; border-radius: 6px; padding: 0 .5rem; transition: background-color .15s, border-color .15s; }
button:hover:not(:disabled) { @apply border-primary-400 bg-primary-50 text-primary-800 dark:border-primary-700 dark:bg-primary-900/30 dark:text-primary-200; }
button:disabled { opacity: .4; cursor: not-allowed; }
button:focus-visible { @apply outline-primary-500; outline-width: 2px; outline-style: solid; outline-offset: 2px; }
.window-tabs { display: flex; gap: .25rem; }
[aria-selected="true"] { @apply border-primary-200 bg-primary-50 text-primary-800 dark:border-primary-700 dark:bg-primary-900/40 dark:text-primary-300; font-weight: 600; }
.observations-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 280px), 1fr)); gap: .75rem; }
.observation-card { @apply border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800/60; min-width: 0; padding: 1rem; border-width: 1px; border-radius: 8px; }
h3 { font-size: .9375rem; font-weight: 600; overflow-wrap: anywhere; }
.model-label { @apply text-gray-500 dark:text-gray-400; font-size: .75rem; overflow-wrap: anywhere; min-height: 2rem; }
dl { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .75rem; margin-top: .75rem; }
dt { @apply text-gray-500 dark:text-gray-400; font-size: .75rem; min-height: 2rem; }
.cache-metric { grid-column: 1 / -1; }
.sample-state { @apply text-gray-500 dark:text-gray-400; display: block; font-size: .75rem; font-weight: 400; min-height: 1.25rem; margin-top: .375rem; }
dd { margin: 0; font-size: .875rem; font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
.observation-state { @apply border-gray-100 text-gray-500 dark:border-dark-700 dark:text-gray-400; border-top-width: 1px; padding-top: .75rem; font-size: .75rem; margin-top: .75rem; min-height: 1.5rem; }
[role="alert"] { @apply text-red-700 dark:text-red-300; }
[role="status"] { @apply text-amber-700 dark:text-amber-300; }
footer { display: flex; align-items: center; justify-content: flex-end; gap: .75rem; margin-top: 1rem; }
</style>
