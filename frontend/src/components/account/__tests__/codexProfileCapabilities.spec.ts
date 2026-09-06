import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexRelaySettings from '../CodexRelaySettings.vue'
import { createDefaultCodexRelaySettings, extractCodexRelayState, type CodexClientProfile } from '../codexRelaySchema'
import { getClientProfiles } from '@/api/admin/clientProfiles'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/clientProfiles', async () => {
  const { clientProfileCatalogFixture } = await import('./clientProfileFixture')
  return { getClientProfiles: vi.fn().mockResolvedValue(clientProfileCatalogFixture), previewClientProfile: vi.fn().mockResolvedValue({ valid: true, conflicts: [], plugin_status: 'unknown' }) }
})

async function renderProfile(profile: CodexClientProfile) {
  const wrapper = mount(CodexRelaySettings, { props: { modelValue: { ...createDefaultCodexRelaySettings(), codex_client_profile: profile } } })
  await flushPromises()
  return wrapper
}

describe('Codex profile capability contract', () => {
  it.each(['pi', 'opencode'] as const)('%s does not advertise WS or Compact', async (profile) => {
    const wrapper = await renderProfile(profile)
    for (const capability of ['ws', 'compact']) {
      const row = wrapper.get(`[data-testid="codex-profile-capability-${capability}"]`)
      expect(row.get('dd').attributes('data-supported')).toBe('false')
      expect(row.text()).toContain('codexProfileUnsupported')
    }
    wrapper.unmount()
  })

  it('does not advertise unresolved auto capabilities', async () => {
    const wrapper = await renderProfile('auto')
    for (const capability of ['http', 'ws', 'compact']) {
      expect(wrapper.get(`[data-testid="codex-profile-capability-${capability}"] dd`).attributes('data-supported')).toBe('pending')
    }
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toContain('codexProfilePending')
    wrapper.unmount()
  })

  it('displays the server catalog version without a frontend version table', async () => {
    const wrapper = await renderProfile('codex_exec')
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toBe('remote-test-version')
    expect(wrapper.get('[data-testid="codex-profile-capability-ws"] dd').attributes('data-supported')).toBe('true')
    wrapper.unmount()
  })

  it('labels legacy Pi as unverified without inventing its installed version', async () => {
    const wrapper = await renderProfile('pi')
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toContain('codexProfileUnverified')
    expect(wrapper.get('[data-testid="codex-profile-fidelity"]').text()).toContain('codexProfileUnverified')
    wrapper.unmount()
  })

  it('updates the summary without mutating account state or activating a bundle', async () => {
    const wrapper = await renderProfile('codex_cli')
    const next = { ...createDefaultCodexRelaySettings(), codex_client_profile: 'opencode' as const }
    await wrapper.setProps({ modelValue: next })
    expect(wrapper.get('[data-testid="codex-profile-capability-ws"] dd').attributes('data-supported')).toBe('false')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    expect(next).not.toHaveProperty('codex_profile_bundle_id')
    wrapper.unmount()
  })

  it('fails visibly with unknown capabilities when the server catalog is unavailable', async () => {
    vi.mocked(getClientProfiles).mockRejectedValueOnce(new Error('offline'))
    const wrapper = await renderProfile('codex_cli')
    expect(wrapper.text()).toContain('codexCatalogUnavailable')
    expect(wrapper.get('[data-testid="codex-profile-capability-ws"] dd').attributes('data-supported')).toBe('pending')
    wrapper.unmount()
  })

  it.each(['false', 'true', 1, {}, null])('does not enable shadow for malformed value %j', (value) => {
    expect(extractCodexRelayState({ codex_relay_shadow_enabled: value }).codex_relay_shadow_enabled).toBe(false)
  })

  it('enables shadow only for an actual boolean true', () => {
    expect(extractCodexRelayState({ codex_relay_shadow_enabled: true }).codex_relay_shadow_enabled).toBe(true)
  })
})
