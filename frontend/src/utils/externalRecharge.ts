import type { CustomMenuItem } from '@/types'

// Preserve the existing shop entry without requiring a database migration.
// An explicit sidebar placement takes precedence over this legacy identifier.
export function isExternalRechargeEntry(item: CustomMenuItem): boolean {
  return item.placement === 'recharge' || (!item.placement && item.id === 'ldxp-recharge')
}

export function getExternalRechargeEntries(items: readonly CustomMenuItem[] = []): CustomMenuItem[] {
  return items.filter((item) => {
    if (!isExternalRechargeEntry(item) || item.enabled === false || item.visibility !== 'user') return false
    try {
      const url = new URL(item.url)
      return url.protocol === 'https:' || url.protocol === 'http:'
    } catch {
      return false
    }
  }).sort((a, b) => a.sort_order - b.sort_order)
}
