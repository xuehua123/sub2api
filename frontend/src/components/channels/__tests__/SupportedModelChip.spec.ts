import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import SupportedModelChip from '../SupportedModelChip.vue'
import type {
  UserAvailableGroup,
  UserSupportedModel,
  UserSupportedModelPricing,
} from '@/api/channels'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function pricing(
  billingMode: UserSupportedModelPricing['billing_mode'],
  perRequestPrice: number | null,
): UserSupportedModelPricing {
  return {
    billing_mode: billingMode,
    input_price: billingMode === 'token' ? perRequestPrice : null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_input_price: null,
    image_output_price: null,
    per_request_price: billingMode === 'token' ? null : perRequestPrice,
    intervals: [],
  }
}

const groups: UserAvailableGroup[] = [
  {
    id: 1,
    name: 'Free group',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
  },
  {
    id: 2,
    name: 'Video group',
    platform: 'openai',
    subscription_type: 'standard',
    rate_multiplier: 1,
    peak_rate_enabled: false,
    peak_start: '',
    peak_end: '',
    peak_rate_multiplier: 1,
    is_exclusive: false,
  },
]

describe('SupportedModelChip group pricing', () => {
  it('renders each visible group price, including explicit zero and video seconds', async () => {
    const model: UserSupportedModel = {
      name: 'video-public',
      platform: 'openai',
      pricing: pricing('token', 9e-6),
      pricing_by_group: {
        '1': pricing('token', 0),
        '2': pricing('video', 0.04),
      },
    }
    const wrapper = mount(SupportedModelChip, {
      props: { model, groups, noPricingLabel: 'No price' },
      global: {
        stubs: {
          Teleport: true,
          PlatformIcon: true,
          PricingRow: {
            props: ['label', 'value', 'unit'],
            template: '<div>{{ label }}={{ value }} {{ unit }}</div>',
          },
        },
      },
    })

    await wrapper.get('[tabindex="0"]').trigger('mouseenter')

    expect(wrapper.text()).toContain('Free group')
    expect(wrapper.text()).toContain('availableChannels.pricing.inputPrice=0')
    expect(wrapper.text()).toContain('Video group')
    expect(wrapper.text()).toContain('availableChannels.pricing.billingModeVideo')
    expect(wrapper.text()).toContain('availableChannels.pricing.perSecondPrice=0.04')
    expect(wrapper.text()).toContain('availableChannels.pricing.unitPerSecond')
  })

  it('falls back to the legacy channel price when pricing_by_group is absent', () => {
    const wrapper = mount(SupportedModelChip, {
      props: {
        model: {
          name: 'legacy',
          platform: 'openai',
          pricing: pricing('token', 3e-6),
        },
        groups,
      },
      global: { stubs: { Teleport: true, PlatformIcon: true } },
    })

    const entries = (wrapper.vm as unknown as {
      displayPricingEntries: Array<{ name: string; pricing: UserSupportedModelPricing | null }>
    }).displayPricingEntries
    expect(entries).toHaveLength(1)
    expect(entries[0]?.name).toBe('')
    expect(entries[0]?.pricing?.input_price).toBe(3e-6)
  })

  it('renders interval cache prices including explicit zero', async () => {
    const tiered = pricing('token', 2e-6)
    tiered.intervals = [{
      min_tokens: 0,
      max_tokens: null,
      tier_label: 'Fast',
      input_price: 4e-6,
      output_price: 20e-6,
      cache_write_price: 0,
      cache_read_price: 0.4e-6,
      per_request_price: null,
    }]
    const wrapper = mount(SupportedModelChip, {
      props: { model: { name: 'tiered', platform: 'openai', pricing: tiered } },
      global: { stubs: { Teleport: true, PlatformIcon: true, PricingRow: true } },
    })

    await wrapper.get('[tabindex="0"]').trigger('mouseenter')
    expect(wrapper.text()).toContain('Fast')
    expect(wrapper.text()).toContain('availableChannels.pricing.cacheWritePrice $0')
    expect(wrapper.text()).toContain('availableChannels.pricing.cacheReadPrice $0.4')
  })

  it('shows the token boundary together with a named tier', async () => {
    const tiered = pricing('token', 2e-6)
    tiered.intervals = [{
      min_tokens: 272000,
      max_tokens: null,
      tier_label: 'Long context',
      input_price: 4e-6,
      output_price: 20e-6,
      cache_write_price: null,
      cache_read_price: null,
      per_request_price: null,
    }]
    const wrapper = mount(SupportedModelChip, {
      props: { model: { name: 'tiered', platform: 'openai', pricing: tiered } },
      global: { stubs: { Teleport: true, PlatformIcon: true, PricingRow: true } },
    })

    await wrapper.get('[tabindex="0"]').trigger('mouseenter')
    expect(wrapper.text()).toContain('Long context · >272K')
  })
})
