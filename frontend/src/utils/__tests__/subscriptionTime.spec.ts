import { describe, expect, it } from 'vitest'

import {
  getCycleResetAt,
  getEffectiveCycleStart,
  getEffectiveNextCycleResetAt,
  getPlanValidityDays,
  parsePlanValidityUnit,
} from '../subscriptionTime'

describe('subscriptionTime', () => {
  it('parses supported validity units without coercing invalid ones', () => {
    expect(parsePlanValidityUnit('day')).toBe('day')
    expect(parsePlanValidityUnit('days')).toBe('day')
    expect(parsePlanValidityUnit('week')).toBe('week')
    expect(parsePlanValidityUnit('months')).toBe('month')
    expect(parsePlanValidityUnit('year')).toBe('year')
    expect(parsePlanValidityUnit('wek')).toBeNull()
    expect(parsePlanValidityUnit('')).toBeNull()
  })

  it('converts plan validity units to the backend day-based assignment value', () => {
    expect(getPlanValidityDays(2, 'weeks')).toBe(14)
    expect(getPlanValidityDays(1, 'month')).toBe(30)
    expect(getPlanValidityDays(2, 'years')).toBe(730)
    expect(getPlanValidityDays(7, 'day')).toBe(7)
    expect(getPlanValidityDays(7, 'unknown')).toBe(7)
    expect(getPlanValidityDays(100, 'year')).toBe(36500)
    expect(getPlanValidityDays(101, 'year')).toBeGreaterThan(36500)
  })

  it('keeps a manual reset window as the active cycle anchor', () => {
    const startsAt = new Date(2026, 3, 24, 15, 30, 0, 0)
    const manualWindowStart = new Date(2026, 4, 20, 9, 0, 0, 0)
    const now = new Date(2026, 4, 22, 12, 0, 0, 0)

    expect(getEffectiveCycleStart(manualWindowStart, startsAt, 24 * 30, now)?.getTime()).toBe(
      manualWindowStart.getTime()
    )
    expect(getEffectiveNextCycleResetAt(manualWindowStart, startsAt, 24 * 30, now)?.getTime()).toBe(
      new Date(2026, 5, 19, 9, 0, 0, 0).getTime()
    )
  })

  it('maps legacy midnight anchors back to the aligned subscription cycle', () => {
    const startsAt = new Date(2026, 3, 24, 15, 30, 0, 0)
    const legacyWindowStart = new Date(2026, 3, 25, 0, 0, 0, 0)
    const now = new Date(2026, 4, 20, 10, 0, 0, 0)

    expect(getEffectiveCycleStart(legacyWindowStart, startsAt, 24 * 30, now)?.getTime()).toBe(
      startsAt.getTime()
    )
    expect(getEffectiveNextCycleResetAt(legacyWindowStart, startsAt, 24 * 30, now)?.getTime()).toBe(
      new Date(2026, 4, 24, 15, 30, 0, 0).getTime()
    )
  })

  it('treats RFC3339 UTC midnight strings like the backend legacy-window check', () => {
    const startsAt = '2026-04-24T15:30:00Z'
    const legacyWindowStart = '2026-04-25T00:00:00Z'
    const now = new Date('2026-05-20T10:00:00+08:00')

    expect(getEffectiveCycleStart(legacyWindowStart, startsAt, 24 * 30, now)?.toISOString()).toBe(
      '2026-04-24T15:30:00.000Z'
    )
  })

  it('keeps a future monthly advance anchor as the active cycle anchor', () => {
    const startsAt = '2026-04-22T16:40:26+08:00'
    const advanceWindowStart = '2026-05-22T16:40:26+08:00'
    const now = new Date('2026-05-21T16:31:00+08:00')

    expect(getEffectiveCycleStart(advanceWindowStart, startsAt, 24 * 30, now)?.toISOString()).toBe(
      '2026-05-22T08:40:26.000Z'
    )
    expect(getCycleResetAt(advanceWindowStart, startsAt, 24 * 30, now)?.toISOString()).toBe(
      '2026-06-21T08:40:26.000Z'
    )
  })

  it('falls back to starts_at-aligned reset time when the window start is missing', () => {
    const startsAt = '2026-04-24T15:30:00+08:00'
    const now = new Date('2026-05-20T10:00:00+08:00')

    expect(getCycleResetAt(null, startsAt, 24 * 30, now)?.toISOString()).toBe(
      '2026-05-24T07:30:00.000Z'
    )
  })

  it('keeps missing-window reset fallback consistent for daily and weekly views', () => {
    const startsAt = '2026-05-18T09:00:00+08:00'
    const now = new Date('2026-05-20T10:00:00+08:00')

    expect(getCycleResetAt(null, startsAt, 24, now)?.toISOString()).toBe(
      '2026-05-21T01:00:00.000Z'
    )
    expect(getCycleResetAt(null, startsAt, 24 * 7, now)?.toISOString()).toBe(
      '2026-05-25T01:00:00.000Z'
    )
  })
})
