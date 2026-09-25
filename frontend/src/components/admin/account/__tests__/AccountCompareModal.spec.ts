import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AccountCompareModal from '../AccountCompareModal.vue'
import type { AccountQuestionEvent } from '@/utils/accountQuestionStream'

const { stream, review } = vi.hoisted(() => ({ stream: vi.fn(), review: vi.fn() }))
vi.mock('@/utils/accountQuestionStream', () => ({ streamAccountQuestion: stream }))
vi.mock('@/api/admin/questionReviews', () => ({ questionAPI: { review, history: vi.fn() } }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getById: vi.fn() } } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
interface Pending { id: number; signal: AbortSignal; event: (event: AccountQuestionEvent) => void; resolve: () => void; reject: (error: Error) => void }
let pending: Pending[]
let wrapper: ReturnType<typeof mount>
beforeEach(() => {
  vi.clearAllMocks()
  pending = []
  stream.mockImplementation((id, _model, _prompt, signal, event) => new Promise<void>((resolve, reject) => {
    pending.push({ id, signal, event, resolve, reject })
    signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })
  }))
  review.mockResolvedValue({ revision: 1 })
})
afterEach(() => wrapper?.unmount())
function open(count = 4) {
  const ids = Array.from({ length: count }, (_, i) => i + 1)
  wrapper = mount(AccountCompareModal, { props: { show: true, accountIds: ids, knownAccounts: ids.map(id => ({ id, name: `Account ${id}` })) }, global: { stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><button data-close @click="$emit(\'close\')">close</button><slot /></div>' } } } })
}
function complete(index: number) {
  const run = pending[index]
  run.event({ type: 'content', text: `Answer ${run.id}` })
  run.event({ type: 'test_complete', success: true })
  run.event({ type: 'question_record', saved: true, record_id: `record-${run.id}` })
  run.resolve()
}
const card = (id: number) => wrapper.get(`[data-account-id="${id}"]`)
describe('account comparison', () => {
  it('runs at most three accounts concurrently with the same question and isolated answers', async () => {
    open()
    await wrapper.get('textarea').setValue('Shared question')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(stream).toHaveBeenCalledTimes(3)
    expect(pending.map(p => p.id)).toEqual([1, 2, 3])
    expect(card(4).get('[role="status"]').text()).toBe('accountCompare.waiting')
    complete(0)
    await flushPromises()
    expect(stream).toHaveBeenCalledTimes(4)
    for (const call of stream.mock.calls) expect(call.slice(1, 3)).toEqual(['gpt-5.4', 'Shared question'])
    for (const call of stream.mock.calls) expect(call[5]).toBe('')
    expect(card(1).get('pre').text()).toBe('Answer 1')
    expect(card(2).get('pre').text()).toBe('accountCompare.empty')
    pending[1].reject(new Error('HTTP 503'))
    complete(2); complete(3)
    await flushPromises()
    expect(card(2).get('[role="alert"]').text()).toBe('HTTP 503')
    expect(card(3).get('[role="status"]').text()).toBe('accountCompare.success')
  })
  it('stops active and queued work and ignores old events after reopening', async () => {
    open()
    await wrapper.get('button.btn-primary').trigger('click')
    await wrapper.get('button.btn-danger').trigger('click')
    await flushPromises()
    expect(pending.every(p => p.signal.aborted)).toBe(true)
    expect(stream).toHaveBeenCalledTimes(3)
    expect(card(4).get('[role="status"]').text()).toBe('accountCompare.cancelled')
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    pending[0].event({ type: 'content', text: 'stale response' })
    await flushPromises()
    expect(wrapper.text()).not.toContain('stale response')
    expect(card(1).get('[role="status"]').text()).toBe('accountCompare.waiting')
  })
  it('binds human reviews to the exact answer record and retries with the original question', async () => {
    open(1)
    await wrapper.get('textarea').setValue('Original question')
    await wrapper.get('select[data-effort]').setValue('high')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(stream.mock.lastCall?.[5]).toBe('high')
    complete(0)
    await flushPromises()
    await card(1).get('select').setValue('normal')
    await card(1).get('form textarea').setValue('Answer is accurate')
    await card(1).get('form').trigger('submit')
    await flushPromises()
    expect(review).toHaveBeenCalledWith('record-1', 'normal', 'Answer is accurate', 0)
    expect(wrapper.emitted('reviewed')).toHaveLength(1)
    await wrapper.get('textarea').setValue('A different question')
    const retry = card(1).findAll('button').find(button => button.text() === 'accountCompare.retry')!
    await wrapper.get('select[data-effort]').setValue('low')
    await retry.trigger('click')
    expect(stream.mock.lastCall?.slice(1, 3)).toEqual(['gpt-5.4', 'Original question'])
    expect(stream.mock.lastCall?.[5]).toBe('high')
  })
})
