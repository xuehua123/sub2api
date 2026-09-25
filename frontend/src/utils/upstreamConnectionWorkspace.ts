import type {
  UpstreamConnection,
  UpstreamConnectionRuntimeAccount,
} from '@/api/admin/upstreamConnections'
export function connectionIsStale(
  row: UpstreamConnection,
  now = Date.now(),
): boolean {
  if (!row.sync_enabled) return false
  if (!row.last_synced_at) return true
  const next = row.next_sync_at
    ? Date.parse(row.next_sync_at)
    : Date.parse(row.last_synced_at) + row.sync_interval_seconds * 1000
  return (
    Number.isFinite(next) &&
    now > next + Math.max(30, row.sync_interval_seconds) * 1000
  )
}
export function groupObservedRates(
  connection: UpstreamConnection & {
    runtime_accounts?: UpstreamConnectionRuntimeAccount[]
  },
  groupID: number,
) {
  const ids = new Set(
    (connection.runtime_accounts ?? [])
      .filter((a) => (a.groups ?? []).some((g) => g.group_id === groupID))
      .map((a) => a.account_id),
  )
  const values = new Set<number>()
  let unknown = false
  let stale = false
  for (const id of ids) {
    const binding = connection.bindings?.find((b) => b.account_id === id)
    if (
      binding?.observed_multiplier == null ||
      !Number.isFinite(binding.observed_multiplier)
    )
      unknown = true
    else {
      values.add(binding.observed_multiplier)
      if (!binding.fresh_until || Date.parse(binding.fresh_until) <= Date.now())
        stale = true
    }
  }
  return {
    values: [...values].sort((a, b) => a - b),
    unknown: unknown || !ids.size,
    stale,
  }
}

export function upstreamWalletTotal(rows: UpstreamConnection[]) {
  let amount = 0,
    unknown = 0,
    unlimited = 0
  for (const row of rows) {
    if (row.wallet_unlimited) {
      unlimited++
      continue
    }
    if (row.wallet_usd == null || !Number.isFinite(row.wallet_usd)) {
      unknown++
      continue
    }
    amount += row.wallet_usd
  }
  return {
    amount,
    unknown,
    unlimited,
    known: rows.length - unknown - unlimited,
  }
}
