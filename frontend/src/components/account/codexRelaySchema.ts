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

export type CodexFingerprintMode = 'off' | 'device' | 'session' | 'window' | 'window40' | 'full'

export interface CodexRelayFormState {

  codex_relay_mode: CodexRelayMode
  codex_identity_policy_version: CodexIdentityPolicyVersion
  codex_client_profile: CodexClientProfile
  codex_relay_shadow_enabled: boolean
  codex_fingerprint_mode: CodexFingerprintMode
}

export type CodexRelaySettingsValue = CodexRelayFormState
export const createDefaultCodexRelaySettings = (): CodexRelayFormState => ({ ...DEFAULT_CODEX_RELAY_STATE })
export const extractCodexRelaySettingsFromExtra = extractCodexRelayState
export const serializeCodexRelaySettingsToExtra = serializeCodexRelayToExtra

export interface ClientProfileCatalogItem {
  id: CodexClientProfile
  appVersion: string
  http: boolean
  ws: boolean
  fidelity: 'passthrough/degraded' | 'degraded' | 'unsupported strict parity' | 'caller-resolved'
}

export const CODEX_CLIENT_PROFILES: readonly ClientProfileCatalogItem[] = [
  {
    id: 'auto',
    appVersion: 'dynamic',
    http: true,
    ws: true,
    fidelity: 'caller-resolved'
  },
  {
    id: 'passthrough',
    appVersion: 'caller supplied',
    http: true,
    ws: true,
    fidelity: 'passthrough/degraded'
  },
  {
    id: 'codex_cli',
    appVersion: '0.148.0',
    http: true,
    ws: true,
    fidelity: 'degraded'
  },
  {
    id: 'codex_exec',
    appVersion: '0.148.0',
    http: true,
    ws: true,
    fidelity: 'degraded'
  },
  {
    id: 'codex_desktop',
    appVersion: '0.148.0',
    http: true,
    ws: true,
    fidelity: 'degraded'
  },
  {
    id: 'opencode',
    appVersion: '1.2.4',
    http: true,
    ws: false,
    fidelity: 'degraded'
  },
  {
    id: 'pi',
    appVersion: 'not asserted',
    http: true,
    ws: false,
    fidelity: 'degraded'
  }
] as const

export const DEFAULT_CODEX_RELAY_STATE: CodexRelayFormState = {
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
  t: (key: string) => string
): CodexRelayValidationResult {
  const errors: Record<string, string> = {}

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
  if (!extra) {
    return { ...DEFAULT_CODEX_RELAY_STATE }
  }

  const mode = extra.codex_relay_mode === 'relay_kernel' ? 'relay_kernel' : 'legacy'
  const policy = extra.codex_identity_policy_version === 'v2' ? 'v2' : 'v1'
  const validProfiles: CodexClientProfile[] = [
    'auto', 'passthrough', 'codex_cli', 'codex_exec', 'codex_desktop', 'opencode', 'pi'
  ]
  const profile = validProfiles.includes(extra.codex_client_profile)
    ? (extra.codex_client_profile as CodexClientProfile)
    : 'auto'

  const shadow = Boolean(extra.codex_relay_shadow_enabled)

  const validFpModes: CodexFingerprintMode[] = ['off', 'device', 'session', 'window', 'window40', 'full']
  const fpMode = validFpModes.includes(extra.codex_fingerprint_mode)
    ? (extra.codex_fingerprint_mode as CodexFingerprintMode)
    : 'off'

  return {
    codex_relay_mode: mode,
    codex_identity_policy_version: policy,
    codex_client_profile: profile,
    codex_relay_shadow_enabled: shadow,
    codex_fingerprint_mode: fpMode
  }
}

/**
 * 白名单序列化，仅序列化合法公开键，杜绝污染敏感/私有运行时状态
 */
export function serializeCodexRelayToExtra(
  state: CodexRelayFormState,
  targetExtra: Record<string, any>
): void {
  // 1. codex_relay_mode
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
    delete targetExtra.codex_fingerprint_mode
  }
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
