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
  is_exclusive: boolean
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
  output_usd_per_m: number | null
  cache_write_usd_per_m: number | null
  cache_read_usd_per_m: number | null
  image_output_usd_per_m: number | null
  per_request_usd: number | null
}

export interface ModelPriceActual {
  input_usd_per_m: number | null
  input_cny_per_m: number | null
  output_usd_per_m: number | null
  output_cny_per_m: number | null
  cache_write_usd_per_m: number | null
  cache_write_cny_per_m: number | null
  cache_read_usd_per_m: number | null
  cache_read_cny_per_m: number | null
  image_output_usd_per_m: number | null
  image_output_cny_per_m: number | null
  per_request_usd: number | null
  per_request_cny: number | null
}

export interface ModelPriceTier {
  key: string
  label: string
  threshold_tokens?: number
  official: ModelPriceValue
  actual: ModelPriceActual
}

export interface ModelPriceModel {
  name: string
  platform: string
  provider: string
  billing_mode: string
  pricing_source: 'official' | 'channel' | 'unknown' | string
  official: ModelPriceValue
  actual: ModelPriceActual
  price_tiers: ModelPriceTier[]
  multiplier: number
  cheaper_factor: number | null
  channel_names: string[]
  official_missing: boolean
}

export interface ModelPriceSummary {
  model_count: number
  priced_count: number
  average_cheaper_factor: number | null
}

export interface ModelPriceResponse {
  usd_cny_rate: number
  groups: ModelPriceGroup[]
  selected_group_id: number | null
  models: ModelPriceModel[]
  summary: ModelPriceSummary
}

export async function getModelPrices(params?: { group_id?: number; signal?: AbortSignal }): Promise<ModelPriceResponse> {
  const { data } = await apiClient.get<ModelPriceResponse>('/model-prices', {
    params: params?.group_id ? { group_id: params.group_id } : undefined,
    signal: params?.signal
  })
  return data
}

export const modelPricesAPI = { getModelPrices }

export default modelPricesAPI
