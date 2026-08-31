import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiMocks = vi.hoisted(() => ({
  listGroups: vi.fn(),
  updateUser: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    groups: { list: apiMocks.listGroups },
    users: { update: apiMocks.updateUser },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn() }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

vi.mock('@/components/common/BaseDialog.vue', () => ({
  default: {
    name: 'BaseDialog',
    props: ['show'],
    template: '<div v-if="show"><slot /><slot name="footer" /></div>',
  },
}))

vi.mock('@/components/common/PlatformIcon.vue', () => ({
  default: { name: 'PlatformIcon', template: '<span />' },
}))

import UserAllowedGroupsModal from '../UserAllowedGroupsModal.vue'
import type { AdminUser, Group } from '@/types'

const makeGroup = (overrides: Partial<Group>): Group => ({
  id: 1,
  name: 'Standard group',
  description: null,
  platform: 'openai',
  rate_multiplier: 1,
  is_exclusive: false,
  status: 'active',
  subscription_type: 'standard',
  daily_limit_usd: null,
  weekly_limit_usd: null,
  monthly_limit_usd: null,
  long_context_pricing_enabled: false,
  allow_image_generation: false,
  allow_batch_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  batch_image_discount_multiplier: 1,
  batch_image_hold_multiplier: 1,
  image_price_1k: null,
  image_price_2k: null,
  image_price_4k: null,
  video_rate_independent: false,
  video_rate_multiplier: 1,
  video_price_480p: null,
  video_price_720p: null,
  video_price_1080p: null,
  web_search_price_per_call: null,
  search_price_per_1k: null,
  audio_realtime_price_per_min: null,
  audio_tts_price_per_million_chars: null,
  audio_stt_price_per_hour: null,
  peak_rate_enabled: false,
  peak_rate_multiplier: 1,
  ...overrides,
} as Group)

const user = {
  id: 99,
  email: 'user@example.com',
  allowed_groups: [2],
  group_rates: {},
  restrict_public_groups: true,
  restrict_to_allowed_groups: false,
} as AdminUser

beforeEach(() => {
  vi.clearAllMocks()
  apiMocks.listGroups.mockResolvedValue({
    items: [
      makeGroup({ id: 1, name: 'Standard group' }),
      makeGroup({ id: 2, name: 'Subscription group', subscription_type: 'subscription' }),
      makeGroup({ id: 3, name: 'Inactive group', status: 'inactive' }),
    ],
  })
  apiMocks.updateUser.mockResolvedValue(undefined)
})

describe('UserAllowedGroupsModal', () => {
  it('keeps active subscription groups in the public allowlist and saves their IDs', async () => {
    const wrapper = mount(UserAllowedGroupsModal, {
      props: { show: false, user },
    })

    await wrapper.setProps({ show: true })
    await flushPromises()

    expect(wrapper.text()).toContain('Subscription group')
    expect(wrapper.text()).not.toContain('Inactive group')

    const saveButton = wrapper.findAll('button').find((button) => button.text() === 'common.save')
    expect(saveButton).toBeDefined()
    await saveButton!.trigger('click')
    await flushPromises()

    expect(apiMocks.updateUser).toHaveBeenCalledWith(99, expect.objectContaining({
      allowed_groups: [2],
      restrict_public_groups: true,
    }))
  })
})
