<template>
  <div class="relative" @pointerleave="active = null">
    <div class="flex h-6 w-full gap-[2px]">
      <div
        v-for="(bar, idx) in bars"
        :key="idx"
        class="min-w-0 flex-1 transition-opacity"
        :class="[bar.colorClass, active === idx ? 'opacity-70' : '']"
        :aria-label="bar.tip || undefined"
        @pointerenter="active = bar.tip ? idx : null"
        @click="active = bar.tip ? idx : null"
      ></div>
    </div>

    <!-- 悬停提示：反色实心块，靠边时贴边对齐，避免被裁掉 -->
    <div
      v-if="tip"
      class="pointer-events-none absolute bottom-full z-10 mb-2 whitespace-nowrap bg-ink px-2.5 py-1.5 text-xs tabular-nums text-surface"
      :style="tip.style"
      role="tooltip"
    >
      {{ tip.text }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { MonitorTimelinePoint } from '@/api/channelMonitor'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'

const props = withDefaults(defineProps<{
  buckets?: MonitorTimelinePoint[]
  /** 纯配额模式没有对话延迟，提示里显示“—” */
  hideLatency?: boolean
  length?: number
}>(), {
  buckets: () => [],
  hideLatency: false,
  length: 60,
})

const { t } = useI18n()
const { formatRelativeTime } = useChannelMonitorFormat()

const active = ref<number | null>(null)

// 等高柱，只用颜色区分：绿正常 / 黄降级 / 红异常，无数据淡灰
const STATUS_COLOR: Record<string, string> = {
  operational: 'bg-ok',
  degraded: 'bg-warn',
  failed: 'bg-bad',
  error: 'bg-bad',
}
const EMPTY_COLOR = 'bg-gray-300 dark:bg-dark-600'

const STATUS_KEY: Record<string, string> = {
  operational: 'ok',
  degraded: 'degraded',
  failed: 'down',
  error: 'down',
}

interface Bar {
  colorClass: string
  tip: string
}

const bars = computed<Bar[]>(() => {
  // 接口按新到旧返回，翻转成旧到新，最右边是最近一次；左侧不足的补灰柱
  const real = [...props.buckets].slice(0, props.length).reverse()
  const result: Bar[] = Array.from(
    { length: Math.max(0, props.length - real.length) },
    () => ({ colorClass: EMPTY_COLOR, tip: '' })
  )
  for (const point of real) {
    const key = STATUS_KEY[point.status]
    if (!key) {
      result.push({ colorClass: EMPTY_COLOR, tip: '' })
      continue
    }
    const latency = props.hideLatency || point.latency_ms == null
      ? '—'
      : `${Math.round(point.latency_ms)} ms`
    result.push({
      colorClass: STATUS_COLOR[point.status],
      tip: `${formatRelativeTime(point.checked_at)} · ${t(`channelStatus.board.state.${key}`)} · ${latency}`,
    })
  }
  return result
})

const tip = computed(() => {
  if (active.value === null) return null
  const bar = bars.value[active.value]
  if (!bar?.tip) return null
  const center = ((active.value + 0.5) / props.length) * 100
  let style: Record<string, string>
  if (center < 15) style = { left: '0' }
  else if (center > 85) style = { right: '0' }
  else style = { left: `${center}%`, transform: 'translateX(-50%)' }
  return { text: bar.tip, style }
})
</script>
