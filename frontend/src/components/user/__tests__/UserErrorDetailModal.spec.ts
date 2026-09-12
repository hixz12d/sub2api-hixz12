import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { UserErrorRequestDetail } from '@/types'

const { getDetail, copy } = vi.hoisted(() => ({ getDetail: vi.fn(), copy: vi.fn() }))
vi.mock('@/api/usage', () => ({ getMyErrorDetail: getDetail }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: copy }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
import UserErrorDetailModal from '../UserErrorDetailModal.vue'

function detail(id: number): UserErrorRequestDetail {
  return { id, created_at: '2026-09-12T00:00:00Z', model: `model-${id}`, inbound_endpoint: '/v1/responses', status_code: 503, category: 'upstream', platform: 'openai', message: '', key_name: 'my-key', key_deleted: false, error_body: '' }
}
function render() {
  return mount(UserErrorDetailModal, { props: { show: true, errorId: 1 }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' } } } })
}

describe('UserErrorDetailModal', () => {
  it('ignores an older response and cancels requests on selection changes and close', async () => {
    const pending = new Map<number, (value: UserErrorRequestDetail) => void>()
    const signals = new Map<number, AbortSignal>()
    getDetail.mockImplementation((id: number, signal: AbortSignal) => {
      signals.set(id, signal)
      return new Promise(resolve => pending.set(id, resolve))
    })
    const wrapper = render()
    await wrapper.setProps({ errorId: 2 })
    expect(signals.get(1)?.aborted).toBe(true)
    pending.get(2)!(detail(2)); await flushPromises()
    pending.get(1)!(detail(1)); await flushPromises()
    expect(wrapper.text()).toContain('model-2')
    expect(wrapper.text()).not.toContain('model-1')
    await wrapper.get('[data-testid="copy-error-report"]').trigger('click')
    expect(JSON.parse(copy.mock.calls.at(-1)![0]).error_id).toBe(2)
    await wrapper.setProps({ errorId: 3 })
    await wrapper.setProps({ show: false })
    expect(signals.get(3)?.aborted).toBe(true)
    pending.get(3)!(detail(3)); await flushPromises()
    expect(wrapper.find('[data-testid="error-advice"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('can retry a failed detail request', async () => {
    getDetail.mockReset().mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(detail(1))
    const wrapper = render()
    await flushPromises()
    expect(wrapper.text()).toContain('usage.errors.detail.loadFailed')
    await wrapper.get('button').trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('model-1')
    expect(wrapper.text()).toContain('usage.errors.detail.advice.upstream')
    wrapper.unmount()
  })
})
