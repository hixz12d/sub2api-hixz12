import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyAccountCreateTemplateValues } from '../accountCreateTemplate'

const {
  listAccountCreateTemplates,
  createAccountCreateTemplate,
  showSuccess,
} = vi.hoisted(() => ({
  listAccountCreateTemplates: vi.fn(),
  createAccountCreateTemplate: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess,
  }),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    settings: {
      listAccountCreateTemplates,
      createAccountCreateTemplate,
      updateAccountCreateTemplate: vi.fn(),
    },
  },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, string>) => params?.summary ?? params?.name ?? key }),
  }
})

import AccountCreateTemplateBar from '../AccountCreateTemplateBar.vue'

const BaseDialogStub = defineComponent({
  name: 'BaseDialog',
  props: { show: { type: Boolean, default: false } },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

const SelectStub = defineComponent({
  name: 'Select',
  props: { modelValue: { type: String, default: '' }, options: { type: Array, default: () => [] } },
  emits: ['update:modelValue'],
  template: '<select :value="modelValue" data-testid="account-create-template-select" @change="$emit(\'update:modelValue\', ($event.target).value)"><option v-for="opt in options" :key="opt.value" :value="opt.value">{{ opt.label }}</option></select>',
})

function mountBar(applySnapshot = vi.fn()) {
  return mount(AccountCreateTemplateBar, {
    props: {
      platform: 'openai',
      accountType: 'oauth',
      proxies: [{ id: 7, name: 'US-1' }],
      groups: [{ id: 38, name: 'Team A' }],
      autoApply: true,
      active: true,
      getSnapshot: () => ({
        ...emptyAccountCreateTemplateValues(),
        proxy_id: 7,
        concurrency: 3,
        openai_ws_mode: 'ctx_pool',
        codex_fingerprint_mode: 'session',
        group_ids: [38],
      }),
      applySnapshot,
    },
    global: {
      stubs: {
        BaseDialog: BaseDialogStub,
        Select: SelectStub,
      },
    },
  })
}

describe('AccountCreateTemplateBar', () => {
  beforeEach(() => {
    listAccountCreateTemplates.mockReset()
    createAccountCreateTemplate.mockReset()
    showSuccess.mockReset()
    listAccountCreateTemplates.mockResolvedValue({
      items: [{
        id: 'tpl-team',
        name: 'Team 轮转',
        platform: 'openai',
        type: 'oauth',
        is_default: true,
        include_groups: true,
        values: {
          ...emptyAccountCreateTemplateValues(),
          proxy_id: 7,
          concurrency: 3,
          openai_ws_mode: 'ctx_pool',
          codex_fingerprint_mode: 'session',
          group_ids: [38],
        },
      }],
    })
  })

  it('auto-applies the default template and can skip groups', async () => {
    const applySnapshot = vi.fn()
    const wrapper = mountBar(applySnapshot)
    await flushPromises()

    expect(applySnapshot).toHaveBeenCalled()
    expect(applySnapshot.mock.calls.at(-1)?.[1]).toEqual({ includeGroups: true })

    await wrapper.get('[data-testid="account-create-template-apply-groups"]').setValue(false)
    await flushPromises()
    expect(applySnapshot.mock.calls.at(-1)?.[1]).toEqual({ includeGroups: false })
    expect(wrapper.get('[data-testid="account-create-template-preview"]').text()).toContain('WS=ctx_pool')
  })

  it('saves the current form as a new template', async () => {
    createAccountCreateTemplate.mockResolvedValue({
      id: 'tpl-new',
      name: 'Pro 固定',
      platform: 'openai',
      type: 'oauth',
      is_default: false,
      include_groups: true,
      values: emptyAccountCreateTemplateValues(),
    })
    const wrapper = mountBar()
    await flushPromises()

    await wrapper.get('[data-testid="account-create-template-save-as"]').trigger('click')
    await wrapper.get('[data-testid="account-create-template-name"]').setValue('Pro 固定')
    await wrapper.get('[data-testid="account-create-template-save-confirm"]').trigger('click')
    await flushPromises()

    expect(createAccountCreateTemplate).toHaveBeenCalledWith(expect.objectContaining({
      name: 'Pro 固定',
      platform: 'openai',
      type: 'oauth',
      include_groups: true,
    }))
    expect(showSuccess).toHaveBeenCalled()
  })
})
