<template>
  <!--
    钱包"重置卡"列表里的一张卡。桌面：状态 | 金额与明细 | 操作；手机：状态与操作在上，明细在下。
    最显眼的是金额；状态用颜色 + 图标一起区分。
  -->
  <li
    class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-2 bg-surface-tile px-4 py-3.5 transition-colors hover:bg-surface-tile-2 sm:grid-cols-[96px_minmax(0,1fr)_auto] sm:px-[18px]"
  >
    <!-- 状态 -->
    <span class="inline-flex items-center gap-1.5 text-[13px] font-medium" :class="statusMeta.color">
      <span v-if="card.status === 'used'" class="mx-1 h-2 w-2 shrink-0 rounded-full bg-current" aria-hidden="true"></span>
      <Icon v-else :name="statusMeta.icon" size="sm" :stroke-width="2" aria-hidden="true" />
      {{ t(`resetCards.status.${card.status}`) }}
    </span>

    <!-- 操作（手机端排在状态右侧） -->
    <div class="justify-self-end sm:order-3">
      <button
        v-if="card.status === 'unused'"
        type="button"
        class="btn btn-primary btn-sm h-8 min-w-[64px]"
        @click="emit('use', card)"
      >
        {{ t('resetCards.list.use') }}
      </button>
    </div>

    <!-- 金额与明细 -->
    <div class="col-span-2 grid min-w-0 gap-1 sm:order-2 sm:col-span-1">
      <div class="flex flex-wrap items-baseline gap-x-2">
        <template v-if="card.status === 'unused'">
          <span class="text-[13px] text-ink-3">{{ t('resetCards.list.estimate') }}</span>
          <span class="text-xl font-light leading-tight tracking-[-0.02em] tabular-nums text-money-text">
            {{ formatResetMoney(card.estimate?.amount ?? 0) }}
          </span>
        </template>
        <template v-else-if="card.status === 'used'">
          <span class="text-[13px] text-ink-3">{{ t('resetCards.list.refunded') }}</span>
          <span class="text-xl font-light leading-tight tracking-[-0.02em] tabular-nums text-ink">
            {{ formatResetMoney(card.refund_amount ?? 0) }}
          </span>
        </template>
        <span v-else class="text-[15px] text-ink-3">{{ t('resetCards.list.notUsed') }}</span>
      </div>

      <p class="m-0 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-ink-3">
        <template v-for="(group, i) in card.groups" :key="group.id">
          <span class="inline-flex min-w-0 items-center gap-1.5">
            <span class="h-2.5 w-2.5 shrink-0" :class="groupSeriesClass(group.id)" aria-hidden="true"></span>
            <span :class="dim ? 'text-ink-3' : 'text-ink-2'">{{ group.name || t('resetCards.deletedGroup') }}</span>
          </span>
          <span v-if="i < card.groups.length - 1" aria-hidden="true">,</span>
        </template>
        <span aria-hidden="true">·</span>
        <span>{{ ruleText }}</span>
        <template v-if="timeText">
          <span aria-hidden="true">·</span>
          <span>{{ timeText }}</span>
        </template>
      </p>

      <!-- 已用的卡：限时额度进度 -->
      <template v-if="credit">
        <template v-if="credit.status === 'active'">
          <p class="m-0 mt-1 flex flex-wrap items-center gap-x-1.5 text-xs tabular-nums" :class="lastDay ? 'text-warn' : 'text-ink-3'">
            <Icon v-if="lastDay" name="exclamationCircle" size="xs" :stroke-width="2" class="shrink-0" aria-hidden="true" />
            <span>{{ t('resetCards.list.creditProgress', { used: formatResetMoney(credit.used_amount), total: formatResetMoney(credit.amount) }) }}</span>
            <span aria-hidden="true">·</span>
            <span>{{ creditLeftText }}</span>
          </p>
          <!-- 文字已经读出数值，进度条只做视觉 -->
          <div class="h-1 w-full max-w-[360px] bg-surface-tile-2" aria-hidden="true">
            <div class="h-full bg-money" :style="{ width: `${usedPct}%` }"></div>
          </div>
        </template>
        <p v-else class="m-0 text-xs tabular-nums text-ink-3">
          {{ t('resetCards.list.settled', { used: formatResetMoney(credit.used_amount), clawback: formatResetMoney(credit.clawback_amount ?? 0) }) }}
        </p>
      </template>
    </div>
  </li>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { groupSeriesClass } from '@/utils/groupSeriesColor'
import {
  daysUntil,
  formatResetLimit,
  formatResetMoney,
  formatResetMonthDay,
  formatResetPercent
} from '@/stores/resetCards'
import type { ResetCard } from '@/types/resetCard'

const props = defineProps<{ card: ResetCard }>()
const emit = defineEmits<{ use: [card: ResetCard] }>()

const { t, locale } = useI18n()

type IconName = 'check' | 'x' | 'ban'

const statusMeta = computed<{ color: string; icon: IconName }>(() => {
  switch (props.card.status) {
    case 'unused':
      return { color: 'text-ok', icon: 'check' }
    case 'used':
      return { color: 'text-ink-2', icon: 'check' }
    case 'revoked':
      return { color: 'text-bad', icon: 'ban' }
    default:
      return { color: 'text-ink-3', icon: 'x' }
  }
})

const dim = computed(() => props.card.status === 'expired' || props.card.status === 'revoked')

const ruleText = computed(() =>
  t('resetCards.list.rule', {
    percent: formatResetPercent(props.card.percent),
    max: formatResetLimit(props.card.max_amount)
  })
)

const timeText = computed(() => {
  const card = props.card
  switch (card.status) {
    case 'unused': {
      const days = daysUntil(card.expires_at)
      if (days <= 0) return t('resetCards.list.expiresToday')
      if (days === 1) return t('resetCards.list.expiresTomorrow')
      return t('resetCards.list.expiresIn', { n: days }, days)
    }
    case 'used':
      return card.used_at ? t('resetCards.list.usedOn', { date: formatResetMonthDay(card.used_at, locale.value) }) : ''
    case 'expired':
      return t('resetCards.list.expiredOn', { date: formatResetMonthDay(card.expires_at, locale.value) })
    default:
      return ''
  }
})

const credit = computed(() => (props.card.status === 'used' ? props.card.credit : null))

const usedPct = computed(() => {
  const c = credit.value
  if (!c || c.amount <= 0) return 0
  return Math.min(100, Math.max(0, (c.used_amount / c.amount) * 100))
})

const creditDays = computed(() => daysUntil(credit.value?.expires_at))
// 最后 1 天（含已到期、等待定时任务结算）标成提醒色
const lastDay = computed(() => creditDays.value <= 0)

const creditLeftText = computed(() => {
  const amount = formatResetMoney(credit.value?.remaining_amount ?? 0)
  const days = creditDays.value
  if (days < 0) return t('resetCards.list.creditLeftSettling', { amount })
  if (days === 0) return t('resetCards.list.creditLeftToday', { amount })
  if (days === 1) return t('resetCards.list.creditLeftTomorrow', { amount })
  return t('resetCards.list.creditLeft', { amount, n: days }, days)
})
</script>
