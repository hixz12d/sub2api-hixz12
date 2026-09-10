import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import QuestionReviewPanel from '../QuestionReviewPanel.vue'
import messages from '@/i18n/locales/en/admin/accounts'
const compiled = { admin: { accounts: { questionReview: Object.fromEntries(Object.entries(messages.accounts.questionReview).map(([k, v]) => [k, () => v])) } } }
const api = vi.hoisted(() => ({ list: vi.fn(), history: vi.fn(), review: vi.fn() }))
vi.mock('@/api/admin/questionReviews', () => ({ questionAPI: api }))
const record = { id: 'record', account_id: 1, request_model: 'test', prompt: ' exact prompt ', answer: '<script>not HTML</script>', transport_state: 'completed', created_at: '2026-09-10T00:00:00Z' }
const make = () => mount(QuestionReviewPanel, { props: { accountId: 1 }, global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: compiled } })], stubs: { Icon: true } } })
describe('QuestionReviewPanel', () => {
  beforeEach(() => { vi.resetAllMocks(); api.list.mockResolvedValue({ items: [record], page: 1, has_more: false }); api.history.mockResolvedValue({ items: [], page: 1, has_more: false }) })
  it('submits a review without sending the answer', async () => {
    const wrapper = make(); await flushPromises()
    expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.findAll('pre')[0].element.textContent).toBe(' exact prompt ')
    await wrapper.get('textarea').setValue('Reviewed manually')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.review).toHaveBeenCalledWith('record', 'unlabeled', 'Reviewed manually', 0)
    wrapper.unmount()
  })
  it('does not overwrite after a revision conflict', async () => {
    api.review.mockRejectedValue(new Error('secret SQL'))
    const wrapper = make(); await flushPromises()
    await wrapper.get('textarea').setValue('reason'); await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('Assessment not saved')
    expect(wrapper.text()).not.toContain('secret SQL')
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
  it('keeps submission disabled if history cannot be loaded', async () => {
    api.history.mockRejectedValue(new Error('offline'))
    const wrapper = make(); await flushPromises()
    expect(wrapper.get('button[type="submit"]').attributes('disabled')).toBeDefined()
    expect(api.review).not.toHaveBeenCalled(); wrapper.unmount()
  })
  it('aborts stale account reads', async () => {
    api.list.mockReturnValue(new Promise(() => {}))
    const wrapper = make(); const signal = api.list.mock.calls[0][2] as AbortSignal
    await wrapper.setProps({ accountId: 2 }); expect(signal.aborted).toBe(true)
    wrapper.unmount(); expect((api.list.mock.calls[1][2] as AbortSignal).aborted).toBe(true)
  })
})
