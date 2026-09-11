export type CodexClientPreset = 'codex' | 'pi' | 'opencode'
export type CodexRelayMode = 'legacy' | 'relay_kernel'
export type CodexIdentityPolicyVersion = 'v1' | 'v2'
export type CodexClientProfile =
  | 'auto'
  | 'passthrough'
  | 'codex_cli'
  | 'codex_exec'
  | 'codex_desktop'
  | 'opencode'
  | 'pi'
  | 'pi-managed'
  | 'opencode-managed'
  | 'pi-0.57.1-oauth-sse-r1'
  | 'opencode-1.2.4-oauth-sse-r1'

export type CodexFingerprintMode = 'off' | 'device' | 'session' | 'window' | 'window40' | 'full'

export interface CodexRelayFormState {
  codex_client_preset?: CodexClientPreset | ''
  codex_installation_policy?: 'legacy_v2' | 'stable_v1'

  codex_relay_mode: CodexRelayMode
  codex_identity_policy_version: CodexIdentityPolicyVersion
  codex_client_profile: CodexClientProfile
  codex_relay_shadow_enabled: boolean
  codex_fingerprint_mode: CodexFingerprintMode
  _persisted?: {
    values: Partial<Record<CodexRelayExtraKey, unknown>>
    initial: Partial<Record<CodexRelayExtraKey, unknown>>
    edited: CodexRelayExtraKey[]
  }
}

export type CodexRelaySettingsValue = CodexRelayFormState
export const createDefaultCodexRelaySettings = (preset?: CodexClientPreset): CodexRelayFormState => ({
  ...DEFAULT_CODEX_RELAY_STATE,
  ...(preset ? { codex_client_preset: preset } : {})
})
export const extractCodexRelaySettingsFromExtra = extractCodexRelayState
export const serializeCodexRelaySettingsToExtra = serializeCodexRelayToExtra

export interface ClientProfileCatalogItem {
  version_policy?: 'auto' | 'hold'
  update_status?: 'pending' | 'current' | 'needs_review' | 'fetch_failed' | 'held' | 'storage_unavailable'
  latest_version?: string
  previous_version?: string
  last_checked?: string
  id: CodexClientProfile
  family: 'caller' | 'codex' | 'opencode' | 'pi'
  variant: string
  appVersion: string
  http: boolean | null
  ws: boolean | null
  compact: boolean | null
  fidelity: 'passthrough/degraded' | 'degraded' | 'unsupported strict parity' | 'caller-resolved'
  recipe: string
  digest: string
  relay_digest?: string
  native_validation: string
}

// Selector compatibility only. Versions and capabilities come from the server.
export const CODEX_CLIENT_PROFILES: readonly { id: CodexClientProfile }[] = [
  { id: 'auto' }, { id: 'passthrough' }, { id: 'codex_cli' },
  { id: 'codex_exec' }, { id: 'codex_desktop' }, { id: 'opencode' }, { id: 'pi' },
  { id: 'pi-0.57.1-oauth-sse-r1' }, { id: 'opencode-1.2.4-oauth-sse-r1' },
  { id: 'pi-managed' }, { id: 'opencode-managed' }
]

export const CODEX_RELAY_EXTRA_KEYS = [
  'codex_client_preset',
  'codex_relay_mode', 'codex_identity_policy_version', 'codex_client_profile',
  'codex_installation_policy', 'codex_relay_shadow_enabled', 'codex_fingerprint_mode'
] as const
export type CodexRelayExtraKey = typeof CODEX_RELAY_EXTRA_KEYS[number]

export function patchCodexRelayState(
  state: CodexRelayFormState,
  patch: Partial<Omit<CodexRelayFormState, '_persisted'>>
): CodexRelayFormState {
  const next = { ...state, ...patch }
  if (state._persisted) {
    next._persisted = {
      ...state._persisted,
      edited: [...new Set([...state._persisted.edited, ...Object.keys(patch) as CodexRelayExtraKey[]])]
    }
  }
  return next
}

export const DEFAULT_CODEX_RELAY_STATE: CodexRelayFormState = {
  codex_installation_policy: 'legacy_v2',
  codex_relay_mode: 'legacy',
  codex_identity_policy_version: 'v1',
  codex_client_profile: 'auto',
  codex_relay_shadow_enabled: false,
  codex_fingerprint_mode: 'off'
}

