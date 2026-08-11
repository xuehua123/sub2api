import { describe, expect, it } from 'vitest'

import {
  resolvePublicApiBaseUrl,
  resolvePublicDocumentationUrl,
  resolvePublicHomeContent,
} from '../publicEndpoint'

const cn2 = {
  hostname: 'cn2.ppxcode.com',
  origin: 'https://cn2.ppxcode.com',
} as Location

const unknownHost = {
  hostname: 'example.com',
  origin: 'https://example.com',
} as Location

describe('public endpoint routing', () => {
  it('uses the current trusted edge origin for user-facing API configuration', () => {
    expect(resolvePublicApiBaseUrl('https://api.psydo.top/', cn2)).toBe('https://cn2.ppxcode.com')
    expect(resolvePublicApiBaseUrl('https://api.psydo.top/', unknownHost)).toBe('https://api.psydo.top/')
  })

  it('maps canonical documentation to the edge documentation host without changing its path', () => {
    expect(resolvePublicDocumentationUrl('https://doc.psydo.top/guide?tab=1#clients', cn2))
      .toBe('https://doc.ppxcode.com/guide?tab=1#clients')
  })

  it('does not rewrite arbitrary documentation URLs', () => {
    expect(resolvePublicDocumentationUrl('https://docs.example.com/guide', cn2))
      .toBe('https://docs.example.com/guide')
  })

  it('rewrites only the configured canonical homepage URL', () => {
    expect(resolvePublicHomeContent(
      'https://api.psydo.top/home/?campaign=summer#top',
      'https://api.psydo.top/',
      cn2,
    )).toBe('https://cn2.ppxcode.com/home/?campaign=summer#top')
    expect(resolvePublicHomeContent('https://api.psydo.top/home', 'https://api.psydo.top/', cn2))
      .toBe('https://cn2.ppxcode.com/home')
  })

  it('leaves arbitrary homepage content and unknown hosts unchanged', () => {
    expect(resolvePublicHomeContent('https://example.com/home/', 'https://api.psydo.top/', cn2))
      .toBe('https://example.com/home/')
    expect(resolvePublicHomeContent('https://api.psydo.top/home/', 'https://api.psydo.top/', unknownHost))
      .toBe('https://api.psydo.top/home/')
  })
})
