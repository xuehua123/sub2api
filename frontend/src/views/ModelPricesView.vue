<template>
  <AppLayout>
    <div class="model-market -m-4 min-h-[calc(100vh-4rem)] bg-[#0f1014] text-slate-100 md:-m-6 lg:-m-8">
      <div class="flex min-h-[calc(100vh-4rem)]">
        <aside class="hidden w-[360px] shrink-0 border-r border-white/[0.08] bg-[#101114] px-4 py-5 xl:block">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <p class="text-lg font-semibold text-white">分组</p>
              <p class="mt-1 text-xs text-slate-500">按销售分组查看价格</p>
            </div>
            <button class="rounded-md border border-white/10 px-2.5 py-1 text-xs text-slate-300 hover:bg-white/[0.06]" @click="resetFilters">
              重置
            </button>
          </div>

          <div class="relative mb-4">
            <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
            <input
              v-model="groupSearchQuery"
              class="h-9 w-full rounded-md border border-white/10 bg-[#1c1e23] pl-9 pr-3 text-xs text-slate-100 outline-none placeholder:text-slate-500 focus:border-blue-400"
              placeholder="搜索分组"
            />
          </div>

          <FilterSection title="平台">
            <FilterPill
              v-for="item in groupPlatformFilters"
              :key="item.value"
              :active="selectedGroupCategory === item.value"
              :label="item.label"
              :count="item.count"
              :tone="item.tone"
              @click="selectedGroupCategory = item.value"
            />
          </FilterSection>

          <FilterSection title="分组列表">
            <FilterPill
              :active="selectedGroupID === undefined"
              label="不选择分组"
              count-label="概览"
              tone="cyan"
              @click="selectedGroupID = undefined"
            />
            <FilterPill
              v-for="group in groupFilters"
              :key="group.id"
              :active="selectedGroupID === group.id"
              :label="group.name"
              :count-label="groupPillLabel(group)"
              tone="cyan"
              @click="selectedGroupID = group.id"
            />
            <FilterPill
              v-if="groupFilters.length === 0"
              :active="false"
              label="暂无分组"
              count-label="-"
              tone="cyan"
            />
          </FilterSection>
        </aside>

        <div v-if="showGroupDrawer" class="mobile-group-overlay xl:hidden" @click.self="showGroupDrawer = false">
          <section class="mobile-group-drawer">
            <div class="mb-4 flex items-center justify-between gap-3">
              <div>
                <p class="text-base font-semibold text-white">选择分组</p>
                <p class="mt-1 text-xs text-slate-500">按销售分组查看价格</p>
              </div>
              <button class="copy-button" title="关闭" @click="showGroupDrawer = false">
                <Icon name="x" size="xs" />
              </button>
            </div>

            <div class="relative mb-4">
              <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
              <input
                v-model="groupSearchQuery"
                class="h-9 w-full rounded-md border border-white/10 bg-[#1c1e23] pl-9 pr-3 text-xs text-slate-100 outline-none placeholder:text-slate-500 focus:border-blue-400"
                placeholder="搜索分组"
              />
            </div>

            <FilterSection title="平台">
              <FilterPill
                v-for="item in groupPlatformFilters"
                :key="item.value"
                :active="selectedGroupCategory === item.value"
                :label="item.label"
                :count="item.count"
                :tone="item.tone"
                @click="selectedGroupCategory = item.value"
              />
            </FilterSection>

            <FilterSection title="分组列表">
              <FilterPill
                :active="selectedGroupID === undefined"
                label="不选择分组"
                count-label="概览"
                tone="cyan"
                @click="selectGroup(undefined)"
              />
              <FilterPill
                v-for="group in groupFilters"
                :key="group.id"
                :active="selectedGroupID === group.id"
                :label="group.name"
                :count-label="groupPillLabel(group)"
                tone="cyan"
                @click="selectGroup(group.id)"
              />
            </FilterSection>
          </section>
        </div>

        <main class="min-w-0 flex-1 px-4 py-5 sm:px-5 lg:px-6">
          <section class="overflow-hidden rounded-lg border border-white/10 bg-[#14161b] shadow-2xl shadow-black/30">
            <div class="relative min-h-[98px] overflow-hidden bg-[radial-gradient(circle_at_15%_20%,rgba(34,211,238,.34),transparent_31%),linear-gradient(105deg,#1e40af,#2563eb_42%,#4338ca)] px-5 py-4">
              <div class="absolute inset-y-0 right-0 w-2/3 bg-[linear-gradient(120deg,transparent,rgba(255,255,255,.16))]" />
              <div class="relative flex h-full items-center justify-between gap-5">
                <div>
                  <div class="flex items-center gap-2">
                    <h1 class="text-xl font-bold text-white">模型价格</h1>
                    <span class="rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-blue-700">
                      共 {{ filteredModels.length }} 个模型
                    </span>
                  </div>
                  <p class="mt-2 max-w-3xl text-sm text-blue-50/90">
                    按当前分组展示我们的到手价格，并对照官方参考价，让优惠力度一眼能看懂。
                  </p>
                </div>
                <div class="hidden shrink-0 grid-cols-3 gap-3 xl:grid">
                  <div v-if="isAdmin" class="hero-stat">
                    <span>官方汇率</span>
                    <strong>{{ formatNumber(response?.usd_cny_rate ?? 7, 4) }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>{{ isAdmin ? '套餐折算' : '我们的价格' }}</span>
                    <strong>{{ selectedGroup?.best_plan ? `¥${formatNumber(selectedGroup.best_plan.cny_per_quota_usd, 4)}` : '参考价' }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>优惠力度</span>
                    <strong>{{ selectedGroup?.best_plan ? saveFactorLabel(groupSaleMultiplier(selectedGroup)) : '-' }}</strong>
                  </div>
                </div>
              </div>
            </div>

            <div class="border-b border-white/10 bg-[#15171c] p-3">
              <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
                <div class="relative min-w-0 flex-1">
                  <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-slate-500" />
                  <input
                    v-model="searchQuery"
                    class="h-10 w-full rounded-md border border-white/10 bg-[#2a2c31] pl-9 pr-3 text-sm text-slate-100 outline-none placeholder:text-slate-500 focus:border-blue-400"
                    placeholder="搜索模型名称"
                  />
                </div>
                <div class="flex flex-wrap items-center gap-2 text-xs">
                  <button class="toolbar-button xl:hidden" @click="showGroupDrawer = true">
                    <Icon name="filter" size="xs" />
                    {{ selectedGroup?.name || '分组' }}
                  </button>
                  <button
                    v-for="item in priceKindFilters"
                    :key="item.value"
                    class="toolbar-button"
                    :class="{ active: selectedPriceKind === item.value }"
                    @click="selectedPriceKind = item.value"
                  >
                    {{ item.label }} {{ item.count }}
                  </button>
                  <button v-if="isAdmin && selectedGroup" class="toolbar-button" :class="{ active: showOnlyIssues }" @click="showOnlyIssues = !showOnlyIssues">
                    只看异常 {{ issueModels.length }}
                  </button>
                  <button v-if="isAdmin" class="toolbar-button" :class="{ active: includeCatalog }" @click="toggleIncludeCatalog">
                    官方目录补充
                  </button>
                  <button v-if="isAdmin" class="toolbar-button" :class="{ active: showHiddenGroups }" @click="toggleShowHiddenGroups">
                    显示隐藏分组
                  </button>
                  <button v-if="isAdmin && selectedGroup" class="toolbar-button" :disabled="savingHiddenGroups" @click="toggleCurrentGroupHidden">
                    {{ selectedGroup.hidden ? '恢复当前分组' : '隐藏当前分组' }}
                  </button>
                  <button class="toolbar-button" :class="{ active: showOfficial }" @click="showOfficial = !showOfficial">
                    官方参考价
                  </button>
                  <button class="toolbar-button" :class="{ active: showMultiplier }" @click="showMultiplier = !showMultiplier">
                    优惠倍数
                  </button>
                  <button class="toolbar-button" @click="copyVisibleModels">
                    <Icon name="copy" size="xs" />
                    复制
                  </button>
                  <button class="toolbar-button" :disabled="loading" @click="loadPrices(selectedGroupID)">
                    <Icon name="refresh" size="xs" :class="loading ? 'animate-spin' : ''" />
                    刷新
                  </button>
                </div>
              </div>
            </div>
          </section>

          <section v-if="isAdmin" class="admin-price-panel mt-4">
            <div class="ops-card">
              <div>
                <span>官方参考价目录</span>
                <strong>{{ catalogStatusLabel }}</strong>
                <small>{{ catalogSyncDetail }}</small>
              </div>
              <button class="toolbar-button" :disabled="syncingCatalog" @click="syncOfficialCatalog">
                <Icon name="refresh" size="xs" :class="syncingCatalog ? 'animate-spin' : ''" />
                同步官方目录
              </button>
            </div>
            <div class="ops-card">
              <div>
                <span>我们的销售价</span>
                <strong>渠道价格 + 套餐单价 + 分组倍率</strong>
                <small>官方同步不会覆盖销售配置</small>
              </div>
              <RouterLink class="toolbar-button" to="/admin/channels/pricing">
                维护渠道价格
              </RouterLink>
            </div>
            <div class="ops-card">
              <div>
                <span>换算参数</span>
                <strong>USD/CNY {{ formatNumber(response?.usd_cny_rate ?? 7, 4) }}</strong>
                <small>{{ selectedGroup ? `当前分组倍率 ${formatNumber(selectedGroup.effective_multiplier, 4)}x` : '选择分组后查看倍率' }}</small>
              </div>
            </div>
          </section>

          <section v-if="selectedGroup" class="mt-4 grid gap-3 md:grid-cols-3">
            <div class="metric-tile">
              <span>当前分组</span>
              <strong>{{ selectedGroup.name }}</strong>
            </div>
            <div class="metric-tile">
              <span>{{ isAdmin ? '套餐单价' : '我们的价格' }}</span>
              <strong v-if="selectedGroup.best_plan">
                ¥{{ formatNumber(selectedGroup.best_plan.cny_per_quota_usd, 4) }}/额度 USD<span v-if="isAdmin"> · 分组 {{ formatNumber(selectedGroup.effective_multiplier, 2) }}x</span> · {{ selectedGroup.best_plan.name }}
              </strong>
              <strong v-else>暂无套餐，显示参考价</strong>
            </div>
            <div class="metric-tile">
              <span>优惠力度</span>
              <strong class="text-emerald-300">{{ selectedGroup.best_plan ? saveFactorLabel(groupSaleMultiplier(selectedGroup)) : '按官方参考' }}</strong>
            </div>
          </section>

          <section v-if="isAdmin && selectedGroup && issueModels.length > 0" class="price-alert mt-4" :class="{ critical: criticalIssueCount > 0 }">
            <div>
              <strong>价格来源异常 {{ issueModels.length }} 个</strong>
              <span>严重 {{ criticalIssueCount }} 个 · 提醒 {{ warningIssueCount }} 个 · {{ issueModels.slice(0, 3).map(item => `${item.name}：${priceIssueLabel(item)}`).join('、') }}</span>
            </div>
            <RouterLink class="toolbar-button" to="/admin/channels/pricing">
              去补价格
            </RouterLink>
          </section>

          <div v-if="loading" class="flex min-h-[420px] items-center justify-center text-sm text-slate-400">
            正在加载模型价格...
          </div>
          <section v-else-if="!selectedGroup" class="group-overview mt-4">
            <div class="overview-heading">
              <div>
                <h2>分组概览</h2>
                <p>选择一个销售分组后，右侧会展示该分组账号实际支持的模型价格。</p>
              </div>
              <button class="toolbar-button xl:hidden" @click="showGroupDrawer = true">
                <Icon name="filter" size="xs" />
                选择分组
              </button>
            </div>
            <div class="overview-grid">
              <button
                v-for="item in groupOverviewCards"
                :key="item.value"
                class="overview-card"
                @click="selectedGroupCategory = item.value"
              >
                <span>{{ item.label }}</span>
                <strong>{{ item.groupCount }} 分组</strong>
                <small>{{ item.modelCount }} 模型 · {{ item.channelCount }} 来源</small>
              </button>
            </div>
            <div class="overview-list">
              <button
                v-for="group in groupFilters.slice(0, 12)"
                :key="group.id"
                class="overview-group"
                @click="selectGroup(group.id)"
              >
                <span>{{ group.name }}</span>
                <strong>{{ group.model_count }} 模型 · {{ group.channel_count }} 来源</strong>
              </button>
            </div>
          </section>
          <div v-else-if="filteredModels.length === 0" class="flex min-h-[420px] items-center justify-center text-sm text-slate-400">
            {{ showOnlyIssues ? '暂无价格异常模型' : '暂无可展示模型' }}
          </div>
          <template v-else>
          <section class="mt-4 hidden overflow-hidden rounded-lg border border-white/[0.08] bg-[#14161b] xl:block">
            <div class="price-table-header" :class="{ admin: isAdmin }">
              <span>模型</span>
              <span>我们的价格</span>
              <span>官方参考价</span>
              <span>阶梯价格</span>
              <span>优惠力度</span>
              <span v-if="isAdmin">来源</span>
            </div>
            <article
              v-for="model in filteredModels"
              :key="model.platform + ':' + model.name"
              class="price-table-row"
              :class="{ admin: isAdmin }"
            >
              <div class="flex min-w-0 items-center gap-3">
                <ProviderMark :provider="model.platform" />
                <div class="min-w-0">
                  <div class="flex min-w-0 items-center gap-2">
                    <h2 class="truncate text-sm font-bold text-slate-100" :title="model.name">{{ model.name }}</h2>
                    <button class="copy-button" title="复制模型名" @click="copyText(model.name)">
                      <Icon name="copy" size="xs" />
                    </button>
                  </div>
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="tag">{{ providerLabel(model.platform) }}</span>
                    <span class="tag tag-purple">{{ billingLabel(model.billing_mode) }}</span>
                    <span v-if="isAdmin && model.channel_names.length > 0" class="tag tag-blue">{{ model.channel_names.length }} 来源</span>
                  </div>
                </div>
              </div>

              <div v-if="isFixedPriceModel(model)" class="price-pair actual">
                <strong>{{ fixedPriceUnitLabel(model) }} {{ formatCNYPerUnit(bestActual(model, 'per_request_cny'), fixedPriceUnit(model)) }}</strong>
                <small v-if="selectedGroup?.best_plan">{{ selectedGroup.best_plan.name }}</small>
              </div>
              <div v-else class="price-pair actual">
                <strong>输入 {{ formatCNYPerMillion(bestActual(model, 'input_cny_per_m')) }}</strong>
                <strong>输出 {{ formatCNYPerMillion(bestActual(model, 'output_cny_per_m')) }}</strong>
                <small v-if="selectedGroup?.best_plan">{{ selectedGroup.best_plan.name }}</small>
              </div>

              <div v-if="isFixedPriceModel(model)" class="price-pair muted">
                <span>官方 {{ formatUSDPerUnit(model.official.per_request_usd, fixedPriceUnit(model)) }}</span>
                <small v-if="isAdmin && showOfficial">{{ sourceLabel(model.pricing_source) }}</small>
              </div>
              <div v-else class="price-pair muted">
                <span>输入 {{ formatUSDPerMillion(model.official.input_usd_per_m) }}</span>
                <span>输出 {{ formatUSDPerMillion(model.official.output_usd_per_m) }}</span>
                <small v-if="isAdmin && showOfficial">缓存读 {{ formatUSD(model.official.cache_read_usd_per_m) }} / 写 {{ formatUSD(model.official.cache_write_usd_per_m) }}</small>
              </div>

              <div class="tier-list">
                <template v-if="tierBadges(model).length > 0">
                  <span
                    v-for="tier in tierBadges(model)"
                    :key="tier.key"
                    class="tier-chip"
                    :title="tier.detail"
                  >
                    {{ tier.label }}
                  </span>
                </template>
                <span v-else class="text-xs text-slate-600">基础价</span>
              </div>

              <div>
                <span v-if="showMultiplier && isActuallyCheaper(model.cheaper_factor)" class="save-badge">
                  便宜 {{ formatNumber(model.cheaper_factor, 1) }} 倍
                </span>
                <span v-else-if="selectedGroup?.best_plan" class="save-badge neutral">
                  套餐折算
                </span>
                <span v-else class="text-xs font-semibold text-slate-500">参考</span>
              </div>

              <div v-if="isAdmin" class="source-cell">
                <span>{{ sourceLabel(model.pricing_source) }}</span>
                <small v-if="model.official_missing">缺官方价</small>
              </div>
            </article>
          </section>

          <section class="mt-4 grid gap-3 lg:grid-cols-2 xl:hidden">
            <article
              v-for="model in filteredModels"
              :key="model.platform + ':' + model.name"
              class="model-card group"
            >
              <div class="flex items-start gap-3">
                <ProviderMark :provider="model.platform" />
                <div class="min-w-0 flex-1">
                  <div class="flex min-w-0 items-start justify-between gap-3">
                    <h2 class="truncate text-base font-bold text-slate-100" :title="model.name">{{ model.name }}</h2>
                    <button class="copy-button" title="复制模型名" @click="copyText(model.name)">
                      <Icon name="copy" size="xs" />
                    </button>
                  </div>
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="tag">{{ providerLabel(model.platform) }}</span>
                    <span class="tag tag-purple">{{ billingLabel(model.billing_mode) }}</span>
                    <span v-if="tierBadges(model).length > 0" class="tag tag-blue">阶梯价格</span>
                  </div>
                </div>
              </div>

              <div class="mt-4 rounded-lg border border-emerald-400/10 bg-emerald-400/[0.035] p-3">
                <div class="mb-2 flex items-center justify-between gap-2">
                  <span class="text-[11px] font-bold text-emerald-200">我们的价格</span>
                  <span v-if="selectedGroup?.best_plan" class="truncate text-[11px] text-slate-500" :title="selectedGroup.best_plan.name">
                    {{ selectedGroup.best_plan.name }}
                  </span>
                </div>
                <div v-if="isFixedPriceModel(model)" class="space-y-2">
                  <PriceRow :label="`${fixedPriceUnitLabel(model)}价格`" :value="bestActual(model, 'per_request_cny')" :suffix="`/ ${fixedPriceUnit(model)}`" highlight />
                </div>
                <div v-else class="space-y-2">
                  <PriceRow label="输入价格" :value="bestActual(model, 'input_cny_per_m')" suffix="/ 百万 tokens" highlight />
                  <PriceRow label="输出价格" :value="bestActual(model, 'output_cny_per_m')" suffix="/ 百万 tokens" highlight />
                  <PriceRow label="缓存读取" :value="bestActual(model, 'cache_read_cny_per_m')" suffix="/ 百万 tokens" />
                  <PriceRow label="缓存创建" :value="bestActual(model, 'cache_write_cny_per_m')" suffix="/ 百万 tokens" />
                </div>
              </div>

              <div v-if="tierBadges(model).length > 0" class="tier-strip">
                <div
                  v-for="tier in tierBadges(model)"
                  :key="tier.key"
                  class="tier-line"
                >
                  <span class="truncate" :title="tier.detail">{{ tier.label }}</span>
                  <strong>可选</strong>
                </div>
              </div>

              <div v-if="showOfficial" class="mt-3 rounded-md border border-white/[0.08] bg-black/[0.16] p-2.5">
                <div class="mb-1.5 text-[11px] font-semibold text-slate-500">官方参考价</div>
                <div v-if="isFixedPriceModel(model)" class="grid grid-cols-2 gap-2 text-xs text-slate-400">
                  <span>官方 {{ formatUSDPerUnit(model.official.per_request_usd, fixedPriceUnit(model)) }}</span>
                </div>
                <div v-else class="grid grid-cols-2 gap-2 text-xs text-slate-400">
                  <span>输入 {{ formatUSDPerMillion(model.official.input_usd_per_m) }}</span>
                  <span>输出 {{ formatUSDPerMillion(model.official.output_usd_per_m) }}</span>
                  <span>缓存读 {{ formatUSDPerMillion(model.official.cache_read_usd_per_m) }}</span>
                  <span>缓存写 {{ formatUSDPerMillion(model.official.cache_write_usd_per_m) }}</span>
                </div>
              </div>

              <div class="mt-4 flex items-center justify-between gap-3">
                <span class="rounded-full bg-violet-500/[0.12] px-2.5 py-1 text-xs font-semibold text-violet-200">{{ billingLabel(model.billing_mode) }}</span>
                <span v-if="showMultiplier && isActuallyCheaper(model.cheaper_factor)" class="save-badge">
                  便宜 {{ formatNumber(model.cheaper_factor, 1) }} 倍
                </span>
                <span v-else-if="selectedGroup?.best_plan" class="save-badge neutral">
                  我们的价格
                </span>
              </div>
            </article>
          </section>
          </template>
        </main>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import modelPricesAPI, {
  type ModelPriceGroup,
  type ModelPriceModel,
  type ModelPriceResponse,
  type ModelPriceTier,
} from '@/api/modelPrices'
import { useAppStore, useAuthStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

type FilterTone = 'purple' | 'cyan' | 'amber' | 'emerald'
type GroupCategory = 'all' | 'claude' | 'openai' | 'gemini' | 'domestic'
type PriceKind = 'all' | 'metered' | 'fixed'
type ActualPriceKey = keyof ModelPriceModel['actual']
interface TierBadge {
  key: string
  label: string
  detail: string
}

const FilterSection = defineComponent({
  props: {
    title: { type: String, required: true },
  },
  setup(props, { slots }) {
    return () => h('section', { class: 'mb-6 border-t border-white/[0.08] pt-4' }, [
      h('h3', { class: 'mb-3 px-1 text-sm font-bold text-slate-100' }, props.title),
      h('div', { class: 'grid gap-2' }, slots.default?.()),
    ])
  },
})

const FilterPill = defineComponent({
  props: {
    active: { type: Boolean, default: false },
    label: { type: String, required: true },
    count: { type: Number, default: undefined },
    countLabel: { type: String, default: '' },
    tone: { type: String as () => FilterTone, default: 'purple' },
  },
  emits: ['click'],
  setup(props, { emit }) {
    return () => h('button', {
      class: ['filter-pill', `tone-${props.tone}`, props.active ? 'active' : ''],
      onClick: () => emit('click'),
    }, [
      h('span', { class: 'truncate' }, props.label),
      props.count !== undefined || props.countLabel
        ? h('span', { class: 'pill-count' }, props.countLabel || String(props.count))
        : null,
    ])
  },
})

const ProviderMark = defineComponent({
  props: {
    provider: { type: String, required: true },
    large: { type: Boolean, default: false },
  },
  setup(props) {
    return () => {
      const label = providerShort(props.provider)
      const isAnthropic = props.provider.toLowerCase().includes('anthropic')
      return h('div', {
        class: [
          'provider-mark',
          props.large ? 'large' : '',
          isAnthropic ? 'anthropic' : 'openai',
        ],
      }, label)
    }
  },
})

const PriceRow = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: Number, default: null },
    suffix: { type: String, default: '' },
    highlight: { type: Boolean, default: false },
  },
  setup(props) {
    return () => h('div', { class: 'price-row' }, [
      h('span', { class: 'text-slate-400' }, props.label),
      h('strong', { class: props.highlight ? 'text-emerald-300' : 'text-slate-200' }, [
        props.value == null ? '暂未定价' : `¥${formatNumber(props.value, props.value < 1 ? 4 : 2)}`,
        props.value == null ? '' : h('small', { class: 'ml-1 text-[11px] font-medium text-slate-500' }, props.suffix),
      ]),
    ])
  },
})

