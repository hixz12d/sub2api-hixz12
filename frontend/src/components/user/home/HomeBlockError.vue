<template>
  <div class="flex items-center justify-between gap-4 bg-surface-tile px-5 py-4 text-sm" role="alert">
    <span class="inline-flex items-center gap-2 text-bad">
      <Icon name="exclamationCircle" size="sm" />
      {{ t('userHome.loadFailed') }}
    </span>
    <button type="button" class="btn btn-secondary btn-sm" @click="$emit('retry')">{{ t('userHome.retry') }}</button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

defineEmits<{ retry: [] }>()
const { t } = useI18n()
</script>
