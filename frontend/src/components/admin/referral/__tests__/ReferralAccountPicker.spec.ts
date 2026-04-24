import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import ReferralAccountPicker from '../ReferralAccountPicker.vue'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, fallback?: string) => fallback || key
  })
}))

describe('ReferralAccountPicker', () => {
  it('renders search results in an overlay so the page layout is not stretched', () => {
    const wrapper = mount(ReferralAccountPicker, {
      props: {
        label: '',
        placeholder: 'Search',
        query: 'user',
        modelValue: null,
        options: [
          { user_id: 1, email: 'user@example.com', username: 'user', referral_code: 'REF001' }
        ],
        inputTestId: 'account-search'
      }
    })

    const option = wrapper.get('[data-test="account-option"]')
    expect(option.element.parentElement?.className).toContain('absolute')
  })

  it('notifies the parent to clear stale results when the query is emptied', async () => {
    const wrapper = mount(ReferralAccountPicker, {
      props: {
        label: '',
        placeholder: 'Search',
        query: 'user',
        modelValue: null,
        options: [
          { user_id: 1, email: 'user@example.com', username: 'user', referral_code: 'REF001' }
        ],
        inputTestId: 'account-search'
      }
    })

    await wrapper.setProps({ query: '' })

    expect(wrapper.emitted('clear')).toBeTruthy()
  })
})
