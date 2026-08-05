import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'
import type { AdminUser, Group } from '@/types'

const { listGroups, updateUser, showError, showSuccess } = vi.hoisted(() => ({
  listGroups: vi.fn(),
  updateUser: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { list: listGroups },
    users: { update: updateUser },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError, showSuccess }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

const group = (id: number, name: string, options: Partial<Group> = {}) => ({
  id,
  name,
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: true,
  status: 'active',
  subscription_type: 'standard',
  ...options,
}) as Group

const user = {
  id: 7,
  email: 'user@example.com',
  allowed_groups: [39, 999],
  group_rates: {},
} as AdminUser

async function mountAndOpen() {
  const wrapper = mount(UserAllowedGroupsModal, {
    props: { show: false, user },
    global: {
      stubs: {
        BaseDialog: {
          props: ['show', 'title', 'width'],
          emits: ['close'],
          template: '<div v-if="show"><slot /><slot name="footer" /></div>',
        },
        PlatformIcon: true,
      },
    },
  })

  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

function saveButton(wrapper: ReturnType<typeof mount>) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text() === 'common.save')
  if (!button) throw new Error('save button not found')
  return button
}

beforeEach(() => {
  vi.clearAllMocks()
  updateUser.mockResolvedValue({})
  listGroups.mockResolvedValue({
    items: [
      group(38, 'Active Exclusive'),
      group(39, 'Disabled Exclusive', { status: 'disabled' as Group['status'] }),
      group(50, 'Disabled Public', { is_exclusive: false, status: 'inactive' }),
      group(51, 'Active Public', { is_exclusive: false }),
    ],
  })
})

describe('UserAllowedGroupsModal', () => {
  it('shows disabled exclusive groups without showing disabled public groups', async () => {
    const wrapper = await mountAndOpen()

    expect(wrapper.get('[data-test="group-config-38"]').text()).toContain('Active Exclusive')
    expect(wrapper.get('[data-test="group-config-39"]').text()).toContain('Disabled Exclusive')
    expect(wrapper.get('[data-test="group-config-39"]').text()).toContain('common.disabled')
    expect(wrapper.text()).toContain('Active Public')
    expect(wrapper.text()).not.toContain('Disabled Public')
  })

  it('preserves allowed group IDs that are not managed by the modal', async () => {
    const wrapper = await mountAndOpen()

    await saveButton(wrapper).trigger('click')
    await flushPromises()

    expect(updateUser).toHaveBeenCalledWith(7, {
      allowed_groups: [999, 39],
      group_rates: undefined,
    })
  })

  it('can explicitly remove a disabled exclusive group while preserving hidden IDs', async () => {
    const wrapper = await mountAndOpen()

    await wrapper.get('[data-test="group-toggle-39"]').trigger('change')
    await saveButton(wrapper).trigger('click')
    await flushPromises()

    expect(updateUser).toHaveBeenCalledWith(7, {
      allowed_groups: [999],
      group_rates: undefined,
    })
  })

  it('disables saving when group loading fails', async () => {
    listGroups.mockRejectedValueOnce(new Error('load failed'))
    const wrapper = await mountAndOpen()

    expect(saveButton(wrapper).attributes('disabled')).toBeDefined()
    expect(showError).toHaveBeenCalledWith('admin.users.failedToLoadGroups')
    expect(updateUser).not.toHaveBeenCalled()
  })
})
