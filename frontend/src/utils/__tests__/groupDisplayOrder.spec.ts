import { describe, expect, it } from 'vitest'
import { sortGroupsForDisplay } from '../groupDisplayOrder'

describe('sortGroupsForDisplay', () => {
  it('uses low multiplier first when groups have the default sort order', () => {
    const groups = [
      { id: 1, name: 'pro', rate_multiplier: 3.3, sort_order: 0 },
      { id: 2, name: 'freebie', rate_multiplier: 0.68, sort_order: 0 },
      { id: 3, name: 'max', rate_multiplier: 16, sort_order: 0 },
    ]

    expect(sortGroupsForDisplay(groups).map((group) => group.id)).toEqual([2, 1, 3])
  })

  it('uses custom sort order only as a tie breaker for equal multipliers', () => {
    const groups = [
      { id: 1, name: 'cheap', rate_multiplier: 0.68, sort_order: 20 },
      { id: 2, name: 'expensive', rate_multiplier: 16, sort_order: 10 },
    ]

    expect(sortGroupsForDisplay(groups).map((group) => group.id)).toEqual([1, 2])
  })

  it('keeps custom order when multipliers are the same', () => {
    const groups = [
      { id: 1, name: 'second', rate_multiplier: 1, sort_order: 20 },
      { id: 2, name: 'first', rate_multiplier: 1, sort_order: 10 },
    ]

    expect(sortGroupsForDisplay(groups).map((group) => group.id)).toEqual([2, 1])
  })

  it('can sort by an effective user multiplier', () => {
    const groups = [
      { id: 1, name: 'default cheap', rate_multiplier: 1, sort_order: 0, userRate: 9 },
      { id: 2, name: 'user cheap', rate_multiplier: 5, sort_order: 0, userRate: 0.5 },
    ]

    expect(sortGroupsForDisplay(groups, {
      getRateMultiplier: (group) => group.userRate,
    }).map((group) => group.id)).toEqual([2, 1])
  })
})