const appStore = useAppStore()
const authStore = useAuthStore()
const response = ref<ModelPriceResponse | null>(null)
const selectedGroupID = ref<number | undefined>(undefined)
const selectedGroupCategory = ref<GroupCategory>('all')
const selectedPriceKind = ref<PriceKind>('all')
const groupSearchQuery = ref('')
const searchQuery = ref('')
const loading = ref(false)
const savingHiddenGroups = ref(false)
const syncingCatalog = ref(false)
const showGroupDrawer = ref(false)
const showOfficial = ref(false)
const showMultiplier = ref(false)
const showOnlyIssues = ref(false)
const includeCatalog = ref(false)
const showHiddenGroups = ref(false)

const isAdmin = computed(() => authStore.isAdmin)
const groups = computed(() => sortGroupsForSales(response.value?.groups ?? []))
const selectedGroup = computed<ModelPriceGroup | undefined>(() =>
  groups.value.find((group) => group.id === selectedGroupID.value),
)
const models = computed(() => response.value?.models ?? [])

const groupPlatformFilters = computed(() => {
  const counts = countBy(groups.value, group => groupCategoryKey(group.platform))
  return [
    { value: 'all' as GroupCategory, label: '全部', count: groups.value.length, tone: 'cyan' as FilterTone },
    { value: 'claude' as GroupCategory, label: 'Claude', count: counts.claude ?? 0, tone: 'amber' as FilterTone },
    { value: 'openai' as GroupCategory, label: 'OpenAI', count: counts.openai ?? 0, tone: 'purple' as FilterTone },
    { value: 'gemini' as GroupCategory, label: 'Gemini', count: counts.gemini ?? 0, tone: 'emerald' as FilterTone },
    { value: 'domestic' as GroupCategory, label: '国模', count: counts.domestic ?? 0, tone: 'cyan' as FilterTone },
  ]
})
const groupOverviewCards = computed(() => {
  const categories: Array<{ value: GroupCategory; label: string }> = [
    { value: 'claude', label: 'Claude' },
    { value: 'openai', label: 'OpenAI' },
    { value: 'gemini', label: 'Gemini' },
    { value: 'domestic', label: '国模' },
  ]
  return categories.map((category) => {
    const overview = response.value?.group_overview?.find(item => item.category === category.value)
    return {
      ...category,
      groupCount: overview?.group_count ?? 0,
      modelCount: overview?.model_count ?? 0,
      channelCount: overview?.channel_count ?? 0,
    }
  })
})
const groupFilters = computed(() => {
  const q = groupSearchQuery.value.trim().toLowerCase()
  return groups.value.filter((group) => {
    if (selectedGroupCategory.value !== 'all' && groupCategoryKey(group.platform) !== selectedGroupCategory.value) return false
    if (!q) return true
    return group.name.toLowerCase().includes(q) || group.platform.toLowerCase().includes(q)
  })
})
const catalogStatusLabel = computed(() => {
  const count = response.value?.catalog_status?.model_count
  if (typeof count === 'number' && Number.isFinite(count) && count > 0) {
    return `${formatNumber(count, 0)} 个模型`
  }
  return '待同步'
})
const catalogSyncDetail = computed(() => {
  const status = response.value?.catalog_status
  if (!status) return '自动同步官方参考价，也可手动同步'
  const updated = formatDateTime(status.last_updated)
  const hash = status.local_hash ? ` · ${status.local_hash}` : ''
  return `最近同步 ${updated}${hash}`
})
const priceKindFilters = computed(() => {
  const counts = countBy(models.value, model => isFixedPriceModel(model) ? 'fixed' : 'metered')
  return [
    { value: 'all' as PriceKind, label: '全部模型', count: models.value.length },
    { value: 'metered' as PriceKind, label: '按量计费', count: counts.metered ?? 0 },
    { value: 'fixed' as PriceKind, label: '固定价格', count: counts.fixed ?? 0 },
  ]
})

const searchableModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return models.value.filter((model) => {
    if (selectedPriceKind.value === 'fixed' && !isFixedPriceModel(model)) return false
    if (selectedPriceKind.value === 'metered' && isFixedPriceModel(model)) return false
    if (!q) return true
    return model.name.toLowerCase().includes(q) ||
      model.platform.toLowerCase().includes(q) ||
      model.provider.toLowerCase().includes(q) ||
      model.channel_names.some(name => name.toLowerCase().includes(q))
  }).sort(compareModelsForSales)
})
const issueModels = computed(() =>
  searchableModels.value.filter(model => priceIssueSeverity(model) !== 'none'),
)
const filteredModels = computed(() => showOnlyIssues.value ? issueModels.value : searchableModels.value)
const criticalIssueCount = computed(() =>
  issueModels.value.filter(model => priceIssueSeverity(model) === 'critical').length,
)
const warningIssueCount = computed(() =>
  issueModels.value.filter(model => priceIssueSeverity(model) === 'warning').length,
)

watch(selectedGroupID, (id, oldID) => {
  if (id !== oldID) {
    loadPrices(id)
  }
})

watch(includeCatalog, () => {
  if (selectedGroupID.value) loadPrices(selectedGroupID.value)
})

watch(showHiddenGroups, () => {
  loadPrices(selectedGroupID.value)
})

