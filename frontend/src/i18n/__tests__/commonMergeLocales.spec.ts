import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const restoredCommonKeys = [
  'common.submitting',
  'common.selectAll',
  'common.autoRefresh.title',
  'common.autoRefresh.enable',
  'common.autoRefresh.countdown',
  'common.autoRefresh.seconds',
] as const

const restoredAdminAvailableChannelKeys = [
  'admin.availableChannels.title',
  'admin.availableChannels.description',
  'admin.availableChannels.searchPlaceholder',
  'admin.availableChannels.columns.name',
  'admin.availableChannels.columns.status',
  'admin.availableChannels.columns.billingSource',
  'admin.availableChannels.columns.groups',
  'admin.availableChannels.columns.supportedModels',
  'admin.availableChannels.empty',
  'admin.availableChannels.noGroups',
  'admin.availableChannels.noModels',
  'admin.availableChannels.noPricing',
  'admin.availableChannels.statusActive',
  'admin.availableChannels.statusDisabled',
  'admin.availableChannels.billingSource.requested',
  'admin.availableChannels.billingSource.upstream',
  'admin.availableChannels.billingSource.channel_mapped',
  'admin.availableChannels.pricing.billingMode',
  'admin.availableChannels.pricing.billingModeToken',
  'admin.availableChannels.pricing.billingModePerRequest',
  'admin.availableChannels.pricing.billingModeImage',
  'admin.availableChannels.pricing.inputPrice',
  'admin.availableChannels.pricing.outputPrice',
  'admin.availableChannels.pricing.cacheWritePrice',
  'admin.availableChannels.pricing.cacheReadPrice',
  'admin.availableChannels.pricing.imageOutputPrice',
  'admin.availableChannels.pricing.perRequestPrice',
  'admin.availableChannels.pricing.intervals',
  'admin.availableChannels.pricing.unitPerMillion',
  'admin.availableChannels.pricing.unitPerRequest',
] as const

describe('common locale keys restored from upstream merges', () => {
  it.each(restoredCommonKeys)('has zh text for %s', key => {
    expectLocaleValue(zh, key)
  })

  it.each(restoredCommonKeys)('has en text for %s', key => {
    expectLocaleValue(en, key)
  })
})

describe('admin available channels locale keys restored from upstream merges', () => {
  it.each(restoredAdminAvailableChannelKeys)('has zh text for %s', key => {
    expectLocaleValue(zh, key)
  })

  it.each(restoredAdminAvailableChannelKeys)('has en text for %s', key => {
    expectLocaleValue(en, key)
  })
})

function resolveLocaleKey(messages: unknown, key: string): unknown {
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') {
      return undefined
    }
    return (current as Record<string, unknown>)[part]
  }, messages)
}

function expectLocaleValue(messages: unknown, key: string): void {
  const value = resolveLocaleKey(messages, key)
  expect(value).toEqual(expect.any(String))
  expect(value).not.toBe(key)
}
