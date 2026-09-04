import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import CodexRelaySettings from '../CodexRelaySettings.vue'
import { createDefaultCodexRelaySettings } from '../codexRelaySchema'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

describe('CodexRelaySettings.vue', () => {
  it('renders correctly with default values', () => {
    const modelValue = createDefaultCodexRelaySettings()
    const wrapper = mount(CodexRelaySettings, {
      props: {
        modelValue,
      },
    })

    expect(wrapper.find('[data-testid="codex-relay-mode-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-identity-policy-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-client-profile-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="codex-relay-shadow-switch"]').exists()).toBe(true)
  })

  it('forces v2 and a managed identity scope when relay kernel is selected', async () => {
    const modelValue = createDefaultCodexRelaySettings()
    const wrapper = mount(CodexRelaySettings, {
      props: {
        modelValue,
      },
    })

    const relayModeSelect = wrapper.findComponent({ name: 'Select' })
    relayModeSelect.vm.$emit('update:modelValue', 'relay_kernel')
    await wrapper.vm.$nextTick()

    const updates = wrapper.emitted('update:modelValue')
    expect(updates).toHaveLength(1)
    expect(updates?.[0]?.[0]).toMatchObject({
      codex_relay_mode: 'relay_kernel',
      codex_identity_policy_version: 'v2',
      codex_fingerprint_mode: 'device',
    })
  })
})