export interface CodexRelayValidationResult {
  valid: boolean
  errors: Record<string, string>
}

/**
 * 校验 Codex Relay 表单联动合法性:
 * 1. 当 codex_relay_mode === 'relay_kernel' 时，必须且强制 codex_identity_policy_version === 'v2'。
 * 2. 当 codex_relay_mode === 'relay_kernel' 时，codex_fingerprint_mode 不能为 'off'（必须托管指纹）。
 */
export function validateCodexRelayState(
  state: CodexRelayFormState,
  t: (key: string) => string,
  transport?: { tlsEnabled?: boolean | null; bulk?: boolean }
): CodexRelayValidationResult {
  const errors: Record<string, string> = {}
  if (state.codex_client_preset) {
    if (!['codex', 'pi', 'opencode'].includes(state.codex_client_preset)) {
      errors.codex_client_preset = t('admin.accounts.openai.codexProfileUnknown')
    }
    // The server resolves and validates the entire preset, including TLS.
    return { valid: Object.keys(errors).length === 0, errors }
  }
  if (!CODEX_CLIENT_PROFILES.some((item) => item.id === state.codex_client_profile)) {
    errors.codex_client_profile = t('admin.accounts.openai.codexProfileUnknown')
  }
  const nativeBundle = state.codex_client_profile.endsWith('-oauth-sse-r1') || ['pi-managed', 'opencode-managed'].includes(state.codex_client_profile)
  if (nativeBundle && transport && transport.tlsEnabled !== false) {
    errors.codex_transport = t(`admin.accounts.openai.${transport.bulk && transport.tlsEnabled == null ? 'codexBundleBulkTLSRequired' : 'codexBundleTLSConflict'}`)
  }
  if (nativeBundle && state.codex_relay_mode !== 'relay_kernel') {
    errors.codex_client_profile = t('admin.accounts.openai.codexBundleRequiresKernel')
  }
  if (state.codex_installation_policy === 'stable_v1' && state.codex_relay_mode !== 'relay_kernel') {
    errors.codex_installation_policy = t('admin.accounts.openai.codexInstallationRequiresKernel')
  }

  if (state.codex_relay_mode === 'relay_kernel') {
    if (state.codex_identity_policy_version !== 'v2') {
      errors.codex_identity_policy_version = t('admin.accounts.openai.codexRelayKernelRequiresV2')
    }
    if (state.codex_fingerprint_mode === 'off') {
      errors.codex_fingerprint_mode = t('admin.accounts.openai.codexRelayKernelRequiresManagedFingerprint')
    }
  }

  return {
    valid: Object.keys(errors).length === 0,
    errors
  }
}

/**
 * 从 extra 中提取并标准化 Codex Relay 状态
 */
export function extractCodexRelayState(extra?: Record<string, any> | null): CodexRelayFormState {
  const source = extra ?? {}
  const validFpModes: CodexFingerprintMode[] = ['off', 'device', 'session', 'window', 'window40', 'full']
  const state: CodexRelayFormState = {
    codex_client_preset: typeof source.codex_client_preset === 'string' ? source.codex_client_preset as CodexClientPreset : '',
    codex_relay_mode: source.codex_relay_mode === 'relay_kernel' ? 'relay_kernel' : 'legacy',
    codex_installation_policy: source.codex_installation_policy === 'stable_v1' ? 'stable_v1' : 'legacy_v2',
    codex_identity_policy_version: source.codex_identity_policy_version === 'v2' ? 'v2' : 'v1',
    codex_client_profile: typeof source.codex_client_profile === 'string' && source.codex_client_profile !== ''
      ? source.codex_client_profile as CodexClientProfile : 'codex_cli',
    codex_relay_shadow_enabled: source.codex_relay_shadow_enabled === true,
    codex_fingerprint_mode: validFpModes.includes(source.codex_fingerprint_mode)
      ? source.codex_fingerprint_mode : source.codex_fingerprint_mode == null ? 'device' : 'off'
  }
  const values: Partial<Record<CodexRelayExtraKey, unknown>> = {}
  const initial: Partial<Record<CodexRelayExtraKey, unknown>> = {}
  for (const key of CODEX_RELAY_EXTRA_KEYS) {
    if (Object.prototype.hasOwnProperty.call(source, key)) values[key] = source[key]
    initial[key] = state[key]
  }
  state._persisted = { values, initial, edited: [] }
  return state
}

