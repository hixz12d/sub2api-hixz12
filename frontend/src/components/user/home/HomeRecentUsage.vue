<template>
  <div>
    <div v-if="loading" class="space-y-1">
      <div v-for="i in 3" :key="i" class="h-[52px] animate-pulse bg-surface-tile"></div>
    </div>
    <div v-else-if="logs.length === 0" class="bg-surface-tile px-5 py-8 text-center text-sm text-ink-3">
      {{ t('userHome.recent.empty') }}
    </div>
    <div v-else class="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
      <table class="w-full min-w-[640px] border-separate border-spacing-y-1 whitespace-nowrap tabular-nums">
        <thead>
          <tr class="text-left text-[13px] text-ink-3">
            <th class="px-[18px] pb-1 font-normal">{{ t('userHome.recent.time') }}</th>
            <th class="px-[18px] pb-1 font-normal">{{ t('userHome.recent.model') }}</th>
            <th class="px-[18px] pb-1 font-normal">{{ t('userHome.recent.group') }}</th>
            <th class="px-[18px] pb-1 text-right font-normal">{{ t('userHome.recent.tokens') }}</th>
            <th class="px-[18px] pb-1 text-right font-normal">{{ t('userHome.recent.cost') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="log in logs" :key="log.id" class="group">
            <td class="bg-surface-tile px-[18px] py-3.5 text-ink-3 transition-colors group-hover:bg-surface-tile-2">{{ formatTime(log.created_at) }}</td>
            <td class="max-w-[220px] truncate bg-surface-tile px-[18px] py-3.5 font-mono text-[13.5px] text-ink transition-colors group-hover:bg-surface-tile-2" :title="log.model">{{ log.model }}</td>
            <td class="whitespace-nowrap bg-surface-tile px-[18px] py-3.5 text-ink transition-colors group-hover:bg-surface-tile-2">
              <span class="mr-3 inline-block h-3.5 w-3.5 align-[-2px]" :class="colorOf(log.group_id ?? 0)"></span>{{ groupName(log) }}
            </td>
            <td class="bg-surface-tile px-[18px] py-3.5 text-right text-ink transition-colors group-hover:bg-surface-tile-2">
              {{ formatHomeTokens(log.input_tokens + log.cache_read_tokens + log.cache_creation_tokens) }} / {{ formatHomeTokens(log.output_tokens) }}
            </td>
            <td class="bg-surface-tile px-[18px] py-3.5 text-right text-ink transition-colors group-hover:bg-surface-tile-2">{{ formatHomeMoney(log.actual_cost, 3) }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UsageLog } from '@/types'
import { formatHomeMoney, formatHomeTokens } from './homeFormat'

const props = defineProps<{
  logs: UsageLog[]
  loading: boolean
  colorOf: (groupId: number) => string
  /** 分组 ID → 名称（来自分组统计），日志里没带分组对象时兜底 */
  groupNames: Record<number, string>
}>()

const { t } = useI18n()

function groupName(log: UsageLog): string {
  const name = log.group?.name?.trim() || (log.group_id != null ? props.groupNames[log.group_id] : '')
  return name || t('userHome.groups.deletedGroup')
}

function formatTime(value: string): string {
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return value
  const pad = (n: number) => String(n).padStart(2, '0')
  const sameDay = d.toDateString() === new Date().toDateString()
  const time = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
  return sameDay ? time : `${d.getMonth() + 1}/${d.getDate()} ${time}`
}
</script>
