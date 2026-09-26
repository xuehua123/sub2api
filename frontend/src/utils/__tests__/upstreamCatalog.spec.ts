import { describe, expect, it } from 'vitest'
import { upstreamWebsite } from '../upstreamCatalog'

describe('upstream website', () => {
  it('preserves a site path prefix and removes API suffixes and tokens', () => {
    expect(upstreamWebsite('https://example.invalid/site/api/v1?token=secret#fragment')).toBe('https://example.invalid/site')
    expect(upstreamWebsite('https://example.invalid')).toBe('https://example.invalid/')
  })
  it('rejects executable URLs, malformed values and embedded credentials', () => {
    for (const raw of ['javascript:alert(1)', '//example.invalid', 'data:text/html,test', 'https://user:secret@example.invalid']) {
      expect(upstreamWebsite(raw)).toBe('')
    }
  })
})
