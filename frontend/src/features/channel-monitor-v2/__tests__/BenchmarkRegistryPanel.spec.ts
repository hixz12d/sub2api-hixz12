import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createI18n } from 'vue-i18n'
import BenchmarkRegistryPanel from '../BenchmarkRegistryPanel.vue'
import messages from '@/i18n/locales/en/channelMonitorV2'
const compiled = { channelMonitorV2: { benchmarks: Object.fromEntries(Object.entries(messages.channelMonitorV2.benchmarks).map(([key, value]) => [key, () => value])) } }

const api = vi.hoisted(() => ({ list: vi.fn(), stage: vi.fn(), approve: vi.fn(), activate: vi.fn(), withdraw: vi.fn() }))
vi.mock('@/api/admin/benchmarks', () => ({ benchmarkAPI: api }))
const release = { id: 'release-1', benchmark_id: 'benchmark-name', version: '4.5.2', sha256: 'a'.repeat(64), state: 'approved', mode: 'gpt', engine_lock_sha256: 'b'.repeat(64) }
const result = { items: [release], channels: [{ name: 'benchmark-name', release_id: release.id, revision: 7, state: 'approved' }], page: 1, has_more: false }
const make = () => mount(BenchmarkRegistryPanel, { global: { plugins: [createI18n({ legacy: false, locale: 'en', messages: { en: compiled } })], stubs: { BaseDialog: { props: ['show'], template: '<div v-if="show"><slot/><slot name="footer"/></div>' }, Icon: true } } })
function button(wrapper: ReturnType<typeof make>, label: string) { return wrapper.findAll('button').find(b => b.text() === label)! }

describe('BenchmarkRegistryPanel', () => {
  beforeEach(() => { vi.resetAllMocks(); api.list.mockResolvedValue(result) })
  it('activates with the displayed channel revision only after confirmation', async () => {
    const wrapper = make(); await flushPromises()
    await button(wrapper, 'Activate').trigger('click')
    expect(api.activate).not.toHaveBeenCalled()
    await button(wrapper, 'Confirm').trigger('click'); await flushPromises()
    expect(api.activate).toHaveBeenCalledWith('release-1', 'benchmark-name', 7, expect.any(AbortSignal))
    wrapper.unmount()
  })
  it('requires a withdrawal reason and preserves conflicts without exposing raw errors', async () => {
    api.withdraw.mockRejectedValue({ status: 409, message: 'private SQL secret' })
    const wrapper = make(); await flushPromises()
    await button(wrapper, 'Withdraw').trigger('click')
    expect(button(wrapper, 'Confirm').attributes('disabled')).toBeDefined()
    await wrapper.get('textarea').setValue('outdated baseline')
    await button(wrapper, 'Confirm').trigger('click'); await flushPromises()
    expect(api.withdraw).toHaveBeenCalledWith('release-1', 'outdated baseline', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('Version conflict')
    expect(wrapper.text()).not.toContain('private SQL secret')
    wrapper.unmount()
  })
  it('aborts requests when the tab unmounts', async () => {
    let resolve!: (value: typeof result) => void
    api.list.mockReturnValue(new Promise(r => { resolve = r }))
    const wrapper = make()
    const signal = api.list.mock.calls[0][1] as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
    resolve(result); await flushPromises()
    expect(api.list).toHaveBeenCalledTimes(1)
  })
  it('shows a permission failure without actions', async () => {
    api.list.mockRejectedValue({ status: 403 })
    const wrapper = make(); await flushPromises()
    expect(wrapper.text()).toContain('Administrator access required')
    expect(wrapper.findAll('li')).toHaveLength(0)
    wrapper.unmount()
  })
})
