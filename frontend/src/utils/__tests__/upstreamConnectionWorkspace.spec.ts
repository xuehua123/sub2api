import { describe, it, expect } from 'vitest'
import {
  connectionIsStale,
  groupObservedRates,
  upstreamWalletTotal,
} from '../upstreamConnectionWorkspace'
import type {
  UpstreamConnection,
  UpstreamConnectionRuntimeAccount,
} from '@/api/admin/upstreamConnections'
const connection = (patch: Partial<UpstreamConnection> = {}) =>
  ({
    sync_enabled: true,
    sync_interval_seconds: 60,
    last_synced_at: '2026-09-25T00:00:00Z',
    next_sync_at: '2026-09-25T00:01:00Z',
    ...patch,
  }) as UpstreamConnection
describe('upstream workspace', () => {
  it('sums only finite converted balances, preserving negative and zero values', () => {
    expect(
      upstreamWalletTotal([
        { wallet_usd: 10 },
        { wallet_usd: 0 },
        { wallet_usd: -2 },
        { wallet_usd: null },
        { wallet_usd: 999, wallet_unlimited: true },
      ] as UpstreamConnection[]),
    ).toEqual({ amount: 8, known: 3, unknown: 1, unlimited: 1 })
  })
  it('does not mark disabled sync as stale', () => {
    expect(
      connectionIsStale(
        connection({ sync_enabled: false }),
        Date.parse('2026-09-25T01:00Z'),
      ),
    ).toBe(false)
  })
  it('allows a sync interval before marking delayed sync stale', () => {
    expect(
      connectionIsStale(connection(), Date.parse('2026-09-25T00:01:59Z')),
    ).toBe(false)
    expect(
      connectionIsStale(connection(), Date.parse('2026-09-25T00:02:01Z')),
    ).toBe(true)
  })
  it('maps local groups through account IDs and preserves zero, multiple and unknown rates', () => {
    const row = connection({
      bindings: [
        { account_id: 1, observed_multiplier: 0, fresh_until: '2099-01-01' },
        { account_id: 2, observed_multiplier: 0.8, fresh_until: '2099-01-01' },
        { account_id: 3, observed_multiplier: null },
        { account_id: 4, observed_multiplier: 9 },
      ] as UpstreamConnection['bindings'],
    })
    const runtime_accounts = [
      { account_id: 1, groups: [{ group_id: 7 }] },
      { account_id: 2, groups: [{ group_id: 7 }] },
      { account_id: 3, groups: [{ group_id: 7 }] },
      { account_id: 4, groups: [{ group_id: 8 }] },
    ] as UpstreamConnectionRuntimeAccount[]
    expect(groupObservedRates({ ...row, runtime_accounts }, 7)).toEqual({
      values: [0, 0.8],
      unknown: true,
      stale: false,
    })
    expect(groupObservedRates({ ...row, runtime_accounts }, 9).unknown).toBe(
      true,
    )
  })
})