async function loadPrices(groupID?: number) {
  loading.value = true
  try {
    response.value = await modelPricesAPI.getModelPrices({
      group_id: groupID,
      include_catalog: includeCatalog.value,
      show_hidden_groups: showHiddenGroups.value,
    })
    if (groupID && response.value.selected_group_id === groupID) {
      selectedGroupID.value = groupID
    } else if (!groupID) {
      selectedGroupID.value = undefined
    } else {
      selectedGroupID.value = undefined
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '加载模型价格失败'))
  } finally {
    loading.value = false
  }
}

function resetFilters() {
  selectedGroupCategory.value = 'all'
  selectedPriceKind.value = 'all'
  groupSearchQuery.value = ''
  searchQuery.value = ''
  showOnlyIssues.value = false
}

function selectGroup(id: number | undefined) {
  selectedGroupID.value = id
  showGroupDrawer.value = false
}

function toggleIncludeCatalog() {
  includeCatalog.value = !includeCatalog.value
}

function toggleShowHiddenGroups() {
  showHiddenGroups.value = !showHiddenGroups.value
}

async function toggleCurrentGroupHidden() {
  if (!isAdmin.value || !selectedGroup.value) return
  const group = selectedGroup.value
  const current = new Set(response.value?.hidden_group_ids ?? [])
  if (group.hidden) {
    current.delete(group.id)
  } else {
    current.add(group.id)
  }
  savingHiddenGroups.value = true
  try {
    const result = await modelPricesAPI.updateHiddenGroups([...current])
    if (response.value) response.value.hidden_group_ids = result.hidden_group_ids
    appStore.showSuccess(group.hidden ? '已恢复分组展示' : '已隐藏分组')
    if (!group.hidden && !showHiddenGroups.value) {
      selectedGroupID.value = undefined
      await loadPrices()
    } else {
      await loadPrices(selectedGroupID.value)
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '保存隐藏分组失败'))
  } finally {
    savingHiddenGroups.value = false
  }
}

