<template>
  <BaseDialog :show="true" :title="t('keys.questionTest.title')" width="wide" @close="close">
    <div class="space-y-5">
      <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800">
        <p class="font-medium text-gray-900 dark:text-white">{{ apiKey.name }}</p>
        <p v-if="apiKey.group" class="mt-1 text-gray-500 dark:text-dark-400">{{ apiKey.group.name }}</p>
        <p class="mt-2 text-gray-500 dark:text-dark-400">{{ t('keys.questionTest.billingHint') }}</p>
      </div>

      <div>
        <label for="key-test-model" class="input-label">{{ t('keys.questionTest.model') }}</label>
        <Select
          id="key-test-model"
          :aria-label="t('keys.questionTest.model')"
          v-model="model"
          :options="modelOptions"
          :disabled="loadingModels || running"
          searchable
          :placeholder="t(loadingModels ? 'common.loading' : 'keys.questionTest.selectModel')"
        />
        <div v-if="modelError" role="alert" class="mt-2 text-sm text-red-600 dark:text-red-400">
          {{ modelError }}
          <button type="button" class="ml-2 underline" @click="loadModels">{{ t('keys.questionTest.retry') }}</button>
        </div>
      </div>

      <div>
        <label for="key-test-question" class="input-label">{{ t('keys.questionTest.question') }}</label>
        <textarea
          id="key-test-question"
          v-model="question"
          class="input min-h-28 resize-y"
          rows="3"
          maxlength="4096"
          :disabled="running"
        />
        <p v-if="questionTooLong" role="alert" class="mt-1 text-sm text-red-600">{{ t('keys.questionTest.questionTooLong') }}</p>
      </div>

      <div class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-700">
        <div class="flex items-center justify-between bg-gray-50 px-4 py-2 text-sm dark:bg-dark-800">
          <span role="status" class="text-gray-600 dark:text-dark-300">{{ t(`keys.questionTest.status.${status}`) }}</span>
          <button v-if="answer" type="button" class="btn btn-secondary btn-sm" @click="copyAnswer">
            {{ t(copied ? 'keys.copied' : 'keys.copyToClipboard') }}
          </button>
        </div>
        <pre data-testid="key-test-answer" class="max-h-96 min-h-40 overflow-auto whitespace-pre-wrap break-words bg-gray-950 p-4 font-mono text-sm leading-relaxed text-gray-100">{{ answer || t('keys.questionTest.answerPlaceholder') }}</pre>
      </div>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <p v-if="truncated" role="alert" class="text-sm text-amber-600 dark:text-amber-400">{{ t('keys.questionTest.truncated') }}</p>
    </div>
    <template #footer>
      <button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
      <button v-if="running" type="button" class="btn btn-secondary" @click="stop">{{ t('keys.questionTest.stop') }}</button>
      <button v-else type="button" class="btn btn-primary" :disabled="!canStart" @click="start">{{ t('keys.questionTest.start') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import { KeyTestError, listKeyTestModels, runKeyQuestion } from '@/api/keyTest'
import { useClipboard } from '@/composables/useClipboard'
import type { ApiKey } from '@/types'

const props = defineProps<{ apiKey: ApiKey }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const model = ref('')
const models = ref<string[]>([])
const loadingModels = ref(false)
const modelError = ref('')
const question = ref("don't search the internet, who is Thibault Sottiaux on X")
const answer = ref('')
const error = ref('')
const truncated = ref(false)
const copied = ref(false)
const status = ref<'idle' | 'running' | 'complete' | 'stopped' | 'error'>('idle')
const running = computed(() => status.value === 'running')
const modelOptions = computed(() => models.value.map(value => ({ value, label: value })))
const questionTooLong = computed(() => new TextEncoder().encode(question.value).length > 4096)
const canStart = computed(() => props.apiKey.status === 'active' && !!model.value && !!question.value.trim() && !questionTooLong.value && !loadingModels.value)
let modelController: AbortController | undefined
let testController: AbortController | undefined

function errorText(cause: unknown): string {
  if (cause instanceof KeyTestError) {
    return cause.code === 'http' && cause.message ? cause.message : t(`keys.questionTest.errors.${cause.code}`)
  }
  return t('keys.questionTest.errors.network')
}

async function loadModels() {
  modelController?.abort()
  const controller = new AbortController()
  modelController = controller
  const timeout = setTimeout(() => controller.abort(), 20_000)
  loadingModels.value = true
  modelError.value = ''
  model.value = ''
  models.value = []
  try {
    const result = await listKeyTestModels(props.apiKey.key, controller.signal)
    if (modelController !== controller) return
    if (controller.signal.aborted) throw new Error('Aborted')
    models.value = result
    model.value = result[0] || ''
    if (!result.length) modelError.value = t('keys.questionTest.noModels')
  } catch (cause) {
    if (modelController !== controller) return
    modelError.value = controller.signal.aborted ? t('keys.questionTest.errors.timeout') : errorText(cause)
  } finally {
    clearTimeout(timeout)
    if (modelController === controller) loadingModels.value = false
  }
}

function stop() {
  testController?.abort()
  testController = undefined
  if (running.value) status.value = 'stopped'
}

async function start() {
  if (!canStart.value || running.value) return
  const controller = new AbortController()
  testController = controller
  const timeout = setTimeout(() => controller.abort(), 300_000)
  status.value = 'running'
  answer.value = ''
  error.value = ''
  truncated.value = false
  copied.value = false
  try {
    const result = await runKeyQuestion(props.apiKey.key, model.value, question.value, controller.signal, text => {
      if (testController === controller && !controller.signal.aborted) answer.value += text
    })
    if (testController !== controller) return
    if (controller.signal.aborted) throw new Error('Aborted')
    truncated.value = result.truncated
    status.value = 'complete'
  } catch (cause) {
    if (testController !== controller) return
    status.value = 'error'
    error.value = controller.signal.aborted ? t('keys.questionTest.errors.timeout') : errorText(cause)
  } finally {
    clearTimeout(timeout)
    if (testController === controller) testController = undefined
  }
}

async function copyAnswer() {
  copied.value = await copyToClipboard(answer.value)
}

function dispose() {
  modelController?.abort()
  modelController = undefined
  stop()
}

function close() {
  dispose()
  emit('close')
}

watch(() => [props.apiKey.id, props.apiKey.group_id], () => {
  dispose()
  answer.value = ''
  error.value = ''
  truncated.value = false
  status.value = 'idle'
  void loadModels()
}, { immediate: true })
onBeforeUnmount(dispose)
</script>
