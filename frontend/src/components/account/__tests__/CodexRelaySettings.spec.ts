import { afterEach, describe, it, expect, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import CodexRelaySettings from '../CodexRelaySettings.vue'
import { createDefaultCodexRelaySettings, type CodexRelayFormState } from '../codexRelaySchema'
import { previewClientProfile, type ClientProfilePreview } from '@/api/admin/clientProfiles'

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/api/admin/clientProfiles', async () => {
  const { clientProfileCatalogFixture } = await import('./clientProfileFixture')
  return { getClientProfiles: vi.fn().mockResolvedValue(clientProfileCatalogFixture), previewClientProfile: vi.fn().mockResolvedValue({ valid: true, conflicts: [], plugin_status: 'unknown' }) }
})
afterEach(() => { vi.useRealTimers(); vi.clearAllMocks() })
const managedBundle = (): CodexRelayFormState => ({ ...createDefaultCodexRelaySettings(), codex_relay_mode: 'relay_kernel', codex_identity_policy_version: 'v2', codex_fingerprint_mode: 'device', codex_client_profile: 'pi-0.57.1-oauth-sse-r1' })

describe('CodexRelaySettings.vue', () => {
  it('renders exactly three client families and keeps compatibility controls in advanced settings', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings() } })
    await flushPromises()
    const options = wrapper.findComponent('[data-testid="codex-client-family-select"]').props('options')
    expect(options.map((item: { label: string }) => item.label)).toEqual(['Codex', 'OpenCode', 'Pi'])
    for (const id of ['codex-relay-mode-select', 'codex-identity-policy-select', 'codex-client-profile-select', 'codex-relay-shadow-switch']) {
      expect(wrapper.get('[data-testid="codex-advanced"]').find(`[data-testid="${id}"]`).exists()).toBe(true)
    }
    wrapper.unmount()
  })

  it('forces v2 and a managed identity scope when relay kernel is selected', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings() } })
    wrapper.findComponent('[data-testid="codex-relay-mode-select"]').vm.$emit('update:modelValue', 'relay_kernel')
    await wrapper.vm.$nextTick()
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toMatchObject({ codex_relay_mode: 'relay_kernel', codex_identity_policy_version: 'v2', codex_fingerprint_mode: 'device' })
    wrapper.unmount()
  })

  it('keeps an existing Exec variant when its Codex family is unchanged', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: { ...createDefaultCodexRelaySettings(), codex_client_profile: 'codex_exec' } } })
    await flushPromises()
    wrapper.findComponent('[data-testid="codex-client-family-select"]').vm.$emit('update:modelValue', 'codex')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    wrapper.unmount()
  })

  it('reports r1 TLS conflicts without silently changing either setting', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: managedBundle(), tlsEnabled: true } })
    await flushPromises()
    expect(wrapper.text()).toContain('codexBundleTLSConflict')
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
    await wrapper.setProps({ tlsEnabled: false })
    expect(wrapper.text()).not.toContain('codexBundleTLSConflict')
    wrapper.unmount()
  })

  it('does not invent a bulk TLS default or send a misleading preview', async () => {
    vi.useFakeTimers()
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: managedBundle(), bulk: true, tlsEnabled: null } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(300)
    expect(wrapper.text()).toContain('codexBundleBulkTLSRequired')
    expect(previewClientProfile).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('discards a late preview for superseded settings', async () => {
    vi.useFakeTimers()
    let resolveOld!: (value: ClientProfilePreview) => void
    vi.mocked(previewClientProfile).mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve }))
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: managedBundle(), tlsEnabled: false } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(250)
    await wrapper.setProps({ tlsEnabled: true })
    await vi.advanceTimersByTimeAsync(250)
    resolveOld({ valid: false, conflicts: ['stale-conflict-must-not-render'], scope: '', requirements: [], session_effect: '', plugin_status: 'unknown' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale-conflict-must-not-render')
    expect(wrapper.text()).toContain('codexBundleTLSConflict')
    wrapper.unmount()
  })
})