async function syncOfficialCatalog() {
  if (!isAdmin.value || syncingCatalog.value) return
  syncingCatalog.value = true
  try {
    const status = await modelPricesAPI.syncCatalog()
    if (response.value) response.value.catalog_status = status
    appStore.showSuccess(`官方目录已同步：${formatNumber(status.model_count, 0)} 个模型`)
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '同步官方目录失败'))
  } finally {
    syncingCatalog.value = false
  }
}

function bestActual(model: ModelPriceModel, key: ActualPriceKey): number | undefined {
  const base = model.actual[key]
  if (typeof base === 'number' && Number.isFinite(base) && base > 0) return base
  for (const tier of model.price_tiers) {
    const value = tier.actual[key]
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) return value
  }
  return undefined
}

function extraTiers(model: ModelPriceModel): ModelPriceTier[] {
  return model.price_tiers.filter((tier) => tier.key !== 'base')
}

function tierBadges(model: ModelPriceModel): TierBadge[] {
  const tiers = extraTiers(model)
  const hasLongContext = tiers.some(tier => Boolean(tier.threshold_tokens))
  const hasFast = tiers.some(tier => tier.label.toLowerCase().includes('fast'))
  const badges: TierBadge[] = []
  if (hasLongContext) {
    const thresholds = tiers
      .map(tier => tier.threshold_tokens)
      .filter((value): value is number => typeof value === 'number' && value > 0)
    const maxThreshold = thresholds.length > 0 ? Math.max(...thresholds) : undefined
    badges.push({
      key: 'long-context',
      label: maxThreshold ? `长上下文 ${formatThresholdTokens(maxThreshold)}` : '长上下文',
      detail: tiers.filter(tier => tier.threshold_tokens).map(tier => `${tierTitle(tier)}：${tierPriceSummary(tier)}`).join('；'),
    })
  }
  if (hasFast) {
    badges.push({
      key: 'fast',
      label: 'Fast 加速价',
      detail: tiers.filter(tier => tier.label.toLowerCase().includes('fast')).map(tier => `${tierTitle(tier)}：${tierPriceSummary(tier)}`).join('；'),
    })
  }
  if (!hasLongContext && !hasFast && tiers.length > 0) {
    badges.push({
      key: 'tiered',
      label: `${tiers.length} 档阶梯价`,
      detail: tiers.map(tier => `${tierTitle(tier)}：${tierPriceSummary(tier)}`).join('；'),
    })
  }
  return badges
}

