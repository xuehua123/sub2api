import { describe, expect, it } from 'vitest'
import en from '../locales/en'
import zh from '../locales/zh'

describe('issue center locale messages', () => {
  it.each([
    'nav.issueCenter',
    'nav.issueManagement',
    'issueCenter.title',
    'issueCenter.new.title',
    'issueCenter.detail.title',
    'issueCenter.detail.adminManage',
    'issueCenter.feed.mine',
    'issueCenter.admin.title',
    'issueCenter.admin.detailTitle',
    'issueCenter.admin.keySearchWarning',
    'issueCenter.admin.errors.reasonRequired',
    'issueCenter.search.placeholder',
    'issueCenter.status.open',
    'issueCenter.category.payment',
    'issueCenter.severity.blocked',
    'issueCenter.language.en',
  ])('has zh and en text for %s', (key) => {
    expect(resolveLocaleKey(zh, key)).toEqual(expect.any(String))
    expect(resolveLocaleKey(en, key)).toEqual(expect.any(String))
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
