<template>
  <!-- 桌面端是一行（列宽与表头共用），手机端同一套结构折成小卡片 -->
  <div
    class="grid grid-cols-[20px_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-3 bg-surface-tile px-4 py-3.5 md:gap-x-5 md:px-5"
    :class="MONITOR_ROW_GRID"
  >
    <span class="flex text-ink-2" aria-hidden="true">
      <ProviderIcon :provider="item.provider" :size="20" />
    </span>

    <div class="truncate text-[15px] font-semibold text-ink" :title="item.name">
      {{ item.name }}
    </div>

    <span class="inline-flex items-center gap-1.5 justify-self-end text-sm font-semibold md:justify-self-start" :class="state.textClass">
      <Icon :name="state.icon" size="sm" :stroke-width="2" />
      {{ t(`channelStatus.board.state.${state.key}`) }}
    </span>

    <!-- 手机端两个指标并排一行；桌面端拆成独立两列 -->
    <div class="col-span-3 grid grid-cols-2 gap-4 md:contents">
      <div class="md:text-right">
        <div class="text-[13px] text-ink-3 md:hidden">{{ t('channelStatus.board.columns.latency') }}</div>
        <div class="text-[15px] tabular-nums text-ink">
          <template v-if="latencyMs != null">{{ latencyMs }}<span class="ml-0.5 text-[13px] text-ink-3">ms</span></template>
          <span v-else class="text-ink-3">—</span>
        </div>
      </div>
      <div class="md:text-right">
        <div class="text-[13px] text-ink-3 md:hidden">{{ t('channelStatus.board.columns.availability', { window: windowLabel }) }}</div>
        <div class="text-[15px] font-semibold tabular-nums" :class="availabilityClass(availability)">
          {{ availabilityText }}
        </div>
      </div>
    </div>

    <MonitorTimeline
      class="col-span-3 md:col-span-1"
      :buckets="item.timeline"
      :hide-latency="isQuota"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView } from '@/api/channelMonitor'
import Icon from '@/components/icons/Icon.vue'
import ProviderIcon from '../ProviderIcon.vue'
import MonitorTimeline from './MonitorTimeline.vue'
import {
  MONITOR_ROW_GRID,
  QUOTA_MODEL_PLACEHOLDER,
  availabilityClass,
  formatAvailabilityPct,
} from './monitorLayout'

const props = defineProps<{
  item: UserMonitorView
  window: '7d' | '15d' | '30d'
  availabilityValue: number | null
}>()

const { t } = useI18n()

const isQuota = computed(() => props.item.primary_model === QUOTA_MODEL_PLACEHOLDER)

const state = computed(() => {
  switch (props.item.primary_status) {
    case 'operational':
      return { key: 'ok', icon: 'checkCircle', textClass: 'text-ok' } as const
    case 'degraded':
      return { key: 'degraded', icon: 'exclamationTriangle', textClass: 'text-warn' } as const
    case 'failed':
    case 'error':
      return { key: 'down', icon: 'xCircle', textClass: 'text-bad' } as const
    default:
      return { key: 'none', icon: 'clock', textClass: 'text-ink-3' } as const
  }
})

const latencyMs = computed(() => {
  if (isQuota.value || props.item.primary_latency_ms == null) return null
  return Math.round(props.item.primary_latency_ms)
})

const windowLabel = computed(() => t(`channelStatus.windowTab.${props.window}`))

// 从未检测过的渠道没有可用率可言，显示“—”而不是 0%
const availability = computed(() => (props.item.primary_status ? props.availabilityValue : null))

const availabilityText = computed(() => {
  const pct = formatAvailabilityPct(availability.value)
  return pct == null ? '—' : `${pct}%`
})
</script>
