<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import {
  CODEX_CLIENT_PROFILES,
  type CodexRelayFormState,
  type CodexClientProfile,
  type CodexIdentityPolicyVersion,
  type CodexRelayMode,
  type CodexFingerprintMode
} from './codexRelaySchema'

const props = defineProps<{
  modelValue: CodexRelayFormState
  errors?: Record<string, string>
  disabled?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: CodexRelayFormState): void
}>()

const { t } = useI18n()

const relayMode = computed<CodexRelayMode>({
  get: () => props.modelValue.codex_relay_mode,
  set: (val) => {
    const next: CodexRelayFormState = {
      ...props.modelValue,
      codex_relay_mode: val
    }
    // 当切换为 relay_kernel 时，强制升级联动：
    // 1. codex_identity_policy_version 强制为 v2
    // 2. 如果指纹为 off，自动提升为推荐的 device
    if (val === 'relay_kernel') {
      next.codex_identity_policy_version = 'v2'
      if (next.codex_fingerprint_mode === 'off') {
        next.codex_fingerprint_mode = 'device'
      }
    }
    if (val === 'legacy') next.codex_installation_policy = 'legacy_v2'
    emit('update:modelValue', next)
  }
})

const isKernelActive = computed(() => relayMode.value === 'relay_kernel')

const installationPolicy = computed<'legacy_v2' | 'stable_v1'>({
  get: () => props.modelValue.codex_installation_policy ?? 'legacy_v2',
  set: (value) => emit('update:modelValue', { ...props.modelValue, codex_installation_policy: value })
})

const installationPolicyOptions = computed(() => [
  { value: 'legacy_v2', label: t('admin.accounts.openai.codexInstallationLegacy') },
  { value: 'stable_v1', label: t('admin.accounts.openai.codexInstallationStable') }
])

const identityPolicy = computed<CodexIdentityPolicyVersion>({
  get: () => props.modelValue.codex_identity_policy_version,
  set: (val) => {
    emit('update:modelValue', {
      ...props.modelValue,
      codex_identity_policy_version: val
    })
  }
})

const clientProfile = computed<CodexClientProfile>({
  get: () => props.modelValue.codex_client_profile,
  set: (val) => {
    emit('update:modelValue', {
      ...props.modelValue,
      codex_client_profile: val
    })
  }
})

const shadowEnabled = computed<boolean>({
  get: () => props.modelValue.codex_relay_shadow_enabled,
  set: (val) => {
    emit('update:modelValue', {
      ...props.modelValue,
      codex_relay_shadow_enabled: val
    })
  }
})

const fingerprintMode = computed<CodexFingerprintMode>({
  get: () => props.modelValue.codex_fingerprint_mode,
  set: (val) => {
    emit('update:modelValue', {
      ...props.modelValue,
      codex_fingerprint_mode: val
    })
  }
})

// 下拉选单
const relayModeOptions = computed(() => [
  { value: 'legacy', label: t('admin.accounts.openai.codexRelayModeLegacy') },
  { value: 'relay_kernel', label: t('admin.accounts.openai.codexRelayModeKernel') }
])

const identityPolicyOptions = computed(() => [
  { value: 'v1', label: t('admin.accounts.openai.codexIdentityPolicyV1') },
  { value: 'v2', label: t('admin.accounts.openai.codexIdentityPolicyV2') }
])

const clientProfileOptions = computed(() => [
  { value: 'pi-0.57.1-oauth-sse-r1', label: 'Pi 0.57.1 (HTTP/SSE bundle)', disabled: !isKernelActive.value },
  { value: 'opencode-1.2.4-oauth-sse-r1', label: 'OpenCode 1.2.4 (HTTP/SSE bundle)', disabled: !isKernelActive.value },
  { value: 'auto', label: t('admin.accounts.openai.codexClientProfileAuto') },
  { value: 'passthrough', label: t('admin.accounts.openai.codexClientProfilePassthrough') },
  { value: 'codex_cli', label: t('admin.accounts.openai.codexClientProfileCodexCli') },
  { value: 'codex_exec', label: t('admin.accounts.openai.codexClientProfileCodexExec') },
  { value: 'codex_desktop', label: t('admin.accounts.openai.codexClientProfileCodexDesktop') },
  { value: 'opencode', label: t('admin.accounts.openai.codexClientProfileOpencode') },
  { value: 'pi', label: t('admin.accounts.openai.codexClientProfilePi') }
])

