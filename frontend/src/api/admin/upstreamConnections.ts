import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export type UpstreamConnectionProvider =
  | 'auto'
  | 'newapi'
  | 'sub2api'
  | 'rixapi'
  | 'shellapi'
  | 'oneapi'
  | 'veloera'
  | 'onehub'
  | 'donehub'

export type UpstreamConnectionAuthMode = 'password' | 'access_token'

export interface UpstreamConnectionCredentialInput {
  username?: string
  password?: string
  access_token?: string
  refresh_token?: string
}

export interface UpstreamGroupObservation {
  id: number
  remote_id: string
  name: string
  rate_multiplier: number | null
  source: string
  confidence: string
  metadata: Record<string, unknown>
  observed_at: string | null
  fresh_until: string | null
}

export interface UpstreamAccountBinding {
  id: number
  account_id: number
  connection_id: number
  remote_token_id: string
  remote_token_name: string
  resolution_kind: string
  remote_group_id: string
  remote_group_name: string
  fallback_groups: string[]
  observed_multiplier: number | null
  confidence: string
  source: string
  apply_policy: 'observe_only'
  status: string
  sync_failures: number
  last_error: string
  resolution_details: Record<string, unknown>
  observed_at: string | null
  fresh_until: string | null
}

export interface UpstreamConnection {
  id: number
  name: string
  provider: UpstreamConnectionProvider
  auth_mode: UpstreamConnectionAuthMode
  management_base_url: string
  forwarding_base_url: string
  credential_configured: boolean
  credential_hint: string
  remote_user_id: string
  proxy_id: number | null
  capabilities: Record<string, unknown>
  status: string
  last_error: string
  sync_enabled: boolean
  sync_interval_seconds: number
  sync_failures: number
  version: number
  wallet_amount: number | null
  wallet_currency: string
  wallet_usd: number | null
  wallet_unlimited: boolean
  wallet_source: string
  wallet_reliability: string
  wallet_observed_at: string | null
  last_discovered_at: string | null
  last_synced_at: string | null
  next_sync_at: string | null
  created_at: string
  updated_at: string
  group_count: number
  binding_count: number
  groups: UpstreamGroupObservation[]
  bindings: UpstreamAccountBinding[]
}

export interface CreateUpstreamConnectionRequest {
  name: string
  provider: UpstreamConnectionProvider
  auth_mode: UpstreamConnectionAuthMode
  management_base_url: string
  forwarding_base_url?: string
  credential: UpstreamConnectionCredentialInput
  remote_user_id?: string
  proxy_id?: number | null
  sync_enabled?: boolean
  sync_interval_seconds?: number
}

export interface UpdateUpstreamConnectionRequest {
  expected_version: number
  name?: string
  provider?: UpstreamConnectionProvider
  auth_mode?: UpstreamConnectionAuthMode
  management_base_url?: string
  forwarding_base_url?: string
  credential?: UpstreamConnectionCredentialInput
  remote_user_id?: string
  proxy_id?: number | null
  clear_proxy?: boolean
  sync_enabled?: boolean
  sync_interval_seconds?: number
}

export interface UpstreamLegacyMigrationSummary {
  scanned_accounts: number
  eligible_accounts: number
  unique_connections: number
  planned_accounts: number
  migrated_accounts: number
  already_migrated: number
  skipped_accounts: number
  failed_accounts: number
}

export interface UpstreamLegacyMigrationItem {
  account_id: number
  account_name: string
  provider: string
  auth_mode: string
  management_base_url: string
  forwarding_base_url: string
  proxy_id: number | null
  legacy_group: string
  action: string
  connection_id: number | null
  message: string
}

export interface UpstreamLegacyMigrationResult {
  dry_run: boolean
  summary: UpstreamLegacyMigrationSummary
  items: UpstreamLegacyMigrationItem[]
  warnings: string[]
}

export async function list(
  page = 1,
  pageSize = 20,
  filters?: { provider?: string; status?: string; search?: string }
): Promise<PaginatedResponse<UpstreamConnection>> {
  const { data } = await apiClient.get<PaginatedResponse<UpstreamConnection>>('/admin/upstream-connections', {
    params: { page, page_size: pageSize, ...filters }
  })
  return data
}

export async function listAll(): Promise<UpstreamConnection[]> {
  const pageSize = 200
  const items: UpstreamConnection[] = []
  for (let page = 1; page <= 100; page += 1) {
    const result = await list(page, pageSize)
    items.push(...result.items)
    if (items.length >= result.total || result.items.length < pageSize) break
  }
  return items
}

export async function get(id: number): Promise<UpstreamConnection> {
  const { data } = await apiClient.get<UpstreamConnection>(`/admin/upstream-connections/${id}`)
  return data
}

export async function create(payload: CreateUpstreamConnectionRequest): Promise<UpstreamConnection> {
  const { data } = await apiClient.post<UpstreamConnection>('/admin/upstream-connections', payload)
  return data
}

export async function update(id: number, payload: UpdateUpstreamConnectionRequest): Promise<UpstreamConnection> {
  const { data } = await apiClient.put<UpstreamConnection>(`/admin/upstream-connections/${id}`, payload)
  return data
}

export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/upstream-connections/${id}`)
}

export async function probe(id: number): Promise<UpstreamConnection> {
  const { data } = await apiClient.post<UpstreamConnection>(`/admin/upstream-connections/${id}/probe`)
  return data
}

export async function bindAccount(connectionId: number, accountId: number): Promise<UpstreamAccountBinding> {
  const { data } = await apiClient.put<UpstreamAccountBinding>(
    `/admin/upstream-connections/${connectionId}/bindings/${accountId}`
  )
  return data
}

export async function getAccountBinding(accountId: number): Promise<UpstreamAccountBinding> {
  const { data } = await apiClient.get<UpstreamAccountBinding>(
    `/admin/upstream-connections/bindings/by-account/${accountId}`
  )
  return data
}

export async function unbindAccount(connectionId: number, accountId: number): Promise<void> {
  await apiClient.delete(`/admin/upstream-connections/${connectionId}/bindings/${accountId}`)
}

export async function previewLegacyMigration(): Promise<UpstreamLegacyMigrationResult> {
  const { data } = await apiClient.post<UpstreamLegacyMigrationResult>(
    '/admin/upstream-connections/migrate-legacy/preview'
  )
  return data
}

export async function migrateLegacy(): Promise<UpstreamLegacyMigrationResult> {
  const { data } = await apiClient.post<UpstreamLegacyMigrationResult>(
    '/admin/upstream-connections/migrate-legacy'
  )
  return data
}

export default {
  list,
  listAll,
  get,
  create,
  update,
  remove,
  probe,
  bindAccount,
  getAccountBinding,
  unbindAccount,
  previewLegacyMigration,
  migrateLegacy
}
