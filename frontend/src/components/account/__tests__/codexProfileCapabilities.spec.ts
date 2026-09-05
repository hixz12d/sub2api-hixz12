import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexRelaySettings from '../CodexRelaySettings.vue'
import {
  CODEX_CLIENT_PROFILES,
  createDefaultCodexRelaySettings,
  extractCodexRelayState,
  type CodexClientProfile
} from '../codexRelaySchema'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

function renderProfile(profile: CodexClientProfile) {
  return mount(CodexRelaySettings, {
    props: {
      modelValue: { ...createDefaultCodexRelaySettings(), codex_client_profile: profile }
    }
  })
}

describe('Codex profile capability contract', () => {
  it.each(['pi', 'opencode'] as const)('%s does not advertise WS or Compact', (profile) => {
    const wrapper = renderProfile(profile)
    for (const capability of ['ws', 'compact']) {
      const row = wrapper.get(`[data-testid="codex-profile-capability-${capability}"]`)
      expect(row.get('dd').attributes('data-supported')).toBe('false')
      expect(row.text()).toContain('codexProfileUnsupported')
    }
    expect(wrapper.text()).not.toContain('WS Degraded')
  })

  it('does not advertise unresolved auto capabilities', () => {
    const wrapper = renderProfile('auto')
    for (const capability of ['ws', 'compact']) {
      expect(wrapper.get(`[data-testid="codex-profile-capability-${capability}"] dd`).attributes('data-supported')).toBe('pending')
    }
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toContain('codexProfilePending')
  })

  it('retains the current backend catalog version instead of substituting a live capture', () => {
    const wrapper = renderProfile('codex_exec')
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toBe('0.148.0')
    expect(wrapper.get('[data-testid="codex-profile-capability-ws"] dd').attributes('data-supported')).toBe('true')
  })

  it('labels Pi as unverified without inventing its installed version', () => {
    const wrapper = renderProfile('pi')
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toContain('codexProfileUnverified')
    expect(wrapper.get('[data-testid="codex-profile-fidelity"]').text()).toContain('codexProfileUnverified')
    expect(CODEX_CLIENT_PROFILES.find((profile) => profile.id === 'pi')?.fidelity).toBe('unsupported strict parity')
  })

  it('updates the summary without mutating account state or activating a bundle', async () => {
    const wrapper = renderProfile('codex_cli')
    const next = { ...createDefaultCodexRelaySettings(), codex_client_profile: 'opencode' as const }
    await wrapper.setProps({ modelValue: next })
    expect(wrapper.get('[data-testid="codex-profile-capability-ws"] dd').attributes('data-supported')).toBe('false')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(next).not.toHaveProperty('codex_profile_bundle_id')
  })

  it.each(['false', 'true', 1, {}, null])('does not enable shadow for malformed value %j', (value) => {
    expect(extractCodexRelayState({ codex_relay_shadow_enabled: value }).codex_relay_shadow_enabled).toBe(false)
  })

  it('enables shadow only for an actual boolean true', () => {
    expect(extractCodexRelayState({ codex_relay_shadow_enabled: true }).codex_relay_shadow_enabled).toBe(true)
  })
})