/**
 * 白名单序列化，仅序列化合法公开键，杜绝污染敏感/私有运行时状态
 */
export function serializeCodexRelayToExtra(
  state: CodexRelayFormState,
  targetExtra: Record<string, any>
): void {
  if (state.codex_client_preset) {
    for (const key of CODEX_RELAY_EXTRA_KEYS) delete targetExtra[key]
    targetExtra.codex_client_preset = state.codex_client_preset
    return
  }
  delete targetExtra.codex_client_preset
  // 1. codex_relay_mode
  if (state.codex_installation_policy === 'stable_v1') {
    targetExtra.codex_installation_policy = 'stable_v1'
  } else {
    delete targetExtra.codex_installation_policy
  }
  if (state.codex_relay_mode === 'relay_kernel') {
    targetExtra.codex_relay_mode = 'relay_kernel'
  } else {
    delete targetExtra.codex_relay_mode
  }

  // 2. codex_identity_policy_version
  if (state.codex_identity_policy_version === 'v2') {
    targetExtra.codex_identity_policy_version = 'v2'
  } else {
    delete targetExtra.codex_identity_policy_version
  }

  // 3. codex_client_profile
  targetExtra.codex_client_profile = state.codex_client_profile

  // 4. codex_relay_shadow_enabled
  if (state.codex_relay_shadow_enabled) {
    targetExtra.codex_relay_shadow_enabled = true
  } else {
    delete targetExtra.codex_relay_shadow_enabled
  }

  // 5. codex_fingerprint_mode
  if (state.codex_fingerprint_mode !== 'off') {
    targetExtra.codex_fingerprint_mode = state.codex_fingerprint_mode
  } else {
    targetExtra.codex_fingerprint_mode = 'off'
  }
  if (state._persisted) {
    for (const key of CODEX_RELAY_EXTRA_KEYS) {
      if (state._persisted.edited.includes(key) || state[key] !== state._persisted.initial[key]) continue
      if (Object.prototype.hasOwnProperty.call(state._persisted.values, key)) {
        targetExtra[key] = state._persisted.values[key]
      } else {
        delete targetExtra[key]
      }
    }
  }
}

/**
 * 批量 JSONB 顶层合并不能靠删键恢复默认值，必须把每个白名单键都写成显式值。
 * 单账号 Create/Edit 仍走 serializeCodexRelayToExtra；批量路径专用本函数。
 */
export function serializeCodexRelayToBulkExtra(
  state: CodexRelayFormState,
  targetExtra: Record<string, any>
): void {
  if (state.codex_client_preset) {
    serializeCodexRelayToExtra(state, targetExtra)
    return
  }
  targetExtra.codex_client_preset = ''
  targetExtra.codex_relay_mode = state.codex_relay_mode
  targetExtra.codex_installation_policy = state.codex_installation_policy ?? 'legacy_v2'
  targetExtra.codex_identity_policy_version = state.codex_identity_policy_version
  targetExtra.codex_client_profile = state.codex_client_profile
  targetExtra.codex_relay_shadow_enabled = Boolean(state.codex_relay_shadow_enabled)
  targetExtra.codex_fingerprint_mode = state.codex_fingerprint_mode
}

export function mapCodexRelayApiError(
  error: {
    reason?: string
    code?: string | number
    message?: string
    response?: { data?: { reason?: string; code?: string | number; message?: string } }
  } | null | undefined,
  t: (key: string) => string
): string | null {
  const reason = String(error?.reason || error?.code || error?.response?.data?.reason || error?.response?.data?.code || '')
  if (reason === 'CODEX_RELAY_SECRET_INVALID' || reason === 'OPENAI_CODEX_RELAY_SECRET_MISSING') {
    return t('admin.accounts.openai.codexRelaySecretMissing')
  }
  return null
}
