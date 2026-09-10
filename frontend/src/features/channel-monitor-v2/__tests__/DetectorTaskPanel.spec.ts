import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import DetectorTaskPanel from '../DetectorTaskPanel.vue'
const api = vi.hoisted(() => ({ plan: vi.fn(), create: vi.fn(), job: vi.fn(), cancel: vi.fn() }))
vi.mock('@/api/admin/detectorTasks', () => ({ detectorTaskAPI: api }))
vi.mock('@/api/admin/benchmarks', () => ({ benchmarkAPI: { list: async () => ({ channels: [{ name: 'baseline', state: 'approved' }] }) } }))
const make = () => mount(DetectorTaskPanel, { global: { plugins: [createI18n({ legacy: false, locale: 'en' })], stubs: { Icon: true } } })
describe('DetectorTaskPanel', () => {
  beforeEach(() => { vi.resetAllMocks(); api.plan.mockResolvedValue({ id: 'plan', configuration_hash: 'hash', planned_requests: 3, expires_at: '2099-01-01T00:00:00Z', estimate_status: 'unknown' }) })
  it('requires confirmation and reuses the key after an uncertain creation', async () => {
    api.create.mockRejectedValue(new Error('timeout'))
    const wrapper = make(); await flushPromises()
    const inputs = wrapper.findAll('form input')
    await inputs[0].setValue(1); await inputs[1].setValue('gpt-test'); await inputs[2].setValue('gpt-test')
    await wrapper.findAll('select')[0].setValue('baseline')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.create).not.toHaveBeenCalled()
    await wrapper.get('input[type="checkbox"]').setValue(true)
    const submit = () => wrapper.findAll('button').find(b => ['Create task', 'Retry same task'].includes(b.text()))!
    await submit().trigger('click'); await flushPromises()
    const key = api.create.mock.calls[0][1]
    await submit().trigger('click'); await flushPromises()
    expect(api.create.mock.calls[1][1]).toBe(key)
    expect(wrapper.get('form input').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
