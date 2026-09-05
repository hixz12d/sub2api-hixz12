import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexRelaySettings from '../CodexRelaySettings.vue'
import Select from '@/components/common/Select.vue'
import {
  createDefaultCodexRelaySettings,
  extractCodexRelayState,
  serializeCodexRelayToExtra,
  serializeCodexRelayToBulkExtra,
  validateCodexRelayState
} from '../codexRelaySchema'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Codex installation migration policy', () => {
  it('defaults old accounts to legacy without opting them into migration', () => {
    expect(extractCodexRelayState({}).codex_installation_policy).toBe('legacy_v2')
    const state = createDefaultCodexRelaySettings()
    const extra: Record<string, unknown> = {}
    serializeCodexRelayToExtra(state, extra)
    expect(extra).not.toHaveProperty('codex_installation_policy')
  })

  it('round-trips explicit stable policy and resets it in both save paths', () => {
    const state = extractCodexRelayState({ codex_relay_mode: 'relay_kernel', codex_identity_policy_version: 'v2', codex_fingerprint_mode: 'device', codex_installation_policy: 'stable_v1' })
    const extra: Record<string, unknown> = { unrelated: 'keep' }
    expect(validateCodexRelayState(state, (key) => key).valid).toBe(true)
    serializeCodexRelayToExtra(state, extra)
    expect(extra.codex_installation_policy).toBe('stable_v1')
    serializeCodexRelayToExtra(createDefaultCodexRelaySettings(), extra)
    expect(extra).not.toHaveProperty('codex_installation_policy')
    serializeCodexRelayToBulkExtra(state, extra)
    expect(extra.codex_installation_policy).toBe('stable_v1')
    serializeCodexRelayToBulkExtra(createDefaultCodexRelaySettings(), extra)
    expect(extra.codex_installation_policy).toBe('legacy_v2')
    expect(extra.unrelated).toBe('keep')
  })

  it('requires the kernel for stable installation', () => {
    const state = createDefaultCodexRelaySettings()
    state.codex_installation_policy = 'stable_v1'
    expect(validateCodexRelayState(state, (key) => key).errors.codex_installation_policy).toBeDefined()
  })

  it('disables migration outside kernel mode and clears desired policy on rollback', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings() } })
    const installation = wrapper.findAllComponents(Select).find((select) => select.attributes('data-testid') === 'codex-installation-policy-select')!
    expect(installation.props('disabled')).toBe(true)
    await wrapper.setProps({ modelValue: { ...createDefaultCodexRelaySettings(), codex_relay_mode: 'relay_kernel', codex_installation_policy: 'stable_v1' } })
    expect(installation.props('disabled')).toBe(false)
    const relay = wrapper.findAllComponents(Select).find((select) => select.attributes('data-testid') === 'codex-relay-mode-select')!
    relay.vm.$emit('update:modelValue', 'legacy')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toMatchObject({ codex_relay_mode: 'legacy', codex_installation_policy: 'legacy_v2' })
  })
})
