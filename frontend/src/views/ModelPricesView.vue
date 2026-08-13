<template>
  <AppLayout>
    <div class="model-market -m-4 min-h-[calc(100vh-4rem)] bg-[var(--bg-main)] text-[var(--text-main)] md:-m-6 lg:-m-8 transition-all duration-200">
      <div class="flex min-h-[calc(100vh-4rem)]">
        <aside class="hidden w-[360px] shrink-0 border-r border-[var(--border-color)] bg-[var(--bg-sidebar)] px-4 py-5 xl:block">
          <div class="mb-5 flex items-center justify-between">
            <div>
              <p class="text-lg font-semibold text-[var(--text-main)]">分组</p>
              <p class="mt-1 text-xs text-[var(--text-muted)]">按销售分组查看价格</p>
            </div>
            <button class="rounded-md border border-[var(--border-color)] px-2.5 py-1 text-xs text-[var(--text-muted)] hover:bg-[var(--bg-button-hover)] transition-all duration-200" @click="resetFilters">
              重置
            </button>
          </div>

          <div v-if="isAdmin" class="selection-panel mb-4" data-testid="group-bulk-panel">
            <div class="selection-panel-row">
              <div>
                <span>分组批量管理</span>
                <strong>{{ selectedGroupIDs.size > 0 ? `已选 ${selectedGroupIDs.size} 个` : '先勾选分组' }}</strong>
              </div>
              <button class="toolbar-button" :disabled="groupFilters.length === 0" data-testid="group-select-visible" @click="toggleAllVisibleGroupsSelected">
                {{ allVisibleGroupsSelected ? '取消全选' : '全选当前' }}
              </button>
            </div>
            <div v-if="selectedGroupIDs.size > 0" class="bulk-action-strip">
              <button class="toolbar-button danger" :disabled="savingHiddenGroups" data-testid="group-bulk-hide" @click="bulkSetGroupsHidden(true)">
                批量隐藏
              </button>
              <button class="toolbar-button" :disabled="savingHiddenGroups" data-testid="group-bulk-restore" @click="bulkSetGroupsHidden(false)">
                批量恢复
              </button>
              <button class="toolbar-button ghost" :disabled="savingHiddenGroups" data-testid="group-bulk-clear" @click="clearSelectedGroups">
                清空
              </button>
            </div>
          </div>

          <div class="relative mb-4">
            <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-light)]" />
            <input
              v-model="groupSearchQuery"
              class="h-9 w-full rounded-md border border-[var(--border-color)] bg-[var(--bg-input)] pl-9 pr-3 text-xs text-[var(--text-main)] outline-none placeholder:text-[var(--text-light)] focus:border-[var(--input-focus-border)] transition-all duration-200"
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
              :show-checkbox="isAdmin"
              :checked="selectedGroupIDs.has(group.id)"
              :test-id="`group-pill-${group.id}`"
              :checkbox-test-id="`group-select-${group.id}`"
              @click="selectedGroupID = group.id"
              @toggle="toggleGroupSelected(group.id)"
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
                <p class="text-base font-semibold text-[var(--text-main)]">选择分组</p>
                <p class="mt-1 text-xs text-[var(--text-muted)]">按销售分组查看价格</p>
              </div>
              <button class="copy-button" title="关闭" @click="showGroupDrawer = false">
                <Icon name="x" size="xs" />
              </button>
            </div>

            <div v-if="isAdmin" class="selection-panel mb-4">
              <div class="selection-panel-row">
                <div>
                  <span>分组批量管理</span>
                  <strong>{{ selectedGroupIDs.size > 0 ? `已选 ${selectedGroupIDs.size} 个` : '先勾选分组' }}</strong>
                </div>
                <button class="toolbar-button" :disabled="groupFilters.length === 0" @click="toggleAllVisibleGroupsSelected">
                  {{ allVisibleGroupsSelected ? '取消全选' : '全选当前' }}
                </button>
              </div>
              <div v-if="selectedGroupIDs.size > 0" class="bulk-action-strip">
                <button class="toolbar-button danger" :disabled="savingHiddenGroups" @click="bulkSetGroupsHidden(true)">
                  批量隐藏
                </button>
                <button class="toolbar-button" :disabled="savingHiddenGroups" @click="bulkSetGroupsHidden(false)">
                  批量恢复
                </button>
                <button class="toolbar-button ghost" :disabled="savingHiddenGroups" @click="clearSelectedGroups">
                  清空
                </button>
              </div>
            </div>

            <div class="relative mb-4">
              <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-light)]" />
              <input
                v-model="groupSearchQuery"
                class="h-9 w-full rounded-md border border-[var(--border-color)] bg-[var(--bg-input)] pl-9 pr-3 text-xs text-[var(--text-main)] outline-none placeholder:text-[var(--text-light)] focus:border-[var(--input-focus-border)] transition-all duration-200"
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
                :show-checkbox="isAdmin"
                :checked="selectedGroupIDs.has(group.id)"
                :test-id="`drawer-group-pill-${group.id}`"
                :checkbox-test-id="`drawer-group-select-${group.id}`"
                @click="selectGroup(group.id)"
                @toggle="toggleGroupSelected(group.id)"
              />
            </FilterSection>
          </section>
        </div>

        <main class="min-w-0 flex-1 px-4 py-5 sm:px-5 lg:px-6">
          <section class="overflow-hidden rounded-lg border border-[var(--border-color)] bg-[var(--bg-panel)] shadow-sm dark:shadow-2xl dark:shadow-black/30 transition-all duration-200">
            <div class="relative min-h-[110px] overflow-hidden bg-gradient-to-br from-indigo-600 via-blue-600 to-indigo-700 px-6 py-5 transition-all duration-300">
              <div class="absolute inset-0 bg-[radial-gradient(circle_at_80%_20%,rgba(255,255,255,0.12),transparent_50%)]" />
              <div class="relative flex h-full items-center justify-between gap-5">
                <div>
                  <div class="flex items-center gap-2">
                    <h1 class="text-xl font-bold text-white">模型价格</h1>
                    <span class="rounded-full bg-white px-2 py-0.5 text-xs font-semibold text-blue-700">
                      共 {{ filteredModels.length }} 个模型
                    </span>
                  </div>
                  <p class="mt-2 max-w-3xl text-sm text-blue-50/90">
                    按当前分组展示官方美元参考价和我们的折扣后人民币成交价，美元小字仅作参考。
                  </p>
                </div>
                <div class="hidden shrink-0 grid-cols-3 gap-3 xl:grid">
                  <div v-if="isAdmin" class="hero-stat">
                    <span>官方汇率</span>
                    <strong>{{ formatNumber(response?.usd_cny_rate ?? 7, 4) }}</strong>
                  </div>
                  <div class="hero-stat">
                    <span>价格口径</span>
                    <strong>人民币 / 百万 tokens</strong>
                  </div>
                  <div class="hero-stat">
                    <span>优惠力度</span>
                    <strong>{{ selectedGroup?.best_plan ? saveFactorLabel(groupSaleMultiplier(selectedGroup)) : '-' }}</strong>
                  </div>
                </div>
              </div>
            </div>

            <div class="border-b border-[var(--border-color)] bg-[var(--bg-panel)] p-3">
              <div class="flex flex-col gap-3 xl:flex-row xl:items-center">
                <div class="relative min-w-0 flex-1">
                  <Icon name="search" size="sm" class="absolute left-3 top-1/2 -translate-y-1/2 text-[var(--text-light)]" />
                  <input
                    v-model="searchQuery"
                    class="h-10 w-full rounded-md border border-[var(--border-color)] bg-[var(--bg-input)] pl-9 pr-3 text-sm text-[var(--text-main)] outline-none placeholder:text-[var(--text-light)] focus:border-[var(--input-focus-border)] transition-all duration-200"
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
                  <button v-if="isAdmin && selectedGroup" class="toolbar-button" :class="{ active: showOnlyHiddenModels }" data-testid="model-hidden-only-filter" @click="showOnlyHiddenModels = !showOnlyHiddenModels">
                    只看隐藏 {{ hiddenModelCount }}
                  </button>
                  <button v-if="isAdmin && selectedGroup" class="toolbar-button" :disabled="filteredModels.length === 0" data-testid="model-select-visible" @click="toggleAllVisibleModelsSelected">
                    {{ allVisibleModelsSelected ? '取消全选模型' : '全选当前模型' }}
                  </button>
                  <button v-if="isAdmin" class="toolbar-button" :class="{ active: includeCatalog }" @click="toggleIncludeCatalog">
                    官方目录补充
                  </button>
                  <button v-if="isAdmin" class="toolbar-button" :class="{ active: showHiddenGroups }" @click="toggleShowHiddenGroups">
                    显示隐藏分组
                  </button>
                  <button v-if="isAdmin && selectedGroup" class="toolbar-button" :class="{ active: showHiddenModels }" @click="toggleShowHiddenModels">
                    显示隐藏模型
                  </button>
                  <button v-if="isAdmin && selectedGroup" class="toolbar-button" :disabled="savingHiddenGroups" @click="toggleCurrentGroupHidden">
                    {{ selectedGroup.hidden ? '恢复当前分组' : '隐藏当前分组' }}
                  </button>
                  <button class="toolbar-button" :class="{ active: showOfficial }" @click="showOfficial = !showOfficial">
					计费基准价
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
				<strong>计费基准价 × 分组倍率 × 套餐换算</strong>
                <small>人民币成交价由下方换算参数控制</small>
              </div>
              <RouterLink class="toolbar-button" to="/admin/channels/pricing">
                维护渠道价格
              </RouterLink>
            </div>
            <div class="ops-card">
              <div>
                <span>换算参数</span>
                <strong>USD/CNY {{ formatNumber(response?.usd_cny_rate ?? 7, 4) }}</strong>
                <small>¥{{ formatNumber(response?.cny_per_quota_usd ?? 0.068, 4) }}/额度 USD · {{ selectedGroup ? `当前分组 ${formatNumber(selectedGroup.effective_multiplier, 4)}x` : '选择分组后查看倍率' }}</small>
              </div>
            </div>
            <div class="ops-card pricing-settings-card">
              <div>
                <span>销售换算参数</span>
                <strong>¥{{ formatNumber(pricingSettings.cnyPerQuotaUSD || response?.cny_per_quota_usd || 0.068, 4) }}/额度 USD</strong>
                <small>线上口径：¥680 兑换 10000 刀</small>
              </div>
              <div class="pricing-settings-form">
                <label>
                  <span>USD/CNY</span>
                  <input v-model.number="pricingSettings.usdCnyRate" type="number" min="0.0001" step="0.0001" />
                </label>
                <label>
                  <span>¥/额度 USD</span>
                  <input v-model.number="pricingSettings.cnyPerQuotaUSD" type="number" min="0.000001" step="0.000001" />
                </label>
                <button class="toolbar-button active" :disabled="savingPricingSettings" @click="savePricingSettings">
                  <Icon name="check" size="xs" />
                  保存
                </button>
                <p class="pricing-preview">{{ pricingPreviewText }}</p>
              </div>
            </div>
          </section>

          <section class="sales-note mt-4">
            <div>
              <span>价格说明</span>
              <strong>页面优先展示折扣后的人民币参考价</strong>
              <p>美元价格仅作为换算参考，实际结算按当前套餐、分组倍率与用量规则执行；固定价格会按每张、每次、视频每秒或音频每分钟展示。</p>
            </div>
            <div v-if="isAdmin" class="sales-note-admin" :class="profitSafety.tone">
              <span>利润安全提示</span>
              <strong>{{ profitSafety.title }}</strong>
              <p>{{ profitSafety.detail }}</p>
            </div>
          </section>

          <section v-if="selectedGroup" class="mt-4 grid gap-3 md:grid-cols-3">
            <div class="metric-tile">
              <span>当前分组</span>
              <strong>{{ selectedGroup.name }}</strong>
            </div>
            <div class="metric-tile">
              <span>{{ isAdmin ? '内部套餐换算' : '价格口径' }}</span>
              <strong v-if="selectedGroup.best_plan">
                <template v-if="isAdmin">
                  ¥{{ formatNumber(response?.cny_per_quota_usd ?? selectedGroup.best_plan.cny_per_quota_usd, 4) }}/额度 USD · 分组 {{ formatNumber(selectedGroup.effective_multiplier, 2) }}x · {{ selectedGroup.best_plan.name }}
                </template>
                <template v-else>
                  折扣后人民币价
                </template>
              </strong>
              <strong v-else>暂无套餐，显示参考价</strong>
            </div>
            <div class="metric-tile">
              <span>优惠力度</span>
			  <strong class="text-emerald-300">{{ selectedGroup.best_plan ? saveFactorLabel(groupSaleMultiplier(selectedGroup)) : '按计费基准价' }}</strong>
            </div>
          </section>

          <section v-if="isAdmin && selectedGroup && issueModels.length > 0" class="price-alert mt-4" :class="{ critical: criticalIssueCount > 0 }">
            <div>
              <strong>价格来源异常 {{ issueModels.length }} 个</strong>
              <span>严重 {{ criticalIssueCount }} 个 · 提醒 {{ warningIssueCount }} 个 · {{ issueSummaryText }}</span>
            </div>
            <RouterLink class="toolbar-button" to="/admin/channels/pricing">
              去补价格
            </RouterLink>
          </section>

          <div v-if="loading" class="flex min-h-[420px] items-center justify-center text-sm text-[var(--text-muted)]">
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
          <div v-else-if="filteredModels.length === 0" class="flex min-h-[420px] items-center justify-center text-sm text-[var(--text-muted)]">
            {{ showOnlyIssues ? '暂无价格异常模型' : '暂无可展示模型' }}
          </div>
          <template v-else>
          <section v-if="isAdmin && selectedGroup" class="model-selection-bar" :class="{ active: selectedModelNames.size > 0 }" data-testid="model-bulk-panel">
            <div class="model-selection-copy">
              <span>模型批量管理</span>
              <strong>{{ selectedModelNames.size > 0 ? `已选 ${selectedModelNames.size} 个模型` : '勾选模型后批量隐藏或恢复' }}</strong>
            </div>
            <div class="bulk-action-strip">
              <button class="toolbar-button" :disabled="filteredModels.length === 0" data-testid="model-select-visible-bar" @click="toggleAllVisibleModelsSelected">
                {{ allVisibleModelsSelected ? '取消全选' : '全选当前' }}
              </button>
              <button class="toolbar-button danger" :disabled="selectedModelNames.size === 0 || savingHiddenModels" data-testid="model-bulk-hide" @click="bulkSetModelsHidden(true)">
                隐藏所选
              </button>
              <button class="toolbar-button" :disabled="selectedModelNames.size === 0 || savingHiddenModels" data-testid="model-bulk-restore" @click="bulkSetModelsHidden(false)">
                恢复所选
              </button>
              <button class="toolbar-button ghost" :disabled="selectedModelNames.size === 0 || savingHiddenModels" data-testid="model-bulk-clear" @click="clearSelectedModels">
                清空
              </button>
            </div>
          </section>
          <section class="mt-4 hidden overflow-hidden rounded-lg border border-[var(--border-color)] bg-[var(--bg-panel)] xl:block transition-all duration-200">
            <div class="price-table-header" :class="{ admin: isAdmin }">
              <span>模型</span>
			  <span>计费基准价</span>
              <span>折扣后价格</span>
              <span>阶梯价格</span>
              <span>优惠力度</span>
              <span v-if="isAdmin">来源</span>
            </div>
            <article
              v-for="model in filteredModels"
              :key="model.platform + ':' + model.name"
              class="price-table-row"
              :class="{ admin: isAdmin, 'hidden-model': model.hidden }"
              data-testid="model-price-row"
              :data-model-name="model.name"
            >
              <div class="flex min-w-0 items-center gap-3">
                <input
                  v-if="isAdmin"
                  class="bulk-checkbox"
                  type="checkbox"
                  :checked="selectedModelNames.has(model.name)"
                  :data-testid="`model-select-${model.name}`"
                  @change="toggleModelSelected(model.name)"
                />
                <ProviderMark :provider="displayProvider(model)" />
                <div class="min-w-0">
                  <div class="flex min-w-0 items-center gap-2">
                    <h2 class="truncate text-sm font-bold text-[var(--text-main)]" :title="model.name">{{ model.name }}</h2>
                    <button class="copy-button" title="复制模型名" @click="copyText(model.name)">
                      <Icon name="copy" size="xs" />
                    </button>
                  </div>
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="tag">{{ providerLabel(displayProvider(model)) }}</span>
                    <span class="tag tag-purple">{{ billingLabel(model.billing_mode) }}</span>
                    <span v-if="isAdmin && model.custom_price" class="tag tag-amber">自定义价</span>
                    <span v-if="isAdmin && model.hidden" class="tag tag-amber">已隐藏</span>
                    <span v-if="isAdmin && model.channel_names.length > 0" class="tag tag-blue">{{ model.channel_names.length }} 来源</span>
                  </div>
                </div>
              </div>

              <div v-if="isFixedPriceModel(model)" class="price-pair muted">
                <strong>{{ fixedPriceUnitLabel(model) }} {{ formatUSDPerUnit(model.official.per_request_usd, fixedPriceUnit(model)) }}</strong>
                <small>官方美元价</small>
              </div>
              <div v-else class="price-pair muted">
                <strong>输入 {{ formatUSDPerMillion(model.official.input_usd_per_m) }}</strong>
                <strong>输出 {{ formatUSDPerMillion(model.official.output_usd_per_m) }}</strong>
                <small>官方美元价</small>
              </div>

              <div v-if="isFixedPriceModel(model)" class="price-pair actual">
                <strong>{{ fixedPriceUnitLabel(model) }} {{ formatCNYPerUnit(bestActual(model, 'per_request_cny'), fixedPriceUnit(model)) }}</strong>
                <small>{{ formatUSDPerUnit(bestActual(model, 'per_request_usd'), fixedPriceUnit(model)) }}</small>
              </div>
              <div v-else class="price-pair actual">
                <strong>输入 {{ formatCNYPerMillion(bestActual(model, 'input_cny_per_m')) }}</strong>
                <strong>输出 {{ formatCNYPerMillion(bestActual(model, 'output_cny_per_m')) }}</strong>
                <small>{{ actualUSDLine(model) }}</small>
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
                <small v-if="priceIssueSeverity(model) !== 'none'">{{ priceIssueDetail(model) }}</small>
                <button class="inline-edit-button" @click="openPriceEditor(model)">改价</button>
                <button class="inline-edit-button danger" :disabled="savingHiddenModels" @click="toggleModelHidden(model)">
                  {{ model.hidden ? '恢复' : '隐藏' }}
                </button>
              </div>
            </article>
          </section>

          <section class="mt-4 grid gap-3 lg:grid-cols-2 xl:hidden">
            <article
              v-for="model in filteredModels"
              :key="model.platform + ':' + model.name"
              class="model-card group"
              :class="{ 'hidden-model': model.hidden }"
              data-testid="model-price-card"
              :data-model-name="model.name"
            >
              <div class="flex items-start gap-3">
                <input
                  v-if="isAdmin"
                  class="bulk-checkbox mt-1"
                  type="checkbox"
                  :checked="selectedModelNames.has(model.name)"
                  :data-testid="`mobile-model-select-${model.name}`"
                  @change="toggleModelSelected(model.name)"
                />
                <ProviderMark :provider="displayProvider(model)" />
                <div class="min-w-0 flex-1">
                  <div class="flex min-w-0 items-start justify-between gap-3">
                    <h2 class="truncate text-base font-bold text-[var(--text-main)]" :title="model.name">{{ model.name }}</h2>
                    <button class="copy-button" title="复制模型名" @click="copyText(model.name)">
                      <Icon name="copy" size="xs" />
                    </button>
                  </div>
                  <div class="mt-1.5 flex flex-wrap items-center gap-1.5">
                    <span class="tag">{{ providerLabel(displayProvider(model)) }}</span>
                    <span class="tag tag-purple">{{ billingLabel(model.billing_mode) }}</span>
                    <span v-if="isAdmin && model.custom_price" class="tag tag-amber">自定义价</span>
                    <span v-if="isAdmin && model.hidden" class="tag tag-amber">已隐藏</span>
                    <span v-if="tierBadges(model).length > 0" class="tag tag-blue">阶梯价格</span>
                  </div>
                </div>
              </div>

              <div class="mt-4 rounded-lg border border-emerald-500/15 bg-emerald-500/[0.02] dark:border-emerald-400/10 dark:bg-emerald-400/[0.035] p-3">
                <div class="mb-2 flex items-center justify-between gap-2">
				  <span class="text-[11px] font-bold text-emerald-600 dark:text-emerald-200">计费基准价</span>
                </div>
                <div v-if="isFixedPriceModel(model)" class="space-y-2">
                  <PriceRow :label="`${fixedPriceUnitLabel(model)}价格`" :value="priceValue(model.official.per_request_usd)" :suffix="`/ ${fixedPriceUnit(model)}`" />
                </div>
                <div v-else class="space-y-2">
                  <PriceRow label="输入价格" :value="priceValue(model.official.input_usd_per_m)" suffix="/ 百万 tokens" />
                  <PriceRow v-if="model.official.image_input_usd_per_m != null" label="图片输入" :value="priceValue(model.official.image_input_usd_per_m)" suffix="/ 百万 tokens" />
                  <PriceRow label="输出价格" :value="priceValue(model.official.output_usd_per_m)" suffix="/ 百万 tokens" />
                  <PriceRow label="缓存读取" :value="priceValue(model.official.cache_read_usd_per_m)" suffix="/ 百万 tokens" />
				  <PriceRow v-if="model.official.cache_write_5m_usd_per_m != null" label="缓存创建（5 分钟）" :value="priceValue(model.official.cache_write_5m_usd_per_m)" suffix="/ 百万 tokens" />
				  <PriceRow v-if="model.official.cache_write_1h_usd_per_m != null" label="缓存创建（1 小时）" :value="priceValue(model.official.cache_write_1h_usd_per_m)" suffix="/ 百万 tokens" />
				  <PriceRow v-if="model.official.cache_write_5m_usd_per_m == null && model.official.cache_write_1h_usd_per_m == null" label="缓存创建" :value="priceValue(model.official.cache_write_usd_per_m)" suffix="/ 百万 tokens" />
                </div>
              </div>

              <div class="mt-3 rounded-lg border border-cyan-500/15 bg-cyan-500/[0.02] dark:border-cyan-400/10 dark:bg-cyan-400/[0.035] p-3">
                <div class="mb-2 flex items-center justify-between gap-2">
                  <span class="text-[11px] font-bold text-cyan-600 dark:text-cyan-200">折扣后人民币价</span>
                  <span v-if="selectedGroup?.best_plan" class="truncate text-[11px] text-[var(--text-light)]" :title="selectedGroup.best_plan.name">
                    {{ selectedGroup.best_plan.name }}
                  </span>
                </div>
                <div v-if="isFixedPriceModel(model)" class="space-y-2">
                  <PriceRow :label="`${fixedPriceUnitLabel(model)}价格`" :value="bestActual(model, 'per_request_cny')" :secondary="formatUSDPerUnit(bestActual(model, 'per_request_usd'), fixedPriceUnit(model))" currency="cny" :suffix="`/ ${fixedPriceUnit(model)}`" highlight />
                </div>
                <div v-else class="space-y-2">
                  <PriceRow label="输入价格" :value="bestActual(model, 'input_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'input_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" highlight />
                  <PriceRow v-if="model.actual.image_input_usd_per_m != null" label="图片输入" :value="bestActual(model, 'image_input_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'image_input_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" />
                  <PriceRow label="输出价格" :value="bestActual(model, 'output_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'output_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" highlight />
                  <PriceRow label="缓存读取" :value="bestActual(model, 'cache_read_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'cache_read_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" />
				  <PriceRow v-if="model.actual.cache_write_5m_usd_per_m != null" label="缓存创建（5 分钟）" :value="bestActual(model, 'cache_write_5m_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'cache_write_5m_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" />
				  <PriceRow v-if="model.actual.cache_write_1h_usd_per_m != null" label="缓存创建（1 小时）" :value="bestActual(model, 'cache_write_1h_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'cache_write_1h_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" />
				  <PriceRow v-if="model.actual.cache_write_5m_usd_per_m == null && model.actual.cache_write_1h_usd_per_m == null" label="缓存创建" :value="bestActual(model, 'cache_write_cny_per_m')" :secondary="formatUSDPerMillion(bestActual(model, 'cache_write_usd_per_m'))" currency="cny" suffix="/ 百万 tokens" />
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

              <div class="mt-4 flex items-center justify-between gap-3">
                <span class="rounded-full bg-violet-500/[0.1] dark:bg-violet-500/[0.12] px-2.5 py-1 text-xs font-semibold text-violet-700 dark:text-violet-200">{{ billingLabel(model.billing_mode) }}</span>
                <div v-if="isAdmin" class="flex items-center gap-2">
                  <button class="toolbar-button" @click="openPriceEditor(model)">改价</button>
                  <button class="toolbar-button danger" :disabled="savingHiddenModels" @click="toggleModelHidden(model)">
                    {{ model.hidden ? '恢复' : '隐藏' }}
                  </button>
                </div>
                <span v-if="showMultiplier && isActuallyCheaper(model.cheaper_factor)" class="save-badge">
                  便宜 {{ formatNumber(model.cheaper_factor, 1) }} 倍
                </span>
                <span v-else-if="selectedGroup?.best_plan" class="save-badge neutral">
                  折扣后价格
                </span>
              </div>
            </article>
          </section>
          </template>
        </main>
      </div>

      <div v-if="priceEditor" class="price-editor-overlay" @click.self="closePriceEditor">
        <section class="price-editor-dialog">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <p class="text-sm font-bold text-[var(--text-muted)]">自定义展示价</p>
              <h3 class="mt-1 truncate text-lg font-black text-[var(--text-main)]" :title="priceEditor.model.name">{{ priceEditor.model.name }}</h3>
              <p class="mt-1 text-xs text-[var(--text-light)]">{{ selectedGroup?.name }} · 美元价格，保存后优先展示给用户</p>
            </div>
            <button class="copy-button" title="关闭" @click="closePriceEditor">
              <Icon name="x" size="xs" />
            </button>
          </div>

          <div class="mt-4 grid gap-3">
            <label class="editor-field">
              <span>计费方式</span>
              <select v-model="priceEditor.billingMode">
                <option value="token">按量计费</option>
                <option value="per_request">按次计费</option>
                <option value="image">图片计费</option>
                <option value="video">视频按秒计费</option>
              </select>
            </label>

            <template v-if="priceEditor.billingMode === 'token'">
              <div class="editor-grid">
                <label class="editor-field">
                  <span>输入 $/百万 tokens</span>
                  <input v-model="priceEditor.inputUSDPerM" type="number" step="any" min="0" placeholder="例如 3.2" />
                </label>
                <label class="editor-field">
                  <span>输出 $/百万 tokens</span>
                  <input v-model="priceEditor.outputUSDPerM" type="number" step="any" min="0" placeholder="例如 11.2" />
                </label>
                <label class="editor-field">
                  <span>缓存写入 $/百万 tokens</span>
                  <input v-model="priceEditor.cacheWriteUSDPerM" type="number" step="any" min="0" placeholder="可不填" />
                </label>
                <label class="editor-field">
                  <span>缓存读取 $/百万 tokens</span>
                  <input v-model="priceEditor.cacheReadUSDPerM" type="number" step="any" min="0" placeholder="可不填" />
                </label>
              </div>
            </template>
            <template v-else>
              <label class="editor-field">
                <span>{{ priceEditor.billingMode === 'image' ? '图片 $/张' : priceEditor.billingMode === 'video' ? '视频 $/秒' : '固定 $/次' }}</span>
                <input v-model="priceEditor.perRequestUSD" type="number" step="any" min="0" placeholder="例如 0.05" />
              </label>
            </template>
          </div>

          <div class="mt-5 flex flex-wrap items-center justify-between gap-2">
            <button class="toolbar-button danger" :disabled="savingCustomPrice || !priceEditor.model.custom_price" @click="clearCustomPrice">
              清除自定义价
            </button>
            <div class="flex items-center gap-2">
              <button class="toolbar-button" @click="closePriceEditor">取消</button>
              <button class="toolbar-button active" :disabled="savingCustomPrice" @click="saveCustomPrice">
                <Icon name="check" size="xs" />
                保存
              </button>
            </div>
          </div>
        </section>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import modelPricesAPI, {
  type ModelPriceCustomPrice,
  type ModelPriceGroup,
  type ModelPriceModel,
  type ModelPriceResponse,
  type ModelPriceTier,
} from '@/api/modelPrices'
import { useAppStore, useAuthStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'

type FilterTone = 'purple' | 'cyan' | 'amber' | 'emerald'
type GroupCategory = 'all' | 'claude' | 'openai' | 'gemini' | 'domestic'
type PriceKind = 'all' | 'metered' | 'fixed'
type ActualPriceKey = keyof ModelPriceModel['actual']
interface TierBadge {
  key: string
  label: string
  detail: string
}
interface PriceEditorState {
  model: ModelPriceModel
  billingMode: string
  inputUSDPerM: string
  outputUSDPerM: string
  cacheWriteUSDPerM: string
  cacheReadUSDPerM: string
  perRequestUSD: string
}

const FilterSection = defineComponent({
  props: {
    title: { type: String, required: true },
  },
  setup(props, { slots }) {
    return () => h('section', { class: 'mb-6 border-t border-[var(--border-color)] pt-4' }, [
      h('h3', { class: 'mb-3 px-1 text-sm font-bold text-[var(--text-main)]' }, props.title),
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
    showCheckbox: { type: Boolean, default: false },
    checked: { type: Boolean, default: false },
    testId: { type: String, default: '' },
    checkboxTestId: { type: String, default: '' },
  },
  emits: ['click', 'toggle'],
  setup(props, { emit }) {
    return () => h('button', {
      class: ['filter-pill', `tone-${props.tone}`, props.active ? 'active' : ''],
      'data-testid': props.testId || undefined,
      onClick: () => emit('click'),
    }, [
      props.showCheckbox
        ? h('input', {
            class: 'bulk-checkbox',
            type: 'checkbox',
            checked: props.checked,
            'data-testid': props.checkboxTestId || undefined,
            onClick: (event: Event) => event.stopPropagation(),
            onChange: (event: Event) => {
              event.stopPropagation()
              emit('toggle')
            },
          })
        : null,
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
    secondary: { type: String, default: '' },
    currency: { type: String as () => 'usd' | 'cny', default: 'usd' },
    highlight: { type: Boolean, default: false },
  },
  setup(props) {
    return () => h('div', { class: 'price-row' }, [
      h('span', { class: 'text-slate-500 dark:text-slate-400' }, props.label),
      h('strong', { class: props.highlight ? 'text-emerald-600 dark:text-emerald-300' : 'text-slate-800 dark:text-slate-200' }, [
        props.value == null ? '暂未定价' : (props.currency === 'cny' ? formatCNY(props.value) : formatUSD(props.value)),
        props.value == null ? '' : h('small', { class: 'ml-1 text-[11px] font-medium text-slate-500 dark:text-slate-400' }, props.suffix),
        props.secondary && props.secondary !== '-' ? h('em', { class: 'block text-[11px] font-medium not-italic text-slate-500 dark:text-slate-400' }, props.secondary) : null,
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
const savingPricingSettings = ref(false)
const showGroupDrawer = ref(false)
const showOfficial = ref(false)
const showMultiplier = ref(false)
const showOnlyIssues = ref(false)
const showOnlyHiddenModels = ref(false)
const includeCatalog = ref(false)
const showHiddenGroups = ref(false)
const showHiddenModels = ref(false)
const savingCustomPrice = ref(false)
const savingHiddenModels = ref(false)
const selectedGroupIDs = ref<Set<number>>(new Set())
const selectedModelNames = ref<Set<string>>(new Set())
const priceEditor = ref<PriceEditorState | null>(null)
const pricingSettings = ref({
  usdCnyRate: 7,
  cnyPerQuotaUSD: 0.068,
})

const isAdmin = computed(() => authStore.isAdmin)
const groups = computed(() => sortGroupsForSales(response.value?.groups ?? []))
const selectedGroup = computed<ModelPriceGroup | undefined>(() =>
  groups.value.find((group) => group.id === selectedGroupID.value),
)
const models = computed(() => response.value?.models ?? [])

const groupPlatformFilters = computed(() => {
  const counts = countBy(groups.value, group => groupCategoryKey(group.platform, group.name))
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
    if (selectedGroupCategory.value !== 'all' && groupCategoryKey(group.platform, group.name) !== selectedGroupCategory.value) return false
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
    if (model.hidden && (!isAdmin.value || !showHiddenModels.value)) return false
    if (showOnlyHiddenModels.value && !model.hidden) return false
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
const hiddenModelCount = computed(() => models.value.filter(model => model.hidden).length)
const allVisibleGroupsSelected = computed(() =>
  groupFilters.value.length > 0 && groupFilters.value.every(group => selectedGroupIDs.value.has(group.id)),
)
const allVisibleModelsSelected = computed(() =>
  filteredModels.value.length > 0 && filteredModels.value.every(model => selectedModelNames.value.has(model.name)),
)
const criticalIssueCount = computed(() =>
  issueModels.value.filter(model => priceIssueSeverity(model) === 'critical').length,
)
const warningIssueCount = computed(() =>
  issueModels.value.filter(model => priceIssueSeverity(model) === 'warning').length,
)
const issueSummaryText = computed(() => {
  const items = issueModels.value.slice(0, 4).map(item => `${item.name}：${priceIssueDetail(item)}`)
  return items.length > 0 ? items.join('、') : '暂无异常'
})
const pricingPreviewText = computed(() => {
  const groupMultiplier = normalizePositive(selectedGroup.value?.effective_multiplier)
  const cnyPerQuotaUSD = normalizePositive(pricingSettings.value.cnyPerQuotaUSD)
  const price = groupMultiplier * cnyPerQuotaUSD
  return `试算：官方 $1/M × 分组 ${formatNumber(groupMultiplier, 4)}x = ${formatCNY(price)} / 百万 tokens`
})
const profitSafety = computed(() => {
  if (!selectedGroup.value) {
    return {
      tone: 'neutral',
      title: '选择分组后检查',
      detail: '选择具体分组后，会按当前换算参数检查模型展示价是否缺失或异常偏低。',
    }
  }
  const visible = filteredModels.value
  const priced = visible.filter(model => hasActualPrice(model))
  const critical = visible.filter(model => priceIssueSeverity(model) === 'critical')
  const lowPrice = priced.filter(model => isSuspiciouslyLowPrice(model))
  if (critical.length > 0) {
    return {
      tone: 'danger',
      title: `有 ${critical.length} 个模型缺关键价格`,
      detail: `先补齐 ${critical.slice(0, 3).map(model => model.name).join('、')}，否则客户看到的价格可能不完整。`,
    }
  }
  if (lowPrice.length > 0) {
    return {
      tone: 'warning',
      title: `有 ${lowPrice.length} 个模型价格异常偏低`,
      detail: `${lowPrice.slice(0, 3).map(model => model.name).join('、')} 的折扣后价低于安全阈值，请确认官方价和分组倍率。`,
    }
  }
  if (priced.length === 0) {
    return {
      tone: 'warning',
      title: '当前没有可检查价格',
      detail: '该分组暂无可展示模型价格，建议检查账号模型映射和渠道定价。',
    }
  }
  return {
    tone: 'safe',
    title: '当前展示价未发现明显风险',
    detail: `已检查 ${priced.length} 个模型，按 ¥${formatNumber(response.value?.cny_per_quota_usd ?? 0.068, 4)}/额度 USD 与分组倍率计算。`,
  }
})

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

watch(showHiddenModels, () => {
  loadPrices(selectedGroupID.value)
})

watch(showOnlyHiddenModels, (enabled) => {
  if (enabled && !showHiddenModels.value) showHiddenModels.value = true
})

watch(filteredModels, () => {
  const visible = new Set(filteredModels.value.map(model => model.name))
  selectedModelNames.value = new Set([...selectedModelNames.value].filter(name => visible.has(name)))
})

watch(groupFilters, () => {
  const visible = new Set(groupFilters.value.map(group => group.id))
  selectedGroupIDs.value = new Set([...selectedGroupIDs.value].filter(id => visible.has(id)))
})

async function loadPrices(groupID?: number) {
  loading.value = true
  try {
    response.value = await modelPricesAPI.getModelPrices({
      group_id: groupID,
      include_catalog: includeCatalog.value,
      show_hidden_groups: showHiddenGroups.value,
      show_hidden_models: showHiddenModels.value,
    })
    syncPricingSettingsFromResponse()
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

function syncPricingSettingsFromResponse() {
  if (!response.value) return
  pricingSettings.value = {
    usdCnyRate: response.value.usd_cny_rate || 7,
    cnyPerQuotaUSD: response.value.cny_per_quota_usd || 0.068,
  }
}

async function savePricingSettings() {
  if (!isAdmin.value || savingPricingSettings.value) return
  const usdCnyRate = Number(pricingSettings.value.usdCnyRate)
  const cnyPerQuotaUSD = Number(pricingSettings.value.cnyPerQuotaUSD)
  if (!Number.isFinite(usdCnyRate) || usdCnyRate <= 0) {
    appStore.showError('USD/CNY 必须大于 0')
    return
  }
  if (!Number.isFinite(cnyPerQuotaUSD) || cnyPerQuotaUSD <= 0) {
    appStore.showError('¥/额度 USD 必须大于 0')
    return
  }
  savingPricingSettings.value = true
  try {
    await adminAPI.settings.updateSettings({
      model_price_usd_cny_rate: usdCnyRate,
      model_price_cny_per_quota_usd: cnyPerQuotaUSD,
    })
    appStore.showSuccess('销售换算参数已保存')
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '保存销售换算参数失败'))
  } finally {
    savingPricingSettings.value = false
  }
}

function resetFilters() {
  selectedGroupCategory.value = 'all'
  selectedPriceKind.value = 'all'
  groupSearchQuery.value = ''
  searchQuery.value = ''
  showOnlyIssues.value = false
  showOnlyHiddenModels.value = false
  selectedGroupIDs.value = new Set()
  selectedModelNames.value = new Set()
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

function toggleShowHiddenModels() {
  showHiddenModels.value = !showHiddenModels.value
  if (!showHiddenModels.value) showOnlyHiddenModels.value = false
}

function toggleGroupSelected(groupID: number) {
  const next = new Set(selectedGroupIDs.value)
  if (next.has(groupID)) next.delete(groupID)
  else next.add(groupID)
  selectedGroupIDs.value = next
}

function toggleAllVisibleGroupsSelected() {
  if (allVisibleGroupsSelected.value) {
    selectedGroupIDs.value = new Set()
    return
  }
  selectedGroupIDs.value = new Set(groupFilters.value.map(group => group.id))
}

function clearSelectedGroups() {
  selectedGroupIDs.value = new Set()
}

function toggleModelSelected(model: string) {
  const next = new Set(selectedModelNames.value)
  if (next.has(model)) next.delete(model)
  else next.add(model)
  selectedModelNames.value = next
}

function toggleAllVisibleModelsSelected() {
  if (allVisibleModelsSelected.value) {
    selectedModelNames.value = new Set()
    return
  }
  selectedModelNames.value = new Set(filteredModels.value.map(model => model.name))
}

function clearSelectedModels() {
  selectedModelNames.value = new Set()
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

async function bulkSetGroupsHidden(hidden: boolean) {
  if (!isAdmin.value || selectedGroupIDs.value.size === 0 || savingHiddenGroups.value) return
  const current = new Set(response.value?.hidden_group_ids ?? [])
  for (const id of selectedGroupIDs.value) {
    if (hidden) current.add(id)
    else current.delete(id)
  }
  savingHiddenGroups.value = true
  try {
    const result = await modelPricesAPI.updateHiddenGroups([...current])
    if (response.value) response.value.hidden_group_ids = result.hidden_group_ids
    selectedGroupIDs.value = new Set()
    appStore.showSuccess(hidden ? '已批量隐藏分组' : '已批量恢复分组')
    if (selectedGroupID.value && hidden && !showHiddenGroups.value && current.has(selectedGroupID.value)) {
      selectedGroupID.value = undefined
      await loadPrices()
    } else {
      await loadPrices(selectedGroupID.value)
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '批量保存分组隐藏状态失败'))
  } finally {
    savingHiddenGroups.value = false
  }
}

async function toggleModelHidden(model: ModelPriceModel) {
  if (!isAdmin.value || !selectedGroupID.value || savingHiddenModels.value) return
  savingHiddenModels.value = true
  try {
    const result = await modelPricesAPI.updateHiddenModel(selectedGroupID.value, model.name, !model.hidden)
    if (response.value) response.value.hidden_model_keys = result.hidden_model_keys
    appStore.showSuccess(model.hidden ? '已恢复模型展示' : '已隐藏模型')
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '保存模型隐藏状态失败'))
  } finally {
    savingHiddenModels.value = false
  }
}

async function bulkSetModelsHidden(hidden: boolean) {
  if (!isAdmin.value || !selectedGroupID.value || selectedModelNames.value.size === 0 || savingHiddenModels.value) return
  savingHiddenModels.value = true
  try {
    const result = await modelPricesAPI.updateHiddenModels(selectedGroupID.value, [...selectedModelNames.value], hidden)
    if (response.value) response.value.hidden_model_keys = result.hidden_model_keys
    selectedModelNames.value = new Set()
    appStore.showSuccess(hidden ? '已批量隐藏模型' : '已批量恢复模型')
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '批量保存模型隐藏状态失败'))
  } finally {
    savingHiddenModels.value = false
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

function openPriceEditor(model: ModelPriceModel) {
  const custom = model.custom_price
  const billingMode = normalizeBillingMode(custom?.billing_mode || model.billing_mode)
  priceEditor.value = {
    model,
    billingMode,
    inputUSDPerM: priceInput(custom?.input_usd_per_m ?? bestActual(model, 'input_usd_per_m')),
    outputUSDPerM: priceInput(custom?.output_usd_per_m ?? bestActual(model, 'output_usd_per_m')),
    cacheWriteUSDPerM: priceInput(custom?.cache_write_usd_per_m ?? bestActual(model, 'cache_write_usd_per_m')),
    cacheReadUSDPerM: priceInput(custom?.cache_read_usd_per_m ?? bestActual(model, 'cache_read_usd_per_m')),
    perRequestUSD: priceInput(custom?.per_request_usd ?? bestActual(model, 'per_request_usd')),
  }
}

function closePriceEditor() {
  if (savingCustomPrice.value) return
  priceEditor.value = null
}

async function saveCustomPrice() {
  if (!isAdmin.value || !selectedGroupID.value || !priceEditor.value || savingCustomPrice.value) return
  const editor = priceEditor.value
  const payload: ModelPriceCustomPrice = editor.billingMode === 'token'
    ? {
        billing_mode: editor.billingMode,
        input_usd_per_m: parseOptionalPrice(editor.inputUSDPerM),
        output_usd_per_m: parseOptionalPrice(editor.outputUSDPerM),
        cache_write_usd_per_m: parseOptionalPrice(editor.cacheWriteUSDPerM),
        cache_read_usd_per_m: parseOptionalPrice(editor.cacheReadUSDPerM),
      }
    : {
        billing_mode: editor.billingMode,
        per_request_usd: parseOptionalPrice(editor.perRequestUSD),
      }
  if (!customPriceHasValue(payload)) {
    appStore.showError('至少填写一个价格')
    return
  }
  savingCustomPrice.value = true
  try {
    await modelPricesAPI.updateCustomPrice({
      group_id: selectedGroupID.value,
      model: editor.model.name,
      ...payload,
    })
    appStore.showSuccess('自定义价格已保存')
    priceEditor.value = null
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '保存自定义价格失败'))
  } finally {
    savingCustomPrice.value = false
  }
}

async function clearCustomPrice() {
  if (!isAdmin.value || !selectedGroupID.value || !priceEditor.value || savingCustomPrice.value) return
  savingCustomPrice.value = true
  try {
    await modelPricesAPI.updateCustomPrice({
      group_id: selectedGroupID.value,
      model: priceEditor.value.model.name,
      billing_mode: normalizeBillingMode(priceEditor.value.model.billing_mode),
      clear: true,
    })
    appStore.showSuccess('自定义价格已清除')
    priceEditor.value = null
    await loadPrices(selectedGroupID.value)
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, '清除自定义价格失败'))
  } finally {
    savingCustomPrice.value = false
  }
}

function priceInput(value: number | null | undefined): string {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 ? String(value) : ''
}

function priceValue(value: number | null | undefined): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

function parseOptionalPrice(value: string): number | null {
  const trimmed = value.trim()
  if (!trimmed) return null
  const parsed = Number.parseFloat(trimmed)
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null
}

function customPriceHasValue(price: ModelPriceCustomPrice): boolean {
  return [
    price.input_usd_per_m,
    price.output_usd_per_m,
    price.cache_write_usd_per_m,
    price.cache_read_usd_per_m,
    price.image_output_usd_per_m,
    price.per_request_usd,
  ].some(value => typeof value === 'number' && Number.isFinite(value) && value >= 0)
}

function normalizeBillingMode(mode: string | undefined): string {
  if (mode === 'image') return 'image'
  if (mode === 'video') return 'video'
  if (mode === 'per_request' || mode === 'request') return 'per_request'
  return 'token'
}

function bestActual(model: ModelPriceModel, key: ActualPriceKey): number | undefined {
  const base = model.actual[key]
  if (typeof base === 'number' && Number.isFinite(base) && base >= 0) return base
  for (const tier of model.price_tiers) {
    const value = tier.actual[key]
    if (typeof value === 'number' && Number.isFinite(value) && value >= 0) return value
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
	const label = !tier.threshold_tokens ? tier.label : `${tier.label} ${formatThresholdTokens(tier.threshold_tokens)}`
	return tier.requires_account_long_context ? `${label}（需账号开启长上下文计费）` : label
}

function tierPriceSummary(tier: ModelPriceTier): string {
  const values: Array<[string, number | null | undefined]> = [
    ['入', tier.actual.input_usd_per_m],
    ['出', tier.actual.output_usd_per_m],
    ['缓存写', tier.actual.cache_write_usd_per_m],
	['缓存写5m', tier.actual.cache_write_5m_usd_per_m],
	['缓存写1h', tier.actual.cache_write_1h_usd_per_m],
    ['缓存读', tier.actual.cache_read_usd_per_m],
  ]
  const parts = values
    .filter((item): item is [string, number] => typeof item[1] === 'number' && Number.isFinite(item[1]))
    .map(([label, value]) => `${label} ${formatUSDPerMillion(value)}`)
  return parts.length > 0 ? parts.join(' / ') : '暂未定价'
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

function groupCategoryKey(platform: string, name = ''): GroupCategory {
  const text = `${platform} ${name}`.toLowerCase()
  if (text.includes('国模') || text.includes('deepseek') || text.includes('qwen') || text.includes('kimi') || text.includes('glm') || text.includes('minimax') || text.includes('moonshot') || text.includes('doubao')) return 'domestic'
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
  if (lower.includes('deepseek')) return 'DeepSeek'
  if (lower.includes('qwen') || lower.includes('tongyi')) return 'Qwen'
  if (lower.includes('kimi') || lower.includes('moonshot')) return 'Kimi'
  if (lower.includes('glm') || lower.includes('zhipu')) return 'GLM'
  if (lower.includes('minimax')) return 'MiniMax'
  if (lower.includes('doubao')) return '豆包'
  if (lower.includes('anthropic')) return 'Anthropic'
  if (lower.includes('openai')) return 'OpenAI'
  if (lower.includes('gemini')) return 'Gemini'
  if (lower.includes('antigravity')) return 'Antigravity'
  return platform || '未知供应商'
}

function providerShort(platform: string): string {
  const label = providerLabel(platform)
  if (label === 'DeepSeek') return 'DS'
  if (label === 'Qwen') return 'QW'
  if (label === 'Kimi') return 'KM'
  if (label === 'GLM') return 'GL'
  if (label === 'MiniMax') return 'MI'
  if (label === '豆包') return '豆'
  if (label === 'Anthropic') return '✹'
  if (label === 'OpenAI') return '◎'
  if (label === 'Gemini') return 'G'
  if (label === 'Antigravity') return 'AG'
  return label.slice(0, 2).toUpperCase()
}

function billingLabel(mode: string): string {
  if (mode === 'request' || mode === 'per_request') return '按次计费'
  if (mode === 'image') return '图片计费'
  if (mode === 'video') return '视频按秒计费'
  return '按量计费'
}

function displayProvider(model: ModelPriceModel): string {
  const name = model.name.toLowerCase()
  if (name.startsWith('deepseek')) return 'deepseek'
  if (name.startsWith('qwen') || name.startsWith('qwq')) return 'qwen'
  if (name.startsWith('kimi') || name.includes('moonshot')) return 'kimi'
  if (name.startsWith('glm')) return 'glm'
  if (name.startsWith('minimax')) return 'minimax'
  if (name.startsWith('doubao')) return 'doubao'
  return model.provider || model.platform
}

function hasConfiguredNumber(value: number | null | undefined): boolean {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

function hasActualPrice(model: ModelPriceModel): boolean {
  const values = Object.values(model.actual)
  if (values.some(value => hasConfiguredNumber(value))) return true
  return model.price_tiers.some(tier => Object.values(tier.actual).some(value => hasConfiguredNumber(value)))
}

function isFixedPriceModel(model: ModelPriceModel): boolean {
  return model.billing_mode === 'request' ||
    model.billing_mode === 'per_request' ||
    model.billing_mode === 'image' ||
    model.billing_mode === 'video' ||
    hasConfiguredNumber(model.official.per_request_usd) ||
    hasConfiguredNumber(model.actual.per_request_usd)
}

function fixedPriceUnit(model: ModelPriceModel): string {
  const name = model.name.toLowerCase()
  if (model.billing_mode === 'image' || name.includes('image') || name.includes('photo') || name.includes('picture') || name.includes('dall-e')) return '张'
  if (model.billing_mode === 'video' || name.includes('video')) return '秒'
  if (name.includes('audio') || name.includes('speech') || name.includes('tts') || name.includes('transcrib')) return '分钟'
  return '次'
}

function fixedPriceUnitLabel(model: ModelPriceModel): string {
  const unit = fixedPriceUnit(model)
  if (unit === '张') return '每张'
  if (unit === '秒') return '每秒'
  if (unit === '分钟') return '每分钟'
  return '每次'
}

function sourceLabel(source: string): string {
  if (source === 'custom') return '自定义价'
  if (source === 'official') return '官方目录'
	if (source === 'fallback') return '内置回退价'
  if (source === 'channel') return '渠道配置'
  if (source === 'group') return '分组配置'
  return '待补价'
}

function priceIssueDetail(model: ModelPriceModel): string {
  if (model.pricing_source === 'unknown' || !hasActualPrice(model)) return '缺渠道价或实际展示价'
  if (model.official_missing) return '缺官方参考价'
  if (isFixedPriceModel(model) && !fixedPriceUnit(model)) return '固定价格单位不明确'
  if (model.channel_names.length === 0) return '没有可用渠道来源'
  return '来源正常'
}

function priceIssueSeverity(model: ModelPriceModel): 'none' | 'warning' | 'critical' {
  if (model.pricing_source === 'custom') return 'none'
  if (model.pricing_source === 'unknown' || !hasActualPrice(model)) return 'critical'
  if (model.official_missing) return 'warning'
  return 'none'
}

function isSuspiciouslyLowPrice(model: ModelPriceModel): boolean {
  const candidates = [
    bestActual(model, 'input_cny_per_m'),
    bestActual(model, 'output_cny_per_m'),
    bestActual(model, 'cache_write_cny_per_m'),
    bestActual(model, 'cache_read_cny_per_m'),
    bestActual(model, 'per_request_cny'),
  ].filter((value): value is number => typeof value === 'number' && Number.isFinite(value))
  if (candidates.length === 0) return false
  if (candidates.some(value => value <= 0)) return true
  const safetyFloor = normalizePositive(selectedGroup.value?.effective_multiplier) *
    normalizePositive(response.value?.cny_per_quota_usd ?? pricingSettings.value.cnyPerQuotaUSD) *
    0.01
  return candidates.some(value => value < safetyFloor)
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

function formatCNY(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '-'
  return `¥${formatNumber(value, value < 1 ? 4 : 2)}`
}

function formatUSDPerMillion(value: number | null | undefined): string {
  const formatted = formatUSD(value)
  return formatted === '-' ? '-' : `${formatted} / 百万 tokens`
}

function formatCNYPerMillion(value: number | null | undefined): string {
  const formatted = formatCNY(value)
  return formatted === '-' ? '-' : `${formatted} / 百万 tokens`
}

function formatUSDPerUnit(value: number | null | undefined, unit: string): string {
  const formatted = formatUSD(value)
  return formatted === '-' ? '-' : `${formatted} / ${unit}`
}

function formatCNYPerUnit(value: number | null | undefined, unit: string): string {
  const formatted = formatCNY(value)
  return formatted === '-' ? '-' : `${formatted} / ${unit}`
}

function actualUSDLine(model: ModelPriceModel): string {
  const input = formatUSDPerMillion(bestActual(model, 'input_usd_per_m'))
  const output = formatUSDPerMillion(bestActual(model, 'output_usd_per_m'))
  if (input === '-' && output === '-') return selectedGroup.value?.best_plan?.name || ''
  return `美元参考：输入 ${input} · 输出 ${output}`
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
  color-scheme: light dark;
  --bg-main: #f8fafc;
  --bg-sidebar: #ffffff;
  --bg-panel: #ffffff;
  --bg-card: #ffffff;
  --bg-card-hover: #f1f5f9;
  --bg-input: #ffffff;
  --bg-button: #f1f5f9;
  --bg-button-hover: #e2e8f0;
  --bg-header: #f1f5f9;

  --text-main: #0f172a;
  --text-muted: #475569;
  --text-light: #64748b;
  --text-white: #ffffff;

  --border-color: #e2e8f0;
  --border-light: #f1f5f9;

  --tag-bg: #f1f5f9;
  --tag-text: #475569;
  --tag-purple-bg: #f3e8ff;
  --tag-purple-text: #6b21a8;
  --tag-blue-bg: #dbeafe;
  --tag-blue-text: #1e40af;
  --tag-amber-bg: #fef3c7;
  --tag-amber-text: #92400e;

  --input-focus-border: #3b82f6;

  --price-actual: #16a34a;
  --price-muted: #475569;

  --alert-bg: #fffbeb;
  --alert-border: #fde68a;
  --alert-text: #92400e;

  --alert-critical-bg: #fef2f2;
  --alert-critical-border: #fca5a5;
  --alert-critical-text: #991b1b;
}

.hero-stat {
  min-width: 110px;
  border-radius: 8px;
  border: 1px solid rgba(255, 255, 255, 0.18);
  background: rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(10px);
  padding: 9px 11px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
  transition: transform 0.2s ease, background 0.2s ease;
}

.hero-stat:hover {
  transform: translateY(-2px);
  background: rgba(255, 255, 255, 0.14);
}

.hero-stat span {
  display: block;
  font-size: 10px;
  font-weight: 700;
  color: rgba(255, 255, 255, 0.72);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.hero-stat strong {
  margin-top: 4px;
  display: block;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  font-weight: 800;
  color: #ffffff;
}

.filter-pill {
  display: flex;
  min-width: 0;
  min-height: 38px;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border-radius: 7px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 7px 10px;
  color: var(--text-muted);
  font-size: 12px;
  font-weight: 700;
  text-align: left;
  transition: all 0.2s ease;
  cursor: pointer;
}

.filter-pill:hover {
  background: var(--bg-card-hover);
  color: var(--text-main);
  border-color: var(--input-focus-border);
}

.filter-pill.active {
  background: var(--bg-button-hover);
  color: var(--text-main);
  border-color: var(--input-focus-border);
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
  background: var(--tag-bg);
  color: var(--text-muted);
  transition: all 0.2s ease;
}

.filter-pill.active .pill-count {
  background: var(--tag-bg);
}

.tone-purple.active .pill-count { color: var(--tag-purple-text); }
.tone-cyan.active .pill-count { color: var(--tag-blue-text); }
.tone-amber.active .pill-count { color: var(--tag-amber-text); }
.tone-emerald.active .pill-count { color: var(--price-actual); }

.toolbar-button {
  display: inline-flex;
  height: 32px;
  align-items: center;
  gap: 6px;
  border-radius: 7px;
  border: 1px solid var(--border-color);
  background: var(--bg-button);
  padding: 0 10px;
  color: var(--text-muted);
  font-weight: 700;
  cursor: pointer;
  transition: all 0.2s ease;
}

.toolbar-button.active,
.toolbar-button:hover:not(:disabled) {
  background: var(--bg-button-hover);
  color: var(--text-main);
  border-color: var(--input-focus-border);
}

.toolbar-button:disabled {
  cursor: not-allowed;
  opacity: 0.56;
}

.toolbar-button.danger {
  border-color: rgba(239, 68, 68, 0.2);
  color: #ef4444;
}

.toolbar-button.danger:hover:not(:disabled) {
  background: rgba(239, 68, 68, 0.08);
  color: #dc2626;
}


.toolbar-button.ghost {
  color: var(--text-light);
}

.selection-panel {
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 10px;
}

.selection-panel-row,
.model-selection-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.selection-panel span,
.model-selection-copy span {
  display: block;
  font-size: 11px;
  font-weight: 700;
  color: var(--text-light);
}

.selection-panel strong,
.model-selection-copy strong {
  display: block;
  margin-top: 3px;
  font-size: 13px;
  color: var(--text-main);
}

.bulk-action-strip {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.selection-panel .bulk-action-strip {
  margin-top: 10px;
  border-top: 1px solid var(--border-color);
  padding-top: 10px;
}

.model-selection-bar {
  margin-top: 16px;
  min-height: 58px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 12px 14px;
  transition: all 0.2s ease;
}

.model-selection-bar.active {
  border-color: var(--alert-critical-border);
  background: var(--alert-critical-bg);
}

.model-selection-bar.active .model-selection-copy strong {
  color: var(--alert-critical-text);
}

.bulk-check-row {
  display: grid;
  min-height: 34px;
  grid-template-columns: 18px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  border-radius: 7px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 6px 8px;
  color: var(--text-main);
  font-size: 12px;
}

.bulk-check-row small {
  color: var(--text-light);
}

.bulk-checkbox {
  height: 15px;
  width: 15px;
  accent-color: var(--input-focus-border);
}

.price-editor-overlay {
  position: fixed;
  inset: 0;
  z-index: 60;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.45);
  backdrop-filter: blur(4px);
  padding: 20px;
}

.price-editor-dialog {
  width: min(94vw, 560px);
  max-height: min(86vh, 720px);
  overflow-y: auto;
  border-radius: 10px;
  border: 1px solid var(--border-color);
  background: var(--bg-sidebar);
  padding: 18px;
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
}

.editor-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px;
}

.editor-field {
  display: grid;
  gap: 6px;
}

.editor-field span {
  font-size: 12px;
  font-weight: 800;
  color: var(--text-muted);
}

.editor-field input,
.editor-field select {
  height: 38px;
  border-radius: 7px;
  border: 1px solid var(--border-color);
  background: var(--bg-input);
  padding: 0 10px;
  color: var(--text-main);
  outline: none;
  transition: all 0.2s ease;
}

.editor-field input:focus,
.editor-field select:focus {
  border-color: var(--input-focus-border);
}

@media (max-width: 640px) {
  .editor-grid {
    grid-template-columns: 1fr;
  }

  .model-selection-bar,
  .selection-panel-row {
    align-items: stretch;
    flex-direction: column;
  }

  .bulk-action-strip .toolbar-button {
    flex: 1 1 auto;
    justify-content: center;
  }
}

.mobile-group-overlay {
  position: fixed;
  inset: 0;
  z-index: 50;
  background: rgba(0, 0, 0, 0.45);
  backdrop-filter: blur(2px);
}

.mobile-group-drawer {
  width: min(92vw, 380px);
  height: 100%;
  overflow-y: auto;
  border-right: 1px solid var(--border-color);
  background: var(--bg-sidebar);
  padding: 18px;
}

.admin-price-panel {
  display: grid;
  gap: 12px;
  grid-template-columns: repeat(4, minmax(0, 1fr));
}

.pricing-settings-card {
  align-items: stretch;
  gap: 12px;
}

.pricing-settings-form {
  display: grid;
  gap: 8px;
}

.pricing-settings-form label {
  display: grid;
  gap: 4px;
}

.pricing-settings-form label span {
  font-size: 11px;
  font-weight: 800;
  color: var(--text-light);
}

.pricing-settings-form input {
  height: 32px;
  border-radius: 7px;
  border: 1px solid var(--border-color);
  background: var(--bg-input);
  padding: 0 9px;
  color: var(--text-main);
  outline: none;
  transition: all 0.2s ease;
}

.pricing-settings-form input:focus {
  border-color: var(--input-focus-border);
}

.pricing-preview {
  margin-top: 2px;
  color: var(--input-focus-border);
  font-size: 11px;
  font-weight: 800;
}

.sales-note {
  display: grid;
  grid-template-columns: minmax(0, 1.5fr) minmax(280px, 0.85fr);
  gap: 12px;
}

.sales-note > div {
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 14px;
}

.sales-note span {
  display: block;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-light);
}

.sales-note strong {
  display: block;
  margin-top: 5px;
  color: var(--text-main);
  font-size: 15px;
}

.sales-note p {
  margin-top: 6px;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.6;
}

.sales-note-admin.safe {
  border-color: rgba(34, 197, 94, 0.2);
  background: rgba(34, 197, 94, 0.05);
}

.sales-note-admin.safe strong {
  color: #16a34a;
}

.sales-note-admin.warning {
  border-color: rgba(245, 158, 11, 0.2);
  background: rgba(245, 158, 11, 0.05);
}

.sales-note-admin.warning strong {
  color: #d97706;
}

.sales-note-admin.danger {
  border-color: rgba(239, 68, 68, 0.2);
  background: rgba(239, 68, 68, 0.05);
}

.sales-note-admin.danger strong {
  color: #dc2626;
}

.ops-card {
  display: flex;
  min-height: 92px;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 14px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.01);
  transition: transform 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;
}

.ops-card:hover {
  border-color: var(--input-focus-border);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
  transform: translateY(-1px);
}

.ops-card div {
  min-width: 0;
}

.ops-card span {
  display: block;
  font-size: 12px;
  color: var(--text-light);
}

.ops-card strong {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 14px;
  color: var(--text-main);
}

.ops-card small {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: var(--text-muted);
}

@media (max-width: 1279px) {
  .admin-price-panel {
    grid-template-columns: 1fr;
  }

  .sales-note {
    grid-template-columns: 1fr;
  }
}

.group-overview {
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-panel);
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
  color: var(--text-main);
}

.overview-heading p {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-muted);
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
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 13px;
  text-align: left;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.01);
  transition: transform 0.2s ease, border-color 0.2s ease, background 0.2s ease, box-shadow 0.2s ease;
}

.overview-card:hover,
.overview-group:hover {
  border-color: var(--input-focus-border);
  background: var(--bg-card-hover);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
  transform: translateY(-2px);
}

.overview-card span,
.overview-group span {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  color: var(--text-light);
}

.overview-card strong,
.overview-group strong {
  display: block;
  margin-top: 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  color: var(--text-main);
}

.overview-card small {
  display: block;
  margin-top: 5px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: var(--text-muted);
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
  border: 1px solid var(--alert-border);
  background: var(--alert-bg);
  padding: 12px 14px;
}

.price-alert.critical {
  border-color: var(--alert-critical-border);
  background: var(--alert-critical-bg);
}

.price-alert strong,
.price-alert span {
  display: block;
}

.price-alert strong {
  color: var(--alert-text);
  font-size: 13px;
}

.price-alert.critical strong {
  color: var(--alert-critical-text);
}

.price-alert span {
  margin-top: 4px;
  font-size: 12px;
  color: var(--text-muted);
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
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 14px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.01);
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

.metric-tile:hover {
  border-color: var(--input-focus-border);
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.04);
}

.metric-tile span {
  display: block;
  font-size: 12px;
  color: var(--text-light);
}

.metric-tile strong {
  margin-top: 6px;
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 15px;
  color: var(--text-main);
}

.model-card {
  min-height: 186px;
  border-radius: 10px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  padding: 16px;
  box-shadow: 0 1px 3px rgba(0,0,0,0.05);
  transition: all 0.2s ease;
}

.model-card:hover {
  border-color: var(--input-focus-border);
  background: var(--bg-card-hover);
  box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06);
}

.price-table-header {
  display: grid;
  min-height: 44px;
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid var(--border-color);
  background: var(--bg-header);
  padding: 0 16px;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-light);
}

.price-table-header.admin {
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px 140px;
}

.price-table-row {
  display: grid;
  min-height: 94px;
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px;
  align-items: center;
  gap: 16px;
  border-bottom: 1px solid var(--border-light);
  padding: 14px 16px;
  transition: all 0.2s ease;
}

.price-table-row.admin {
  grid-template-columns: minmax(280px, 1.35fr) minmax(220px, 1fr) minmax(200px, 0.9fr) minmax(180px, 0.9fr) 120px 140px;
}

.price-table-row:last-child {
  border-bottom: 0;
}

.price-table-row:hover {
  background: var(--bg-card-hover);
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
  color: var(--price-actual);
}

.price-pair.muted span {
  color: var(--price-muted);
}

.price-pair small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11px;
  color: var(--text-light);
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
  border: 1px solid rgba(14, 165, 233, 0.15);
  background: rgba(14, 165, 233, 0.05);
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 700;
  color: #0284c7;
  transition: all 0.2s ease;
}

.tier-chip.dim {
  border-color: var(--border-color);
  background: var(--bg-button);
  color: var(--text-muted);
}

.source-cell {
  display: grid;
  gap: 5px;
  font-size: 12px;
  font-weight: 800;
  color: var(--text-muted);
}

.source-cell small {
  font-size: 11px;
  font-weight: 700;
  color: #d97706;
}

.inline-edit-button {
  width: fit-content;
  border-radius: 999px;
  border: 1px solid rgba(13, 148, 136, 0.22);
  background: rgba(13, 148, 136, 0.08);
  padding: 3px 8px;
  font-size: 11px;
  font-weight: 800;
  color: #0d9488;
  cursor: pointer;
  transition: all 0.2s ease;
}

.inline-edit-button:hover {
  border-color: rgba(13, 148, 136, 0.5);
  background: rgba(13, 148, 136, 0.18);
}

.provider-mark {
  display: flex;
  width: 36px;
  height: 36px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 999px;
  background: var(--bg-button);
  border: 1px solid var(--border-color);
  font-size: 18px;
  font-weight: 900;
}

.provider-mark.large {
  width: 48px;
  height: 48px;
  font-size: 24px;
}

.provider-mark.openai {
  color: var(--text-muted);
}

.provider-mark.anthropic {
  color: #ea580c;
}

.copy-button {
  display: inline-flex;
  width: 24px;
  height: 24px;
  align-items: center;
  justify-content: center;
  border-radius: 6px;
  border: 1px solid var(--border-color);
  background: var(--bg-card);
  color: var(--text-light);
  cursor: pointer;
  transition: all 0.2s ease;
}

.copy-button:hover {
  color: var(--text-main);
  background: var(--bg-button-hover);
  border-color: var(--input-focus-border);
}

.tag {
  border-radius: 999px;
  background: var(--tag-bg);
  padding: 2px 7px;
  font-size: 11px;
  font-weight: 700;
  color: var(--text-muted);
}

.tag-purple {
  color: var(--tag-purple-text);
  background: var(--tag-purple-bg);
}

.tag-blue {
  color: var(--tag-blue-text);
  background: var(--tag-blue-bg);
}

.tag-amber {
  color: var(--tag-amber-text);
  background: var(--tag-amber-bg);
}

.tier-strip {
  margin-top: 10px;
  display: grid;
  gap: 6px;
  border-radius: 8px;
  border: 1px solid var(--border-color);
  background: var(--bg-card-hover);
  padding: 8px;
}

.tier-line {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 10px;
  font-size: 11px;
  color: var(--text-muted);
}

.tier-line strong {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 800;
  color: var(--text-main);
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
  background: rgba(22, 163, 74, 0.1);
  padding: 4px 9px;
  color: #16a34a;
  font-size: 12px;
  font-weight: 800;
}

.save-badge.neutral {
  background: var(--tag-bg);
  color: var(--text-muted);
}
</style>

<style>
/* Unscoped dark mode overrides specifically prefixed for model-market to avoid compiler stripping */
.dark .model-market {
  color-scheme: dark;
  --bg-main: #0f1014;
  --bg-sidebar: #101114;
  --bg-panel: #14161b;
  --bg-card: #14161b;
  --bg-card-hover: #171a21;
  --bg-input: #1c1e23;
  --bg-button: rgba(255, 255, 255, 0.035);
  --bg-button-hover: rgba(255, 255, 255, 0.12);
  --bg-header: #191c23;

  --text-main: #cbd5e1;
  --text-muted: #94a3b8;
  --text-light: #64748b;
  --text-white: #ffffff;

  --border-color: rgba(255, 255, 255, 0.08);
  --border-light: rgba(255, 255, 255, 0.05);

  --tag-bg: rgba(255, 255, 255, 0.07);
  --tag-text: rgb(203 213 225);
  --tag-purple-bg: rgba(139, 92, 246, 0.14);
  --tag-purple-text: #c4b5fd;
  --tag-blue-bg: rgba(59, 130, 246, 0.14);
  --tag-blue-text: #93c5fd;
  --tag-amber-bg: rgba(245, 158, 11, 0.14);
  --tag-amber-text: #fde68a;

  --input-focus-border: #38bdf8;

  --price-actual: #86efac;
  --price-muted: rgb(148 163 184);

  --alert-bg: rgba(251, 191, 36, 0.08);
  --alert-border: rgba(251, 191, 36, 0.22);
  --alert-text: #fde68a;

  --alert-critical-bg: rgba(248, 113, 113, 0.09);
  --alert-critical-border: rgba(248, 113, 113, 0.28);
  --alert-critical-text: #fecaca;
}

.dark .model-market .toolbar-button.danger {
  border-color: rgba(248, 113, 113, 0.2);
  color: #fecaca;
}

.dark .model-market .toolbar-button.danger:hover:not(:disabled) {
  background: rgba(248, 113, 113, 0.12);
  color: #fca5a5;
}

.dark .model-market .price-editor-dialog {
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.5);
}

.dark .model-market .sales-note-admin.safe strong {
  color: #86efac;
}

.dark .model-market .sales-note-admin.warning strong {
  color: #fde68a;
}

.dark .model-market .sales-note-admin.danger strong {
  color: #fecaca;
}

.dark .model-market .tier-chip {
  border: 1px solid rgba(125, 211, 252, 0.14);
  background: rgba(14, 165, 233, 0.08);
  color: #bae6fd;
}

.dark .model-market .source-cell small {
  color: #fbbf24;
}

.dark .model-market .inline-edit-button {
  border: 1px solid rgba(45, 212, 191, 0.22);
  background: rgba(20, 184, 166, 0.1);
  color: #5eead4;
}

.dark .model-market .inline-edit-button:hover {
  border-color: rgba(94, 234, 212, 0.5);
  background: rgba(20, 184, 166, 0.18);
}

.dark .model-market .provider-mark.openai {
  color: #dbeafe;
}

.dark .model-market .provider-mark.anthropic {
  color: #fb923c;
}

.dark .model-market .copy-button:hover {
  color: white;
  background: rgba(255, 255, 255, 0.08);
}

.dark .model-market .tag {
  background: rgba(255, 255, 255, 0.07);
  color: rgb(203 213 225);
}

.dark .model-market .save-badge {
  background: rgba(16, 185, 129, 0.13);
  color: #6ee7b7;
}
</style>
