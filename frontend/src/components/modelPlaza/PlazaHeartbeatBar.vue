<template>
  <div
    class="flex w-full items-stretch rounded-none"
    :class="variant === 'full' ? 'h-[28px] gap-px' : 'h-[18px] gap-[2px]'"
    role="img"
    :aria-label="t('modelPlaza.heartbeat.ariaLabel')"
    @mouseleave="hideTooltip"
  >
    <div
      v-for="(slot, idx) in displaySlots"
      :key="slot.start ? `${slot.start}-${idx}` : idx"
      class="min-w-0 flex-1 rounded-none transition-opacity duration-150 hover:opacity-80"
      :class="slotColorClass(slot.state)"
      @mouseenter="showTooltip($event, slot)"
    />

    <Teleport to="body">
      <div
        v-if="activeTooltip"
        role="tooltip"
        class="pointer-events-none fixed z-[9999] -translate-x-1/2 -translate-y-full whitespace-nowrap rounded-none bg-gray-950 px-2.5 py-1.5 text-xs leading-snug text-white dark:bg-white dark:text-black"
        :style="{ left: `${activeTooltip.left}px`, top: `${activeTooltip.top}px` }"
      >
        <div
          v-if="activeTooltip.timeRange"
          class="font-mono text-[11px] tabular-nums opacity-75"
        >
          {{ activeTooltip.timeRange }}
        </div>
        <div class="font-medium tabular-nums">
          {{ activeTooltip.summary }}
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PlazaHeartbeatSlot, PlazaHeartbeatSlotState } from '@/api/modelPlaza'

interface Props {
  slots: PlazaHeartbeatSlot[]
  variant?: 'compact' | 'full'
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'compact'
})

const { t } = useI18n()

const displaySlots = computed<PlazaHeartbeatSlot[]>(() => {
  if (props.slots.length > 0) {
    return props.slots
  }
  const fallbackCount = props.variant === 'full' ? 72 : 12
  return Array.from({ length: fallbackCount }, () => ({
    start: '',
    end: '',
    success_rate: null,
    state: 'no_data' as PlazaHeartbeatSlotState
  }))
})

function slotColorClass(state: PlazaHeartbeatSlotState): string {
  switch (state) {
    case 'normal':
      return 'bg-ok'
    case 'partial_failure':
      return 'bg-warn'
    case 'many_failures':
      return 'bg-bad'
    case 'low_sample':
    case 'no_data':
    default:
      return 'bg-surface-tile-2 border border-rule'
  }
}

interface TooltipState {
  left: number
  top: number
  timeRange: string
  summary: string
}

const activeTooltip = ref<TooltipState | null>(null)

function pad2(n: number): string {
  return String(n).padStart(2, '0')
}

function formatSlotRange(startIso: string, endIso: string): string {
  if (!startIso && !endIso) return ''
  const start = startIso ? new Date(startIso) : null
  const end = endIso ? new Date(endIso) : null
  if (!start || Number.isNaN(start.getTime())) return ''

  const startHm = `${pad2(start.getHours())}:${pad2(start.getMinutes())}`
  if (!end || Number.isNaN(end.getTime())) {
    return startHm
  }
  const endHm = `${pad2(end.getHours())}:${pad2(end.getMinutes())}`
  const sameDay =
    start.getFullYear() === end.getFullYear() &&
    start.getMonth() === end.getMonth() &&
    start.getDate() === end.getDate()

  if (sameDay) {
    return `${startHm}–${endHm}`
  }
  const startDay = `${pad2(start.getMonth() + 1)}-${pad2(start.getDate())}`
  const endDay = `${pad2(end.getMonth() + 1)}-${pad2(end.getDate())}`
  return `${startDay} ${startHm}–${endDay} ${endHm}`
}

function formatSlotSummary(slot: PlazaHeartbeatSlot): string {
  if (slot.state === 'low_sample') {
    return t('modelPlaza.heartbeat.lowSample')
  }
  if (slot.state === 'no_data' || slot.success_rate == null || !Number.isFinite(slot.success_rate)) {
    return t('modelPlaza.heartbeat.noData')
  }
  const pct = Math.max(0, Math.min(1, slot.success_rate)) * 100
  const rateText = pct >= 100 ? '100%' : `${pct.toFixed(1)}%`
  const stateLabel =
    slot.state === 'normal'
      ? t('modelPlaza.heartbeat.normal')
      : slot.state === 'partial_failure'
        ? t('modelPlaza.heartbeat.partialFailure')
        : t('modelPlaza.heartbeat.manyFailures')
  return `${stateLabel} · ${t('modelPlaza.heartbeat.successRate', { rate: rateText })}`
}

function showTooltip(event: MouseEvent, slot: PlazaHeartbeatSlot) {
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const rect = target.getBoundingClientRect()
  const rawLeft = rect.left + rect.width / 2
  const viewportWidth = typeof window !== 'undefined' ? window.innerWidth : 1024
  const clampedLeft = Math.max(88, Math.min(viewportWidth - 88, rawLeft))
  activeTooltip.value = {
    left: clampedLeft,
    top: Math.max(44, rect.top - 8),
    timeRange: formatSlotRange(slot.start, slot.end),
    summary: formatSlotSummary(slot)
  }
}

function hideTooltip() {
  activeTooltip.value = null
}

onMounted(() => {
  window.addEventListener('scroll', hideTooltip, true)
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', hideTooltip, true)
})
</script>
