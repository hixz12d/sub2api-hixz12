import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProxySelector from '../ProxySelector.vue'
import type { Proxy } from '@/types'

const { listGroups } = vi.hoisted(() => ({ listGroups: vi.fn() }))
vi.mock('@/api/admin/proxyGroups', () => ({ listProxyGroups: listGroups }))
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: {} } }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
const proxies = [
  { id: 1, name: 'IPv6 A', host: '192.0.2.1', port: 1001, protocol: 'socks5' },
  { id: 2, name: 'IPv6 B', host: '192.0.2.1', port: 1002, protocol: 'socks5' },
  { id: 3, name: 'IPv4', host: '192.0.2.2', port: 1001, protocol: 'socks5' },
] as Proxy[]
let wrapper: ReturnType<typeof mount>
beforeEach(() => {
  vi.clearAllMocks()
  listGroups.mockResolvedValue([{ id: 8, name: 'IPv6 egress', max_accounts_per_proxy: 2, proxy_ids: [1, 2], available_proxy_ids: [2] }])
})
afterEach(() => wrapper?.unmount())
async function open() {
  wrapper = mount(ProxySelector, { props: { modelValue: null, proxies, allowGroup: true }, global: { stubs: { Icon: true } } })
  await wrapper.get('.select-trigger').trigger('click')
  await flushPromises()
  await wrapper.get('select').setValue('8')
}
describe('proxy group selection', () => {
  it('filters by membership and emits a concrete preview plus the allocation group', async () => {
    await open()
    expect(wrapper.findAll('.select-option-label').map(n => n.text())).not.toContain('IPv4')
    expect(wrapper.text()).not.toContain('192.0.2.2')
    await wrapper.get('button.btn-primary').trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[2]])
    expect(wrapper.emitted('update:groupId')).toEqual([[8]])
  })
  it('prevents allocation when all group proxies are full', async () => {
    listGroups.mockResolvedValue([{ id: 8, name: 'IPv6 egress', max_accounts_per_proxy: 2, proxy_ids: [1, 2], available_proxy_ids: [] }])
    await open()
    expect(wrapper.get('button.btn-primary').attributes('disabled')).toBeDefined()
    expect(wrapper.text()).toContain('proxyGroups.full')
  })
  it('clears automatic assignment when selecting a specific proxy', async () => {
    await open()
    await wrapper.findAll('.select-option')[1].trigger('click')
    expect(wrapper.emitted('update:modelValue')).toEqual([[1]])
    expect(wrapper.emitted('update:groupId')).toEqual([[null]])
  })
  it('shows only ungrouped proxies and exposes fetch failures', async () => {
    await open()
    await wrapper.get('select').setValue('0')
    expect(wrapper.text()).toContain('192.0.2.2')
    expect(wrapper.text()).not.toContain('IPv6 A')
    await wrapper.get('.select-trigger').trigger('click')
    listGroups.mockRejectedValue(new Error('offline'))
    await wrapper.get('.select-trigger').trigger('click')
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe('proxyGroups.loadFailed')
  })
})
