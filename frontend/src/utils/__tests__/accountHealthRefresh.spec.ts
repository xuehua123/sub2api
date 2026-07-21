import { describe, expect, it } from 'vitest'

import {
  accountHealthScopeKey,
  routeWantsHealthDrawer,
  shouldSkipAccountHealthWhenColumnHidden,
  shouldThrottleAccountHealthRefresh,
  shouldUpdateAccountHealthPageThrottle
} from '../accountHealthRefresh'

describe('accountHealthRefresh', () => {
  it('builds a stable sorted scope key for page account sets', () => {
    expect(accountHealthScopeKey([3, 1, 2])).toBe('1,2,3')
    expect(accountHealthScopeKey([2, 1])).not.toBe(accountHealthScopeKey([1, 2, 3]))
    expect(accountHealthScopeKey([])).toBe('')
  })

  it('does not throttle when the account set changes (page/filter/sort)', () => {
    expect(
      shouldThrottleAccountHealthRefresh({
        force: false,
        scopeKey: '1,2',
        lastScopeKey: '3,4',
        lastLoadedAt: Date.now(),
        minIntervalMs: 15_000
      })
    ).toBe(false)
  })

  it('throttles only the same scope within the interval', () => {
    const now = 1_000_000
    expect(
      shouldThrottleAccountHealthRefresh({
        force: false,
        scopeKey: '1,2',
        lastScopeKey: '1,2',
        lastLoadedAt: now - 5_000,
        now,
        minIntervalMs: 15_000
      })
    ).toBe(true)

    expect(
      shouldThrottleAccountHealthRefresh({
        force: true,
        scopeKey: '1,2',
        lastScopeKey: '1,2',
        lastLoadedAt: now - 5_000,
        now,
        minIntervalMs: 15_000
      })
    ).toBe(false)
  })

  it('loads health for drawer/deep-link even when the column is hidden', () => {
    expect(
      shouldSkipAccountHealthWhenColumnHidden({
        columnHidden: true,
        hasAccountIDsOverride: false,
        drawerOpen: false,
        drawerAccountId: null
      })
    ).toBe(true)

    expect(
      shouldSkipAccountHealthWhenColumnHidden({
        columnHidden: true,
        hasAccountIDsOverride: true,
        drawerOpen: false,
        drawerAccountId: null
      })
    ).toBe(false)

    expect(
      shouldSkipAccountHealthWhenColumnHidden({
        columnHidden: true,
        hasAccountIDsOverride: false,
        drawerOpen: true,
        drawerAccountId: null
      })
    ).toBe(false)

    expect(
      shouldSkipAccountHealthWhenColumnHidden({
        columnHidden: true,
        hasAccountIDsOverride: false,
        drawerOpen: false,
        drawerAccountId: 42
      })
    ).toBe(false)

    // Old /admin/ops/account-health bookmark (health=1, often no account_id).
    expect(
      shouldSkipAccountHealthWhenColumnHidden({
        columnHidden: true,
        hasAccountIDsOverride: false,
        drawerOpen: false,
        drawerAccountId: null,
        routeHealthDeepLink: true
      })
    ).toBe(false)

    // Global probe settings must load even when health column is hidden.
    expect(
      shouldSkipAccountHealthWhenColumnHidden({
        columnHidden: true,
        hasAccountIDsOverride: false,
        drawerOpen: false,
        drawerAccountId: null,
        forceSettingsLoad: true
      })
    ).toBe(false)
  })

  it('does not update page throttle state after partial (single-account) fetches', () => {
    expect(shouldUpdateAccountHealthPageThrottle({ isPartialFetch: true })).toBe(false)
    expect(shouldUpdateAccountHealthPageThrottle({ isPartialFetch: false })).toBe(true)
  })

  it('detects health deep-link query flags', () => {
    expect(routeWantsHealthDrawer('1')).toBe(true)
    expect(routeWantsHealthDrawer('true')).toBe(true)
    expect(routeWantsHealthDrawer(['1'])).toBe(true)
    expect(routeWantsHealthDrawer(undefined)).toBe(false)
    expect(routeWantsHealthDrawer('0')).toBe(false)
  })
})
