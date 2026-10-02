<template>
  <!-- 钱绿磁贴：余额大字占左，今日花费和剩余天数在右侧上下两格 -->
  <router-link
    :to="walletPath"
    class="wallet-tile grid h-full min-h-[150px] grid-cols-[1.3fr_1fr] grid-rows-2 bg-money text-money-ink transition-colors hover:bg-money-hover"
    :aria-label="t('userHome.wallet.open')"
  >
    <div class="row-span-2 flex min-w-0 flex-col justify-between px-4 py-4 sm:px-[22px] sm:py-[18px]">
      <span class="text-sm font-medium text-money-dim">{{ t('userHome.wallet.balance') }}</span>
      <div class="min-w-0">
        <div v-if="loading" class="h-12 w-40 animate-pulse bg-white/20"></div>
        <div v-else class="truncate leading-none tracking-[-0.035em] tabular-nums" :class="balanceSizeClass">
          {{ formatHomeMoney(balance) }}
        </div>
        <div v-if="frozen > 0" class="mt-2 text-xs tabular-nums text-money-dim">
          {{ t('userHome.wallet.frozen', { amount: formatHomeMoney(frozen) }) }}
        </div>
      </div>
    </div>

    <div class="flex min-w-0 flex-col justify-center gap-1 border-l border-white/30 px-4 py-3 sm:px-[22px]">
      <span class="text-sm font-medium text-money-dim">{{ t('userHome.wallet.todaySpend') }}</span>
      <div v-if="loading" class="h-7 w-20 animate-pulse bg-white/20"></div>
      <span v-else class="truncate text-2xl font-medium leading-none tracking-[-0.02em] tabular-nums sm:text-[32px]">
        {{ formatHomeMoney(todayCost) }}
      </span>
    </div>

    <div class="flex min-w-0 flex-col justify-center gap-1 border-l border-t border-white/30 px-4 py-3 sm:px-[22px]">
      <span class="text-sm font-medium text-money-dim">{{ t('userHome.wallet.runway') }}</span>
      <span class="truncate text-[17px] leading-none tabular-nums sm:text-xl">{{ runwayText }}</span>
      <span v-if="runwayBasis" class="hidden text-xs leading-snug text-money-dim sm:block">{{ runwayBasis }}</span>
    </div>
  </router-link>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatHomeMoney } from './homeFormat'

const props = defineProps<{
  balance: number
  frozen: number
  todayCost: number
  /** 近 7 天实际花费合计；null 表示还没加载到 */
  weekCost: number | null
  walletPath: string
  loading: boolean
}>()

const { t } = useI18n()

// 余额位数多时缩小字号，保证整串数字放得下
const balanceSizeClass = computed(() => {
  const len = formatHomeMoney(props.balance).length
  if (len <= 7) return 'text-[40px] sm:text-[44px] 2xl:text-[56px]'
  if (len <= 9) return 'text-[34px] sm:text-[38px] 2xl:text-[46px]'
  return 'text-[28px] sm:text-[32px] 2xl:text-[38px]'
})

const dailyAvg = computed(() => (props.weekCost == null ? null : props.weekCost / 7))

const runwayText = computed(() => {
  if (dailyAvg.value == null || dailyAvg.value <= 0) return t('userHome.wallet.runwayUnknown')
  const days = props.balance / dailyAvg.value
  if (days > 365) return t('userHome.wallet.runwayOverYear')
  return t('userHome.wallet.runwayDays', { days: Math.max(0, Math.floor(days)) })
})

const runwayBasis = computed(() => {
  if (dailyAvg.value == null) return ''
  if (dailyAvg.value <= 0) return t('userHome.wallet.runwayNoUsage')
  return t('userHome.wallet.runwayBasis', { amount: formatHomeMoney(dailyAvg.value) })
})
</script>
