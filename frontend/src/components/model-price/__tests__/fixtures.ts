import type { ModelPriceModel, ModelPriceResponse } from '@/api/modelPrices'
export function model(
  overrides: Partial<ModelPriceModel> = {},
): ModelPriceModel {
  const result: ModelPriceModel = {
    name: 'test-model',
    platform: 'openai',
    provider: 'openai',
    billing_mode: 'token',
    pricing_source: 'channel',
    official: {
      input_usd_per_m: 3,
      output_usd_per_m: 12,
      cache_write_usd_per_m: null,
      cache_read_usd_per_m: null,
      image_output_usd_per_m: null,
      per_request_usd: null,
    },
    actual: {
      input_usd_per_m: 0.6,
      input_cny_per_m: 4.2,
      output_usd_per_m: 2.4,
      output_cny_per_m: 16.8,
      cache_write_usd_per_m: null,
      cache_write_cny_per_m: null,
      cache_read_usd_per_m: 0.2,
      cache_read_cny_per_m: 1.4,
      image_output_usd_per_m: null,
      image_output_cny_per_m: null,
      per_request_usd: null,
      per_request_cny: null,
    },
    price_tiers: [],
    multiplier: 0.2,
    cheaper_factor: 5,
    channel_names: ['main'],
    official_missing: false,
    custom_price: null,
    hidden: false,
    ...overrides,
  }
  if (!Object.prototype.hasOwnProperty.call(overrides, "catalog_price")) result.catalog_price = {model:result.name,billing_mode:result.billing_mode,price:result.official,tiers:result.price_tiers}
  return result
}
export function response(
  overrides: Partial<ModelPriceResponse> = {},
): ModelPriceResponse {
  return {
    usd_cny_rate: 7,
    cny_per_quota_usd: 0.068,
    groups: [
      {
        id: 46,
        name: 'OpenAI',
        platform: 'openai',
        subscription_type: 'subscription',
        rate_multiplier: 1,
        effective_multiplier: 0.2,
        image_rate_independent: false,
        image_rate_multiplier: 1,
        is_exclusive: false,
        hidden: false,
        model_count: 2,
        channel_count: 1,
      },
      {
        id: 47,
        name: 'Claude',
        platform: 'anthropic',
        subscription_type: 'subscription',
        rate_multiplier: 1,
        effective_multiplier: 1,
        image_rate_independent: false,
        image_rate_multiplier: 1,
        is_exclusive: false,
        hidden: false,
        model_count: 1,
        channel_count: 1,
      },
    ],
    group_overview: [],
    selected_group_id: null,
    models: [],
    summary: { model_count: 0, priced_count: 0, average_cheaper_factor: null },
    include_catalog: false,
    show_hidden_groups: false,
    show_hidden_models: false,
    hidden_group_ids: [],
    hidden_model_keys: [],
    selected_group_hidden: false,
    ...overrides,
  }
}
