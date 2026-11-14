<template>
  <!-- 钱绿实心磁贴：金额最大，规则写小字，两格写两个期限；不放说明段落 -->
  <div class="tile-money grid gap-[10px]">
    <div class="grid grid-cols-[minmax(0,1fr)_auto] items-end gap-3">
      <div class="grid min-w-0 gap-2.5">
        <span class="text-[13px] leading-snug text-money-dim">{{ amountLabel }}</span>
        <div
          class="whitespace-nowrap font-light leading-none tracking-[-0.035em] tabular-nums"
          :class="amountSizeClass"
          aria-hidden="true"
        >
          {{ formatResetMoney(displayAmount) }}
        </div>
        <span class="sr-only">{{ formatResetMoney(targetAmount) }}</span>
      </div>

      <figure
        v-if="bars.length"
        class="rc-viz m-0 grid justify-items-end gap-1"
        :class="vizClass"
        :aria-label="t('resetCards.tile.vizAria', { percent: percentText })"
      >
        <svg
          :viewBox="`0 0 ${VB_W} ${VB_H}`"
          class="block h-auto w-[84px] overflow-visible sm:w-[120px]"
          aria-hidden="true"
        >
          <g v-for="(bar, i) in bars" :key="i" :style="{ '--i': i, '--dx': `${bar.dx}px` }">
            <rect class="rc-base" :x="bar.x" :y="bar.baseY" :width="BAR_W" :height="bar.baseH" />
            <rect v-if="bar.cutH > 0" class="rc-cut" :x="bar.x" :y="bar.cutY" :width="BAR_W" :height="bar.cutH" />
          </g>
        </svg>
        <figcaption class="text-xs text-money-ink">{{ t('resetCards.tile.vizCaption') }}</figcaption>
      </figure>
    </div>

    <ul class="m-0 flex list-none flex-wrap gap-x-4 gap-y-1 p-0 text-sm">
      <li v-for="group in card.groups" :key="group.id" class="inline-flex min-w-0 items-center gap-2">
        <span class="h-3 w-3 shrink-0 bg-money-ink" aria-hidden="true"></span>
        <span class="[overflow-wrap:anywhere]">{{ group.name || t('resetCards.deletedGroup') }}</span>
      </li>
    </ul>

    <div class="border-t border-white/25 pt-2.5 text-[13px] text-money-ink">
      {{ ruleText }}
    </div>

    <dl class="m-0 grid grid-cols-2 gap-px bg-white/25">
      <div class="grid gap-0.5 bg-money pt-2 pr-3">
        <dt class="text-xs text-money-ink">{{ t('resetCards.tile.cardTerm') }}</dt>
        <dd class="m-0 text-[15px] sm:text-base">{{ cardTermText }}</dd>
      </div>
      <div class="grid gap-0.5 bg-money pl-3.5 pt-2">
        <dt class="text-xs text-money-ink">{{ t('resetCards.tile.creditTerm') }}</dt>
        <dd class="m-0 text-[15px] sm:text-base">{{ creditTermText }}</dd>
      </div>
    </dl>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ResetCard, ResetCardUseResponse } from '@/types/resetCard'
import {
  daysUntil,
  formatResetLimit,
  formatResetMoney,
  formatResetMonthDay,
  formatResetPercent
} from '@/stores/resetCards'

const props = withDefaults(
  defineProps<{
    card: ResetCard
    /** 用卡成功的结果：一出现就播放"柱顶飞向金额 + 金额滚动"，卡面切到已使用 */
    result?: ResetCardUseResponse | null
    /** 可返还为 0（预估为 0，或服务端返回 RESET_CARD_NOTHING_TO_REFUND） */
    nothing?: boolean
  }>(),
  { result: null, nothing: false }
)

const { t, locale } = useI18n()

// SVG 画布：7 根柱 × 12 宽，间隔 6，正好 120
const VB_W = 120
const VB_H = 84
const BAR_W = 12
const BAR_GAP = 6
const MAX_H = 78

const reducedMotion = () =>
  typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches

const used = computed(() => !!props.result || props.card.status === 'used')
const estimate = computed(() => props.card.estimate)

const targetAmount = computed(() => {
  if (props.result) return props.result.refund_amount
  if (props.card.status === 'used') return props.card.refund_amount ?? 0
  if (props.nothing) return 0
  return estimate.value?.amount ?? 0
})

// 柱子动画阶段：idle 还没长出 → go 长出并呼吸 → fly 用卡后飞向金额
const vizState = ref<'idle' | 'go' | 'fly'>(props.result ? 'fly' : 'idle')

// ---------- 金额显示与滚动 ----------
const displayAmount = ref(targetAmount.value)
let rafId = 0

function rollTo(target: number) {
  cancelAnimationFrame(rafId)
  if (reducedMotion()) {
    displayAmount.value = target
    return
  }
  const duration = 900
  const start = performance.now()
  const tick = (now: number) => {
    const p = Math.min(1, (now - start) / duration)
    const eased = 1 - Math.pow(1 - p, 3)
    displayAmount.value = target * eased
    if (p < 1) rafId = requestAnimationFrame(tick)
  }
  displayAmount.value = 0
  rafId = requestAnimationFrame(tick)
}

watch(
  () => props.result,
  (result, prev) => {
    if (result && !prev) {
      vizState.value = 'fly'
      rollTo(result.refund_amount)
    }
  }
)

// 预估刷新或变成 0 时直接替换数字（不滚动）
watch(targetAmount, (value) => {
  if (!props.result) displayAmount.value = value
})

