import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const requiredSettingsFeatureKeys = [
  'admin.settings.tabs.features',
  'admin.settings.features.channelMonitor.title',
  'admin.settings.features.channelMonitor.description',
  'admin.settings.features.channelMonitor.configureLink',
  'admin.settings.features.channelMonitor.enabled',
  'admin.settings.features.channelMonitor.enabledHint',
  'admin.settings.features.channelMonitor.defaultInterval',
  'admin.settings.features.channelMonitor.defaultIntervalHint',
  'admin.settings.features.availableChannels.title',
  'admin.settings.features.availableChannels.description',
  'admin.settings.features.availableChannels.configureLink',
  'admin.settings.features.availableChannels.enabled',
  'admin.settings.features.availableChannels.enabledHint'
] as const

describe('settings feature locale keys', () => {
  it.each(requiredSettingsFeatureKeys)('has zh text for %s', key => {
    expect(resolveLocaleKey(zh, key)).toEqual(expect.any(String))
    expect(resolveLocaleKey(zh, key)).not.toBe(key)
  })

  it.each(requiredSettingsFeatureKeys)('has en text for %s', key => {
    expect(resolveLocaleKey(en, key)).toEqual(expect.any(String))
    expect(resolveLocaleKey(en, key)).not.toBe(key)
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
