export interface GroupDisplayOrderItem {
  id?: number | null
  name?: string | null
  rate_multiplier?: number | null
  sort_order?: number | null
}

export interface GroupDisplayOrderOptions<T> {
  getId?: (item: T) => number | null | undefined
  getName?: (item: T) => string | null | undefined
  getRateMultiplier?: (item: T) => number | null | undefined
  getSortOrder?: (item: T) => number | null | undefined
}

function finiteNumber(value: unknown): number | null {
  const numberValue = typeof value === 'number' ? value : Number(value)
  return Number.isFinite(numberValue) ? numberValue : null
}

function displayRate(value: unknown): number {
  const rate = finiteNumber(value)
  return rate !== null && rate > 0 ? rate : 1
}

function displaySortOrder(value: unknown): number {
  return finiteNumber(value) ?? Number.MAX_SAFE_INTEGER
}

function displayName(value: unknown): string {
  return String(value ?? '').trim().toLowerCase()
}

function displayId(value: unknown): number {
  return finiteNumber(value) ?? Number.MAX_SAFE_INTEGER
}

export function compareGroupsForDisplay<T extends GroupDisplayOrderItem>(
  left: T,
  right: T,
  options: GroupDisplayOrderOptions<T> = {},
): number {
  const leftRate = displayRate(options.getRateMultiplier?.(left) ?? left.rate_multiplier)
  const rightRate = displayRate(options.getRateMultiplier?.(right) ?? right.rate_multiplier)
  if (leftRate !== rightRate) return leftRate - rightRate

  const leftSortOrder = displaySortOrder(options.getSortOrder?.(left) ?? left.sort_order)
  const rightSortOrder = displaySortOrder(options.getSortOrder?.(right) ?? right.sort_order)
  if (leftSortOrder !== rightSortOrder) return leftSortOrder - rightSortOrder

  const leftName = displayName(options.getName?.(left) ?? left.name)
  const rightName = displayName(options.getName?.(right) ?? right.name)
  if (leftName !== rightName) return leftName < rightName ? -1 : 1

  return displayId(options.getId?.(left) ?? left.id) - displayId(options.getId?.(right) ?? right.id)
}

export function sortGroupsForDisplay<T extends GroupDisplayOrderItem>(
  groups: readonly T[],
  options: GroupDisplayOrderOptions<T> = {},
): T[] {
  return [...groups].sort((left, right) => compareGroupsForDisplay(left, right, options))
}
