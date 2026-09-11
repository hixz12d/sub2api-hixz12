<template>
  <div class="border-t border-gray-200 pt-4 mt-4 dark:border-dark-400">
    <div class="flex items-center justify-between gap-3">
      <label :for="inputId" class="input-label">{{ t('admin.groups.modelsList.title') }}</label>
      <Toggle :model-value="modelValue.enabled" @update:model-value="setEnabled" />
    </div>
    <textarea
      v-if="modelValue.enabled"
      :id="inputId"
      :value="modelText"
      @change="modelText = ($event.target as HTMLTextAreaElement).value"
      class="input mt-3 min-h-28 font-mono text-sm"
      :aria-label="t('admin.groups.modelsList.title')"
      spellcheck="false"
      data-testid="models-list-config"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import type { ModelsListConfig } from '@/types'

const props = defineProps<{ modelValue: ModelsListConfig }>()
const emit = defineEmits<{ 'update:modelValue': [value: ModelsListConfig] }>()
const { t } = useI18n()
const inputId = useId()
const setEnabled = (enabled: boolean) => emit('update:modelValue', { ...props.modelValue, enabled })
const modelText = computed({
  get: () => props.modelValue.models.join('\n'),
  set: (value: string) => emit('update:modelValue', {
    ...props.modelValue,
    models: [...new Set(value.split(/\r?\n/).map(model => model.trim()).filter(Boolean))]
  })
})
</script>
