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
        <!-- 有重置卡限时额度时，余额拆成"常规 + 限时"两格（只是显示上的拆分） -->
        <dl v-if="limitedShown > 0" class="mb-0 mt-4 grid max-w-[440px] grid-cols-2 gap-px bg-white/25">
          <div class="grid min-w-0 gap-0.5 bg-money pr-3 pt-2">
            <dt class="text-xs text-money-dim">{{ t('resetCards.wallet.regular') }}</dt>
            <dd class="m-0 whitespace-nowrap text-lg font-light leading-tight tracking-[-0.02em] tabular-nums sm:text-xl">{{ formatMoney(regularShown) }}</dd>
          </div>
          <div class="grid min-w-0 gap-0.5 bg-money pl-3.5 pt-2">
            <dt class="text-xs text-money-dim">{{ limitedLabel }}</dt>
            <dd class="m-0 whitespace-nowrap text-lg font-light leading-tight tracking-[-0.02em] tabular-nums sm:text-xl">{{ formatMoney(limitedShown) }}</dd>
          </div>
        </dl>
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
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { daysUntil } from '@/stores/resetCards'

const props = withDefaults(
  defineProps<{
    balance: number
    frozen: number
    /** 返利余额；null 表示返利功能未开启或还没取到 */
    rebate: number | null
    canRecharge: boolean
    /** 重置卡限时额度剩余；0 时外观与原来完全一致 */
    limited?: number
    /** 最早一笔限时额度的到期时间 */
    limitedExpiresAt?: string | null
  }>(),
  { limited: 0, limitedExpiresAt: null }
)
defineEmits<{ recharge: [] }>()

const { t } = useI18n()

// 限时额度最多显示到总余额；常规余额 = 余额 − 显示的限时额度，不显示负数
const limitedShown = computed(() => {
  const limited = Number(props.limited) || 0
  const balance = Number(props.balance) || 0
  return limited > 0 ? Math.max(0, Math.min(limited, balance)) : 0
})
const regularShown = computed(() => Math.max((Number(props.balance) || 0) - limitedShown.value, 0))

const limitedLabel = computed(() => {
  const days = daysUntil(props.limitedExpiresAt)
  if (!props.limitedExpiresAt || days < 0) return t('resetCards.wallet.limitedPlain')
  if (days === 0) return t('resetCards.wallet.limitedToday')
  if (days === 1) return t('resetCards.wallet.limitedTomorrow')
  return t('resetCards.wallet.limited', { n: days }, days)
})

// 与顶栏一致，金额用 $ 计
function formatMoney(value: number): string {
  return Number.isFinite(value) ? `$${value.toFixed(2)}` : '$0.00'
}
</script>
