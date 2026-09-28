import { describe, expect, it } from 'vitest'
import type { CustomMenuItem } from '@/types'
import { getExternalRechargeEntries, isExternalRechargeEntry } from '../externalRecharge'

const shop: CustomMenuItem = {
  id: 'ldxp-recharge', label: '链动小铺充值', url: 'https://example.com/shop-recharge/',
  icon_svg: '', visibility: 'user', sort_order: 0,
}

describe('external recharge entries', () => {
  it('recognizes the existing shop without changing its address or creating a duplicate', () => {
    expect(isExternalRechargeEntry(shop)).toBe(true)
    expect(getExternalRechargeEntries([shop])).toEqual([shop])
    expect(shop).not.toHaveProperty('placement')
  })

  it('hides disabled, admin-only, invalid and ordinary menu entries', () => {
    const entries: CustomMenuItem[] = [
      { ...shop, enabled: false },
      { ...shop, visibility: 'admin' },
      { ...shop, url: 'javascript:alert(1)' },
      { ...shop, url: 'md:guide' },
      { ...shop, id: 'docs' },
      { ...shop, placement: 'sidebar' },
    ]
    expect(getExternalRechargeEntries(entries)).toEqual([])
    expect(getExternalRechargeEntries()).toEqual([])
  })

  it('supports additional recharge pages and re-enabling a saved entry', () => {
    const saved: CustomMenuItem = { ...shop, enabled: false, placement: 'recharge' }
    const second: CustomMenuItem = { ...shop, id: 'another-shop', placement: 'recharge', sort_order: 2 }
    expect(getExternalRechargeEntries([second, { ...saved, enabled: true }]).map(item => item.id))
      .toEqual(['ldxp-recharge', 'another-shop'])
    expect(saved.url).toBe(shop.url)
  })
})
