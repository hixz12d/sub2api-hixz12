<template>
  <div class="flex flex-col gap-1">
    <h1 class="text-[40px] font-light leading-[1.08] tracking-[-0.035em] text-ink sm:text-[54px]">
      {{ greeting }}
    </h1>
    <p class="text-[15px] font-light text-ink-2 sm:text-[17px]">
      <i18n-t v-if="todayRequests > 0" keypath="userHome.todaySummary" tag="span">
        <template #requests><b class="font-semibold tabular-nums text-ink">{{ formatHomeCount(todayRequests) }}</b></template>
        <template #cost><b class="font-semibold tabular-nums text-ink">{{ formatHomeMoney(todayCost) }}</b></template>
      </i18n-t>
      <span v-else>{{ t('userHome.todayIdle') }}</span>
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { formatHomeCount, formatHomeMoney } from './homeFormat'

const props = defineProps<{
  name: string
  todayRequests: number
  todayCost: number
}>()

const { t } = useI18n()

const greeting = computed(() => {
  const h = new Date().getHours()
  const key = h < 5 ? 'evening' : h < 11 ? 'morning' : h < 13 ? 'noon' : h < 18 ? 'afternoon' : 'evening'
  return t(`userHome.greeting.${key}`, { name: props.name })
})
</script>