function tierTitle(tier: ModelPriceTier): string {
  if (!tier.threshold_tokens) return tier.label
  return `${tier.label} ${formatThresholdTokens(tier.threshold_tokens)}`
}

function tierPriceSummary(tier: ModelPriceTier): string {
  const input = formatCNYPerMillion(tier.actual.input_cny_per_m)
  const output = formatCNYPerMillion(tier.actual.output_cny_per_m)
  if (input === '-' && output === '-') return '暂未定价'
  return `入 ${input} / 出 ${output}`
}

function formatCompactTokens(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return '-'
  if (tokens >= 1000) return `${formatNumber(tokens / 1000, 0)}K`
  return formatNumber(tokens, 0)
}

function formatThresholdTokens(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return '-'
  return `${formatCompactTokens(tokens + 1)}+`
}

function isActuallyCheaper(factor: number | null | undefined): factor is number {
  return factor != null && Number.isFinite(factor) && factor > 1
}

function countBy<T>(items: T[], getKey: (item: T) => string): Record<string, number> {
  return items.reduce<Record<string, number>>((acc, item) => {
    const key = getKey(item) || 'unknown'
    acc[key] = (acc[key] ?? 0) + 1
    return acc
  }, {})
}

function groupCategoryKey(platform: string): GroupCategory {
  const lower = platform.toLowerCase()
  if (lower.includes('anthropic') || lower.includes('claude')) return 'claude'
  if (lower.includes('openai')) return 'openai'
  if (lower.includes('gemini') || lower.includes('google') || lower.includes('vertex')) return 'gemini'
  return 'domestic'
}

function groupPillLabel(group: ModelPriceGroup): string {
  const usage = `${group.model_count} 模型 · ${group.channel_count} 来源`
  return group.hidden ? `隐藏 · ${usage}` : usage
}

function sortGroupsForSales(items: ModelPriceGroup[]): ModelPriceGroup[] {
  return [...items].sort((a, b) => {
    const platformDiff = platformRank(a.platform) - platformRank(b.platform)
    if (platformDiff !== 0) return platformDiff
    const numberDiff = groupNumberRank(b.name) - groupNumberRank(a.name)
    if (numberDiff !== 0) return numberDiff
    return a.name.localeCompare(b.name, 'zh-CN')
  })
}

function compareModelsForSales(a: ModelPriceModel, b: ModelPriceModel): number {
  const rankDiff = modelVersionRank(b.name) - modelVersionRank(a.name)
  if (rankDiff !== 0) return rankDiff
  return a.name.localeCompare(b.name, 'zh-CN', { numeric: true })
}

function platformRank(platform: string): number {
  const lower = platform.toLowerCase()
  if (lower.includes('anthropic') || lower.includes('antigravity')) return 1
  if (lower.includes('openai')) return 2
  if (lower.includes('gemini')) return 3
  return 10
}

function groupNumberRank(name: string): number {
  const matches = name.match(/\d+(?:\.\d+)?/g)
  if (!matches) return 0
  return Math.max(...matches.map(value => Number.parseFloat(value)).filter(Number.isFinite))
}

function modelVersionRank(name: string): number {
  const matches = name.match(/\d+(?:\.\d+)?/g)
  if (!matches) return 0
  return Math.max(...matches.map(value => Number.parseFloat(value)).filter(Number.isFinite))
}

function providerLabel(platform: string): string {
  const lower = platform.toLowerCase()
  if (lower.includes('anthropic')) return 'Anthropic'
  if (lower.includes('openai')) return 'OpenAI'
  if (lower.includes('gemini')) return 'Gemini'
  if (lower.includes('antigravity')) return 'Antigravity'
  return platform || '未知供应商'
}

