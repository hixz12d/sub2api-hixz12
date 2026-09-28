<template>
  <section v-if="entries.length" class="card p-5" aria-labelledby="external-recharge-methods-heading">
    <h2 id="external-recharge-methods-heading" class="font-medium text-gray-900 dark:text-white">{{ t('payment.externalRecharge.title') }}</h2>
    <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.externalRecharge.description') }}</p>
    <div class="mt-4 divide-y divide-gray-100 dark:divide-dark-700">
      <RouterLink v-for="entry in entries" :key="entry.id" :to="`/custom/${entry.id}`" class="flex min-h-12 items-center justify-between gap-4 rounded px-2 py-3 text-primary-600 hover:bg-primary-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-primary-400 dark:hover:bg-dark-700" data-testid="external-recharge-method">
        <span class="min-w-0 break-words font-medium">{{ entry.label }}</span>
        <span class="flex shrink-0 items-center gap-1 text-sm">{{ t('payment.externalRecharge.open') }}<Icon name="chevronRight" size="sm" /></span>
      </RouterLink>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import type { CustomMenuItem } from '@/types'

defineProps<{ entries: CustomMenuItem[] }>()
const { t } = useI18n()
</script>
