import { describe, expect, it } from 'vitest'
import {
  availableTagOptions,
  matchesTagFilter,
  normalizeTags,
  TAG_MAX_COUNT,
  TAG_MAX_LENGTH
} from '../accountHealthTags'

describe('accountHealthTags', () => {
  it('normalizes tags with trimming, unicode, case-insensitive dedupe, length and count limits', () => {
    const longTag = '长'.repeat(TAG_MAX_LENGTH + 8)
    const input = [' pro ', 'PRO', '', 'plus', '生图', longTag]
    for (let index = 0; index < 30; index += 1) {
      input.push(`tag-${index}`)
    }

    const tags = normalizeTags(input)

    expect(tags).toHaveLength(TAG_MAX_COUNT)
    expect(tags.slice(0, 5)).toEqual(['pro', 'plus', '生图', '长'.repeat(TAG_MAX_LENGTH), 'tag-0'])
    expect(tags).not.toContain('')
  })

  it('matches tag filters in any and all modes', () => {
    const tags = ['pro', '生图', 'CCMAX']

    expect(matchesTagFilter(tags, ['plus', '生图'], 'any')).toBe(true)
    expect(matchesTagFilter(tags, ['pro', 'ccmax'], 'all')).toBe(true)
    expect(matchesTagFilter(tags, ['pro', 'plus'], 'all')).toBe(false)
    expect(matchesTagFilter(tags, [], 'any')).toBe(true)
  })

  it('merges default tag options with returned tags and counts matches', () => {
    const items: Array<{ tags: string[] }> = [
      { tags: ['pro', '生图'] },
      { tags: ['PLUS', 'ccmax'] },
      { tags: ['custom'] }
    ]
    const options = availableTagOptions(items)

    expect(options.map(option => option.tag).slice(0, 5)).toEqual(['pro', 'plus', '生图', 'ccmax', 'custom'])
    expect(options.find(option => option.tag === 'plus')?.count).toBe(1)
    expect(options.find(option => option.tag === 'custom')?.count).toBe(1)
  })
})
