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
    emit('update:modelValue', next)
  }
})

const isKernelActive = computed(() => relayMode.value === 'relay_kernel')

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
</script>

<template>
  <div class="col-span-full border-t border-dashed border-gray-200 dark:border-dark-600 pt-4 mt-2">
    <!-- Header with optional Active Badge -->
    <div class="flex items-center justify-between mb-3">
      <div class="flex items-center gap-2">
        <h4 class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.openai.codexRelayKernelSectionTitle') }}
        </h4>
        <span
          v-if="isKernelActive"
          class="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20"
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

        <!-- Readonly Profile Capabilities Info -->
        <div class="mt-2 p-2 rounded bg-gray-50 dark:bg-dark-700/60 border border-gray-200 dark:border-dark-600 text-xs flex flex-wrap items-center gap-x-3 gap-y-1 text-gray-600 dark:text-gray-300">
          <div>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.codexProfileAppVersion') }}:</span>
            <span class="ml-1 font-mono text-gray-900 dark:text-white">{{ currentProfileMeta.appVersion }}</span>
          </div>
          <div class="flex items-center gap-1.5">
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.codexProfileCapabilities') }}:</span>
            <span
              class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono"
              :class="currentProfileMeta.http ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-zinc-500/10 text-zinc-500'"
            >
              HTTP
            </span>
            <span
              class="inline-flex items-center px-1.5 py-0.5 rounded text-[10px] font-mono"
              :class="currentProfileMeta.ws ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400' : 'bg-amber-500/10 text-amber-600 dark:text-amber-400'"
            >
              {{ currentProfileMeta.ws ? 'WS' : 'WS Degraded' }}
            </span>
          </div>
          <div>
            <span class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openai.codexProfileFidelity') }}:</span>
            <span class="ml-1 font-mono text-[11px] text-gray-900 dark:text-white">{{ currentProfileMeta.fidelity }}</span>
          </div>
        </div>
      </div>

      <!-- 4. Fingerprint Mode -->
      <div>
        <label class="input-label mb-1">
          {{ t('admin.accounts.openai.codexFingerprintMode') }}
        </label>
        <p class="text-xs text-gray-500 dark:text-gray-400 mb-1.5 leading-relaxed">
          {{ t('admin.accounts.openai.codexFingerprintModeDesc') }}
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
