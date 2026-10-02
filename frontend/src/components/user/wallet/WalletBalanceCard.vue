<template>
  <!-- 钱绿磁贴：可用余额大字 + 返利余额（有才显示）+ 充值按钮 -->
  <div class="grid gap-tile-gap" :class="rebate != null ? 'sm:grid-cols-[2fr_1fr]' : ''">
    <div class="flex min-h-[132px] flex-col justify-between gap-4 bg-money px-5 py-[18px] text-money-ink sm:flex-row sm:items-end sm:px-[22px]">
      <div class="min-w-0">
        <div class="text-sm font-medium text-money-dim">{{ t('wallet.balance') }}</div>
        <div class="mt-3 truncate text-[44px] font-normal leading-none tracking-[-0.035em] tabular-nums sm:text-[56px]">
          {{ formatMoney(balance) }}
        </div>
        <div v-if="frozen > 0" class="mt-2 text-xs tabular-nums text-money-dim">
          {{ t('wallet.frozen', { amount: formatMoney(frozen) }) }}
        </div>
      </div>
      <button
        v-if="canRecharge"
        type="button"
        class="inline-flex h-11 shrink-0 items-center justify-center gap-2 bg-white px-5 text-[15px] font-semibold text-black transition-colors hover:bg-gray-100"
        @click="$emit('recharge')"
      >
        <Icon name="plus" size="sm" />
        {{ t('wallet.recharge') }}
      </button>
    </div>

    <div v-if="rebate != null" class="tile flex flex-col justify-between">
      <div class="text-sm text-ink-2">{{ t('wallet.rebate') }}</div>
      <div class="mt-3 truncate text-[32px] font-light leading-none tracking-[-0.03em] tabular-nums text-ink">{{ formatMoney(rebate) }}</div>
      <div class="mt-2 text-xs text-ink-3">{{ t('wallet.rebateHint') }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  balance: number
  frozen: number
  /** 返利余额；null 表示返利功能未开启或还没取到 */
  rebate: number | null
  canRecharge: boolean
}>()
defineEmits<{ recharge: [] }>()

const { t } = useI18n()

// 与顶栏一致，金额用 $ 计
function formatMoney(value: number): string {
  return Number.isFinite(value) ? `$${value.toFixed(2)}` : '$0.00'
}
</script>
