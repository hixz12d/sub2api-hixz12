<template>
  <section class="card" aria-labelledby="external-recharge-heading">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 id="external-recharge-heading" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.payment.externalRecharge.title') }}</h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.externalRecharge.description') }}</p>
    </div>
    <div class="space-y-5 p-6">
      <p v-if="entries.length === 0" class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.externalRecharge.empty') }}</p>
      <div v-for="item in entries" :key="item.id" class="space-y-3 border-b border-gray-100 pb-5 last:border-0 last:pb-0 dark:border-dark-700" data-testid="external-recharge-entry">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="font-medium text-gray-900 dark:text-white">{{ item.label || t('admin.settings.payment.externalRecharge.newEntry') }}</span>
          <div class="flex items-center gap-3">
            <span class="text-sm text-gray-600 dark:text-gray-300">{{ item.enabled !== false ? t('admin.settings.payment.externalRecharge.shown') : t('admin.settings.payment.externalRecharge.hidden') }}</span>
            <Toggle :model-value="item.enabled !== false" :aria-label="t('admin.settings.payment.externalRecharge.toggle', { name: item.label })" data-testid="external-recharge-enabled" @update:model-value="update(item.id, { enabled: $event })" />
            <button type="button" class="btn btn-secondary btn-sm" :aria-label="t('admin.settings.payment.externalRecharge.remove', { name: item.label })" @click="remove(item.id)">{{ t('common.delete') }}</button>
          </div>
        </div>
        <div class="grid gap-3 sm:grid-cols-2">
          <label class="block">
            <span class="input-label">{{ t('admin.settings.payment.externalRecharge.name') }}</span>
            <input :value="item.label" type="text" class="input" maxlength="50" required data-testid="external-recharge-name" @input="update(item.id, { label: ($event.target as HTMLInputElement).value })" />
          </label>
          <label class="block">
            <span class="input-label">{{ t('admin.settings.payment.externalRecharge.url') }}</span>
            <input :value="item.url" type="url" class="input" maxlength="2048" required placeholder="https://" data-testid="external-recharge-url" @input="update(item.id, { url: ($event.target as HTMLInputElement).value })" />
          </label>
        </div>
      </div>
      <button type="button" class="btn btn-secondary" :disabled="modelValue.length >= 20" data-testid="add-external-recharge" @click="add">{{ t('admin.settings.payment.externalRecharge.add') }}</button>
      <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.payment.externalRecharge.saveHint') }}</p>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import type { CustomMenuItem } from '@/types'
import { isExternalRechargeEntry } from '@/utils/externalRecharge'

const props = defineProps<{ modelValue: CustomMenuItem[] }>()
const emit = defineEmits<{ 'update:modelValue': [items: CustomMenuItem[]] }>()
const { t } = useI18n()
const entries = computed(() => props.modelValue.filter(isExternalRechargeEntry))

function update(id: string, patch: Partial<CustomMenuItem>) {
  emit('update:modelValue', props.modelValue.map(item => item.id === id
    ? { ...item, placement: 'recharge', visibility: 'user', ...patch }
    : item))
}

function remove(id: string) {
  emit('update:modelValue', props.modelValue.filter(item => item.id !== id))
}

function add() {
  emit('update:modelValue', [...props.modelValue, {
    id: `recharge-${crypto.randomUUID().slice(0, 16)}`,
    label: t('admin.settings.payment.externalRecharge.newEntry'),
    url: '',
    icon_svg: '',
    visibility: 'user',
    placement: 'recharge',
    enabled: true,
    sort_order: props.modelValue.length,
  }])
}
</script>
