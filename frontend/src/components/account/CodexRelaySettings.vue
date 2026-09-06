<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import { getClientProfiles, previewClientProfile, type ClientProfileCatalog, type ClientProfilePreview } from '@/api/admin/clientProfiles'
import {
  patchCodexRelayState, serializeCodexRelayToExtra, validateCodexRelayState,
  type CodexRelayFormState, type CodexClientProfile, type CodexRelayMode
} from './codexRelaySchema'

const props = withDefaults(defineProps<{
  modelValue: CodexRelayFormState
  errors?: Record<string, string>
  disabled?: boolean
  accountId?: number
  accountType?: string
  tlsEnabled?: boolean | null
  bulk?: boolean
}>(), { accountType: 'oauth', tlsEnabled: true, bulk: false })
const emit = defineEmits<{ (e: 'update:modelValue', value: CodexRelayFormState): void }>()
const { t } = useI18n()
const label = (key: string) => t(`admin.accounts.openai.${key}`)
const catalog = ref<ClientProfileCatalog | null>(null)
const catalogError = ref(false)
const preview = ref<ClientProfilePreview | null>(null)
const previewError = ref(false)
const previewPending = ref(false)
const operation = ref<'responses' | 'compact' | 'resume'>('responses')
const transport = ref<'http' | 'ws'>('http')
let catalogController: AbortController | undefined
let previewController: AbortController | undefined
let timer: ReturnType<typeof setTimeout> | undefined

function patch(value: Partial<Omit<CodexRelayFormState, '_persisted'>>) {
  emit('update:modelValue', patchCodexRelayState(props.modelValue, value))
}
const relayMode = computed<CodexRelayMode>({
  get: () => props.modelValue.codex_relay_mode,
  set: (value) => patch(value === 'relay_kernel'
    ? { codex_relay_mode: value, codex_identity_policy_version: 'v2', codex_fingerprint_mode: props.modelValue.codex_fingerprint_mode === 'off' ? 'device' : props.modelValue.codex_fingerprint_mode }
    : { codex_relay_mode: value, codex_installation_policy: 'legacy_v2' })
})
const isKernelActive = computed(() => relayMode.value === 'relay_kernel')
const clientProfile = computed<CodexClientProfile>({ get: () => props.modelValue.codex_client_profile, set: (value) => patch({ codex_client_profile: value }) })
const currentProfileMeta = computed(() => catalog.value?.profiles.find((item) => item.id === clientProfile.value))
const family = computed({
  get: () => {
    const id = clientProfile.value
    if (id.startsWith('codex_')) return 'codex'
    if (id === 'pi' || id.startsWith('pi-')) return 'pi'
    if (id === 'opencode' || id.startsWith('opencode-')) return 'opencode'
    return ''
  },
  set: (value: string) => {
    if (value === family.value) return
    const selectors: Record<string, CodexClientProfile> = { codex: 'codex_cli', opencode: 'opencode', pi: 'pi' }
    if (selectors[value]) clientProfile.value = selectors[value]
  }
})
const management = computed({
  get: () => ['auto', 'passthrough'].includes(clientProfile.value) ? clientProfile.value : 'explicit',
  set: (value: string) => { clientProfile.value = value === 'explicit' ? (family.value ? clientProfile.value : 'codex_cli') : value as CodexClientProfile }
})
const familyOptions = [ { value: 'codex', label: 'Codex' }, { value: 'opencode', label: 'OpenCode' }, { value: 'pi', label: 'Pi' } ]
const managementOptions = computed(() => [
  { value: 'auto', label: label('codexClientProfileAuto') },
  { value: 'passthrough', label: label('codexClientProfilePassthrough') },
  { value: 'explicit', label: label('codexProfileExplicit') }
])
const clientProfileOptions = computed(() => {
  const options = (catalog.value?.profiles ?? []).filter((item) => item.family === family.value).map((item) => ({
    value: item.id, label: item.id, disabled: item.variant === 'shared-r1' && !isKernelActive.value
  }))
  if (!options.some((item) => item.value === clientProfile.value)) options.unshift({ value: clientProfile.value, label: clientProfile.value, disabled: true })
  return options
})
const relayModeOptions = computed(() => [
  { value: 'legacy', label: label('codexRelayModeLegacy') }, { value: 'relay_kernel', label: label('codexRelayModeKernel') }
])
const identityPolicyOptions = computed(() => [
  { value: 'v1', label: label('codexIdentityPolicyV1') }, { value: 'v2', label: label('codexIdentityPolicyV2') }
])
const fingerprintOptions = computed(() => ['off', 'device', 'session', 'window', 'window40', 'full'].map((value) => ({
  value, label: label(`codexFingerprint${value[0].toUpperCase()}${value.slice(1)}`), disabled: value === 'off' && isKernelActive.value
})))
const installationOptions = computed(() => [
  { value: 'legacy_v2', label: label('codexInstallationLegacy') }, { value: 'stable_v1', label: label('codexInstallationStable') }
])
const localErrors = computed(() => ({ ...props.errors, ...validateCodexRelayState(props.modelValue, t, { tlsEnabled: props.tlsEnabled, bulk: props.bulk }).errors }))
const profileVersionLabel = computed(() => {
  const version = currentProfileMeta.value?.appVersion
  if (!version || version === 'dynamic') return label('codexProfilePending')
  if (version === 'caller supplied') return label('codexProfileCallerSupplied')
  if (version === 'not asserted') return label('codexProfileUnverified')
  return version
})
const profileFidelityLabel = computed(() => {
  const keys = { 'caller-resolved': 'codexProfilePending', 'passthrough/degraded': 'codexProfileCallerSupplied', degraded: 'codexProfileDegraded', 'unsupported strict parity': 'codexProfileUnverified' }
  return label(currentProfileMeta.value ? keys[currentProfileMeta.value.fidelity] : 'codexProfilePending')
})
const capabilities = computed(() => [
  { name: 'HTTP', supported: currentProfileMeta.value?.http ?? null },
  { name: 'WS', supported: currentProfileMeta.value?.ws ?? null },
  { name: 'Compact', supported: currentProfileMeta.value?.compact ?? null }
])
const capabilityLabel = (value: boolean | null) => label(value === null ? 'codexProfilePending' : value ? 'codexProfileSupported' : 'codexProfileUnsupported')

