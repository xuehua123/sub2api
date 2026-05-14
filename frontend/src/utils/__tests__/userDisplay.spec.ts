import { describe, expect, it } from 'vitest'
import { leadingGraphemes, maskEmailForDisplay, userDisplayName } from '../userDisplay'

describe('user display helpers', () => {
  it('masks fallback email with exactly three middle asterisks', () => {
    expect(maskEmailForDisplay('LoveJsXuehua@Example.COM')).toBe('lovej***ehua@example.com')
    expect(maskEmailForDisplay('abc@qq.com')).toBe('a***@qq.com')
  })

  it('prefers emoji usernames over masked email fallback', () => {
    expect(userDisplayName('🚀皮皮虾', 'user@example.com')).toBe('🚀皮皮虾')
    expect(userDisplayName('', 'abcdef@example.com')).toBe('ab***f@example.com')
  })

  it('does not split basic emoji when building avatar text', () => {
    expect(leadingGraphemes('🚀皮皮虾', 2)).toBe('🚀皮')
    expect(leadingGraphemes('👨‍👩‍👧‍👦家庭', 2)).toBe('👨‍👩‍👧‍👦家')
  })
})
