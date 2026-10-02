<template>
  <div>
    <div v-if="loading && items.length === 0" class="flex flex-col gap-2 md:gap-1">
      <div v-for="i in 5" :key="i" class="h-[118px] animate-pulse bg-surface-tile md:h-[52px]"></div>
    </div>

    <EmptyState
      v-else-if="items.length === 0"
      :title="t('channelStatus.empty.title')"
      :description="t('channelStatus.empty.description')"
    />

    <div v-else>
      <!-- 表头只在桌面端显示，列宽与渠道行一致 -->
      <div
        class="hidden items-end gap-x-5 px-5 pb-1.5 text-[13px] text-ink-3 md:grid"
        :class="MONITOR_ROW_GRID"
      >
        <span></span>
        <span>{{ t('channelStatus.board.columns.channel') }}</span>
        <span>{{ t('channelStatus.board.columns.status') }}</span>
        <span class="text-right">{{ t('channelStatus.board.columns.latency') }}</span>
        <span class="text-right">{{ t('channelStatus.board.columns.availability', { window: windowLabel }) }}</span>
        <span class="flex justify-between">
          <span>{{ t('channelStatus.board.columns.timeline', { n: 60 }) }}</span>
          <span class="flex items-center gap-3">
            <span v-for="l in legend" :key="l.key" class="inline-flex items-center gap-1.5">
              <i class="inline-block h-2.5 w-1.5" :class="l.colorClass"></i>{{ t(`channelStatus.board.state.${l.key}`) }}
            </span>
          </span>
        </span>
      </div>

      <div class="flex flex-col gap-2 md:gap-1">
        <MonitorCard
          v-for="item in items"
          :key="item.id"
          :item="item"
          :window="window"
          :availability-value="resolveAvailability(item)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { UserMonitorView, UserMonitorDetail } from '@/api/channelMonitor'
import EmptyState from '@/components/common/EmptyState.vue'
import MonitorCard from './MonitorCard.vue'
import { MONITOR_ROW_GRID } from './monitorLayout'

const props = defineProps<{
  items: UserMonitorView[]
  window: '7d' | '15d' | '30d'
  loading: boolean
  detailCache: Record<number, UserMonitorDetail>
}>()

const { t } = useI18n()

const windowLabel = computed(() => t(`channelStatus.windowTab.${props.window}`))

const legend = [
  { key: 'ok', colorClass: 'bg-ok' },
  { key: 'degraded', colorClass: 'bg-warn' },
  { key: 'down', colorClass: 'bg-bad' },
]

function resolveAvailability(item: UserMonitorView): number | null {
  if (props.window === '7d') {
    return item.availability_7d ?? null
  }
  const detail = props.detailCache[item.id]
  if (!detail) return null
  const primary = detail.models.find(m => m.model === item.primary_model)
  if (!primary) return null
  return props.window === '15d' ? primary.availability_15d ?? null : primary.availability_30d ?? null
}
</script>