const codexFingerprintModeOptions = computed(() => [
  { value: 'off', label: t('admin.accounts.openai.codexFingerprintOff'), disabled: isKernelActive.value },
  { value: 'device', label: t('admin.accounts.openai.codexFingerprintDevice') },
  { value: 'session', label: t('admin.accounts.openai.codexFingerprintSession') },
  { value: 'window', label: t('admin.accounts.openai.codexFingerprintWindow') },
  { value: 'window40', label: t('admin.accounts.openai.codexFingerprintWindow40') },
  { value: 'full', label: t('admin.accounts.openai.codexFingerprintFull') }
])

// 当前选中的 Client Profile 元数据
const currentProfileMeta = computed(() => {
  return (
    CODEX_CLIENT_PROFILES.find((p) => p.id === clientProfile.value) ||
    CODEX_CLIENT_PROFILES[0]
  )
})

const profileVersionLabel = computed(() => {
  const version = currentProfileMeta.value.appVersion
  if (version === 'dynamic') return t('admin.accounts.openai.codexProfilePending')
  if (version === 'caller supplied') return t('admin.accounts.openai.codexProfileCallerSupplied')
  if (version === 'not asserted') return t('admin.accounts.openai.codexProfileUnverified')
  return version
})

const profileFidelityLabel = computed(() => {
  const keys = {
    'caller-resolved': 'codexProfilePending',
    'passthrough/degraded': 'codexProfileCallerSupplied',
    degraded: 'codexProfileDegraded',
    'unsupported strict parity': 'codexProfileUnverified'
  } as const
  return t(`admin.accounts.openai.${keys[currentProfileMeta.value.fidelity]}`)
})

const profileCapabilities = computed(() => [
  { name: 'HTTP', supported: currentProfileMeta.value.http },
  { name: 'WS', supported: currentProfileMeta.value.ws },
  { name: 'Compact', supported: currentProfileMeta.value.compact }
])

function capabilityLabel(supported: boolean | null): string {
  if (supported === null) return t('admin.accounts.openai.codexProfilePending')
  return t(`admin.accounts.openai.${supported ? 'codexProfileSupported' : 'codexProfileUnsupported'}`)
}
</script>