async function loadCatalog() {
  catalogController?.abort()
  const controller = new AbortController()
  catalogController = controller
  catalogError.value = false
  try {
    const result = await getClientProfiles(controller.signal)
    if (!controller.signal.aborted) catalog.value = result
  } catch {
    if (!controller.signal.aborted) { catalog.value = null; catalogError.value = true }
  }
}
watch([() => props.modelValue, () => props.tlsEnabled, () => props.accountId, () => props.accountType, () => props.disabled, () => catalog.value?.revision, operation, transport], () => {
  clearTimeout(timer)
  previewController?.abort()
  preview.value = null
  previewError.value = false
  previewPending.value = false
  if (!catalog.value || props.disabled || (props.bulk && props.tlsEnabled == null)) return
  const controller = new AbortController()
  previewController = controller
  previewPending.value = true
  timer = setTimeout(async () => {
    const extra: Record<string, unknown> = {}
    serializeCodexRelayToExtra(props.modelValue, extra)
    if (props.tlsEnabled != null) extra.enable_tls_fingerprint = props.tlsEnabled
    try {
      const result = await previewClientProfile({ account_id: props.accountId, platform: 'openai', type: props.accountType, extra,
        operation: operation.value, transport: transport.value, catalog_revision: catalog.value!.revision }, controller.signal)
      if (!controller.signal.aborted) preview.value = result
    } catch {
      if (!controller.signal.aborted) previewError.value = true
    } finally {
      if (!controller.signal.aborted) previewPending.value = false
    }
  }, 200)
}, { deep: true })
onMounted(loadCatalog)
onBeforeUnmount(() => { clearTimeout(timer); catalogController?.abort(); previewController?.abort() })
</script>

