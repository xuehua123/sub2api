import { apiClient } from './client'

export interface ModelPriceGroup {
  id: number
  name: string
  platform: string
  subscription_type: string
  rate_multiplier: number
  effective_multiplier: number
  user_rate_multiplier?: number
  image_rate_independent: boolean
  image_rate_multiplier: number
  video_rate_independent?: boolean
  video_rate_multiplier?: number
  is_exclusive: boolean
  hidden: boolean
  model_count: number
  channel_count: number
  best_plan?: ModelPricePlan
}

export interface ModelPricePlan {
  id: number
  name: string
  price_cny: number
  quota_usd: number
  cny_per_quota_usd: number
  usd_multiplier: number
}

export interface ModelPriceValue {
  input_usd_per_m: number | null
  image_input_usd_per_m?: number | null
  output_usd_per_m: number | null
  cache_write_usd_per_m: number | null
  cache_write_5m_usd_per_m?: number | null
  cache_write_1h_usd_per_m?: number | null
  cache_read_usd_per_m: number | null
  image_output_usd_per_m: number | null
  per_request_usd: number | null
}

export interface ModelPriceActual {
  input_usd_per_m: number | null
  input_cny_per_m: number | null
  image_input_usd_per_m?: number | null
  image_input_cny_per_m?: number | null
  output_usd_per_m: number | null
  output_cny_per_m: number | null
  cache_write_usd_per_m: number | null
  cache_write_cny_per_m: number | null
  cache_write_5m_usd_per_m?: number | null
  cache_write_5m_cny_per_m?: number | null
  cache_write_1h_usd_per_m?: number | null
  cache_write_1h_cny_per_m?: number | null
  cache_read_usd_per_m: number | null
  cache_read_cny_per_m: number | null
  image_output_usd_per_m: number | null
  image_output_cny_per_m: number | null
  per_request_usd: number | null
  per_request_cny: number | null
}

export interface ModelPriceCustomPrice {
  billing_mode?: string
  input_usd_per_m?: number | null
  output_usd_per_m?: number | null
  cache_write_usd_per_m?: number | null
  cache_read_usd_per_m?: number | null
  image_output_usd_per_m?: number | null
  per_request_usd?: number | null
  updated_at?: string
}

export interface ModelPriceTier {
  key: string
  label: string
  threshold_tokens?: number
  requires_account_long_context?: boolean
  official: ModelPriceValue
  actual: ModelPriceActual
}

export interface ModelPriceModel {
  name: string
  platform: string
  provider: string
  billing_mode: string
  pricing_source: 'official' | 'channel' | 'group' | 'unknown' | string
  official: ModelPriceValue
  actual: ModelPriceActual
  price_tiers: ModelPriceTier[]
  multiplier: number
  cheaper_factor: number | null
  channel_names: string[]
  official_missing: boolean
  custom_price?: ModelPriceCustomPrice | null
  hidden: boolean
}

export interface ModelPriceSummary {
  model_count: number
  priced_count: number
  average_cheaper_factor: number | null
}

export interface ModelPriceGroupOverview {
  category: 'claude' | 'openai' | 'gemini' | 'domestic' | string
  group_count: number
  model_count: number
  channel_count: number
}

export interface ModelPriceCatalogStatus {
  model_count: number
  last_updated?: string
  local_hash?: string
}

export interface ModelPriceResponse {
  usd_cny_rate: number
  cny_per_quota_usd: number
  groups: ModelPriceGroup[]
  group_overview: ModelPriceGroupOverview[]
  selected_group_id: number | null
  models: ModelPriceModel[]
  summary: ModelPriceSummary
  catalog_status?: ModelPriceCatalogStatus | null
  include_catalog: boolean
  show_hidden_groups: boolean
  show_hidden_models: boolean
  hidden_group_ids: number[]
  hidden_model_keys: string[]
  selected_group_hidden: boolean
}

export interface GetModelPricesParams {
  group_id?: number
  include_catalog?: boolean
  show_hidden_groups?: boolean
  show_hidden_models?: boolean
  signal?: AbortSignal
}

export async function getModelPrices(params?: GetModelPricesParams): Promise<ModelPriceResponse> {
  const query: Record<string, number | boolean> = {}
  if (params?.group_id) query.group_id = params.group_id
  if (params?.include_catalog) query.include_catalog = true
  if (params?.show_hidden_groups) query.show_hidden_groups = true
  if (params?.show_hidden_models) query.show_hidden_models = true
  const { data } = await apiClient.get<ModelPriceResponse>('/model-prices', {
    params: Object.keys(query).length > 0 ? query : undefined,
    signal: params?.signal
  })
  return data
}

export async function syncCatalog(): Promise<ModelPriceCatalogStatus> {
  const { data } = await apiClient.post<ModelPriceCatalogStatus>('/admin/model-prices/sync-catalog')
  return data
}

export async function updateHiddenGroups(hiddenGroupIDs: number[]): Promise<{ hidden_group_ids: number[] }> {
  const { data } = await apiClient.patch<{ hidden_group_ids: number[] }>('/admin/model-prices/hidden-groups', {
    hidden_group_ids: hiddenGroupIDs,
  })
  return data
}

export async function updateHiddenModel(groupID: number, model: string, hidden: boolean): Promise<{ hidden_model_keys: string[] }> {
  const { data } = await apiClient.patch<{ hidden_model_keys: string[] }>('/admin/model-prices/hidden-model', {
    group_id: groupID,
    model,
    hidden,
  })
  return data
}

export async function updateHiddenModels(groupID: number, models: string[], hidden: boolean): Promise<{ hidden_model_keys: string[] }> {
  const { data } = await apiClient.patch<{ hidden_model_keys: string[] }>('/admin/model-prices/hidden-model', {
    group_id: groupID,
    models,
    hidden,
  })
  return data
}

export interface UpdateModelPriceCustomPriceRequest extends ModelPriceCustomPrice {
  group_id: number
  model: string
  clear?: boolean
}

export async function updateCustomPrice(req: UpdateModelPriceCustomPriceRequest): Promise<{ custom_prices: Record<string, ModelPriceCustomPrice> }> {
  const { data } = await apiClient.patch<{ custom_prices: Record<string, ModelPriceCustomPrice> }>('/admin/model-prices/custom-price', req)
  return data
}

export const modelPricesAPI = { getModelPrices, syncCatalog, updateHiddenGroups, updateHiddenModel, updateHiddenModels, updateCustomPrice }

export default modelPricesAPI
