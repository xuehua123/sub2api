import { getRemainingDurationParts } from './subscriptionQuota'

export type PlanValidityUnit = 'day' | 'week' | 'month' | 'year'
export const MAX_PLAN_VALIDITY_DAYS = 36500

export function formatRemainingDurationCompact(
  targetAt: Date | string,
  now: Date = new Date()
): string | null {
  const parts = getRemainingDurationParts(targetAt, now)
  if (!parts) return null

  if (parts.days > 0) {
    return `${parts.days}d ${parts.hours}h`
  }

  if (parts.hours > 0) {
    return `${parts.hours}h ${parts.minutes}m`
  }

  return `${Math.max(parts.minutes, 1)}m`
}

export function getRemainingHours(targetAt: Date | string, now: Date = new Date()): number | null {
  const targetTime = targetAt instanceof Date ? targetAt.getTime() : new Date(targetAt).getTime()
  const nowTime = now.getTime()
  if (!Number.isFinite(targetTime) || !Number.isFinite(nowTime)) return null
  return (targetTime - nowTime) / (1000 * 60 * 60)
}

export function parsePlanValidityUnit(unit: string | null | undefined): PlanValidityUnit | null {
  switch ((unit || '').trim().toLowerCase()) {
    case 'day':
    case 'days':
      return 'day'
    case 'week':
    case 'weeks':
      return 'week'
    case 'month':
    case 'months':
      return 'month'
    case 'year':
    case 'years':
      return 'year'
    default:
      return null
  }
}

export function getPlanValidityDays(days: number, unit: string | null | undefined): number {
  const value = Number.isFinite(days) && days > 0 ? days : 0
  switch (parsePlanValidityUnit(unit)) {
    case 'week':
      return value * 7
    case 'month':
      return value * 30
    case 'year':
      return value * 365
    default:
      return value
  }
}

export function normalizePlanValidityUnit(
  unit: string | null | undefined,
  fallback: PlanValidityUnit = 'day'
): PlanValidityUnit {
  return parsePlanValidityUnit(unit) ?? fallback
}

function resolveTime(value: Date | string | null | undefined): Date | null {
  if (!value) return null
  const resolved = value instanceof Date ? value : new Date(value)
  return Number.isFinite(resolved.getTime()) ? resolved : null
}

function parseStartOfDayParts(value: string): [number, number, number, string | null] | null {
  const match = value.trim().match(/[T\s](\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?(?:Z|[+\-]\d{2}:\d{2})?$/)
  if (!match) return null
  return [Number(match[1]), Number(match[2]), Number(match[3]), match[4] ?? null]
}

function isStartOfDayValue(value: Date | string | null | undefined, resolvedValue?: Date): boolean {
  if (typeof value === 'string') {
    const timeParts = parseStartOfDayParts(value)
    if (timeParts) {
      const [hours, minutes, seconds, fraction] = timeParts
      const fractionIsZero = fraction == null || /^0+$/.test(fraction)
      return hours === 0 && minutes === 0 && seconds === 0 && fractionIsZero
    }
  }

  const resolved = resolvedValue ?? resolveTime(value)
  if (!resolved) return false

  return (
    resolved.getHours() === 0 &&
    resolved.getMinutes() === 0 &&
    resolved.getSeconds() === 0 &&
    resolved.getMilliseconds() === 0
  )
}

function isAlignedCycleAnchor(windowStart: Date, startsAt: Date, cycleMs: number): boolean {
  if (cycleMs <= 0 || windowStart.getTime() < startsAt.getTime()) return false
  return (windowStart.getTime() - startsAt.getTime()) % cycleMs === 0
}

function isLegacyCycleAnchor(
  windowStartValue: Date | string | null | undefined,
  windowStart: Date,
  startsAt: Date,
  cycleMs: number
): boolean {
  if (cycleMs <= 0 || !isStartOfDayValue(windowStartValue, windowStart)) return false
  if (windowStart.getTime() < startsAt.getTime()) return true
  return !isAlignedCycleAnchor(windowStart, startsAt, cycleMs)
}

function advanceWindowStart(windowStart: Date, cycleHours: number, now: Date): Date {
  const cycleMs = cycleHours * 60 * 60 * 1000
  if (!Number.isFinite(windowStart.getTime()) || !Number.isFinite(now.getTime()) || cycleMs <= 0) {
    return windowStart
  }
  if (now.getTime() < windowStart.getTime() + cycleMs) {
    return windowStart
  }

  const elapsed = now.getTime() - windowStart.getTime()
  const steps = Math.max(Math.floor(elapsed / cycleMs), 1)
  return new Date(windowStart.getTime() + steps * cycleMs)
}

export function getAlignedCycleStart(
  startsAt: Date | string | null | undefined,
  cycleHours: number,
  now: Date = new Date()
): Date | null {
  const startTime = startsAt instanceof Date ? startsAt.getTime() : startsAt ? new Date(startsAt).getTime() : Number.NaN
  const nowTime = now.getTime()
  const cycleMs = cycleHours * 60 * 60 * 1000

  if (!Number.isFinite(startTime) || !Number.isFinite(nowTime) || cycleMs <= 0) return null
  if (nowTime < startTime) return new Date(startTime)

  const steps = Math.floor((nowTime - startTime) / cycleMs)
  return new Date(startTime + steps * cycleMs)
}

export function getNextCycleResetAt(
  startsAt: Date | string | null | undefined,
  cycleHours: number,
  now: Date = new Date()
): Date | null {
  const currentStart = getAlignedCycleStart(startsAt, cycleHours, now)
  if (!currentStart) return null
  return new Date(currentStart.getTime() + cycleHours * 60 * 60 * 1000)
}

export function getEffectiveCycleStart(
  windowStart: Date | string | null | undefined,
  startsAt: Date | string | null | undefined,
  cycleHours: number,
  now: Date = new Date()
): Date | null {
  const resolvedWindowStart = resolveTime(windowStart)
  if (!resolvedWindowStart) return null

  const windowBasedStart = advanceWindowStart(resolvedWindowStart, cycleHours, now)
  const alignedStart = getAlignedCycleStart(startsAt, cycleHours, now)
  const resolvedStartsAt = resolveTime(startsAt)
  const cycleMs = cycleHours * 60 * 60 * 1000

  if (resolvedWindowStart.getTime() > now.getTime()) {
    return windowBasedStart
  }

  if (
    alignedStart &&
    resolvedStartsAt &&
    (isLegacyCycleAnchor(windowStart, resolvedWindowStart, resolvedStartsAt, cycleMs) ||
      isAlignedCycleAnchor(resolvedWindowStart, resolvedStartsAt, cycleMs))
  ) {
    return alignedStart
  }

  return windowBasedStart
}

export function getEffectiveNextCycleResetAt(
  windowStart: Date | string | null | undefined,
  startsAt: Date | string | null | undefined,
  cycleHours: number,
  now: Date = new Date()
): Date | null {
  const currentStart = getEffectiveCycleStart(windowStart, startsAt, cycleHours, now)
  if (!currentStart) return null
  return new Date(currentStart.getTime() + cycleHours * 60 * 60 * 1000)
}

export function getCycleResetAt(
  windowStart: Date | string | null | undefined,
  startsAt: Date | string | null | undefined,
  cycleHours: number,
  now: Date = new Date()
): Date | null {
  return (
    getEffectiveNextCycleResetAt(windowStart, startsAt, cycleHours, now) ??
    getNextCycleResetAt(startsAt, cycleHours, now)
  )
}