function providerShort(platform: string): string {
  const label = providerLabel(platform)
  if (label === 'Anthropic') return '✹'
  if (label === 'OpenAI') return '◎'
  if (label === 'Gemini') return 'G'
  if (label === 'Antigravity') return 'AG'
  return label.slice(0, 2).toUpperCase()
}

function billingLabel(mode: string): string {
  if (mode === 'request') return '按次计费'
  if (mode === 'image') return '图片计费'
  return '按量计费'
}

function hasPositiveNumber(value: number | null | undefined): boolean {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function hasActualPrice(model: ModelPriceModel): boolean {
  const values = Object.values(model.actual)
  if (values.some(value => hasPositiveNumber(value))) return true
  return model.price_tiers.some(tier => Object.values(tier.actual).some(value => hasPositiveNumber(value)))
}

function isFixedPriceModel(model: ModelPriceModel): boolean {
  return model.billing_mode === 'request' ||
    hasPositiveNumber(model.official.per_request_usd) ||
    hasPositiveNumber(model.actual.per_request_cny)
}

function fixedPriceUnit(model: ModelPriceModel): string {
  const name = model.name.toLowerCase()
  if (model.billing_mode === 'image' || name.includes('image') || name.includes('photo') || name.includes('picture') || name.includes('dall-e')) return '张'
  if (name.includes('video')) return '分钟'
  if (name.includes('audio') || name.includes('speech') || name.includes('tts') || name.includes('transcrib')) return '分钟'
  return '次'
}

function fixedPriceUnitLabel(model: ModelPriceModel): string {
  const unit = fixedPriceUnit(model)
  if (unit === '张') return '每张'
  if (unit === '分钟') return '每分钟'
  return '每次'
}

function sourceLabel(source: string): string {
  if (source === 'official') return '官方目录'
  if (source === 'channel') return '渠道配置'
  return '待补价'
}

function priceIssueLabel(model: ModelPriceModel): string {
  if (priceIssueSeverity(model) === 'critical') return '待补价'
  if (model.official_missing) return '缺官方参考价'
  return '来源正常'
}

function priceIssueSeverity(model: ModelPriceModel): 'none' | 'warning' | 'critical' {
  if (model.pricing_source === 'unknown' || !hasActualPrice(model)) return 'critical'
  if (model.official_missing) return 'warning'
  return 'none'
}

function groupSaleMultiplier(group: ModelPriceGroup | undefined): number | undefined {
  if (!group?.best_plan) return undefined
  return normalizePositive(group.effective_multiplier) * group.best_plan.usd_multiplier
}

function normalizePositive(value: number | null | undefined): number {
  if (value == null || !Number.isFinite(value) || value <= 0) return 1
  return value
}

function saveFactorLabel(multiplier: number | null | undefined): string {
  if (multiplier == null || !Number.isFinite(multiplier) || multiplier <= 0) return '参考'
  if (multiplier >= 1) return '套餐价'
  return `便宜 ${formatNumber(1 / multiplier, 1)} 倍`
}

function formatUSD(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `$${formatNumber(value, value < 1 ? 4 : 2)}`
}

function formatUSDPerMillion(value: number | null | undefined): string {
  const formatted = formatUSD(value)
  return formatted === '-' ? '-' : `${formatted} / 百万 tokens`
}

function formatUSDPerUnit(value: number | null | undefined, unit: string): string {
  const formatted = formatUSD(value)
  return formatted === '-' ? '-' : `${formatted} / ${unit}`
}

function formatCNYCompact(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `¥${formatNumber(value, value < 1 ? 4 : 2)}`
}

function formatCNYPerMillion(value: number | null | undefined): string {
  const formatted = formatCNYCompact(value)
  return formatted === '-' ? '-' : `${formatted} / 百万 tokens`
}

function formatCNYPerUnit(value: number | null | undefined, unit: string): string {
  const formatted = formatCNYCompact(value)
  return formatted === '-' ? '-' : `${formatted} / ${unit}`
}

function formatNumber(value: number | null | undefined, digits = 2): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return new Intl.NumberFormat('zh-CN', {
    minimumFractionDigits: 0,
    maximumFractionDigits: digits,
  }).format(value)
}

function formatDateTime(value: string | null | undefined): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

async function copyText(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess('已复制')
  } catch {
    appStore.showError('复制失败')
  }
}

async function copyVisibleModels() {
  const text = filteredModels.value.map(model => model.name).join('\n')
  if (!text) return
  await copyText(text)
}

onMounted(() => loadPrices())
</script>

<style scoped>
.model-market {
  color-scheme: dark;
}

.hero-stat {
  min-width: 110px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.16);
  background: rgba(15, 23, 42, 0.24);
  padding: 9px 11px;
}

.hero-stat span {
  display: block;
  font-size: 11px;
  font-weight: 700;
  color: rgba(219, 234, 254, 0.72);
}

.hero-stat strong {
  margin-top: 3px;
  display: block;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  color: white;
}

.filter-pill {
  display: flex;
  min-width: 0;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border-radius: 7px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: rgba(255, 255, 255, 0.025);
  padding: 7px 10px;
  color: rgb(203 213 225);
  font-size: 12px;
  font-weight: 700;
  text-align: left;
}

.filter-pill.active {
  background: rgba(255, 255, 255, 0.12);
  color: white;
}

.pill-count {
  flex: none;
  border-radius: 999px;
  padding: 1px 6px;
  max-width: 96px;
  white-space: normal;
  text-align: center;
  line-height: 1.15;
  font-size: 10px;
  background: rgba(255, 255, 255, 0.12);
}

