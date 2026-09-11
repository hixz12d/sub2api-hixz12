import { describe, expect, it } from 'vitest'
import {
  CODEX_CLIENT_PROFILES,
  patchCodexRelayState,
  createDefaultCodexRelaySettings,
  extractCodexRelaySettingsFromExtra,
  serializeCodexRelaySettingsToExtra,
  serializeCodexRelayToBulkExtra,
  mapCodexRelayApiError,
  validateCodexRelayState,
  type CodexRelayFormState
} from '../codexRelaySchema'

describe('codexRelaySchema', () => {
  const dummyT = (key: string) => key

  it('serializes managed presets without requiring manual TLS or identity choices', () => {
    const state = createDefaultCodexRelaySettings('pi')
    expect(validateCodexRelayState(state, dummyT, { tlsEnabled: null, bulk: true }).valid).toBe(true)
    const extra: Record<string, unknown> = {}
    serializeCodexRelaySettingsToExtra(state, extra)
    expect(extra.codex_client_preset).toBe('pi')
    serializeCodexRelayToBulkExtra({ ...state, codex_client_preset: '' }, extra)
    expect(extra.codex_client_preset).toBe('')
  })

  it('preserves a stored preset and rejects unknown presets', () => {
    const original = { codex_client_preset: 'opencode' }
    const extra = { ...original }
    serializeCodexRelaySettingsToExtra(extractCodexRelaySettingsFromExtra(original), extra)
    expect(extra).toEqual(original)
    expect(validateCodexRelayState(extractCodexRelaySettingsFromExtra({ codex_client_preset: 'unknown' }), dummyT).valid).toBe(false)
  })

  it.each([{}, { codex_client_profile: 'codex_exec', codex_fingerprint_mode: 'window40' }, { codex_client_profile: 'future-client', codex_relay_shadow_enabled: 'false' }])('round-trips persisted public values without activating defaults: %j', (original) => {
    const state = extractCodexRelaySettingsFromExtra(original)
    const extra = { ...original, unrelated: 'preserved' }
    serializeCodexRelaySettingsToExtra(state, extra)
    expect(extra).toEqual({ ...original, unrelated: 'preserved' })
    expect(extra).not.toHaveProperty('_persisted')
  })

  it('persists an explicit choice while leaving other missing legacy fields untouched', () => {
    const state = patchCodexRelayState(extractCodexRelaySettingsFromExtra({}), { codex_client_profile: 'auto' })
    const extra = {}
    serializeCodexRelaySettingsToExtra(state, extra)
    expect(extra).toEqual({ codex_client_profile: 'auto' })
  })

  it('requires an explicit native TLS choice for shared bundles, including bulk edits', () => {
    const state: CodexRelayFormState = { ...createDefaultCodexRelaySettings(), codex_client_profile: 'pi-0.57.1-oauth-sse-r1', codex_relay_mode: 'relay_kernel', codex_identity_policy_version: 'v2', codex_fingerprint_mode: 'device' }
    expect(validateCodexRelayState(state, dummyT, { tlsEnabled: true }).valid).toBe(false)
    expect(validateCodexRelayState(state, dummyT, { tlsEnabled: null, bulk: true }).valid).toBe(false)
    expect(validateCodexRelayState(state, dummyT, { tlsEnabled: false, bulk: true }).valid).toBe(true)
  })

  it('creates default settings with expected defaults', () => {
    const defaults = createDefaultCodexRelaySettings()
    expect(defaults.codex_relay_mode).toBe('legacy')
    expect(defaults.codex_identity_policy_version).toBe('v1')
    expect(defaults.codex_client_profile).toBe('auto')
    expect(defaults.codex_relay_shadow_enabled).toBe(false)
    expect(defaults.codex_fingerprint_mode).toBe('off')
  })

  it('validates codex relay state correctly', () => {
    const state = createDefaultCodexRelaySettings()
    const validRes = validateCodexRelayState(state, dummyT)
    expect(validRes.valid).toBe(true)
    expect(validRes.errors).toEqual({})

    // relay_kernel 必须配套 identity policy v2 且 fingerprint 不能为 off
    const invalidState: CodexRelayFormState = {
      codex_relay_mode: 'relay_kernel',
      codex_identity_policy_version: 'v1',
      codex_client_profile: 'auto',
      codex_relay_shadow_enabled: false,
      codex_fingerprint_mode: 'off'
    }
    const invalidRes = validateCodexRelayState(invalidState, dummyT)
    expect(invalidRes.valid).toBe(false)
    expect(invalidRes.errors.codex_identity_policy_version).toBeDefined()
    expect(invalidRes.errors.codex_fingerprint_mode).toBeDefined()
  })

  it('extracts settings from extra JSON cleanly with defaults fallback', () => {
    const emptyExtra = {}
    const fromEmpty = extractCodexRelaySettingsFromExtra(emptyExtra)
    expect(fromEmpty.codex_relay_mode).toBe('legacy')
    expect(fromEmpty.codex_client_profile).toBe('codex_cli')
    expect(fromEmpty.codex_fingerprint_mode).toBe('device')

    const customExtra = {
      codex_relay_mode: 'relay_kernel',
      codex_identity_policy_version: 'v2',
      codex_client_profile: 'codex_cli',
      codex_relay_shadow_enabled: true,
      codex_fingerprint_mode: 'session'
    }
    const fromCustom = extractCodexRelaySettingsFromExtra(customExtra)
    expect(fromCustom.codex_relay_mode).toBe('relay_kernel')
    expect(fromCustom.codex_identity_policy_version).toBe('v2')
    expect(fromCustom.codex_client_profile).toBe('codex_cli')
    expect(fromCustom.codex_relay_shadow_enabled).toBe(true)
    expect(fromCustom.codex_fingerprint_mode).toBe('session')
  })

  it('serializes only whitelist keys to extra and removes defaults cleanly', () => {
    const extra: Record<string, any> = {
      other_key: 'preserve_this',
      codex_relay_mode: 'relay_kernel',
      codex_client_profile: 'codex_exec',
      codex_fingerprint_mode: 'full'
    }

    const defaultState = createDefaultCodexRelaySettings()
    // 默认值会清理可省略键；auto 必须显式写入，以覆盖后端的 codex_cli 兼容默认值。
    serializeCodexRelaySettingsToExtra(defaultState, extra)

    expect(extra.other_key).toBe('preserve_this')
    expect(extra.codex_relay_mode).toBeUndefined()
    expect(extra.codex_client_profile).toBe('auto')
    expect(extra.codex_identity_policy_version).toBeUndefined()
    expect(extra.codex_relay_shadow_enabled).toBeUndefined()
    expect(extra.codex_fingerprint_mode).toBe('off')

    const customState: CodexRelayFormState = {
      codex_relay_mode: 'relay_kernel',
      codex_identity_policy_version: 'v2',
      codex_client_profile: 'pi',
      codex_relay_shadow_enabled: true,
      codex_fingerprint_mode: 'window'
    }
    serializeCodexRelaySettingsToExtra(customState, extra)

    expect(extra.codex_relay_mode).toBe('relay_kernel')
    expect(extra.codex_identity_policy_version).toBe('v2')
    expect(extra.codex_client_profile).toBe('pi')
    expect(extra.codex_relay_shadow_enabled).toBe(true)
    expect(extra.codex_fingerprint_mode).toBe('window')
  })

  it('serializes bulk extra with explicit default sentinels', () => {
    const extra: Record<string, any> = {
      other_key: 'preserve_this',
      codex_relay_mode: 'relay_kernel',
      codex_identity_policy_version: 'v2',
      codex_client_profile: 'codex_exec',
      codex_relay_shadow_enabled: true,
      codex_fingerprint_mode: 'full'
    }

    serializeCodexRelayToBulkExtra(createDefaultCodexRelaySettings(), extra)

    expect(extra.other_key).toBe('preserve_this')
    expect(extra.codex_relay_mode).toBe('legacy')
    expect(extra.codex_identity_policy_version).toBe('v1')
    expect(extra.codex_client_profile).toBe('auto')
    expect(extra.codex_relay_shadow_enabled).toBe(false)
    expect(extra.codex_fingerprint_mode).toBe('off')
  })

  it('contains expected catalog items in CODEX_CLIENT_PROFILES', () => {
    const ids = CODEX_CLIENT_PROFILES.map((p) => p.id)
    expect(ids).toContain('auto')
    expect(ids).toContain('passthrough')
    expect(ids).toContain('codex_cli')
    expect(ids).toContain('codex_exec')
    expect(ids).toContain('codex_desktop')
    expect(ids).toContain('opencode')
    expect(ids).toContain('pi')
  })

  it('maps relay secret API errors to the operator-facing warning', () => {
    expect(mapCodexRelayApiError({ reason: 'CODEX_RELAY_SECRET_INVALID' }, dummyT)).toBe('admin.accounts.openai.codexRelaySecretMissing')
    expect(mapCodexRelayApiError({ response: { data: { reason: 'OPENAI_CODEX_RELAY_SECRET_MISSING' } } }, dummyT)).toBe('admin.accounts.openai.codexRelaySecretMissing')
    expect(mapCodexRelayApiError({ message: 'unrelated' }, dummyT)).toBeNull()
  })
})
