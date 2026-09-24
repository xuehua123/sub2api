<template>
 <div class="plaza-catalog-table" data-testid="public-catalog-prices">
  <p class="px-5 py-3 text-xs text-gray-500 dark:text-gray-400">官方目录原价 · USD，不含渠道、分组倍率或套餐折扣。</p>
  <article v-for="model in models" :key="model.platform+':'+model.name" class="catalog-row" :data-model-name="model.name">
   <div class="catalog-primary"><strong>{{model.name}}</strong>
    <template v-if="model.catalog_price">
     <dl v-if="model.catalog_price.billing_mode==='token'"><div><dt>{{model.catalog_price.price.input_usd_per_m!=null?'输入':'图片输入'}} / 百万 tokens</dt><dd>{{money(model.catalog_price.price.input_usd_per_m ?? model.catalog_price.price.image_input_usd_per_m,'USD')}}</dd></div><div><dt>{{model.catalog_price.price.output_usd_per_m!=null?'输出':'图片输出'}} / 百万 tokens</dt><dd>{{money(model.catalog_price.price.output_usd_per_m ?? model.catalog_price.price.image_output_usd_per_m,'USD')}}</dd></div></dl>
     <dl v-else><div><dt>{{model.catalog_price.billing_mode==='image'?'每张':'单位待确认'}}</dt><dd>{{model.catalog_price.billing_mode==='image'?money(model.catalog_price.price.per_request_usd,'USD'):'暂不可展示'}}</dd></div></dl>
    </template><span v-else class="text-sm text-gray-500">官方原价缺失</span>
   </div>
   <details v-if="model.catalog_price" class="mt-2 text-xs"><summary>官方价格明细 · 缓存与阶梯</summary><p v-if="model.catalog_price.model!==model.name" class="mt-2">目录模型：{{model.catalog_price.model}}</p><div v-for="tier in tiers(model)" :key="tier.key" class="mt-3 overflow-auto"><p class="mb-2 font-medium">{{tier.label}}<span v-if="tier.threshold_tokens"> · 输入 &gt; {{tier.threshold_tokens}} tokens</span></p><table class="w-full text-left"><tbody><tr v-for="field in fields(model,tier.official)" :key="field.key"><td class="py-1">{{field.label}}</td><td class="py-1 text-right font-mono">{{money(tier.official[field.usd],'USD')}} / {{field.key==='quantity'?'张':'百万 tokens'}}</td></tr></tbody></table></div></details>
  </article>
 </div>
</template>
<script setup lang="ts">
import type {PlazaModel} from '@/api/modelPlaza'
import type {ModelPriceValue} from '@/api/modelPrices'
import {money,priceFields,validPrice} from '@/components/model-price/pricing'
defineProps<{models:PlazaModel[]}>()
function tiers(model:PlazaModel){const c=model.catalog_price;if(!c)return [];return [{key:'base',label:'基础价',threshold_tokens:undefined as number|undefined,official:c.price},...c.tiers.filter(t=>t.key!=='base')]}
function fields(model:PlazaModel,value:ModelPriceValue){return priceFields.filter(f=>validPrice(value[f.usd])&&(model.catalog_price?.billing_mode==='image'?f.key==='quantity':true))}
</script>
<style scoped>
.catalog-row {padding:18px 20px;border-top:1px solid var(--ppx-line,#dce5ec);color:var(--ppx-ink,inherit);}
.catalog-primary {display:grid;grid-template-columns:minmax(0,1fr) minmax(0,1.3fr);align-items:center;gap:18px;}
.catalog-primary strong {overflow-wrap:anywhere;font-size:14px;font-weight:600;}
.catalog-primary dl {display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:14px;}
.catalog-primary dt {font-size:11px;color:var(--ppx-muted,#526a7b);}
.catalog-primary dd {font:600 16px/1.8 ui-monospace,monospace;overflow-wrap:anywhere;}
summary {cursor:pointer;color:var(--ppx-muted,#526a7b);}
@media(max-width:640px){.catalog-primary{grid-template-columns:minmax(0,1fr)}.catalog-row{padding:16px}}
</style>
