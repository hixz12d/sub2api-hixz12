<template>
  <!-- 钱包"重置卡"页签内容（只在新版钱包里出现，由钱包页提供外壳） -->
  <section aria-labelledby="reset-cards-heading">
    <div class="mb-3 flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
      <h2 id="reset-cards-heading" ref="headingRef" tabindex="-1" class="outline-none text-[26px] font-light leading-none tracking-[-0.02em] text-ink sm:text-[30px]">
        {{ t('resetCards.list.heading') }}
      </h2>
      <span v-if="store.loaded && store.unusedCount > 0" class="text-[13px] tabular-nums text-ink-3">
        {{ t('resetCards.list.availableCount', { n: store.unusedCount }) }}
      </span>
    </div>

    <!-- 首次加载 -->
    <ul v-if="!store.loaded && !store.error" class="m-0 grid list-none gap-1 p-0" aria-busy="true">
      <li v-for="n in 3" :key="n" class="h-[76px] animate-pulse bg-surface-tile"></li>
    </ul>

    <!-- 加载失败 -->
    <div v-else-if="!store.loaded && store.error" class="tile flex flex-wrap items-center justify-between gap-3">
      <span class="inline-flex items-center gap-2 text-sm text-bad">
        <Icon name="exclamationCircle" size="sm" :stroke-width="2" aria-hidden="true" />
        {{ t('resetCards.list.loadFailed') }}
      </span>
      <button type="button" class="btn btn-secondary btn-sm" @click="store.fetch(true)">
        {{ t('resetCards.list.retry') }}
      </button>
    </div>

    <!-- 空状态 -->
    <div v-else-if="sortedCards.length === 0" class="tile py-8">
      <p class="m-0 text-[17px] font-semibold text-ink">{{ t('resetCards.list.emptyTitle') }}</p>
      <p class="m-0 mt-2 max-w-[60ch] text-sm text-ink-2">{{ t('resetCards.list.emptyDesc') }}</p>
    </div>

    <ul v-else class="m-0 grid list-none gap-1 p-0">
      <ResetCardListItem v-for="card in sortedCards" :key="card.id" :card="card" @use="activeCardId = $event.id" />
    </ul>

    <ResetCardPopup :card-id="activeCardId" @close="onPopupClose" />
  </section>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ResetCardListItem from '@/components/user/resetCard/ResetCardListItem.vue'
import ResetCardPopup from '@/components/user/resetCard/ResetCardPopup.vue'
import { useResetCardsStore } from '@/stores/resetCards'
import type { ResetCard, ResetCardStatus } from '@/types/resetCard'

const { t } = useI18n()
const store = useResetCardsStore()

const activeCardId = ref<number | null>(null)
const headingRef = ref<HTMLElement | null>(null)

// 用卡后列表里的"使用"按钮会消失，焦点交给标题，键盘用户不会掉到页面顶部
function onPopupClose() {
  activeCardId.value = null
  void nextTick(() => {
    if (!document.activeElement || document.activeElement === document.body) headingRef.value?.focus()
  })
}

const STATUS_ORDER: Record<ResetCardStatus, number> = { unused: 0, used: 1, expired: 2, revoked: 3 }

function time(iso: string | null | undefined): number {
  const v = iso ? new Date(iso).getTime() : 0
  return Number.isFinite(v) ? v : 0
}

// 可用的在上，按到期时间从近到远；然后是已使用、已过期、已作废（组内按最近发生的在前）
const sortedCards = computed<ResetCard[]>(() =>
  [...store.cards].sort((a, b) => {
    const byStatus = STATUS_ORDER[a.status] - STATUS_ORDER[b.status]
    if (byStatus !== 0) return byStatus
    if (a.status === 'unused') return time(a.expires_at) - time(b.expires_at)
    if (a.status === 'used') return time(b.used_at) - time(a.used_at)
    if (a.status === 'expired') return time(b.expires_at) - time(a.expires_at)
    return time(b.created_at) - time(a.created_at)
  })
)

onMounted(() => {
  // 进入页签时拿一次最新数据（预计返还每天都会变）
  void store.fetch(true)
})
</script>
