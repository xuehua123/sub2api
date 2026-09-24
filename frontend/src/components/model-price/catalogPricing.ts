import type {ModelPriceModel,ModelPriceValue,ModelPriceActual} from '@/api/modelPrices'
import {validPrice,priceFields} from './pricing'

// An old server may omit catalog_price. Never substitute the overridden
// historical "official" field, which is actually the runtime billing baseline.
export function catalogModel(model:ModelPriceModel,exchangeRate:number):ModelPriceModel {
 const catalog=model.catalog_price
 const empty:ModelPriceValue={input_usd_per_m:null,output_usd_per_m:null,cache_write_usd_per_m:null,cache_read_usd_per_m:null,image_output_usd_per_m:null,per_request_usd:null}
 const convert=(price:ModelPriceValue):ModelPriceActual=>{
  const actual={} as ModelPriceActual
  for(const f of priceFields){const usd=price[f.usd];actual[f.usd]=validPrice(usd)?usd:null;actual[f.cny]=validPrice(usd)&&validPrice(exchangeRate)&&exchangeRate>0?usd*exchangeRate:null}
  return actual
 }
 const price=catalog?.price||empty
 return {...model,billing_mode:catalog?.billing_mode||'unknown',pricing_source:catalog?'official':'unknown',official:price,actual:convert(price),price_tiers:(catalog?.tiers||[]).map(t=>({...t,actual:convert(t.official)})),official_missing:!catalog,custom_price:null,multiplier:1,cheaper_factor:null}
}
