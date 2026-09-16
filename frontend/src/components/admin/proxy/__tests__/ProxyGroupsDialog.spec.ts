import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ProxyGroupsDialog from '../ProxyGroupsDialog.vue'

const { list, groups, save, remove } = vi.hoisted(() => ({ list: vi.fn(), groups: vi.fn(), save: vi.fn(), remove: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { list } } }))
vi.mock('@/api/admin/proxyGroups', () => ({ listProxyGroups: groups, saveProxyGroup: save, deleteProxyGroup: remove }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))
let wrapper: ReturnType<typeof mount>
beforeEach(() => {
  vi.clearAllMocks()
  groups.mockResolvedValue([{ id: 8, name: 'IPv6', max_accounts_per_proxy: 2, proxy_ids: [1, 2], available_proxy_ids: [1] }])
  list.mockImplementation(async (page: number) => ({ items: [{ id: page, name: `Proxy ${page}`, host: '192.0.2.1', port: 1000 + page, status: page === 1 ? 'active' : 'inactive' }], total: 2, pages: 2 }))
  save.mockResolvedValue({ id: 9 })
})
afterEach(() => wrapper?.unmount())
async function open() {
  wrapper = mount(ProxyGroupsDialog, { props: { show: true, selectedIds: [2] }, global: { stubs: { BaseDialog: { template: '<div><slot /></div>' }, ConfirmDialog: true } } })
  await flushPromises()
}
describe('proxy group management', () => {
  it('loads all pages, includes inactive members, and creates with the default limit', async () => {
    await open()
    expect(list).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Proxy 2')
    await wrapper.get('input[maxlength="100"]').setValue('IPv4 egress')
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(save).toHaveBeenCalledWith(null, { name: 'IPv4 egress', max_accounts_per_proxy: 2, proxy_ids: [2] })
    expect(wrapper.emitted('saved')).toHaveLength(1)
  })
  it('preserves existing members when editing and exposes capacity errors', async () => {
    await open()
    await wrapper.findAll('button').find(button => button.text().startsWith('IPv6'))!.trigger('click')
    expect(wrapper.findAll<HTMLInputElement>('input[type="checkbox"]').every(input => input.element.checked)).toBe(true)
    await wrapper.get('input[type="number"]').setValue(1)
    save.mockRejectedValue({ response: { data: { message: '代理已达到分组账号上限' } } })
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(save).toHaveBeenCalledWith(8, { name: 'IPv6', max_accounts_per_proxy: 1, proxy_ids: [1, 2] })
    expect(wrapper.get('[role="alert"]').text()).toContain('账号上限')
    expect(wrapper.emitted('saved')).toBeUndefined()
  })
})