.tone-purple.active .pill-count { color: #c4b5fd; }
.tone-cyan.active .pill-count { color: #67e8f9; }
.tone-amber.active .pill-count { color: #fcd34d; }
.tone-emerald.active .pill-count { color: #6ee7b7; }

.toolbar-button {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 6px;
  border-radius: 7px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.035);
  padding: 0 10px;
  color: rgb(203 213 225);
  font-weight: 700;
}

.toolbar-button.active,
.toolbar-button:hover {
  background: rgba(255, 255, 255, 0.12);
  color: white;
}

.toolbar-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.mobile-group-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: rgba(0, 0, 0, 0.58);
}

.mobile-group-drawer {
  width: min(92vw, 380px);
  height: 100%;
  overflow-y: auto;
  border-right: 1px solid rgba(255, 255, 255, 0.08);
  background: #101114;
  padding: 18px;
}

.admin-price-panel {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.ops-card {
  display: flex;
  min-height: 92px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: #15171c;
  padding: 14px;
}

.ops-card div {
  min-width: 0;
}

.ops-card span {
  display: block;
  font-size: 12px;
  color: rgb(100 116 139);
}

.ops-card strong {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  color: white;
}

.ops-card small {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: rgb(148 163 184);
}

@media (max-width: 1279px) {
  .admin-price-panel {
    grid-template-columns: 1fr;
  }
}

.group-overview {
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: #14161b;
  padding: 16px;
}

.overview-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.overview-heading h2 {
  font-size: 16px;
  font-weight: 800;
  color: white;
}

.overview-heading p {
  margin-top: 4px;
  font-size: 12px;
  color: rgb(100 116 139);
}

.overview-grid {
  margin-top: 16px;
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.overview-card,
.overview-group {
  min-width: 0;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(255, 255, 255, 0.03);
  padding: 13px;
  text-align: left;
}

.overview-card:hover,
.overview-group:hover {
  border-color: rgba(96, 165, 250, 0.42);
  background: rgba(255, 255, 255, 0.06);
}

.overview-card span,
.overview-group span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: rgb(148 163 184);
}

.overview-card strong,
.overview-group strong {
  display: block;
  margin-top: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  color: white;
}

.overview-card small {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: rgb(100 116 139);
}

.overview-list {
  margin-top: 14px;
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.price-alert {
  display: flex;
  min-height: 58px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border-radius: 8px;
  border: 1px solid rgba(251, 191, 36, 0.22);
  background: rgba(251, 191, 36, 0.08);
  padding: 12px 14px;
}

.price-alert.critical {
  border-color: rgba(248, 113, 113, 0.28);
  background: rgba(248, 113, 113, 0.09);
}

.price-alert strong,
.price-alert span {
  display: block;
}

.price-alert strong {
  color: #fde68a;
  font-size: 13px;
}

.price-alert.critical strong {
  color: #fecaca;
}

.price-alert span {
  margin-top: 4px;
  font-size: 12px;
  color: rgb(203 213 225);
}

@media (max-width: 1279px) {
  .overview-grid,
  .overview-list {
    grid-template-columns: 1fr;
  }

  .price-alert {
    align-items: flex-start;
    flex-direction: column;
  }
}

.metric-tile {
  min-height: 72px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  background: #15171c;
  padding: 14px;
}

.metric-tile span {
  display: block;
  font-size: 12px;
  color: rgb(100 116 139);
}

.metric-tile strong {
  margin-top: 6px;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  color: white;
}

.model-card {
  min-height: 186px;
  border-radius: 10px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: #14161b;
  padding: 16px;
  box-shadow: inset 0 1px rgba(255, 255, 255, 0.03);
}

.model-card:hover {
  border-color: rgba(96, 165, 250, 0.45);
  background: #171a21;
}

.price-table-header {
  display: grid;
  min-height: 44px;
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  background: #191c23;
  padding: 0 16px;
  font-size: 12px;
  font-weight: 800;
  color: rgb(100 116 139);
}

.price-table-header.admin {
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px 100px;
}

.price-table-row {
  display: grid;
  min-height: 94px;
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  padding: 14px 16px;
}

.price-table-row.admin {
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px 100px;
}

.price-table-row:last-child {
  border-bottom: 0;
}

.price-table-row:hover {
  background: rgba(255, 255, 255, 0.025);
}

.price-pair {
  display: grid;
  gap: 5px;
  min-width: 0;
}

.price-pair strong,
.price-pair span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
}

.price-pair.actual strong {
  color: #86efac;
}

.price-pair.muted span {
  color: rgb(148 163 184);
}

.price-pair small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: rgb(100 116 139);
}

.tier-list {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  gap: 6px;
}

.tier-chip {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border-radius: 999px;
  border: 1px solid rgba(125, 211, 252, 0.14);
  background: rgba(14, 165, 233, 0.08);
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 700;
  color: #bae6fd;
}

.tier-chip.dim {
  border-color: rgba(255, 255, 255, 0.09);
  background: rgba(255, 255, 255, 0.04);
  color: rgb(148 163 184);
}

.source-cell {
  display: grid;
  gap: 5px;
  font-size: 12px;
  font-weight: 800;
  color: rgb(203 213 225);
}

.source-cell small {
  font-size: 11px;
  font-weight: 700;
  color: #fbbf24;
}

.provider-mark {
  display: flex;
  width: 36px;
  height: 36px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: #1d2027;
  font-size: 18px;
  font-weight: 900;
}

.provider-mark.large {
  width: 48px;
  height: 48px;
  font-size: 24px;
}

.provider-mark.openai {
  color: #dbeafe;
}

.provider-mark.anthropic {
  color: #fb923c;
}

.copy-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  color: rgb(148 163 184);
}

.copy-button:hover {
  color: white;
  background: rgba(255, 255, 255, 0.08);
}

.tag {
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.07);
  padding: 2px 7px;
  font-size: 11px;
  font-weight: 700;
  color: rgb(203 213 225);
}

.tag-purple {
  color: #c4b5fd;
  background: rgba(139, 92, 246, 0.14);
}

.tag-blue {
  color: #93c5fd;
  background: rgba(59, 130, 246, 0.14);
}

.tier-strip {
  margin-top: 10px;
  display: grid;
  gap: 6px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.07);
  background: rgba(255, 255, 255, 0.025);
  padding: 8px;
}

.tier-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  font-size: 11px;
  color: rgb(148 163 184);
}

.tier-line strong {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 800;
  color: rgb(203 213 225);
}

.price-row {
  display: flex;
  min-height: 21px;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
}

.price-row strong {
  text-align: right;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
}

.save-badge {
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.13);
  padding: 4px 9px;
  color: #6ee7b7;
  font-size: 12px;
  font-weight: 800;
}

.save-badge.neutral {
  background: rgba(148, 163, 184, 0.12);
  color: rgb(203 213 225);
}
</style>
