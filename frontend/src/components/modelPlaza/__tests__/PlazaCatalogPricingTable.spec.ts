import {describe,it,expect} from 'vitest'
import {mount} from '@vue/test-utils'
import Table from '../PlazaCatalogPricingTable.vue'
import {model} from '@/components/model-price/__tests__/fixtures'
import type {PlazaModel} from '@/api/modelPlaza'
describe('public original prices',()=>{
 it('uses only catalog_price, never discounted channel pricing',()=>{const catalog=model().catalog_price!;const m={name:'public',platform:'openai',catalog_price:catalog,official_pricing:{input_price:0.0009,output_price:0.0009,cache_read_price:null,cache_write_price:null},pricing:{billing_mode:'token',input_price:0.0000001,output_price:0.0000001}} as PlazaModel;const w=mount(Table,{props:{models:[m]}});expect(w.text()).toContain('US$3');expect(w.text()).toContain('US$12');expect(w.text()).not.toContain('实付价格');expect(w.text()).not.toContain('US$900')})
 it('keeps missing catalog data missing even on an older server',()=>{const w=mount(Table,{props:{models:[{name:'missing',platform:'openai',pricing:null,official_pricing:{input_price:0.000003,output_price:0.000012,cache_read_price:null,cache_write_price:null}}]}});expect(w.text()).toContain('官方原价缺失');expect(w.text()).not.toContain('US$3')})
 it('shows image-token prices and explicit zeros',()=>{const c=model().catalog_price!;c.price={...c.price,input_usd_per_m:null,output_usd_per_m:null,image_input_usd_per_m:0,image_output_usd_per_m:30};const w=mount(Table,{props:{models:[{name:'image',platform:'openai',pricing:null,official_pricing:null,catalog_price:c}]}});expect(w.text()).toContain('图片输入');expect(w.text()).toContain('US$0');expect(w.text()).toContain('US$30')})
})
