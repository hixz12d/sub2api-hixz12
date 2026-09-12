<template>
  <details class="rounded-lg border border-gray-200 dark:border-dark-700" @toggle="expanded = ($event.target as HTMLDetailsElement).open">
    <summary class="cursor-pointer px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">{{ t('connectionDiagnostics.title') }}</summary>
    <div v-if="expanded" class="space-y-4 px-4 pb-4 text-sm" data-testid="connection-diagnostics">
      <p class="text-gray-600 dark:text-dark-300">{{ t('connectionDiagnostics.scope') }}</p>
      <p :class="configurationError ? 'text-red-600' : 'text-emerald-700 dark:text-emerald-400'">{{ t('connectionDiagnostics.' + (configurationError || 'configurationReady')) }}</p>
      <p class="break-all font-mono text-xs text-gray-500">{{ normalizedBase }}</p>
      <div class="grid gap-3 sm:grid-cols-2">
        <label class="block">{{ t('connectionDiagnostics.model') }}
          <input v-model.trim="model" class="input mt-1 w-full" :disabled="busy" data-testid="diagnostic-model" />
        </label>
        <label class="block">{{ t('connectionDiagnostics.clientVersion') }}
          <input v-model.trim="clientVersion" class="input mt-1 w-full" maxlength="100" placeholder="codex --version" />
        </label>
      </div>
      <p class="text-xs leading-5 text-gray-500 dark:text-dark-400">{{ t('connectionDiagnostics.compatibility') }}</p>
      <div class="flex flex-wrap gap-2">
        <button type="button" class="btn btn-secondary" :disabled="busy || !!configurationError" data-testid="check-models" @click="checkModels">{{ t('connectionDiagnostics.checkModels') }}</button>
        <button v-if="busy" type="button" class="btn btn-secondary" @click="cancelRun">{{ t('connectionDiagnostics.cancel') }}</button>
      </div>
      <div class="space-y-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-900" role="status" aria-live="polite">
        <p>{{ t('connectionDiagnostics.models') }}: {{ statusText(modelsState) }}</p>
        <p v-if="modelsState === 'passed'">{{ t('connectionDiagnostics.modelAvailability') }}: {{ t('connectionDiagnostics.' + (models.includes(model) ? 'available' : 'modelMissing')) }}</p>
        <p>{{ t('connectionDiagnostics.httpStream') }}: {{ statusText(streamState) }}</p>
        <p v-if="failure" class="text-red-600 dark:text-red-400">{{ t('connectionDiagnostics.' + failure.code) }}<span v-if="failure.status"> (HTTP {{ failure.status }})</span></p>
      </div>
      <label class="flex items-start gap-2 text-xs leading-5 text-gray-600 dark:text-dark-300">
        <input v-model="allowPaidTest" type="checkbox" class="mt-1" :disabled="busy" data-testid="allow-paid-test" />
        {{ t('connectionDiagnostics.paidNotice') }}
      </label>
      <button type="button" class="btn btn-secondary" :disabled="busy || !allowPaidTest || !!configurationError || !model || modelsState !== 'passed' || !models.includes(model)" data-testid="check-stream" @click="checkStream">{{ t('connectionDiagnostics.checkStream') }}</button>
      <div class="space-y-2 border-t border-gray-200 pt-3 dark:border-dark-700">
        <p class="font-medium">{{ t('connectionDiagnostics.localTitle') }}</p>
        <p class="text-xs leading-5 text-gray-600 dark:text-dark-300">{{ t('connectionDiagnostics.localHint') }}</p>
        <div class="flex flex-wrap gap-2">
          <a :href="scriptUrl" download="connection-diagnostics.mjs" class="btn btn-secondary">{{ t('connectionDiagnostics.download') }}</a>
          <button type="button" class="btn btn-secondary" :disabled="!!configurationError || !model" @click="copyToClipboard(localCommand)">{{ t('connectionDiagnostics.copyCommand') }}</button>
          <button type="button" class="btn btn-secondary" @click="copyReport">{{ t('connectionDiagnostics.copyReport') }}</button>
        </div>
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('connectionDiagnostics.commandHint') }}</p>
      </div>
    </div>
  </details>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { diagnosticBaseUrl, diagnosticFailure, probeDiagnosticModels, probeDiagnosticStream } from '@/utils/connectionDiagnostics'

