<template>
  <BaseDialog
    :show="show"
    :title="title"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div>
        <label class="input-label" for="account-create-template-name">
          {{ t('admin.accounts.createTemplate.name') }}
        </label>
        <input
          id="account-create-template-name"
          v-model="name"
          type="text"
          class="input"
          maxlength="80"
          data-testid="account-create-template-name"
          :placeholder="t('admin.accounts.createTemplate.namePlaceholder')"
        />
      </div>
      <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="includeGroups"
          type="checkbox"
          class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          data-testid="account-create-template-include-groups"
        />
        <span>
          <span class="font-medium">{{ t('admin.accounts.createTemplate.includeGroups') }}</span>
          <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.createTemplate.includeGroupsHint') }}
          </span>
        </span>
      </label>
      <label class="flex items-start gap-2 text-sm text-gray-700 dark:text-gray-300">
        <input
          v-model="isDefault"
          type="checkbox"
          class="mt-0.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
          data-testid="account-create-template-is-default"
        />
        <span>
          <span class="font-medium">{{ t('admin.accounts.createTemplate.isDefault') }}</span>
          <span class="mt-0.5 block text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.createTemplate.isDefaultHint') }}
          </span>
        </span>
      </label>
    </div>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="submitting || !name.trim()"
          data-testid="account-create-template-save-confirm"
          @click="handleConfirm"
        >
          {{ submitting ? t('common.submitting') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{
  show: boolean
  title: string
  initialName: string
  initialIncludeGroups: boolean
  initialIsDefault: boolean
  submitting?: boolean
}>()

const emit = defineEmits<{
  close: []
  confirm: [payload: { name: string; includeGroups: boolean; isDefault: boolean }]
}>()

const { t } = useI18n()
const name = ref('')
const includeGroups = ref(false)
const isDefault = ref(false)

watch(
  () => props.show,
  (show) => {
    if (!show) return
    name.value = props.initialName
    includeGroups.value = props.initialIncludeGroups
    isDefault.value = props.initialIsDefault
  },
  { immediate: true },
)

const handleConfirm = () => {
  const trimmed = name.value.trim()
  if (!trimmed) return
  emit('confirm', {
    name: trimmed,
    includeGroups: includeGroups.value,
    isDefault: isDefault.value,
  })
}
</script>
