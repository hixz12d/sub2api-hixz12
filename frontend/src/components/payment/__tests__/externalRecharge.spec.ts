import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ExternalRechargeSettings from '../ExternalRechargeSettings.vue'
import ExternalRechargeMethods from '../ExternalRechargeMethods.vue'
import type { CustomMenuItem } from '@/types'
vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string, values?: { name: string }) => values ? `${key}:${values.name}` : key }),
}))

const shop: CustomMenuItem = {
  id: 'ldxp-recharge', label: '链动小铺充值', url: 'https://example.com/shop-recharge/',
  icon_svg: '', visibility: 'user', sort_order: 0,
}
const docs: CustomMenuItem = { ...shop, id: 'docs', label: '使用文档', sort_order: 1 }

describe('external recharge settings', () => {
  it('can disable, restore and delete the old shop while preserving unrelated menus', async () => {
    const wrapper = mount(ExternalRechargeSettings, { props: { modelValue: [shop, docs] } })
    expect(wrapper.findAll('[data-testid="external-recharge-entry"]')).toHaveLength(1)
    const toggle = wrapper.get('[data-testid="external-recharge-enabled"]')
    await toggle.trigger('click')
    const disabled = wrapper.emitted('update:modelValue')![0][0] as CustomMenuItem[]
    expect(disabled).toEqual([{ ...shop, enabled: false, placement: 'recharge' }, docs])
    expect(shop).not.toHaveProperty('enabled')
    await wrapper.setProps({ modelValue: disabled })
    expect(toggle.attributes('aria-checked')).toBe('false')
    expect(wrapper.text()).toContain('admin.settings.payment.externalRecharge.hidden')
    await toggle.trigger('click')
    const enabled = wrapper.emitted('update:modelValue')![1][0] as CustomMenuItem[]
    expect(enabled[0]).toMatchObject({ enabled: true, url: shop.url })
    await wrapper.setProps({ modelValue: enabled })
    await wrapper.get('button[aria-label="admin.settings.payment.externalRecharge.remove:链动小铺充值"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')![2][0]).toEqual([docs])
    wrapper.unmount()
  })

  it('adds a separate external entry and allows editing its name and link', async () => {
    const wrapper = mount(ExternalRechargeSettings, { props: { modelValue: [docs] } })
    await wrapper.get('[data-testid="add-external-recharge"]').trigger('click')
    const added = wrapper.emitted('update:modelValue')![0][0] as CustomMenuItem[]
    expect(added[1]).toMatchObject({ placement: 'recharge', visibility: 'user', enabled: true })
    await wrapper.setProps({ modelValue: added })
    await wrapper.get('[data-testid="external-recharge-url"]').setValue('https://example.com/new-shop')
    expect((wrapper.emitted('update:modelValue')![1][0] as CustomMenuItem[])[1].url).toBe('https://example.com/new-shop')
    wrapper.unmount()
  })

  it('opens the existing embedded recharge route instead of creating a PerPay order', () => {
    const wrapper = mount(ExternalRechargeMethods, {
      props: { entries: [shop] },
      global: { stubs: { RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' } } },
    })
    expect(wrapper.get('a').attributes('href')).toBe('/custom/ldxp-recharge')
    expect(wrapper.text()).toContain('链动小铺充值')
    wrapper.unmount()
  })
})
