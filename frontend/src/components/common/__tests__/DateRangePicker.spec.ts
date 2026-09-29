import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'

import DateRangePicker from '../DateRangePicker.vue'

const messages: Record<string, string> = {
  'dates.today': 'Today',
  'dates.yesterday': 'Yesterday',
  'dates.last24Hours': 'Last 24 Hours',
  'dates.last7Days': 'Last 7 Days',
  'dates.last14Days': 'Last 14 Days',
  'dates.last30Days': 'Last 30 Days',
  'dates.thisMonth': 'This Month',
  'dates.lastMonth': 'Last Month',
  'dates.startDate': 'Start Date',
  'dates.endDate': 'End Date',
  'dates.apply': 'Apply',
  'dates.selectDateRange': 'Select date range'
}

vi.mock('vue-i18n', () => ({
  createI18n: () => ({ global: { t: (key: string) => key, locale: { value: 'en' } } }),
  useI18n: () => ({
    t: (key: string) => messages[key] ?? key,
    locale: ref('en')
  })
}))

const formatLocalDate = (date: Date): string => {
  const year = date.getFullYear()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

describe('DateRangePicker', () => {
  it('supports calendar presets and blocks invalid custom payment ranges', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-08-01', endDate: '2026-08-31', calendarOnly: true, maxDate: '2026-09-28', maxDays: 366 },
      global: { stubs: { Icon: true } }
    })
    await wrapper.find('.date-picker-trigger').trigger('click')
    expect(wrapper.text()).toContain('This Month')
    expect(wrapper.text()).toContain('Last Month')
    expect(wrapper.text()).not.toContain('Last 24 Hours')
    const inputs = wrapper.findAll('input')
    await inputs[0].setValue('2024-01-01')
    expect(wrapper.find('.date-picker-apply').attributes('disabled')).toBeDefined()
    await inputs[0].setValue('2026-08-01')
    await inputs[1].setValue('2026-09-29')
    expect(wrapper.find('.date-picker-apply').attributes('disabled')).toBeDefined()
    await inputs[1].setValue('2026-08-31')
    await wrapper.find('.date-picker-apply').trigger('click')
    expect(wrapper.emitted('change')?.[0]).toEqual([expect.objectContaining({ startDate: '2026-08-01', endDate: '2026-08-31' })])
    wrapper.unmount()
  })
  it('uses last 24 hours as the default recognized preset', () => {
    const now = new Date()
    const yesterday = new Date(now.getTime() - 24 * 60 * 60 * 1000)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: formatLocalDate(yesterday),
        endDate: formatLocalDate(now)
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    expect(wrapper.text()).toContain('Last 24 Hours')
  })

  it('emits range updates with last24Hours preset when applied', async () => {
    const now = new Date()
    const today = formatLocalDate(now)

    const wrapper = mount(DateRangePicker, {
      props: {
        startDate: today,
        endDate: today
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    await wrapper.find('.date-picker-trigger').trigger('click')
    const presetButton = wrapper.findAll('.date-picker-preset').find((node) =>
      node.text().includes('Last 24 Hours')
    )
    expect(presetButton).toBeDefined()

    await presetButton!.trigger('click')
    await wrapper.find('.date-picker-apply').trigger('click')

    const nowAfterClick = new Date()
    const yesterdayAfterClick = new Date(nowAfterClick.getTime() - 24 * 60 * 60 * 1000)
    const expectedStart = formatLocalDate(yesterdayAfterClick)
    const expectedEnd = formatLocalDate(nowAfterClick)

    expect(wrapper.emitted('update:startDate')?.[0]).toEqual([expectedStart])
    expect(wrapper.emitted('update:endDate')?.[0]).toEqual([expectedEnd])
    expect(wrapper.emitted('change')?.[0]).toEqual([
      {
        startDate: expectedStart,
        endDate: expectedEnd,
        preset: 'last24Hours'
      }
    ])
  })

  it('does not show an unapplied draft after dismissing the menu', async () => {
    const wrapper = mount(DateRangePicker, {
      props: { startDate: '2026-08-02', endDate: '2026-08-31', calendarOnly: true },
      global: { stubs: { Icon: true } }
    })

    const trigger = wrapper.find('.date-picker-trigger')
    await trigger.trigger('click')
    const lastMonth = wrapper.findAll('.date-picker-preset').find((node) => node.text().includes('Last Month'))
    await lastMonth!.trigger('click')
    expect(wrapper.find('.date-picker-value').text()).toContain('Aug')
    expect(wrapper.emitted('change')).toBeUndefined()

    document.body.click()
    await wrapper.vm.$nextTick()
    expect(wrapper.find('.date-picker-value').text()).toContain('Aug')
    expect(wrapper.find('.date-picker-value').text()).not.toContain('Last Month')
    expect(wrapper.emitted('change')).toBeUndefined()
  })
})