<template>
  <div class="col-span-full border-t border-dashed border-gray-200 dark:border-dark-600 pt-4 mt-2">
    <!-- Header with optional Active Badge -->
    <div class="flex items-center justify-between mb-3">
      <div class="flex min-w-0 flex-wrap items-center gap-2">
        <h4 class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.openai.codexRelayKernelSectionTitle') }}
        </h4>
        <span
          v-if="isKernelActive"
          class="inline-flex shrink-0 whitespace-nowrap items-center px-2 py-0.5 rounded text-xs font-semibold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20"
        >
          {{ t('admin.accounts.openai.codexRelayKernelBadge') }}
        </span>
      </div>
    </div>
    <p class="text-xs text-gray-500 dark:text-gray-400 mb-4 leading-relaxed">
      {{ t('admin.accounts.openai.codexRelayKernelSectionDesc') }}
    </p>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <!-- 1. Relay Mode -->
      <div>
        <label class="input-label mb-1">
          {{ t('admin.accounts.openai.codexRelayMode') }}
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5 leading-relaxed">
          {{ t('admin.accounts.openai.codexRelayModeDesc') }}
        </p>
        <Select
          v-model="relayMode"
          :options="relayModeOptions"
          :disabled="disabled"
          data-testid="codex-relay-mode-select"
        />
        <p v-if="errors?.codex_relay_mode" class="mt-1 text-xs text-red-600 dark:text-red-400">
          {{ errors.codex_relay_mode }}
        </p>
      </div>

      <!-- 2. Identity Policy Version -->
      <div>
        <div class="flex items-center justify-between mb-1">
          <label class="input-label mb-0">
            {{ t('admin.accounts.openai.codexIdentityPolicyVersion') }}
          </label>
        </div>
        <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5 leading-relaxed">
          {{ t('admin.accounts.openai.codexIdentityPolicyVersionDesc') }}
        </p>
        <Select
          v-model="identityPolicy"
          :options="identityPolicyOptions"
          :disabled="disabled || isKernelActive"
          data-testid="codex-identity-policy-select"
        />
        <p v-if="errors?.codex_identity_policy_version" class="mt-1 text-xs text-red-600 dark:text-red-400">
          {{ errors.codex_identity_policy_version }}
        </p>
      </div>

      <!-- 3. Client Profile (with capabilities preview) -->
      <div>
        <label class="input-label mb-1">
          {{ t('admin.accounts.openai.codexClientProfile') }}
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5 leading-relaxed">
          {{ t('admin.accounts.openai.codexClientProfileDesc') }}
        </p>
        <Select
          v-model="clientProfile"
          :options="clientProfileOptions"
          :disabled="disabled"
          data-testid="codex-client-profile-select"
        />

        <dl class="mt-3 space-y-2 text-xs" data-testid="codex-profile-summary">
          <div class="flex flex-wrap justify-between gap-x-3 gap-y-1">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.codexProfileAppVersion') }}</dt>
            <dd class="min-w-0 break-words text-gray-900 dark:text-white" data-testid="codex-profile-version">{{ profileVersionLabel }}</dd>
          </div>
          <div class="flex flex-wrap justify-between gap-x-3 gap-y-1">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.codexProfileFidelity') }}</dt>
            <dd class="min-w-0 break-words text-gray-700 dark:text-gray-300" data-testid="codex-profile-fidelity">{{ profileFidelityLabel }}</dd>
          </div>
          <div
            v-for="capability in profileCapabilities"
            :key="capability.name"
            class="flex flex-wrap justify-between gap-x-3 gap-y-1"
            :data-testid="`codex-profile-capability-${capability.name.toLowerCase()}`"
          >
            <dt class="font-mono text-gray-500 dark:text-gray-400">{{ capability.name }}</dt>
            <dd
              :data-supported="capability.supported === null ? 'pending' : String(capability.supported)"
              :class="capability.supported === true ? 'text-emerald-700 dark:text-emerald-400' : 'text-gray-600 dark:text-gray-400'"
            >{{ capabilityLabel(capability.supported) }}</dd>
          </div>
        </dl>
      </div>

      <!-- 4. Fingerprint Mode -->
      <div>
        <label class="input-label mb-1">
          {{ t('admin.accounts.openai.codexFingerprintMode') }}
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5 leading-relaxed">
          {{ t(isKernelActive ? 'admin.accounts.openai.codexKernelFingerprintDesc' : 'admin.accounts.openai.codexFingerprintModeDesc') }}
        </p>
        <Select
          v-model="fingerprintMode"
          :options="codexFingerprintModeOptions"
          :disabled="disabled"
          data-testid="codex-fingerprint-mode-select"
        />
        <p v-if="errors?.codex_fingerprint_mode" class="mt-1 text-xs text-red-600 dark:text-red-400">
          {{ errors.codex_fingerprint_mode }}
        </p>
      </div>

      <div>
        <label class="input-label mb-1">{{ t('admin.accounts.openai.codexInstallationPolicy') }}</label>
        <Select
          v-model="installationPolicy"
          :options="installationPolicyOptions"
          :disabled="disabled || !isKernelActive"
          data-testid="codex-installation-policy-select"
        />
        <p v-if="errors?.codex_installation_policy" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ errors.codex_installation_policy }}</p>
      </div>
      <!-- 5. Shadow Comparison Switch -->
      <div class="col-span-full pt-1">
        <div class="flex items-center justify-between">
          <div class="max-w-[80%]">
            <label class="input-label mb-0 cursor-pointer">
              {{ t('admin.accounts.openai.codexRelayShadowEnabled') }}
            </label>
            <p class="text-xs text-gray-500 dark:text-gray-400 leading-relaxed">
              {{ t('admin.accounts.openai.codexRelayShadowEnabledDesc') }}
            </p>
          </div>
          <Toggle
            v-model="shadowEnabled"
            :disabled="disabled"
            data-testid="codex-relay-shadow-switch"
          />
        </div>
      </div>
    </div>
  </div>
</template>
