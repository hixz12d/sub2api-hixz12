import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import Panel from '../QuestionReviewPanel.vue'
import messages from '@/i18n/locales/en/admin/accounts'
const api = vi.hoisted(() => ({ toggleStatus: vi.fn(), history: vi.fn() }))
vi.mock('@/api/admin/accounts', () => ({ toggleStatus: api.toggleStatus }))
vi.mock('@/api/admin/questionReviews', () => ({ questionAPI: {
  list: async () => ({ items: [{ id: 'record', request_model: 'model', created_at: '2026-01-01', transport_state: 'completed' }], page: 1, has_more: false }), history: api.history, review: vi.fn()
} }))
const compiled = { admin: { accounts: { questionReview: Object.fromEntries(Object.entries(messages.accounts.questionReview).map(([k, v]) => [k, () => v])) } } }
const make = () => mount(Panel, { props: { accountId: 7 }, global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: compiled } })], stubs: { Icon: true, ConfirmDialog: { props: ['show'], emits: ['confirm', 'cancel'], template: '<div v-if="show"><button data-confirm @click="$emit(\'confirm\')">Confirm</button><button data-cancel @click="$emit(\'cancel\')">Cancel</button></div>' } } } })
describe('Independent account disable', () => {
  beforeEach(() => { vi.resetAllMocks(); api.history.mockResolvedValue({ items: [{ id: 'review', verdict: 'degraded', revision: 1, created_at: '2026-01-01' }], page: 1, has_more: false }) })
  it('does not disable on review load or cancelled confirmation', async () => {
    const wrapper = make(); await flushPromises()
    expect(api.toggleStatus).not.toHaveBeenCalled()
    await wrapper.findAll('button').find(b => b.text() === 'Disable account separately')!.trigger('click')
    expect(api.toggleStatus).not.toHaveBeenCalled()
    await wrapper.get('[data-cancel]').trigger('click')
    expect(api.toggleStatus).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('disables only the explicitly confirmed account', async () => {
    const wrapper = make(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'Disable account separately')!.trigger('click')
    await wrapper.get('[data-confirm]').trigger('click'); await flushPromises()
    expect(api.toggleStatus).toHaveBeenCalledTimes(1)
    expect(api.toggleStatus).toHaveBeenCalledWith(7, 'inactive')
    expect(wrapper.emitted('account-updated')).toHaveLength(1); wrapper.unmount()
  })
  it('clears confirmation when account changes', async () => {
    const wrapper = make(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === 'Disable account separately')!.trigger('click')
    await wrapper.setProps({ accountId: 8 }); await flushPromises()
    expect(wrapper.find('[data-confirm]').exists()).toBe(false)
    expect(api.toggleStatus).not.toHaveBeenCalled(); wrapper.unmount()
  })
})
