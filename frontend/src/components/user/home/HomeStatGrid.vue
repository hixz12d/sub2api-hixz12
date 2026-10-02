<template>
  <div class="tile-grid grid-cols-2 lg:grid-cols-4">
    <div v-for="cell in cells" :key="cell.key" class="tile min-w-0">
      <div class="flex items-center gap-1.5 text-sm text-ink-2">
        {{ cell.label }}
        <span
          v-if="cell.hint"
          class="inline-flex cursor-help text-ink-3"
          :title="cell.hint"
          tabindex="0"
          :aria-label="cell.hint"
        >
          <Icon name="questionCircle" size="sm" />
        </span>
      </div>
      <div v-if="loading" class="mt-3 h-9 w-24 animate-pulse bg-surface-tile-2 lg:mt-[18px]"></div>
      <div v-else class="mt-3 truncate text-[30px] font-light leading-none tracking-[-0.03em] tabular-nums text-ink lg:mt-[18px] lg:text-[40px]">
        {{ cell.value }}<small v-if="cell.unit" class="ml-1.5 text-[15px] font-normal tracking-normal text-ink-3">{{ cell.unit }}</small>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { formatHomeCount, formatHomeMoney, formatHomeTokens } from './homeFormat'

const props = defineProps<{
  requests: number
  tokens: number
  cost: number
  /** null：有效样本不足 */
  speed: number | null
  loading: boolean
  speedLoading: boolean
}>()

const { t } = useI18n()

const cells = computed(() => [
  { key: 'req', label: t('userHome.usage.requests'), value: formatHomeCount(props.requests), unit: t('userHome.usage.requestsUnit') },
  { key: 'tok', label: t('userHome.usage.tokens'), value: formatHomeTokens(props.tokens) },
  { key: 'cost', label: t('userHome.usage.cost'), value: formatHomeMoney(props.cost) },
  {
    key: 'tps',
    label: t('userHome.usage.speed'),
    value: props.speedLoading ? '…' : props.speed == null ? '—' : props.speed.toFixed(1),
    unit: props.speed == null ? '' : t('userHome.usage.speedUnit'),
    hint: t('userHome.usage.speedHint')
  }
])
</script>
