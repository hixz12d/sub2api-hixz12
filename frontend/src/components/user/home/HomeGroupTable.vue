<template>
  <div>
    <div v-if="loading" class="space-y-1">
      <div v-for="i in 3" :key="i" class="h-[52px] animate-pulse bg-surface-tile"></div>
    </div>
    <div v-else-if="rows.length === 0" class="bg-surface-tile px-5 py-8 text-center text-sm text-ink-3">
      {{ t('userHome.groups.empty') }}
    </div>
    <div v-else class="-mx-4 overflow-x-auto px-4 sm:mx-0 sm:px-0">
      <table class="w-full min-w-[560px] border-separate border-spacing-y-1 whitespace-nowrap tabular-nums">
        <thead>
          <tr class="text-left text-[13px] text-ink-3">
            <th class="px-[18px] pb-1 font-normal">{{ t('userHome.groups.group') }}</th>
            <th class="px-[18px] pb-1 text-right font-normal">{{ t('userHome.groups.requests') }}</th>
            <th class="px-[18px] pb-1 text-right font-normal">{{ t('userHome.groups.tokens') }}</th>
            <th class="px-[18px] pb-1 text-right font-normal">{{ t('userHome.groups.cost') }}</th>
            <th class="px-[18px] pb-1 text-right font-normal">
              <span class="inline-flex items-center gap-1">
                {{ t('userHome.groups.hitRate') }}
                <span class="inline-flex cursor-help" :title="t('userHome.groups.hitRateHint')" tabindex="0" :aria-label="t('userHome.groups.hitRateHint')">
                  <Icon name="questionCircle" size="sm" />
                </span>
              </span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.key" class="group">
            <td class="whitespace-nowrap bg-surface-tile px-[18px] py-3.5 font-semibold text-ink transition-colors group-hover:bg-surface-tile-2">
              <span class="mr-3 inline-block h-3.5 w-3.5 align-[-2px]" :class="row.colorClass"></span>{{ row.name }}
            </td>
            <td class="bg-surface-tile px-[18px] py-3.5 text-right text-ink transition-colors group-hover:bg-surface-tile-2">{{ formatHomeCount(row.requests) }}</td>
            <td class="bg-surface-tile px-[18px] py-3.5 text-right text-ink transition-colors group-hover:bg-surface-tile-2">{{ formatHomeTokens(row.tokens) }}</td>
            <td class="bg-surface-tile px-[18px] py-3.5 text-right text-ink transition-colors group-hover:bg-surface-tile-2">{{ formatHomeMoney(row.cost) }}</td>
            <td class="bg-surface-tile px-[18px] py-3.5 text-right transition-colors group-hover:bg-surface-tile-2">
              <span
                class="inline-flex items-center justify-end gap-3"
                :class="row.levelClass"
                :title="row.hit == null ? t('userHome.groups.tooFew') : undefined"
              >
                <span class="min-w-[44px] font-semibold">{{ row.hit == null ? '—' : `${Math.round(row.hit * 100)}%` }}</span>
                <span class="hidden h-2 w-24 bg-surface-tile-2 sm:block dark:bg-dark-600">
                  <i class="block h-full bg-current" :style="{ width: `${Math.round((row.hit ?? 0) * 100)}%` }"></i>
                </span>
              </span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { GroupStat } from '@/types'
import { formatHomeCount, formatHomeMoney, formatHomeTokens, groupCacheHitRate } from './homeFormat'

const props = defineProps<{
  groups: GroupStat[]
  loading: boolean
  /** 分组 ID → 识别色类名（与最近使用共用同一套配色） */
  colorOf: (groupId: number) => string
}>()

const { t } = useI18n()

const rows = computed(() =>
  [...props.groups]
    .sort((a, b) => (b.actual_cost ?? 0) - (a.actual_cost ?? 0))
    .map((g) => {
      const hit = groupCacheHitRate(g)
      return {
        key: g.group_id,
        name: g.group_name?.trim() || t('userHome.groups.deletedGroup'),
        requests: g.requests,
        tokens: g.total_tokens,
        cost: g.actual_cost,
        hit,
        colorClass: props.colorOf(g.group_id),
        levelClass: hit == null ? 'text-ink-3' : hit >= 0.7 ? 'text-ok' : hit >= 0.4 ? 'text-ink-2' : 'text-warn'
      }
    })
)
</script>
