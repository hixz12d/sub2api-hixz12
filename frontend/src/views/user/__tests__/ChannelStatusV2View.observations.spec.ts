import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import ChannelStatusV2View from '../ChannelStatusV2View.vue'
import ObservedStatusCards from '@/features/channel-monitor-v2/ObservedStatusCards.vue'
import FilterMultiSelect from '@/features/channel-monitor-v2/FilterMultiSelect.vue'
import * as api from '@/api/channelMonitorV2'

const mocks = vi.hoisted(() => ({ auth: { isAdmin: false }, showError: vi.fn(), replace: vi.fn() }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => mocks.auth }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.showError }) }))
vi.mock('vue-router', () => ({ useRoute: () => ({ query: {} }), useRouter: () => ({ replace: mocks.replace }) }))
vi.mock('@/utils/featureFlags', () => ({ isChannelMonitorThroughputHidden: () => false, isChannelMonitorUserRankingHidden: () => false }))
vi.mock('@/api/channelMonitorV2', () => ({ getCards: vi.fn(), getDimensions: vi.fn(), getSnapshot: vi.fn(), getMatrix: vi.fn(), getModels: vi.fn(), getErrors: vi.fn(), getUsers: vi.fn() }))

const coverage = { data_through: '2026-09-09T00:00:00Z', coverage_complete: true }
const latency = { sample_count: 0, p50_ms: null, p90_ms: null, p95_ms: null, avg_ms: null }
const metric = { request_count: 0, success_rate: 0, rpm: 0, tpm: 0, cache_rate: 0, ttft: latency, duration: latency }
const cards = { items: [], has_more: false, as_of: '2026-09-09T00:00:00Z' }
let wrapper: VueWrapper | undefined
function mountPage() {
  wrapper = shallowMount(ChannelStatusV2View, {
    global: {
      plugins: [createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })],
      stubs: { AppLayout: { template: '<main><slot /></main>' }, ObservedStatusCards: false },
    },
  })
  return wrapper
}
beforeEach(() => {
  vi.resetAllMocks()
  vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
  mocks.auth.isAdmin = false
  vi.mocked(api.getCards).mockResolvedValue(cards as never)
  vi.mocked(api.getDimensions).mockResolvedValue({ platforms: [], groups: [], models: [] })
  vi.mocked(api.getSnapshot).mockResolvedValue({ coverage, config: { refresh_interval_seconds: 60 }, metrics: metric, health: { overall: 'unknown' } } as never)
  vi.mocked(api.getMatrix).mockResolvedValue({ coverage, items: [] } as never)
  vi.mocked(api.getModels).mockResolvedValue({ items: [] } as never)
})
afterEach(() => { wrapper?.unmount(); wrapper = undefined; vi.useRealTimers() })

describe('V2 page observation integration', () => {
  it.each([false, true])('uses the correct API role and loads cards once on mount (admin=%s)', async admin => {
    mocks.auth.isAdmin = admin
    const page = mountPage()
    await flushPromises()
    expect(page.findComponent(ObservedStatusCards).exists()).toBe(true)
    expect(api.getCards).toHaveBeenCalledTimes(1)
    expect(vi.mocked(api.getCards).mock.calls[0]![2]).toBe(admin)
    expect(vi.mocked(api.getSnapshot).mock.calls[0]![1]).toBe(admin)
  })
  it('refreshes cards on the page button and the existing polling timer', async () => {
    const page = mountPage()
    await flushPromises()
    await page.find('button[title="common.refresh"]').trigger('click')
    await flushPromises()
    expect(api.getCards).toHaveBeenCalledTimes(2)
    await vi.advanceTimersByTimeAsync(60000)
    await flushPromises()
    expect(api.getCards).toHaveBeenCalledTimes(3)
    expect(vi.mocked(api.getCards).mock.lastCall?.[1]).toEqual({ page: 1, page_size: 12, as_of: undefined })
    page.unmount()
    wrapper = undefined
    await vi.advanceTimersByTimeAsync(120000)
    expect(api.getCards).toHaveBeenCalledTimes(3)
  })
  it('propagates dimension filters without a duplicate card request', async () => {
    const page = mountPage()
    await flushPromises()
    page.findAllComponents(FilterMultiSelect)[0]!.vm.$emit('update:modelValue', ['openai'])
    await flushPromises()
    expect(api.getCards).toHaveBeenCalledTimes(2)
    expect(vi.mocked(api.getCards).mock.lastCall?.[0].platforms).toEqual(['openai'])
    expect(mocks.replace).toHaveBeenCalled()
  })
  it('ignores failures from requests superseded by a newer refresh', async () => {
    let rejectOld!: (error: Error) => void
    vi.mocked(api.getSnapshot).mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
    const page = mountPage()
    await flushPromises()
    page.findAllComponents(FilterMultiSelect)[0]!.vm.$emit('update:modelValue', ['openai'])
    await flushPromises()
    rejectOld(new Error('late transport failure'))
    await flushPromises()
    expect(mocks.showError).not.toHaveBeenCalled()
  })
  it('does not restore a stale model list after filters change', async () => {
    let resolveOld!: (value: never) => void
    vi.mocked(api.getModels).mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve }))
    const page = mountPage()
    await flushPromises()
    page.findAllComponents(FilterMultiSelect)[0]!.vm.$emit('update:modelValue', ['openai'])
    await flushPromises()
    resolveOld({ items: [{ platform: 'openai', model: 'stale-model', metrics: metric, health: { overall: 'unknown' } }] } as never)
    await flushPromises()
    expect(page.text()).not.toContain('stale-model')
    expect(mocks.showError).not.toHaveBeenCalled()
  })
  it('isolates a cards failure and recovers through page refresh', async () => {
    vi.mocked(api.getCards).mockRejectedValueOnce(new Error('unavailable'))
    const page = mountPage()
    await flushPromises()
    expect(page.findComponent(ObservedStatusCards).find('[role="alert"]').exists()).toBe(true)
    expect(mocks.showError).not.toHaveBeenCalled()
    await page.find('button[title="common.refresh"]').trigger('click')
    await flushPromises()
    expect(page.findComponent(ObservedStatusCards).find('[role="alert"]').exists()).toBe(false)
  })
})
