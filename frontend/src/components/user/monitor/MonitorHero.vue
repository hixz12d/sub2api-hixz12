<template>
  <section class="mb-6 flex flex-wrap items-end justify-between gap-x-6 gap-y-4 lg:mb-8">
    <div class="min-w-0">
      <h1
        v-if="overallStatus"
        class="flex items-center gap-3 text-[26px] font-light leading-none tracking-[-0.02em] text-ink lg:text-[30px]"
      >
        <Icon
          :name="overallStatus === 'operational' ? 'checkCircle' : 'exclamationTriangle'"
          size="lg"
          :stroke-width="1.6"
          :class="overallStatus === 'operational' ? 'text-ok' : 'text-warn'"
        />
        {{ t(`channelStatus.board.overall.${overallStatus}`) }}
      </h1>
      <p class="mt-2.5 text-[13px] tabular-nums text-ink-3">
        {{ checkedText }}
      </p>
    </div>

    <div class="flex flex-wrap items-center gap-3">
      <div class="flex gap-1" role="group" :aria-label="t('channelStatus.board.windowLabel')">
        <button
          v-for="opt in windowOptions"
          :key="opt.value"
          type="button"
          class="h-9 px-4 text-sm font-semibold tabular-nums transition-colors"
          :class="window === opt.value ? 'bg-money text-money-ink' : 'bg-surface-tile text-ink-2 hover:bg-surface-tile-2 hover:text-ink'"
          :aria-pressed="window === opt.value"
          @click="emit('update:window', opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>

      <AutoRefreshButton
        v-if="autoRefresh"
        :enabled="autoRefresh.enabled.value"
        :interval-seconds="autoRefresh.intervalSeconds.value"
        :countdown="autoRefresh.countdown.value"
        :intervals="autoRefresh.intervals"
        @update:enabled="autoRefresh.setEnabled"
        @update:interval="autoRefresh.setInterval"
      />
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
export type MonitorWindow = '7d' | '15d' | '30d'
export type OverallStatus = 'operational' | 'degraded'

const props = defineProps<{
  /** 没有渠道时为 null，不显示总状态 */
  overallStatus: OverallStatus | null
  /** 所有渠道里最近一次检测的时间（ISO） */
  lastCheckedAt: string | null
  window: MonitorWindow
  autoRefresh?: {
    enabled: { value: boolean }
    intervalSeconds: { value: number }
    countdown: { value: number }
    intervals: readonly number[]
    setEnabled: (v: boolean) => void
    setInterval: (v: number) => void
  }
}>()

const emit = defineEmits<{
  (e: 'update:window', value: MonitorWindow): void
}>()

const { t } = useI18n()

const windowOptions = computed<{ value: MonitorWindow; label: string }[]>(() => [
  { value: '7d', label: t('channelStatus.windowTab.7d') },
  { value: '15d', label: t('channelStatus.windowTab.15d') },
  { value: '30d', label: t('channelStatus.windowTab.30d') },
])

// “xx 秒前检测”每秒走一次
const now = ref(Date.now())
let timer: number | undefined
onMounted(() => { timer = window.setInterval(() => { now.value = Date.now() }, 1000) })
onBeforeUnmount(() => window.clearInterval(timer))

const checkedText = computed(() => {
  const ts = props.lastCheckedAt ? Date.parse(props.lastCheckedAt) : NaN
  if (Number.isNaN(ts)) return t('channelStatus.board.neverChecked')
  const sec = Math.max(0, Math.floor((now.value - ts) / 1000))
  if (sec < 60) return t('channelStatus.board.checkedSecondsAgo', { n: sec })
  const min = Math.floor(sec / 60)
  if (min < 60) return t('channelStatus.board.checkedMinutesAgo', { n: min })
  const hour = Math.floor(min / 60)
  if (hour < 24) return t('channelStatus.board.checkedHoursAgo', { n: hour })
  return t('channelStatus.board.checkedDaysAgo', { n: Math.floor(hour / 24) })
})
</script>
