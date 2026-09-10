<template>
  <p v-if="failed" class="text-xs text-gray-500">{{ zh ? '人工评估暂不可用' : 'Human assessments unavailable' }}</p>
  <p v-else-if="summary" class="break-words text-xs" :class="summary.degraded ? 'text-amber-700 dark:text-amber-400' : 'text-gray-500'">
    {{ group ? (zh ? '渠道历史人工评估（全部模型）' : 'Channel human assessments (all models)') : (zh ? '历史人工评估（账号/模型）' : 'Past human assessments (account/model)') }}:
    {{ zh ? '疑似降智' : 'Suspected degradation' }} {{ summary.degraded }} ·
    {{ zh ? '正常' : 'Normal' }} {{ summary.normal }} ·
    {{ zh ? '未标注' : 'Unlabeled' }} {{ summary.unlabeled }}
  </p>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { HumanAssessment } from '@/composables/useHumanAssessments'
defineProps<{ summary?: HumanAssessment; failed?: boolean; group?: boolean }>()
const { locale } = useI18n()
const zh = computed(() => locale.value.startsWith('zh'))
</script>