// 金额随位数缩小，保证不换行、不截断（按最终金额定字号，滚动中不跳）
const amountSizeClass = computed(() => {
  const len = formatResetMoney(targetAmount.value).length
  if (len <= 7) return 'text-[48px] sm:text-[64px]'
  if (len <= 9) return 'text-[40px] sm:text-[52px]'
  return 'text-[32px] sm:text-[40px]'
})

// ---------- 7 根柱 ----------
const vizClass = computed(() => (vizState.value === 'idle' ? '' : `is-${vizState.value}`))

const bars = computed(() => {
  const raw = estimate.value?.daily
  if (!raw || raw.length === 0) return []
  const daily = raw.slice(-7).map((v) => Math.max(0, Number(v) || 0))
  while (daily.length < 7) daily.unshift(0)
  const max = Math.max(...daily)

  // 亮白那截 = 实际会返还的比例；触顶时按实际返还额折算，否则就是 percent
  let ratio = Math.min(1, Math.max(0, props.card.percent / 100))
  const est = estimate.value
  if (est && est.base_spend > 0) ratio = Math.min(ratio, est.amount / est.base_spend)
  if (props.nothing) ratio = 0

  return daily.map((v, i) => {
    const x = i * (BAR_W + BAR_GAP)
    let h: number
    let cut = 0
    if (max <= 0) {
      h = MAX_H * 0.36 // 全是 0：7 根等高淡柱
    } else if (v <= 0) {
      h = 2
    } else {
      h = Math.max(4, (v / max) * MAX_H)
      cut = h * ratio
    }
    const y = VB_H - h
    const cutH = cut > 2 ? cut - 1.5 : 0
    return {
      x,
      baseY: cutH > 0 ? y + cut : y,
      baseH: cutH > 0 ? h - cut : h,
      cutY: y,
      cutH,
      // 飞向左侧金额：越靠右的柱飞得越远
      dx: -(x + 48)
    }
  })
})

onMounted(() => {
  if (vizState.value === 'fly') return
  void nextTick(() => {
    rafId = requestAnimationFrame(() => {
      if (vizState.value === 'idle') vizState.value = 'go'
    })
  })
})

onBeforeUnmount(() => cancelAnimationFrame(rafId))

// ---------- 文案 ----------
const percentText = computed(() => formatResetPercent(props.card.percent))

const amountLabel = computed(() => {
  if (used.value) return t('resetCards.tile.refundedLabel')
  if (props.nothing) return t('resetCards.tile.nothingLabel')
  return t('resetCards.tile.estimateLabel')
})

const ruleText = computed(() =>
  used.value
    ? t('resetCards.tile.ruleUsed')
    : t('resetCards.tile.rule', {
        percent: percentText.value,
        max: formatResetLimit(props.card.max_amount)
      })
)

const cardTermText = computed(() => {
  if (used.value) return t('resetCards.tile.cardUsed')
  const days = daysUntil(props.card.expires_at)
  if (days <= 0) return t('resetCards.tile.cardToday')
  if (days === 1) return t('resetCards.tile.cardTomorrow')
  return t('resetCards.tile.cardDays', { n: days }, days)
})

const creditTermText = computed(() => {
  const until = props.result?.credit_expires_at ?? (used.value ? props.card.credit?.expires_at : null)
  if (until) return t('resetCards.tile.creditUntil', { date: formatResetMonthDay(until, locale.value) })
  const days = Number(props.card.credit_valid_days)
  return days > 0 ? t('resetCards.tile.creditDays', { n: days }, days) : t('resetCards.tile.creditLimited')
})
</script>

<style scoped>
/* 柱身：白色 22%；柱顶亮白那截：实心白，代表要返还的部分 */
.rc-base,
.rc-cut {
  fill: rgb(var(--c-money-ink));
  transform-box: fill-box;
  transform-origin: 50% 100%;
  transform: scaleY(0);
}

.rc-base {
  opacity: 0.22;
}

/* 打开时从左到右依次长出（间隔 70ms），之后柱顶一明一暗，柱与柱错开 */
.rc-viz.is-go .rc-base {
  animation: rc-grow 0.55s cubic-bezier(0.2, 0.8, 0.2, 1) forwards;
  animation-delay: calc(var(--i) * 70ms);
}

.rc-viz.is-go .rc-cut {
  animation:
    rc-grow 0.55s cubic-bezier(0.2, 0.8, 0.2, 1) forwards,
    rc-breathe 2.6s ease-in-out infinite;
  animation-delay:
    calc(var(--i) * 70ms + 200ms),
    calc(var(--i) * 180ms + 1s);
}

/* 用卡后：亮白那截飞向左侧金额并淡出 */
.rc-viz.is-fly .rc-base {
  transform: scaleY(1);
}

.rc-viz.is-fly .rc-cut {
  transform: scaleY(1);
  animation: rc-fly 0.7s cubic-bezier(0.5, 0, 0.75, 0) forwards;
  animation-delay: calc(var(--i) * 50ms);
}

@keyframes rc-grow {
  to {
    transform: scaleY(1);
  }
}

@keyframes rc-breathe {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.55;
  }
}

@keyframes rc-fly {
  from {
    transform: scaleY(1);
    opacity: 1;
  }
  to {
    transform: translate(var(--dx), -36px) scaleY(0.2);
    opacity: 0;
  }
}

/* 减少动画：全部关掉，直接显示最终状态 */
@media (prefers-reduced-motion: reduce) {
  .rc-base,
  .rc-cut,
  .rc-viz.is-go .rc-base,
  .rc-viz.is-go .rc-cut {
    animation: none !important;
    transform: scaleY(1);
  }

  .rc-viz.is-fly .rc-cut {
    animation: none !important;
    opacity: 0;
  }
}
</style>
