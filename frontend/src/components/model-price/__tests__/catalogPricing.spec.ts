import {describe,it,expect} from 'vitest'
import {catalogModel} from '../catalogPricing'
import {estimate,type Usage} from '../pricing'
import {model} from './fixtures'
describe('official catalog pricing',()=>{
 it('uses catalog prices even when channel and custom prices disagree',()=>{const m=model({pricing_source:'custom',multiplier:0.01});m.official={...m.official,input_usd_per_m:999};m.actual.input_cny_per_m=0.01;const p=catalogModel(m,7);expect(p.official.input_usd_per_m).toBe(3);expect(p.actual.input_cny_per_m).toBe(21);expect(p.multiplier).toBe(1);expect(p.custom_price).toBeNull()})
 it('fails closed on old servers and missing catalog entries',()=>{for(const catalog_price of [undefined,null]){const p=catalogModel(model({catalog_price}),7);expect(p.official.input_usd_per_m).toBeNull();expect(p.actual.input_cny_per_m).toBeNull();expect(p.official_missing).toBe(true)}})
 it('preserves zero official prices and never treats absent exchange rates as free',()=>{const m=model();m.catalog_price!.price.input_usd_per_m=0;expect(catalogModel(m,7).actual.input_cny_per_m).toBe(0);expect(catalogModel(m,0).actual.input_cny_per_m).toBeNull()})
 it('estimates only original price times currency conversion',()=>{const p=catalogModel(model(),7);const usage:Usage={input:1e6,output:0,cache_read:0,cache_write:0,cache_write_5m:0,cache_write_1h:0,image_input:0,image_output:0,quantity:1,requests:2};expect(estimate(p,'base',usage).total).toBe(42)})
})