<template>
  <section class="col-span-full mt-2 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="codex-settings">
    <div class="mb-4 flex flex-wrap items-center justify-between gap-2">
      <h4 class="text-sm font-medium text-gray-900 dark:text-white">{{ label('codexClientConfiguration') }}</h4>
      <span class="text-xs" :class="isKernelActive ? 'text-emerald-700 dark:text-emerald-400' : 'text-gray-500'">{{ label(isKernelActive ? 'codexRelayKernelBadge' : 'codexRelayModeLegacy') }}</span>
    </div>
    <div v-if="catalogError" class="mb-3 flex flex-wrap items-center gap-3 text-xs text-red-600" role="alert">
      {{ label('codexCatalogUnavailable') }}
      <button type="button" class="underline" @click="loadCatalog">{{ t('common.retry') }}</button>
    </div>
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
      <div>
        <label class="input-label">{{ label('codexManagementMode') }}</label>
        <Select v-model="management" :options="managementOptions" :disabled="disabled" data-testid="codex-management-select" />
      </div>
      <div>
        <label class="input-label">{{ label('codexClientFamily') }}</label>
        <Select v-model="family" :options="familyOptions" :disabled="disabled || management !== 'explicit' || !catalog" data-testid="codex-client-family-select" />
      </div>
    </div>
    <div v-for="(error, key) in localErrors" :key="key" class="mt-2 text-xs text-red-600 dark:text-red-400" role="alert">{{ error }}</div>

    <dl class="mt-4 grid grid-cols-1 gap-x-6 gap-y-2 text-xs sm:grid-cols-2" data-testid="codex-profile-summary">
      <div class="flex flex-wrap justify-between gap-2"><dt class="text-gray-500">{{ label('codexProfileAppVersion') }}</dt><dd class="break-all" data-testid="codex-profile-version">{{ profileVersionLabel }}</dd></div>
      <div class="flex flex-wrap justify-between gap-2"><dt class="text-gray-500">{{ label('codexProfileFidelity') }}</dt><dd class="break-words" data-testid="codex-profile-fidelity">{{ profileFidelityLabel }}</dd></div>
      <div v-for="item in capabilities" :key="item.name" class="flex justify-between gap-2" :data-testid="`codex-profile-capability-${item.name.toLowerCase()}`">
        <dt class="font-mono text-gray-500">{{ item.name }}</dt><dd :data-supported="item.supported === null ? 'pending' : String(item.supported)">{{ capabilityLabel(item.supported) }}</dd>
      </div>
      <div class="flex flex-wrap justify-between gap-2"><dt class="text-gray-500">TLS/H2</dt><dd>{{ label('codexProfileUnverified') }}</dd></div>
    </dl>

    <details class="mt-4 border-t border-gray-200 pt-3 dark:border-dark-600" data-testid="codex-advanced">
      <summary class="cursor-pointer text-sm font-medium">{{ label('codexAdvancedConfiguration') }}</summary>
      <div class="mt-3 grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div><label class="input-label">{{ label('codexProfileVariant') }}</label><Select v-model="clientProfile" :options="clientProfileOptions" :disabled="disabled || !catalog" data-testid="codex-client-profile-select" /></div>
        <div><label class="input-label">{{ label('codexRelayMode') }}</label><Select v-model="relayMode" :options="relayModeOptions" :disabled="disabled" data-testid="codex-relay-mode-select" /></div>
        <div><label class="input-label">{{ label('codexIdentityPolicyVersion') }}</label><Select :model-value="modelValue.codex_identity_policy_version" :options="identityPolicyOptions" :disabled="disabled || isKernelActive" data-testid="codex-identity-policy-select" @update:model-value="patch({ codex_identity_policy_version: $event as CodexRelayFormState['codex_identity_policy_version'] })" /></div>
        <div><label class="input-label">{{ label('codexFingerprintMode') }}</label><Select :model-value="modelValue.codex_fingerprint_mode" :options="fingerprintOptions" :disabled="disabled" data-testid="codex-fingerprint-mode-select" @update:model-value="patch({ codex_fingerprint_mode: $event as CodexRelayFormState['codex_fingerprint_mode'] })" /></div>
        <div><label class="input-label">{{ label('codexInstallationPolicy') }}</label><Select :model-value="modelValue.codex_installation_policy ?? 'legacy_v2'" :options="installationOptions" :disabled="disabled || !isKernelActive" data-testid="codex-installation-policy-select" @update:model-value="patch({ codex_installation_policy: $event as CodexRelayFormState['codex_installation_policy'] })" /></div>
        <div class="flex items-center justify-between gap-3"><label class="input-label mb-0">{{ label('codexRelayShadowEnabled') }}</label><Toggle :model-value="modelValue.codex_relay_shadow_enabled" :disabled="disabled" data-testid="codex-relay-shadow-switch" @update:model-value="patch({ codex_relay_shadow_enabled: $event })" /></div>
      </div>
      <p v-if="catalog && !catalog.new_managed_default_enabled" class="mt-3 text-xs text-amber-700 dark:text-amber-400">{{ label('codexManagedDefaultGated') }}</p>
    </details>

    <div class="mt-4 border-t border-gray-200 pt-3 dark:border-dark-600" data-testid="codex-effective-preview">
      <div class="mb-3 grid grid-cols-2 gap-3">
        <div><label class="input-label">{{ label('codexPreviewOperation') }}</label><Select v-model="operation" :options="[{ value: 'responses', label: 'Responses' }, { value: 'compact', label: 'Compact' }, { value: 'resume', label: 'Resume' }]" :disabled="disabled" /></div>
        <div><label class="input-label">{{ label('codexPreviewTransport') }}</label><Select v-model="transport" :options="[{ value: 'http', label: 'HTTP/SSE' }, { value: 'ws', label: 'WebSocket' }]" :disabled="disabled" /></div>
      </div>
      <p v-if="previewPending" class="text-xs text-gray-500" role="status">{{ label('codexProfilePending') }}</p>
      <p v-else-if="previewError" class="text-xs text-red-600" role="alert">{{ label('codexPreviewUnavailable') }}</p>
      <p v-else-if="bulk && tlsEnabled == null" class="text-xs text-amber-700 dark:text-amber-400">{{ label('codexBulkPreviewPending') }}</p>
      <template v-else-if="preview">
        <p v-for="error in preview.conflicts" :key="error" class="mb-2 break-words text-xs text-red-600" role="alert">{{ error }}</p>
        <dl v-if="preview.transport" class="space-y-2 text-xs">
          <div class="flex flex-wrap justify-between gap-2"><dt class="text-gray-500">{{ label('codexEffectiveSender') }}</dt><dd class="break-all">{{ preview.transport.sender }}</dd></div>
          <div class="flex flex-wrap justify-between gap-2"><dt class="text-gray-500">TLS / HTTP2</dt><dd class="break-all">{{ preview.transport.tls_recipe }} / {{ preview.transport.http2_recipe }}</dd></div>
        </dl>
        <p v-if="preview.plugin_status === 'unknown'" class="mt-2 text-xs text-amber-700 dark:text-amber-400">{{ label('codexPluginPreviewPending') }}</p>
      </template>
      <p class="mt-3 text-xs text-gray-500">{{ label('codexExistingPinsRetained') }}</p>
    </div>
  </section>
</template>
