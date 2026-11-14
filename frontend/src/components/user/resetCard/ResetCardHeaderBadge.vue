<template>
  <!-- 顶栏小标：有可用的重置卡时才出现，点进钱包"重置卡"页签 -->
  <router-link
    v-if="visible"
    to="/wallet/reset-cards"
    class="inline-flex h-10 shrink-0 items-center gap-1.5 px-2 text-ink-2 transition-colors hover:bg-surface-tile hover:text-ink sm:px-2.5"
    :aria-label="label"
    :title="label"
  >
    <Icon name="creditCard" size="md" />
    <span
      class="inline-flex h-5 min-w-[20px] items-center justify-center bg-money px-1 text-xs font-semibold tabular-nums text-money-ink"
      aria-hidden="true"
    >
      {{ countText }}
    </span>
  </router-link>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useResetCardsStore } from '@/stores/resetCards'

const { t } = useI18n()
const store = useResetCardsStore()

const visible = computed(() => store.enabled && store.unusedCount > 0)
const countText = computed(() => (store.unusedCount > 99 ? '99+' : String(store.unusedCount)))
const label = computed(() => t('resetCards.badge.aria', { n: store.unusedCount }, store.unusedCount))
</script>
