/**
 * Helpers for AccountsView health-column refresh decisions.
 * Kept pure so page-change throttle and deep-link bypass can be unit-tested.
 */

export function accountHealthScopeKey(ids: number[]): string {
  return ids
    .filter((id) => Number.isFinite(id) && id > 0)
    .slice()
    .sort((a, b) => a - b)
    .join(',')
}

export function shouldSkipAccountHealthWhenColumnHidden(options: {
  columnHidden: boolean
  hasAccountIDsOverride: boolean
  drawerOpen: boolean
  drawerAccountId: number | null | undefined
  /** True when route has health=1 from old /ops/account-health bookmarks (may lack account_id). */
  routeHealthDeepLink?: boolean
}): boolean {
  if (!options.columnHidden) return false
  // Drawer / deep-link must still load data when the column itself is hidden.
  if (options.hasAccountIDsOverride) return false
  if (options.drawerOpen) return false
  if (options.drawerAccountId != null) return false
  if (options.routeHealthDeepLink) return false
  return true
}

export function shouldThrottleAccountHealthRefresh(options: {
  force: boolean
  scopeKey: string
  lastScopeKey: string
  lastLoadedAt: number
  now?: number
  minIntervalMs: number
}): boolean {
  if (options.force) return false
  if (!options.lastLoadedAt) return false
  // Different account set (page/filter/sort) must never be throttled.
  if (options.scopeKey !== options.lastScopeKey) return false
  const now = options.now ?? Date.now()
  return now - options.lastLoadedAt < options.minIntervalMs
}

/** Whether a successful health fetch should update page-level throttle timestamps. */
export function shouldUpdateAccountHealthPageThrottle(options: {
  isPartialFetch: boolean
}): boolean {
  // Single-account / drawer fetches must not rewrite the page scope key or lastLoadedAt.
  return !options.isPartialFetch
}

export function routeWantsHealthDrawer(healthQuery: unknown): boolean {
  const value = Array.isArray(healthQuery) ? healthQuery[0] : healthQuery
  return value === '1' || value === 'true'
}
