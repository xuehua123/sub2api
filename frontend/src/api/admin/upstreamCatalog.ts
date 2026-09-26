import { apiClient } from '../client'
export interface CatalogGroup {
  connection_id: number
  connection_name: string
  management_base_url: string
  provider: string
  remote_key: string
  remote_id: string
  name: string
  rate_multiplier: number | null
  source: string
  confidence: string
  observed_at: string | null
  fresh_until: string | null
  tags: string[]
  favorite: boolean
  account_ids: number[]
  binding_count: number
  freshness: 'fresh' | 'stale' | 'unknown' | 'error'
  auto_tags: string[]
  excluded_auto_tags: string[]
  model_count: number
  model_preview: string[]
  model_status: ModelSnapshot['status']
  model_coverage: ModelSnapshot['coverage']
  models_observed_at: string | null
}
export interface CatalogFilters {
  page: number
  page_size: number
  search?: string
  connection_ids?: string
  provider?: string
  tag?: string
  binding?: string
  freshness?: string
  sort?: string
  favorites?: boolean
  group_by_connection?: boolean
  min_rate?: number
  max_rate?: number
}
export interface CatalogResult {
  items: CatalogGroup[]
  total: number
  page: number
  page_size: number
  tags: string[]
}
export async function getGroupCatalog(
  params: CatalogFilters,
  signal?: AbortSignal,
): Promise<CatalogResult> {
  const { data } = await apiClient.get<CatalogResult>(
    '/admin/upstream-connections/group-catalog',
    { params, signal },
  )
  return data
}
export async function annotateGroups(
  groups: Pick<CatalogGroup, 'connection_id' | 'remote_key'>[],
  update: { add_tags?: string[]; remove_tags?: string[]; favorite?: boolean; reset_auto_tags?: boolean },
): Promise<void> {
  await apiClient.patch('/admin/upstream-connections/group-annotations', {
    groups,
    ...update,
  })
}
export interface ModelSnapshot {
  models: string[]
  auto_tags: string[]
  source: string
  coverage: 'unknown' | 'published' | 'bound_keys'
  status: 'unknown' | 'ready' | 'partial' | 'error' | 'stale' | 'syncing' | 'pending'
  source_count?: number
  ready_source_count?: number
  pending_source_count?: number
  failed_source_count?: number
  stale_source_count?: number
  refresh_in_progress?: boolean
  error_code: string
  observed_at: string | null
  fresh_until: string | null
}
export async function getGroupModels(group: Pick<CatalogGroup, 'connection_id' | 'remote_key'>, signal?: AbortSignal): Promise<ModelSnapshot> {
  const { data } = await apiClient.get<ModelSnapshot>(`/admin/upstream-connections/${group.connection_id}/group-models`, { params: { remote_key: group.remote_key }, signal })
  return data
}
export async function syncGroupModels(group: Pick<CatalogGroup, 'connection_id' | 'remote_key'>, signal?: AbortSignal): Promise<ModelSnapshot> {
  const { data } = await apiClient.post<ModelSnapshot>(`/admin/upstream-connections/${group.connection_id}/group-models/sync`, { remote_key: group.remote_key }, { signal, timeout: 60000 })
  return data
}
export interface CostHistory {
  daily: { date: string; account_cost: number; requests: number }[]
  items: {
    connection_id: number
    name: string
    requests: number
    account_cost: number
  }[]
  start_at: string
  end_at: string
  timezone: string
  total_cost: number
  total_requests: number
}
export async function getCostHistory(
  start: string,
  end: string,
  ids: number[],
  signal?: AbortSignal,
): Promise<CostHistory> {
  const { data } = await apiClient.get<CostHistory>(
    '/admin/upstream-connections/cost-history',
    {
      params: {
        start_date: start,
        end_date: end,
        connection_ids: ids.join(','),
      },
      signal,
    },
  )
  return data
}