const props = defineProps<{ baseUrl: string; apiKey: string; initialModel: string; windows: boolean; authMode: string }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const expanded = ref(false)
const model = ref(props.initialModel)
const clientVersion = ref('')
const allowPaidTest = ref(false)
const models = ref<string[]>([])
type State = 'idle' | 'running' | 'passed' | 'failed' | 'cancelled'
const modelsState = ref<State>('idle')
const streamState = ref<State>('idle')
const failure = ref<{ code: string; status?: number } | null>(null)
const busy = ref(false)
let controller: AbortController | null = null
let version = 0
const scriptUrl = `${import.meta.env.BASE_URL}connection-diagnostics.mjs`
const normalizedBase = computed(() => { try { return diagnosticBaseUrl(props.baseUrl) } catch { return '' } })
const configurationError = computed(() => {
  if (!props.apiKey.trim()) return 'missingKey'
  try { diagnosticBaseUrl(props.baseUrl); return '' } catch (error) { return diagnosticFailure(error).code }
})
const statusText = (state: State) => t('connectionDiagnostics.' + state)

function cancelRun() {
  version++
  controller?.abort()
  controller = null
  busy.value = false
  if (modelsState.value === 'running') modelsState.value = 'cancelled'
  if (streamState.value === 'running') streamState.value = 'cancelled'
}
watch(() => [props.baseUrl, props.apiKey, props.initialModel, props.authMode], () => {
  cancelRun(); model.value = props.initialModel; models.value = []; modelsState.value = 'idle'; streamState.value = 'idle'; failure.value = null; allowPaidTest.value = false
})
watch(model, () => { streamState.value = 'idle'; allowPaidTest.value = false })
onUnmounted(cancelRun)

async function run(kind: 'models' | 'stream') {
  if (busy.value || configurationError.value || (kind === 'stream' && (!allowPaidTest.value || !models.value.includes(model.value)))) return
  cancelRun()
  const current = version
  const abort = new AbortController()
  controller = abort
  const state = kind === 'models' ? modelsState : streamState
  if (kind === 'models') { models.value = []; streamState.value = 'idle' }
  state.value = 'running'; busy.value = true; failure.value = null
  const timer = setTimeout(() => abort.abort(), kind === 'models' ? 15000 : 45000)
  try {
    if (kind === 'models') {
      const result = await probeDiagnosticModels(props.baseUrl, props.apiKey, abort.signal)
      if (version === current) models.value = result
    } else {
      await probeDiagnosticStream(props.baseUrl, props.apiKey, model.value, abort.signal)
    }
    if (version === current) state.value = 'passed'
  } catch (error) {
    if (version === current) { state.value = 'failed'; failure.value = diagnosticFailure(error) }
  } finally {
    clearTimeout(timer)
    if (version === current) { busy.value = false; controller = null }
  }
}
const checkModels = () => run('models')
const checkStream = () => run('stream')
const localCommand = computed(() => {
  const quote = (value: string) => props.windows ? `'${value.replace(/'/g, "''")}'` : `'${value.replace(/'/g, "'\\''")}'`
  const env = props.windows ? `$env:SUB2API_API_KEY=${quote(props.apiKey)}\n` : `SUB2API_API_KEY=${quote(props.apiKey)} `
  return `${env}node ./connection-diagnostics.mjs --base-url ${quote(normalizedBase.value)} --model ${quote(model.value)}`
})
function copyReport() {
  let report = JSON.stringify({ time: new Date().toISOString(), base_url: normalizedBase.value, model: model.value, auth_mode: props.authMode, client_version: clientVersion.value, models: modelsState.value, model_available: modelsState.value === 'passed' ? models.value.includes(model.value) : null, http_stream: streamState.value, websocket: 'not_tested_in_browser', error: failure.value }, null, 2)
  if (props.apiKey) report = report.split(props.apiKey).join('[redacted]')
  report = report.replace(/sk-[\w-]{8,}/gi, '[redacted]').replace(/Bearer\s+[^\s",;]+/gi, 'Bearer [redacted]')
  void copyToClipboard(report)
}
</script>
