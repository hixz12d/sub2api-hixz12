import { beforeEach, describe, expect, it, vi } from 'vitest'

const { patch } = vi.hoisted(() => ({
  patch: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { patch }
}))

import { updateAccountPriorities } from '@/api/admin/groups'

describe('admin group account priority API', () => {
  beforeEach(() => {
    patch.mockReset()
    patch.mockResolvedValue({
      data: [{ account_id: 17, group_id: 9, priority: 1 }]
    })
  })

  it('sends optimistic-concurrency items to the group priority endpoint', async () => {
    const items = [{ account_id: 17, priority: 1, expected_priority: 50 }]

    const result = await updateAccountPriorities(9, items)

    expect(patch).toHaveBeenCalledWith('/admin/groups/9/account-priorities', { items })
    expect(result).toEqual([{ account_id: 17, group_id: 9, priority: 1 }])
  })
})
