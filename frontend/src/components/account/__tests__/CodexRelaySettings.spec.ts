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
  it('offers only client presets in normal mode and hides diagnostic choices', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings('codex') } })
    await flushPromises()
    const selector = wrapper.findComponent('[data-testid="codex-client-preset-select"]')
    expect(selector.props('options').map((item: { label: string }) => item.label)).toEqual(['Codex', 'Pi', 'OpenCode'])
    expect(wrapper.find('[data-testid="codex-management-select"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="codex-effective-preview"]').element.tagName).toBe('DETAILS')
    expect(wrapper.get('[data-testid="codex-effective-preview"]').attributes('open')).toBeUndefined()
    selector.vm.$emit('update:modelValue', 'pi')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toMatchObject({ codex_client_preset: 'pi' })
    expect(wrapper.emitted('update:tlsEnabled')?.[0]).toEqual([false])
    wrapper.unmount()
  })

  it('previews a managed Pi preset in bulk without a separate TLS choice', async () => {
    vi.useFakeTimers()
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings('pi'), bulk: true, tlsEnabled: null } })
    await flushPromises()
    await vi.advanceTimersByTimeAsync(250)
    expect(previewClientProfile).toHaveBeenCalledWith(expect.objectContaining({ extra: expect.objectContaining({ codex_client_preset: 'pi', enable_tls_fingerprint: false }) }), expect.anything())
    expect(wrapper.text()).not.toContain('codexBundleBulkTLSRequired')
    wrapper.unmount()
  })

  it('shows a pending adaptation without claiming the latest release is active', async () => {
    const { clientProfileCatalogFixture } = await import('./clientProfileFixture')
    const { getClientProfiles } = await import('@/api/admin/clientProfiles')
    vi.mocked(getClientProfiles).mockResolvedValueOnce({
      ...clientProfileCatalogFixture,
      profiles: clientProfileCatalogFixture.profiles.map((item) => item.id === 'pi-managed'
        ? { ...item, update_status: 'needs_review' as const, latest_version: '0.99.0' } : item)
    })
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings('pi') } })
    await flushPromises()
    expect(wrapper.get('[data-testid="codex-update-status"]').text()).toContain('codexUpdateNeedsReview')
    expect(wrapper.get('[data-testid="codex-profile-version"]').text()).toBe('remote-test-version')
    wrapper.unmount()
  })

  it('copies the resolved tuple when leaving a preset for custom settings', async () => {
    const wrapper = mount(CodexRelaySettings, { props: { modelValue: createDefaultCodexRelaySettings('pi') } })
    await flushPromises()
    expect(wrapper.text()).toContain('codexPresetAutoVersion')
    expect(wrapper.get('[data-testid="codex-update-status"]').text()).toContain('codexUpdateCurrent')
    await wrapper.get('[data-testid="codex-custom-settings"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]?.[0]).toMatchObject({
      codex_client_preset: '', codex_client_profile: 'pi-managed',
      codex_relay_mode: 'relay_kernel', codex_identity_policy_version: 'v2', codex_fingerprint_mode: 'device'
    })
    expect(wrapper.emitted('update:tlsEnabled')?.[0]).toEqual([false])
    wrapper.unmount()
  })

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
