<template>
  <AppLayout>
    <div class="space-y-5 pb-10">
      <section class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-sky-50 text-sky-600 dark:bg-sky-900/30 dark:text-sky-300">
                <Icon name="chart" size="md" />
              </span>
              <div class="min-w-0">
                <h1 class="truncate text-xl font-semibold text-gray-900 dark:text-white">账号健康</h1>
                <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                  <span>生成 {{ formatTime(response?.generated_at) }}</span>
                  <span v-if="lastUpdated">刷新 {{ formatLocalTime(lastUpdated) }}</span>
                  <span v-if="settingsForm.enabled" class="rounded-full bg-emerald-100 px-2 py-0.5 font-medium text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">通知规则已启用</span>
                  <span v-else class="rounded-full bg-slate-100 px-2 py-0.5 font-medium text-slate-600 dark:bg-slate-700 dark:text-slate-300">通知规则已关闭</span>
                </div>
              </div>
            </div>
          </div>

          <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
            <label class="min-w-0">
              <span class="sr-only">平台</span>
              <input
                v-model.trim="platformFilter"
                type="text"
                class="input h-9 min-w-[10rem]"
                placeholder="platform"
                @keyup.enter="fetchData"
              />
            </label>
            <label class="min-w-0">
              <span class="sr-only">分组 ID</span>
              <select
                v-model.trim="groupIdInput"
                class="input h-9 min-w-[10rem]"
                @change="fetchData"
              >
                <option value="">全部分组</option>
                <option v-for="group in healthGroupOptions" :key="group.id" :value="String(group.id)">
                  {{ group.name }}
                </option>
              </select>
            </label>
            <label class="inline-flex h-9 items-center gap-2 whitespace-nowrap text-xs text-gray-500 dark:text-gray-400">
              <input v-model="showHistoricalGroups" type="checkbox" class="checkbox" />
              <span>显示历史分组</span>
            </label>
            <button type="button" class="btn btn-secondary h-9 whitespace-nowrap" :disabled="loading" @click="fetchData">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span>刷新</span>
            </button>
            <button type="button" class="btn btn-secondary h-9 whitespace-nowrap" @click="openNotificationRules">
              <Icon name="bell" size="sm" />
              <span>告警规则</span>
            </button>
          </div>
        </div>
      </section>

      <section v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-200">
        {{ errorMessage }}
      </section>

      <section class="grid grid-cols-2 gap-3 lg:grid-cols-6">
        <div v-for="card in summaryCards" :key="card.label" class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-2 text-2xl font-semibold" :class="card.className">{{ card.value }}</div>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div class="flex min-w-0 flex-1 flex-col gap-3">
            <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
              <label class="min-w-0 flex-1">
                <span class="sr-only">搜索账号</span>
                <input
                  v-model.trim="searchQuery"
                  type="text"
                  class="input h-9"
                  placeholder="搜索账号、ID、平台、分组、建议、模型、标签"
                />
              </label>
              <div class="inline-flex h-9 w-fit rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-900">
                <button
                  type="button"
                  class="rounded-md px-3 text-sm font-semibold transition"
                  :class="viewMode === 'card' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                  @click="viewMode = 'card'"
                >
                  卡片
                </button>
                <button
                  type="button"
                  class="rounded-md px-3 text-sm font-semibold transition"
                  :class="viewMode === 'list' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                  @click="viewMode = 'list'"
                >
                  列表
                </button>
              </div>
            </div>
            <div class="flex gap-2 overflow-x-auto pb-1">
              <button
                v-for="filter in quickFilters"
                :key="filter.key"
                type="button"
                class="shrink-0 rounded-full border px-3 py-1 text-xs font-semibold transition"
                :class="quickFilter === filter.key ? 'border-sky-300 bg-sky-100 text-sky-700 dark:border-sky-800 dark:bg-sky-900/40 dark:text-sky-200' : 'border-gray-200 bg-gray-50 text-gray-500 hover:border-gray-300 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400 dark:hover:text-white'"
                @click="quickFilter = filter.key"
              >
                {{ filter.label }} {{ filter.count }}
              </button>
            </div>
            <div class="flex min-w-0 gap-2 overflow-x-auto pb-1">
              <button
                v-for="option in availableTagOptions"
                :key="option.tag"
                type="button"
                class="shrink-0 rounded-full border px-3 py-1 text-xs font-semibold transition"
                :class="isTagFilterSelected(option.tag) ? 'border-emerald-300 bg-emerald-100 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200' : 'border-gray-200 bg-gray-50 text-gray-500 hover:border-gray-300 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400 dark:hover:text-white'"
                @click="toggleTagFilter(option.tag)"
              >
                {{ option.tag }} {{ option.count }}
              </button>
              <button
                v-if="selectedTagFilters.length > 0"
                type="button"
                class="shrink-0 rounded-full border border-gray-200 bg-white px-3 py-1 text-xs font-semibold text-gray-500 hover:text-gray-900 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400 dark:hover:text-white"
                @click="clearTagFilters"
              >
                清空标签
              </button>
            </div>
          </div>
          <div class="shrink-0 text-xs text-gray-500 dark:text-gray-400">
            显示 {{ visibleItems.length }} / {{ items.length }}
          </div>
        </div>
      </section>

      <section
        v-if="selectedCount > 0"
        class="rounded-lg border border-sky-200 bg-sky-50 p-3 shadow-sm dark:border-sky-900/50 dark:bg-sky-900/20"
      >
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div class="text-sm font-semibold text-sky-800 dark:text-sky-100">已选择 {{ selectedCount }} 个账号</div>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="bulkSchedulableLoading" @click="bulkToggleSchedulable(true)">
              开启调度
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="bulkSchedulableLoading" @click="bulkToggleSchedulable(false)">
              关闭调度
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="bulkSchedulableLoading" @click="clearSelection">
              清空选择
            </button>
          </div>
        </div>
      </section>

      <section v-if="loading && !response" class="rounded-lg border border-gray-200 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        加载中...
      </section>

      <section v-else-if="!response?.enabled" class="rounded-lg border border-gray-200 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        运维监控未启用
      </section>

      <section v-else-if="visibleItems.length === 0" class="rounded-lg border border-gray-200 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        暂无账号健康数据
      </section>

      <section v-else-if="viewMode === 'list'" class="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="hidden min-w-[1680px] grid-cols-[44px_minmax(280px,1.45fr)_118px_repeat(5,126px)_minmax(190px,1fr)_150px_170px_150px] gap-3 border-b border-gray-200 bg-gray-50 px-3 py-2 text-xs font-semibold text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400 lg:grid">
          <label class="flex items-center justify-center">
            <input
              type="checkbox"
              class="h-4 w-4 rounded border-gray-300 text-sky-600 focus:ring-sky-500"
              :checked="allVisibleSelected"
              @change="toggleSelectAllVisible"
            />
          </label>
          <span>账号 / 标签</span>
          <span>调度</span>
          <span v-for="window in windowOrder" :key="`head-${window}`">{{ window }}</span>
          <span>最近60</span>
          <span>模型</span>
          <span>建议</span>
          <span class="text-right">操作</span>
        </div>
        <div class="divide-y divide-gray-200 dark:divide-dark-700">
          <div v-for="item in visibleItems" :key="item.account_id" class="lg:min-w-[1680px]">
            <article class="grid gap-3 px-3 py-3 lg:grid-cols-[44px_minmax(280px,1.45fr)_118px_repeat(5,126px)_minmax(190px,1fr)_150px_170px_150px] lg:items-center">
              <div class="flex items-center gap-2">
                <input
                  type="checkbox"
                  class="h-4 w-4 rounded border-gray-300 text-sky-600 focus:ring-sky-500"
                  :checked="isAccountSelected(item.account_id)"
                  @change="onAccountSelectionChange(item.account_id, $event)"
                />
                <button
                  type="button"
                  class="inline-flex h-7 w-7 items-center justify-center rounded-md border border-gray-200 text-gray-500 transition hover:text-gray-900 dark:border-dark-700 dark:text-gray-400 dark:hover:text-white"
                  @click="toggleExpanded(item.account_id)"
                >
                  <Icon :name="isExpanded(item.account_id) ? 'chevronDown' : 'chevronRight'" size="xs" />
                </button>
              </div>

              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-xs font-bold" :class="platformAvatarClass(item)">
                    {{ platformInitial(item.platform) }}
                  </span>
                  <div class="min-w-0">
                    <div class="flex min-w-0 items-center gap-2">
                      <span class="truncate text-sm font-semibold text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</span>
                      <span class="shrink-0 text-xs text-gray-400">#{{ item.account_id }}</span>
                    </div>
                    <div class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">{{ item.platform || '-' }} · {{ item.group_name || '-' }} · group {{ item.group_id }}</div>
                  </div>
                </div>
                <div class="mt-2 flex flex-wrap items-center gap-1.5">
                  <span
                    v-for="tag in visibleTagsForItem(item)"
                    :key="`${item.account_id}-tag-${tag}`"
                    class="rounded-md bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                  >
                    {{ tag }}
                  </span>
                  <span v-if="hiddenTagCount(item) > 0" class="rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-dark-700 dark:text-slate-300">
                    +{{ hiddenTagCount(item) }}
                  </span>
                  <span v-if="tagsForItem(item).length === 0" class="rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-dark-700 dark:text-slate-300">
                    无标签
                  </span>
                  <button
                    type="button"
                    class="rounded-md border border-gray-200 px-2 py-0.5 text-[11px] font-semibold text-gray-500 transition hover:text-gray-900 dark:border-dark-700 dark:text-gray-400 dark:hover:text-white"
                    @click="startEditingTags(item)"
                  >
                    标签
                  </button>
                </div>
                <div v-if="editingTagsAccountId === item.account_id" class="mt-2 rounded-lg border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-900">
                  <input
                    v-model="tagEditDraft"
                    type="text"
                    class="input h-8 text-xs"
                    placeholder="pro, plus, 生图"
                    @keyup.enter="updateAccountTags(item)"
                    @keyup.esc="cancelEditingTags"
                  />
                  <div class="mt-2 flex justify-end gap-1.5">
                    <button type="button" class="btn btn-secondary btn-sm" :disabled="isSavingTags(item.account_id)" @click="cancelEditingTags">取消</button>
                    <button type="button" class="btn btn-primary btn-sm" :disabled="isSavingTags(item.account_id)" @click="updateAccountTags(item)">
                      {{ isSavingTags(item.account_id) ? '保存中' : '保存' }}
                    </button>
                  </div>
                </div>
              </div>

              <div class="min-w-0">
                <button
                  type="button"
                  class="inline-flex h-8 w-full items-center justify-center gap-2 rounded-lg border px-2 text-xs font-semibold transition disabled:cursor-wait disabled:opacity-70"
                  :class="item.is_schedulable ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300' : 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-700/60 dark:text-gray-300'"
                  :disabled="isTogglingSchedulable(item.account_id)"
                  @click="toggleAccountSchedulable(item)"
                >
                  <span class="relative h-4 w-7 rounded-full transition" :class="item.is_schedulable ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-dark-600'">
                    <span class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition" :class="item.is_schedulable ? 'left-3.5' : 'left-0.5'"></span>
                  </span>
                  <span>{{ item.is_schedulable ? '调度开' : '调度关' }}</span>
                </button>
                <div class="mt-1 flex flex-wrap gap-1">
                  <StatusBadge :text="item.is_opened ? '开' : '关'" :kind="item.is_opened ? 'success' : 'muted'" />
                  <StatusBadge :text="item.is_available ? '可调度' : '不可调度'" :kind="item.is_available ? 'success' : 'warning'" />
                </div>
              </div>

              <div v-for="window in windowOrder" :key="`${item.account_id}-${window}`" class="rounded-lg bg-gray-50 px-2 py-1.5 dark:bg-dark-700/60">
                <div class="flex items-center justify-between gap-2 text-[11px] text-gray-400">
                  <span>{{ window }}</span>
                  <span>{{ windowCountText(item, window) }}</span>
                </div>
                <div class="mt-1 text-sm font-semibold" :class="windowMetricClass(item, window)">
                  {{ windowMetricText(item, window) }}
                </div>
                <div class="mt-1 truncate text-[11px] font-medium" :class="windowFirstTokenClass(item, window)">
                  首 {{ windowFirstTokenText(item, window) }}
                </div>
                <div class="mt-1 h-1 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-600">
                  <div class="h-full rounded-full" :class="windowBarClass(item, window)" :style="{ width: `${windowBarPercent(item, window)}%` }"></div>
                </div>
              </div>

              <div class="min-w-0">
                <div class="grid grid-cols-[repeat(60,minmax(2px,1fr))] gap-0.5">
                  <span
                    v-for="(sample, idx) in recentSamplesForDisplay(item)"
                    :key="`${item.account_id}-list-${idx}-${sample?.created_at || 'empty'}`"
                    class="h-4 rounded-sm"
                    :class="sampleClass(sample)"
                    :title="sampleTitle(sample)"
                  ></span>
                </div>
                <div class="mt-1 truncate text-[11px] text-gray-400">{{ recentTimelineTitle(item) }}</div>
              </div>

              <div class="min-w-0 text-xs text-gray-600 dark:text-gray-300">
                <div class="truncate font-semibold">{{ probeModelEffective(item) }}</div>
                <div class="truncate text-[11px] text-gray-400">{{ item.probe_model_id ? '账号独立模型' : '继承全局' }}</div>
              </div>

              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-1.5">
                  <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="severityClass(item.recommendation.severity)">
                    {{ item.recommendation.severity || 'P3' }}
                  </span>
                  <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="notifyClass(item.recommendation.notify_mode)">
                    {{ notifyLabel(item.recommendation.notify_mode) }}
                  </span>
                </div>
                <div class="mt-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ actionLabel(item.recommendation.action) }}</div>
                <div class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">{{ item.recommendation.title }}</div>
              </div>

              <div class="flex justify-end gap-1.5">
                <button type="button" class="btn btn-secondary btn-sm" @click="openDetail(item)">详情</button>
                <button type="button" class="btn btn-secondary btn-sm" @click="goToAccount(item)">账号</button>
              </div>
            </article>

            <div v-if="isExpanded(item.account_id)" class="px-3 pb-3">
              <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-900/70">
                <div class="grid grid-cols-1 gap-3 xl:grid-cols-[1.5fr_1fr_1fr]">
                  <div>
                    <div class="text-xs font-semibold text-gray-500 dark:text-gray-400">完整窗口矩阵</div>
                    <div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-5">
                      <div v-for="window in windowOrder" :key="`${item.account_id}-expanded-${window}`" class="rounded-lg border border-gray-200 bg-white p-2.5 dark:border-dark-700 dark:bg-dark-800">
                        <div class="flex items-center justify-between gap-2">
                          <span class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ window }}</span>
                          <span class="text-xs text-gray-400">{{ windowCountText(item, window) }}</span>
                        </div>
                        <div class="mt-2 text-lg font-semibold" :class="windowMetricClass(item, window)">{{ windowMetricText(item, window) }}</div>
                        <div class="mt-1 grid grid-cols-2 gap-1 text-[11px] text-gray-500 dark:text-gray-400">
                          <span>{{ windowLeftMeta(item, window) }}</span>
                          <span>{{ windowRightMeta(item, window) }}</span>
                          <span class="col-span-2" :class="windowFirstTokenClass(item, window)">首 {{ windowFirstTokenText(item, window) }}</span>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div class="rounded-lg border p-3" :class="recommendationPanelClass(item)">
                    <div class="flex flex-wrap items-center gap-1.5">
                      <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="severityClass(item.recommendation.severity)">{{ item.recommendation.severity || 'P3' }}</span>
                      <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="notifyClass(item.recommendation.notify_mode)">{{ notifyLabel(item.recommendation.notify_mode) }}</span>
                      <span v-if="item.probe?.checked_at" class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="probeStatusClass(item)">{{ probeStatusLabel(item) }}</span>
                    </div>
                    <div class="mt-3 text-sm font-semibold text-gray-900 dark:text-white">{{ item.recommendation.title }}</div>
                    <p class="mt-2 break-words text-xs text-gray-600 dark:text-gray-300">{{ item.recommendation.reason || '-' }}</p>
                    <div class="mt-3 grid grid-cols-2 gap-2 text-xs">
                      <div class="rounded-lg bg-white/60 p-2 dark:bg-dark-900/30">
                        <div class="text-gray-500 dark:text-gray-400">建议动作</div>
                        <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ actionLabel(item.recommendation.action) }}</div>
                      </div>
                      <div class="rounded-lg bg-white/60 p-2 dark:bg-dark-900/30">
                        <div class="text-gray-500 dark:text-gray-400">恢复状态</div>
                        <div class="mt-1 font-semibold" :class="item.recommendation.recovery_ready ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-700 dark:text-gray-200'">
                          {{ item.recommendation.recovery_ready ? '可恢复' : '未满足' }}
                        </div>
                      </div>
                    </div>
                  </div>
                  <div class="rounded-lg border border-gray-200 bg-white p-3 text-xs dark:border-dark-700 dark:bg-dark-800">
                    <div class="font-semibold text-gray-900 dark:text-white">运行说明</div>
                    <div class="mt-2 space-y-2 text-gray-600 dark:text-gray-300">
                      <div>探测状态：{{ probeText(item) || probeStatusLabel(item) }}</div>
                      <div>限流：{{ item.is_rate_limited ? cooldownText(item) || '限流中' : '无' }}</div>
                      <div>过载：{{ item.is_overloaded ? cooldownText(item) || '过载冷却中' : '无' }}</div>
                      <div>临停：{{ item.is_temp_unschedulable ? '临时不可调度' : '无' }}</div>
                      <div class="break-words">错误：{{ item.error_message || '无' }}</div>
                      <div>探测模型：{{ probeModelEffective(item) }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <section v-else class="grid grid-cols-1 gap-3 xl:grid-cols-2 2xl:grid-cols-4">
        <article
          v-for="item in visibleItems"
          :key="item.account_id"
          class="overflow-hidden rounded-lg border bg-white shadow-sm dark:bg-dark-800"
          :class="accountBorderClass(item)"
        >
          <div class="p-4">
            <div class="flex items-start justify-between gap-3">
              <div class="flex min-w-0 items-start gap-2.5">
                <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-sm font-bold" :class="platformAvatarClass(item)">
                  {{ platformInitial(item.platform) }}
                </div>
                <div class="min-w-0">
                  <div class="flex min-w-0 flex-wrap items-center gap-2">
                    <h2 class="min-w-0 truncate text-base font-semibold text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</h2>
                    <span class="shrink-0 text-xs text-gray-400">#{{ item.account_id }}</span>
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>{{ item.platform || '-' }}</span>
                    <span>{{ item.group_name || '-' }}</span>
                    <span>group {{ item.group_id }}</span>
                  </div>
                  <div class="mt-2 flex flex-wrap items-center gap-1.5">
                    <span
                      v-for="tag in visibleTagsForItem(item)"
                      :key="`${item.account_id}-card-tag-${tag}`"
                      class="rounded-md bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                    >
                      {{ tag }}
                    </span>
                    <span v-if="hiddenTagCount(item) > 0" class="rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-dark-700 dark:text-slate-300">+{{ hiddenTagCount(item) }}</span>
                    <span v-if="tagsForItem(item).length === 0" class="rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-dark-700 dark:text-slate-300">无标签</span>
                    <button
                      type="button"
                      class="rounded-md border border-gray-200 px-2 py-0.5 text-[11px] font-semibold text-gray-500 transition hover:text-gray-900 dark:border-dark-700 dark:text-gray-400 dark:hover:text-white"
                      @click="startEditingTags(item)"
                    >
                      标签
                    </button>
                  </div>
                  <div v-if="editingTagsAccountId === item.account_id" class="mt-2 rounded-lg border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-900">
                    <input
                      v-model="tagEditDraft"
                      type="text"
                      class="input h-8 text-xs"
                      placeholder="pro, plus, 生图"
                      @keyup.enter="updateAccountTags(item)"
                      @keyup.esc="cancelEditingTags"
                    />
                    <div class="mt-2 flex justify-end gap-1.5">
                      <button type="button" class="btn btn-secondary btn-sm" :disabled="isSavingTags(item.account_id)" @click="cancelEditingTags">取消</button>
                      <button type="button" class="btn btn-primary btn-sm" :disabled="isSavingTags(item.account_id)" @click="updateAccountTags(item)">
                        {{ isSavingTags(item.account_id) ? '保存中' : '保存' }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
              <div class="flex max-w-[16rem] shrink-0 flex-wrap items-center justify-end gap-1.5">
                <button
                  type="button"
                  class="btn btn-secondary btn-sm shrink-0"
                  @click="openDetail(item)"
                >
                  <Icon name="eye" size="xs" />
                  <span>详情</span>
                </button>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm shrink-0"
                  @click="goToAccount(item)"
                >
                  <Icon name="externalLink" size="xs" />
                  <span>账号</span>
                </button>
                <label
                  v-if="!item.is_opened"
                  class="inline-flex h-8 cursor-pointer items-center gap-2 rounded-lg border px-2 text-xs font-semibold transition"
                  :class="autoProbeToggleClass(item)"
                  :title="autoProbeToggleTitle(item)"
                >
                  <input
                    type="checkbox"
                    class="sr-only"
                    :checked="isAutoProbeEnabled(item)"
                    :disabled="isTogglingAutoProbe(item.account_id)"
                    @change="onAutoProbeChange(item, $event)"
                  />
                  <span class="relative h-4 w-7 rounded-full transition" :class="isAutoProbeEnabled(item) ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-dark-600'">
                    <span
                      class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition"
                      :class="isAutoProbeEnabled(item) ? 'left-3.5' : 'left-0.5'"
                    ></span>
                  </span>
                  <span>{{ isAutoProbeEnabled(item) ? '自动探测' : '停探测' }}</span>
                </label>
                <button
                  type="button"
                  class="btn btn-secondary btn-sm shrink-0"
                  :disabled="isProbing(item.account_id)"
                  @click="runProbe(item)"
                >
                  <Icon name="beaker" size="xs" :class="isProbing(item.account_id) ? 'animate-pulse' : ''" />
                  <span>{{ isProbing(item.account_id) ? '探测中' : '探测' }}</span>
                </button>
              </div>
            </div>

            <div class="mt-3 flex flex-wrap gap-2">
              <StatusBadge :text="item.is_opened ? '账号已打开' : '账号已关闭'" :kind="item.is_opened ? 'success' : 'muted'" />
              <StatusBadge :text="item.is_available ? '可调度' : '不可调度'" :kind="item.is_available ? 'success' : 'warning'" />
              <StatusBadge :text="probeModelText(item)" kind="muted" />
              <StatusBadge v-if="!item.is_opened && item.probe_auto_disabled" text="自动探测已关" kind="muted" />
              <StatusBadge v-if="!hasTraffic(item)" text="暂无流量" kind="muted" />
              <StatusBadge v-if="item.is_rate_limited" text="限流中" kind="warning" />
              <StatusBadge v-if="item.is_overloaded" text="过载冷却" kind="warning" />
              <StatusBadge v-if="item.is_temp_unschedulable" text="临时暂停" kind="warning" />
              <StatusBadge v-if="item.has_error" text="错误状态" kind="danger" />
              <button
                type="button"
                class="inline-flex h-7 items-center gap-2 rounded-lg border px-2 text-xs font-semibold transition disabled:cursor-wait disabled:opacity-70"
                :class="item.is_schedulable ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300' : 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-700/60 dark:text-gray-300'"
                :disabled="isTogglingSchedulable(item.account_id)"
                @click="toggleAccountSchedulable(item)"
              >
                <span class="relative h-4 w-7 rounded-full transition" :class="item.is_schedulable ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-dark-600'">
                  <span class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition" :class="item.is_schedulable ? 'left-3.5' : 'left-0.5'"></span>
                </span>
                <span>{{ item.is_schedulable ? '调度开' : '调度关' }}</span>
              </button>
            </div>
            <div class="mt-4 grid grid-cols-1 gap-2 sm:grid-cols-3">
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ primaryMetricLabel(item) }}</div>
                <div class="mt-1 truncate text-2xl font-semibold 2xl:text-3xl" :class="primaryMetricClass(item)">
                  {{ primaryMetricText(item) }}
                </div>
                <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ primaryMetricHint(item) }}</div>
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">延迟</div>
                <div class="mt-1 truncate text-2xl font-semibold text-gray-900 dark:text-white 2xl:text-3xl">
                  {{ latencyText(item) }}
                </div>
                <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ latencyHint(item) }}</div>
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">首 Token · 5m</div>
                <div class="mt-1 truncate text-2xl font-semibold 2xl:text-3xl" :class="firstTokenMetricClass(item)">
                  {{ firstTokenMetricText(item) }}
                </div>
                <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ firstTokenMetricHint(item) }}</div>
              </div>
            </div>

            <div class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-700">
              <div class="flex items-end justify-between gap-3">
                <div class="text-sm text-gray-500 dark:text-gray-400">{{ headlineHealthLabel(item) }}</div>
                <div class="text-3xl font-semibold 2xl:text-4xl" :class="headlineHealthClass(item)">
                  {{ headlineHealthText(item) }}
                </div>
              </div>
              <div class="mt-3 h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div
                  class="h-full rounded-full transition-all"
                  :class="headlineHealthBarClass(item)"
                  :style="{ width: `${headlineHealthPercent(item)}%` }"
                ></div>
              </div>
              <div class="mt-2 flex justify-between gap-3 text-xs text-gray-500 dark:text-gray-400">
                <span>{{ headlineHealthLeftMeta(item) }}</span>
                <span class="truncate text-right">{{ headlineHealthRightMeta(item) }}</span>
              </div>
              <div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3 2xl:grid-cols-5">
                <div v-for="window in windowOrder" :key="window" class="rounded-lg border border-gray-200 p-2.5 dark:border-dark-700">
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ window }}</span>
                    <span class="text-xs text-gray-400">{{ windowCountText(item, window) }}</span>
                  </div>
                  <div class="mt-2 text-lg font-semibold" :class="windowMetricClass(item, window)">
                    {{ windowMetricText(item, window) }}
                  </div>
                  <div v-if="item.is_opened" class="mt-1 truncate text-[11px] font-medium" :class="windowFirstTokenClass(item, window)">
                    首 {{ windowFirstTokenText(item, window) }}
                  </div>
                  <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <div
                      class="h-full rounded-full transition-all"
                      :class="windowBarClass(item, window)"
                      :style="{ width: `${windowBarPercent(item, window)}%` }"
                    ></div>
                  </div>
                  <div class="mt-2 flex justify-between text-[11px] text-gray-500 dark:text-gray-400">
                    <span>{{ windowLeftMeta(item, window) }}</span>
                    <span>{{ windowRightMeta(item, window) }}</span>
                  </div>
                </div>
              </div>
            </div>

            <div class="mt-4">
              <div class="flex items-center justify-between gap-3">
                <div class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ recentTimelineTitle(item) }}</div>
                <div class="text-xs text-gray-400">PAST -> NOW</div>
              </div>
              <div class="mt-2 grid grid-cols-[repeat(60,minmax(3px,1fr))] gap-1">
                <span
                  v-for="(sample, idx) in recentSamplesForDisplay(item)"
                  :key="`${item.account_id}-${idx}-${sample?.created_at || 'empty'}`"
                  class="h-7 min-w-0 rounded-sm"
                  :class="sampleClass(sample)"
                  :title="sampleTitle(sample)"
                ></span>
              </div>
            </div>

            <div class="mt-4 rounded-lg border p-3" :class="recommendationPanelClass(item)">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="severityClass(item.recommendation.severity)">
                      {{ item.recommendation.severity || 'P3' }}
                    </span>
                    <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="notifyClass(item.recommendation.notify_mode)">
                      {{ notifyLabel(item.recommendation.notify_mode) }}
                    </span>
                    <span v-if="item.probe?.checked_at" class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="probeStatusClass(item)">
                      {{ probeStatusLabel(item) }}
                    </span>
                  </div>
                  <h3 class="mt-3 text-base font-semibold text-gray-900 dark:text-white">{{ item.recommendation.title }}</h3>
                  <p class="mt-2 break-words text-sm text-gray-600 dark:text-gray-300">{{ item.recommendation.reason || '-' }}</p>
                </div>
                <Icon :name="recommendationIcon(item)" size="lg" :class="recommendationIconClass(item)" />
              </div>

              <div class="mt-4 grid grid-cols-2 gap-2">
                <div class="rounded-lg bg-white/60 p-2 text-xs dark:bg-dark-900/30">
                  <div class="text-gray-500 dark:text-gray-400">建议动作</div>
                  <div class="mt-1 font-semibold text-gray-900 dark:text-white">{{ actionLabel(item.recommendation.action) }}</div>
                </div>
                <div class="rounded-lg bg-white/60 p-2 text-xs dark:bg-dark-900/30">
                  <div class="text-gray-500 dark:text-gray-400">恢复状态</div>
                  <div class="mt-1 font-semibold" :class="item.recommendation.recovery_ready ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-700 dark:text-gray-200'">
                    {{ item.recommendation.recovery_ready ? '可恢复' : '未满足' }}
                  </div>
                </div>
              </div>

              <div v-if="cooldownText(item)" class="mt-3 rounded-lg bg-white/60 px-3 py-2 text-xs text-gray-600 dark:bg-dark-900/30 dark:text-gray-300">
                {{ cooldownText(item) }}
              </div>
            </div>
          </div>
        </article>
      </section>

      <div
        v-if="selectedAccount"
        class="fixed inset-0 z-50 flex justify-end bg-slate-950/45 backdrop-blur-sm"
        @click.self="closeDetail"
      >
        <aside class="h-full w-full max-w-3xl overflow-y-auto border-l border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900">
          <div class="sticky top-0 z-10 border-b border-gray-200 bg-white/95 px-4 py-3 backdrop-blur dark:border-dark-700 dark:bg-dark-900/95">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="flex min-w-0 items-center gap-2">
                  <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-sm font-bold" :class="platformAvatarClass(selectedAccount)">
                    {{ platformInitial(selectedAccount.platform) }}
                  </span>
                  <div class="min-w-0">
                    <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-white">{{ selectedAccount.account_name || `#${selectedAccount.account_id}` }}</h2>
                    <p class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400">
                      #{{ selectedAccount.account_id }} · {{ selectedAccount.platform || '-' }} · {{ selectedAccount.group_name || '-' }}
                    </p>
                  </div>
                </div>
              </div>
              <button type="button" class="btn btn-secondary btn-sm" @click="closeDetail">
                <Icon name="x" size="xs" />
                <span>关闭</span>
              </button>
            </div>
          </div>

          <div class="space-y-4 p-4">
            <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div class="min-w-0">
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">标签与调度</div>
                  <div class="mt-2 flex flex-wrap items-center gap-1.5">
                    <span
                      v-for="tag in visibleTagsForItem(selectedAccount)"
                      :key="`${selectedAccount.account_id}-detail-tag-${tag}`"
                      class="rounded-md bg-emerald-50 px-2 py-0.5 text-[11px] font-semibold text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300"
                    >
                      {{ tag }}
                    </span>
                    <span v-if="hiddenTagCount(selectedAccount) > 0" class="rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-dark-700 dark:text-slate-300">+{{ hiddenTagCount(selectedAccount) }}</span>
                    <span v-if="tagsForItem(selectedAccount).length === 0" class="rounded-md bg-slate-100 px-2 py-0.5 text-[11px] font-semibold text-slate-500 dark:bg-dark-700 dark:text-slate-300">无标签</span>
                  </div>
                </div>
                <div class="flex flex-wrap gap-2">
                  <button type="button" class="btn btn-secondary btn-sm" @click="startEditingTags(selectedAccount)">
                    标签
                  </button>
                  <button
                    type="button"
                    class="inline-flex h-8 items-center gap-2 rounded-lg border px-2 text-xs font-semibold transition disabled:cursor-wait disabled:opacity-70"
                    :class="selectedAccount.is_schedulable ? 'border-emerald-200 bg-emerald-50 text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300' : 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-700/60 dark:text-gray-300'"
                    :disabled="isTogglingSchedulable(selectedAccount.account_id)"
                    @click="toggleAccountSchedulable(selectedAccount)"
                  >
                    <span class="relative h-4 w-7 rounded-full transition" :class="selectedAccount.is_schedulable ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-dark-600'">
                      <span class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition" :class="selectedAccount.is_schedulable ? 'left-3.5' : 'left-0.5'"></span>
                    </span>
                    <span>{{ selectedAccount.is_schedulable ? '调度开' : '调度关' }}</span>
                  </button>
                </div>
              </div>
              <div v-if="editingTagsAccountId === selectedAccount.account_id" class="mt-3 rounded-lg border border-gray-200 bg-white p-2 shadow-sm dark:border-dark-700 dark:bg-dark-900">
                <input
                  v-model="tagEditDraft"
                  type="text"
                  class="input h-8 text-xs"
                  placeholder="pro, plus, 生图"
                  @keyup.enter="updateAccountTags(selectedAccount)"
                  @keyup.esc="cancelEditingTags"
                />
                <div class="mt-2 flex justify-end gap-1.5">
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="isSavingTags(selectedAccount.account_id)" @click="cancelEditingTags">取消</button>
                  <button type="button" class="btn btn-primary btn-sm" :disabled="isSavingTags(selectedAccount.account_id)" @click="updateAccountTags(selectedAccount)">
                    {{ isSavingTags(selectedAccount.account_id) ? '保存中' : '保存' }}
                  </button>
                </div>
              </div>
            </section>

            <section class="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ primaryMetricLabel(selectedAccount) }}</div>
                <div class="mt-1 text-3xl font-semibold" :class="primaryMetricClass(selectedAccount)">{{ primaryMetricText(selectedAccount) }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ primaryMetricHint(selectedAccount) }}</div>
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">延迟</div>
                <div class="mt-1 text-3xl font-semibold text-gray-900 dark:text-white">{{ latencyText(selectedAccount) }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ latencyHint(selectedAccount) }}</div>
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">首 Token · 5m</div>
                <div class="mt-1 text-3xl font-semibold" :class="firstTokenMetricClass(selectedAccount)">{{ firstTokenMetricText(selectedAccount) }}</div>
                <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ firstTokenMetricHint(selectedAccount) }}</div>
              </div>
            </section>

            <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
              <div class="flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between">
                <div class="min-w-0 flex-1">
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">账号探测模型</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">生效：{{ probeModelEffective(selectedAccount) }}，未配置时继承全局 {{ settingsForm.probe.model_id || DEFAULT_PROBE_MODEL }}</div>
                  <div class="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
                    <label class="min-w-0">
                      <span class="block text-[11px] font-medium text-gray-500 dark:text-gray-400">从账号模型列表选择</span>
                      <select
                        class="input mt-1 h-9"
                        :value="probeModelDraft(selectedAccount)"
                        :disabled="isLoadingProbeModels(selectedAccount.account_id)"
                        @focus="loadProbeModels(selectedAccount)"
                        @change="onProbeModelDraftInput(selectedAccount, $event)"
                      >
                        <option value="">继承全局模型</option>
                        <option v-for="option in probeModelOptions(selectedAccount)" :key="option.value" :value="option.value">
                          {{ option.label }}
                        </option>
                      </select>
                    </label>
                    <label class="min-w-0">
                      <span class="block text-[11px] font-medium text-gray-500 dark:text-gray-400">手动模型 ID</span>
                      <input
                        type="text"
                        class="input mt-1 h-9"
                        :value="probeModelDraft(selectedAccount)"
                        placeholder="gpt-5.4-mini"
                        @input="onProbeModelDraftInput(selectedAccount, $event)"
                      />
                    </label>
                  </div>
                </div>
                <div class="flex flex-wrap gap-2">
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="isLoadingProbeModels(selectedAccount.account_id)" @click="loadProbeModels(selectedAccount, true)">
                    <Icon name="refresh" size="xs" :class="isLoadingProbeModels(selectedAccount.account_id) ? 'animate-spin' : ''" />
                    <span>拉取模型</span>
                  </button>
                  <button type="button" class="btn btn-primary btn-sm" :disabled="isSavingProbeModel(selectedAccount.account_id)" @click="saveProbeModel(selectedAccount)">
                    <Icon name="check" size="xs" />
                    <span>{{ isSavingProbeModel(selectedAccount.account_id) ? '保存中' : '保存模型' }}</span>
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" :disabled="isProbing(selectedAccount.account_id)" @click="runProbe(selectedAccount)">
                    <Icon name="beaker" size="xs" :class="isProbing(selectedAccount.account_id) ? 'animate-pulse' : ''" />
                    <span>{{ isProbing(selectedAccount.account_id) ? '探测中' : '立即探测' }}</span>
                  </button>
                  <button type="button" class="btn btn-secondary btn-sm" @click="goToAccount(selectedAccount)">
                    <Icon name="externalLink" size="xs" />
                    <span>账号管理</span>
                  </button>
                </div>
              </div>
              <div v-if="!selectedAccount.is_opened" class="mt-3 flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-800">
                <div>
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">关闭账号自动探测</div>
                  <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ isAutoProbeEnabled(selectedAccount) ? '后台会按规则探测这个关闭账号' : '这个关闭账号不会被后台自动探测' }}</div>
                </div>
                <label
                  class="inline-flex h-8 cursor-pointer items-center gap-2 rounded-lg border px-2 text-xs font-semibold transition"
                  :class="autoProbeToggleClass(selectedAccount)"
                >
                  <input
                    type="checkbox"
                    class="sr-only"
                    :checked="isAutoProbeEnabled(selectedAccount)"
                    :disabled="isTogglingAutoProbe(selectedAccount.account_id)"
                    @change="onAutoProbeChange(selectedAccount, $event)"
                  />
                  <span class="relative h-4 w-7 rounded-full transition" :class="isAutoProbeEnabled(selectedAccount) ? 'bg-emerald-500' : 'bg-gray-400 dark:bg-dark-600'">
                    <span class="absolute top-0.5 h-3 w-3 rounded-full bg-white transition" :class="isAutoProbeEnabled(selectedAccount) ? 'left-3.5' : 'left-0.5'"></span>
                  </span>
                  <span>{{ isAutoProbeEnabled(selectedAccount) ? '自动探测' : '停探测' }}</span>
                </label>
              </div>
            </section>

            <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
              <div class="flex items-end justify-between gap-3">
                <div>
                  <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ headlineHealthLabel(selectedAccount) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ headlineHealthLeftMeta(selectedAccount) }}</div>
                </div>
                <div class="text-4xl font-semibold" :class="headlineHealthClass(selectedAccount)">{{ headlineHealthText(selectedAccount) }}</div>
              </div>
              <div class="mt-3 h-2.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                <div class="h-full rounded-full transition-all" :class="headlineHealthBarClass(selectedAccount)" :style="{ width: `${headlineHealthPercent(selectedAccount)}%` }"></div>
              </div>
              <div class="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-5">
                <div v-for="window in windowOrder" :key="`detail-${window}`" class="rounded-lg border border-gray-200 p-2.5 dark:border-dark-700">
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ window }}</span>
                    <span class="text-xs text-gray-400">{{ windowCountText(selectedAccount, window) }}</span>
                  </div>
                  <div class="mt-2 text-xl font-semibold" :class="windowMetricClass(selectedAccount, window)">{{ windowMetricText(selectedAccount, window) }}</div>
                  <div v-if="selectedAccount.is_opened" class="mt-1 truncate text-[11px] font-medium" :class="windowFirstTokenClass(selectedAccount, window)">
                    首 {{ windowFirstTokenText(selectedAccount, window) }}
                  </div>
                  <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <div class="h-full rounded-full" :class="windowBarClass(selectedAccount, window)" :style="{ width: `${windowBarPercent(selectedAccount, window)}%` }"></div>
                  </div>
                  <div class="mt-2 flex justify-between text-[11px] text-gray-500 dark:text-gray-400">
                    <span>{{ windowLeftMeta(selectedAccount, window) }}</span>
                    <span>{{ windowRightMeta(selectedAccount, window) }}</span>
                  </div>
                </div>
              </div>
            </section>

            <section class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
              <div class="flex items-center justify-between gap-3">
                <div class="text-sm font-semibold text-gray-900 dark:text-white">{{ recentTimelineTitle(selectedAccount) }}</div>
                <div class="text-xs text-gray-400">PAST -> NOW</div>
              </div>
              <div class="mt-3 grid grid-cols-[repeat(60,minmax(3px,1fr))] gap-1">
                <span
                  v-for="(sample, idx) in recentSamplesForDisplay(selectedAccount)"
                  :key="`${selectedAccount.account_id}-detail-${idx}-${sample?.created_at || 'empty'}`"
                  class="h-8 min-w-0 rounded-sm"
                  :class="sampleClass(sample)"
                  :title="sampleTitle(sample)"
                ></span>
              </div>
            </section>

            <section class="rounded-lg border p-3" :class="recommendationPanelClass(selectedAccount)">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="severityClass(selectedAccount.recommendation.severity)">
                      {{ selectedAccount.recommendation.severity || 'P3' }}
                    </span>
                    <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="notifyClass(selectedAccount.recommendation.notify_mode)">
                      {{ notifyLabel(selectedAccount.recommendation.notify_mode) }}
                    </span>
                    <span v-if="selectedAccount.probe?.checked_at" class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="probeStatusClass(selectedAccount)">
                      {{ probeStatusLabel(selectedAccount) }}
                    </span>
                  </div>
                  <h3 class="mt-3 text-lg font-semibold text-gray-900 dark:text-white">{{ selectedAccount.recommendation.title }}</h3>
                  <p class="mt-2 break-words text-sm text-gray-600 dark:text-gray-300">{{ selectedAccount.recommendation.reason || '-' }}</p>
                  <p v-if="probeText(selectedAccount)" class="mt-2 break-words text-xs text-gray-500 dark:text-gray-400">{{ probeText(selectedAccount) }}</p>
                </div>
                <Icon :name="recommendationIcon(selectedAccount)" size="lg" :class="recommendationIconClass(selectedAccount)" />
              </div>
            </section>
          </div>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAdminSettingsStore } from '@/stores'
import { adminAPI } from '@/api/admin'
import {
  opsAPI,
  type OpsAccountHealthItem,
  type OpsAccountHealthFirstTokenStats,
  type OpsAccountHealthResponse,
  type OpsAccountHealthSettings,
  type OpsAccountHealthSample,
  type OpsAccountHealthWindow,
  type OpsAccountHealthWindowStats
} from '@/api/admin/ops'
import type { AdminGroup, ClaudeModel } from '@/types'
import {
  availableTagOptions as buildAvailableTagOptions,
  matchesTagFilter as matchesTags,
  normalizeTags,
  tagKey,
  tagsForItem
} from './accountHealthTags'

const DEFAULT_PROBE_MODEL = 'gpt-5.4-mini'

type FirstTokenWindow = OpsAccountHealthWindow | '5m'
type HealthViewMode = 'card' | 'list'
type HealthQuickFilter =
  | 'all'
  | 'abnormal'
  | 'opened'
  | 'closed'
  | 'available'
  | 'unavailable'
  | 'probing'
  | 'probe_disabled'
  | 'can_open'
  | 'close_now'
  | 'p1'
  | 'p2'
  | 'no_data'

interface ProbeModelOption {
  value: string
  label: string
}

const StatusBadge = defineComponent({
  name: 'StatusBadge',
  props: {
    text: { type: String, required: true },
    kind: { type: String, default: 'muted' }
  },
  setup(props) {
    const classes: Record<string, string> = {
      success: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
      warning: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
      danger: 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300',
      muted: 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
    }
    return () => h('span', {
      class: `inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium ${classes[props.kind] || classes.muted}`
    }, props.text)
  }
})

const appStore = useAppStore()
const adminSettingsStore = useAdminSettingsStore()
const route = useRoute()
const router = useRouter()

const response = ref<OpsAccountHealthResponse | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const platformFilter = ref('')
const groupIdInput = ref('')
const healthGroups = ref<AdminGroup[]>([])
const showHistoricalGroups = ref(false)
const viewMode = ref<HealthViewMode>('list')
const quickFilter = ref<HealthQuickFilter>('all')
const searchQuery = ref('')
const selectedTagFilters = ref<string[]>([])
const selectedAccountIds = ref<Set<number>>(new Set())
const expandedAccountIds = ref<Set<number>>(new Set())
const editingTagsAccountId = ref<number | null>(null)
const tagEditDraft = ref('')
const selectedAccountID = ref<number | null>(null)
const lastUpdated = ref<Date | null>(null)
const probingAccounts = ref<Set<number>>(new Set())
const togglingAutoProbeAccounts = ref<Set<number>>(new Set())
const togglingSchedulableAccounts = ref<Set<number>>(new Set())
const bulkSchedulableLoading = ref(false)
const loadingModelAccounts = ref<Set<number>>(new Set())
const savingModelAccounts = ref<Set<number>>(new Set())
const savingTagsAccountId = ref<number | null>(null)
const probeModelOptionsByAccount = ref<Record<number, ProbeModelOption[]>>({})
const probeModelDraftByAccount = ref<Record<number, string>>({})
const autoRefreshMs = 45_000
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

const windowOrder: OpsAccountHealthWindow[] = ['1m', '5m', '10m', '30m', '1h']
const probeWindowMinutes: Record<OpsAccountHealthWindow, number> = {
  '1m': 1,
  '5m': 5,
  '10m': 10,
  '30m': 30,
  '1h': 60
}

interface ProbeWindowSummary {
  count: number
  successCount: number
  errorCount: number
  successRatePercent: number
  errorRatePercent: number
  avgDurationMs: number | null
}

const settingsForm = ref<OpsAccountHealthSettings>(defaultAccountHealthSettings())

const groupId = computed(() => {
  const parsed = Number(groupIdInput.value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
})

const isDefaultVisibleGroup = (group: AdminGroup): boolean =>
  group.status === 'active' &&
  (
    (group.balance_enabled ?? group.subscription_type !== 'subscription') ||
    (group.subscription_enabled ?? group.subscription_type === 'subscription') ||
    Boolean(group.plan_auto_grant_enabled)
  )

const healthGroupOptions = computed(() => {
  if (showHistoricalGroups.value) return healthGroups.value

  const selectedID = groupId.value
  const visible = healthGroups.value.filter(isDefaultVisibleGroup)
  if (!selectedID || visible.some(group => group.id === selectedID)) {
    return visible
  }

  const selected = healthGroups.value.find(group => group.id === selectedID)
  return selected ? [...visible, selected] : visible
})

const items = computed(() => response.value?.items ?? [])

const sortedItems = computed(() => {
  return [...items.value].sort((a, b) => {
    const sevDiff = severityRank(a.recommendation.severity) - severityRank(b.recommendation.severity)
    if (sevDiff !== 0) return sevDiff
    if (a.recommendation.immediate !== b.recommendation.immediate) return a.recommendation.immediate ? -1 : 1
    const actionDiff = actionRank(a.recommendation.action) - actionRank(b.recommendation.action)
    if (actionDiff !== 0) return actionDiff
    const aStat = primaryStat(a)
    const bStat = primaryStat(b)
    return (aStat?.success_rate_percent ?? 101) - (bStat?.success_rate_percent ?? 101)
  })
})

const visibleItems = computed(() => sortedItems.value.filter(item => matchesQuickFilter(item) && matchesTagFilter(item) && matchesSearch(item)))

const selectedCount = computed(() => selectedAccountIds.value.size)

const visibleAccountIds = computed(() => visibleItems.value.map(item => item.account_id))

const allVisibleSelected = computed(() => {
  const ids = visibleAccountIds.value
  return ids.length > 0 && ids.every(id => selectedAccountIds.value.has(id))
})

const selectedAccount = computed(() => {
  if (!selectedAccountID.value) return null
  return items.value.find(item => item.account_id === selectedAccountID.value) || null
})

const availableTagOptions = computed(() => buildAvailableTagOptions(items.value))

const quickFilters = computed<Array<{ key: HealthQuickFilter; label: string; count: number }>>(() => [
  { key: 'all', label: '全部', count: items.value.length },
  { key: 'abnormal', label: '异常优先', count: items.value.filter(isAbnormal).length },
  { key: 'opened', label: '已打开', count: items.value.filter(item => item.is_opened).length },
  { key: 'closed', label: '已关闭', count: items.value.filter(item => !item.is_opened).length },
  { key: 'available', label: '可调度', count: items.value.filter(item => item.is_available).length },
  { key: 'unavailable', label: '不可调度', count: items.value.filter(item => !item.is_available).length },
  { key: 'probing', label: '正在探测', count: items.value.filter(item => !item.is_opened && isAutoProbeEnabled(item)).length },
  { key: 'probe_disabled', label: '探测关闭', count: items.value.filter(item => !item.is_opened && item.probe_auto_disabled).length },
  { key: 'can_open', label: '可恢复', count: items.value.filter(item => item.recommendation.action === 'can_open').length },
  { key: 'close_now', label: '建议关闭', count: items.value.filter(item => item.recommendation.action === 'close_now').length },
  { key: 'p1', label: 'P1', count: items.value.filter(item => item.recommendation.severity === 'P1').length },
  { key: 'p2', label: 'P2', count: items.value.filter(item => item.recommendation.severity === 'P2').length },
  { key: 'no_data', label: '无数据', count: items.value.filter(item => !hasTraffic(item) && probeSamples(item).length === 0).length }
])

const summaryCards = computed(() => {
  const total = items.value.length
  const opened = items.value.filter(item => item.is_opened).length
  const closeNow = items.value.filter(item => item.recommendation.action === 'close_now').length
  const canOpen = items.value.filter(item => item.recommendation.action === 'can_open').length
  const immediate = items.value.filter(item => item.recommendation.notify_mode === 'immediate').length
  const available = items.value.filter(item => item.is_available).length
  return [
    { label: '建议关闭', value: closeNow, className: closeNow ? 'text-red-600 dark:text-red-300' : 'text-gray-900 dark:text-white' },
    { label: '可恢复', value: canOpen, className: canOpen ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-900 dark:text-white' },
    { label: '立即通知', value: immediate, className: immediate ? 'text-red-600 dark:text-red-300' : 'text-gray-900 dark:text-white' },
    { label: '可调度', value: available, className: 'text-emerald-600 dark:text-emerald-300' },
    { label: '已打开', value: opened, className: 'text-sky-600 dark:text-sky-300' },
    { label: '账号总数', value: total, className: 'text-gray-900 dark:text-white' }
  ]
})

function openNotificationRules() {
  void router.push('/admin/notification-robots')
}

function matchesQuickFilter(item: OpsAccountHealthItem): boolean {
  switch (quickFilter.value) {
    case 'abnormal':
      return isAbnormal(item)
    case 'opened':
      return item.is_opened
    case 'closed':
      return !item.is_opened
    case 'available':
      return item.is_available
    case 'unavailable':
      return !item.is_available
    case 'probing':
      return !item.is_opened && isAutoProbeEnabled(item)
    case 'probe_disabled':
      return !item.is_opened && Boolean(item.probe_auto_disabled)
    case 'can_open':
      return item.recommendation.action === 'can_open'
    case 'close_now':
      return item.recommendation.action === 'close_now'
    case 'p1':
      return item.recommendation.severity === 'P1'
    case 'p2':
      return item.recommendation.severity === 'P2'
    case 'no_data':
      return !hasTraffic(item) && probeSamples(item).length === 0
    default:
      return true
  }
}

function matchesSearch(item: OpsAccountHealthItem): boolean {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return true
  const haystack = [
    String(item.account_id),
    item.account_name,
    item.platform,
    item.group_name,
    String(item.group_id),
    item.recommendation?.title,
    item.recommendation?.reason,
    item.probe_model_id,
    item.probe_model_effective,
    ...tagsForItem(item)
  ]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  return haystack.includes(query)
}

function visibleTagsForItem(item: OpsAccountHealthItem): string[] {
  return tagsForItem(item).slice(0, 4)
}

function hiddenTagCount(item: OpsAccountHealthItem): number {
  return Math.max(0, tagsForItem(item).length - 4)
}

function matchesTagFilter(item: OpsAccountHealthItem): boolean {
  return matchesTags(tagsForItem(item), selectedTagFilters.value, 'any')
}

function isTagFilterSelected(tag: string): boolean {
  const key = tagKey(tag)
  return selectedTagFilters.value.some(selected => tagKey(selected) === key)
}

function toggleTagFilter(tag: string) {
  const key = tagKey(tag)
  if (!key) return
  if (isTagFilterSelected(tag)) {
    selectedTagFilters.value = []
    return
  }
  selectedTagFilters.value = [tag]
}

function clearTagFilters() {
  selectedTagFilters.value = []
}

function isAccountSelected(accountID: number): boolean {
  return selectedAccountIds.value.has(accountID)
}

function toggleAccountSelection(accountID: number, selected?: boolean) {
  const next = new Set(selectedAccountIds.value)
  const shouldSelect = selected ?? !next.has(accountID)
  if (shouldSelect) {
    next.add(accountID)
  } else {
    next.delete(accountID)
  }
  selectedAccountIds.value = next
}

function onAccountSelectionChange(accountID: number, event: Event) {
  toggleAccountSelection(accountID, Boolean((event.target as HTMLInputElement | null)?.checked))
}

function toggleSelectAllVisible(event?: Event) {
  const target = event?.target as HTMLInputElement | null
  const checked = target?.checked ?? !allVisibleSelected.value
  const next = new Set(selectedAccountIds.value)
  for (const id of visibleAccountIds.value) {
    if (checked) {
      next.add(id)
    } else {
      next.delete(id)
    }
  }
  selectedAccountIds.value = next
}

function clearSelection() {
  selectedAccountIds.value = new Set()
}

function isExpanded(accountID: number): boolean {
  return expandedAccountIds.value.has(accountID)
}

function toggleExpanded(accountID: number) {
  const next = new Set(expandedAccountIds.value)
  if (next.has(accountID)) {
    next.delete(accountID)
  } else {
    next.add(accountID)
  }
  expandedAccountIds.value = next
}

function isAbnormal(item: OpsAccountHealthItem): boolean {
  return item.recommendation.severity === 'P1' ||
    item.recommendation.severity === 'P2' ||
    item.recommendation.action === 'close_now' ||
    item.recommendation.action === 'can_open' ||
    item.has_error
}

watch(
  () => response.value?.settings,
  (settings) => {
    if (settings) {
      applySettingsToForm(settings)
    }
  },
  { immediate: true }
)

watch(items, (nextItems) => {
  const liveIds = new Set(nextItems.map(item => item.account_id))
  selectedAccountIds.value = new Set([...selectedAccountIds.value].filter(id => liveIds.has(id)))
  expandedAccountIds.value = new Set([...expandedAccountIds.value].filter(id => liveIds.has(id)))
})

async function fetchData() {
  loading.value = true
  errorMessage.value = ''
  try {
    const data = await opsAPI.getAccountHealth({
      platform: platformFilter.value,
      group_id: groupId.value,
      recent_limit: 60
    })
    response.value = data
    lastUpdated.value = new Date()
  } catch (err: any) {
    errorMessage.value = err?.response?.data?.message || err?.response?.data?.detail || '账号健康数据加载失败'
  } finally {
    loading.value = false
  }
}

async function loadHealthGroups() {
  try {
    const groups = await adminAPI.groups.getAll()
    healthGroups.value = groups || []
  } catch (err) {
    console.error('Failed to load account health groups:', err)
  }
}

function startEditingTags(item: OpsAccountHealthItem) {
  editingTagsAccountId.value = item.account_id
  tagEditDraft.value = tagsForItem(item).join(', ')
}

function cancelEditingTags() {
  editingTagsAccountId.value = null
  tagEditDraft.value = ''
}

function isSavingTags(accountID: number): boolean {
  return savingTagsAccountId.value === accountID
}

async function updateAccountTags(item: OpsAccountHealthItem) {
  if (!item?.account_id || isSavingTags(item.account_id)) return
  const previousTags = tagsForItem(item)
  const nextTags = normalizeTags(tagEditDraft.value)
  item.tags = nextTags
  savingTagsAccountId.value = item.account_id
  try {
    const state = await adminAPI.accounts.updateTags(item.account_id, nextTags)
    item.tags = normalizeTags(state.tags)
    appStore.showSuccess('账号标签已保存')
    cancelEditingTags()
    await fetchData()
  } catch (err: any) {
    item.tags = previousTags
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '账号标签保存失败')
  } finally {
    savingTagsAccountId.value = null
  }
}

function isTogglingSchedulable(accountID: number): boolean {
  return togglingSchedulableAccounts.value.has(accountID)
}

function setTogglingSchedulable(accountID: number, toggling: boolean) {
  const next = new Set(togglingSchedulableAccounts.value)
  if (toggling) {
    next.add(accountID)
  } else {
    next.delete(accountID)
  }
  togglingSchedulableAccounts.value = next
}

function applySchedulableState(item: OpsAccountHealthItem, schedulable: boolean) {
  item.is_schedulable = schedulable
  item.is_opened = item.status === 'active' && schedulable
  item.is_available = item.status === 'active' &&
    schedulable &&
    !item.is_rate_limited &&
    !item.is_overloaded &&
    !item.is_temp_unschedulable &&
    !item.has_error
}

async function toggleAccountSchedulable(item: OpsAccountHealthItem, schedulable = !item.is_schedulable) {
  if (!item?.account_id || isTogglingSchedulable(item.account_id)) return
  const previous = {
    is_schedulable: item.is_schedulable,
    is_opened: item.is_opened,
    is_available: item.is_available,
    status: item.status
  }
  setTogglingSchedulable(item.account_id, true)
  applySchedulableState(item, schedulable)
  try {
    const updated = await adminAPI.accounts.setSchedulable(item.account_id, schedulable)
    item.status = updated.status
    applySchedulableState(item, Boolean(updated.schedulable))
    appStore.showSuccess(updated.schedulable ? '已开启账号调度' : '已关闭账号调度')
    await fetchData()
  } catch (err: any) {
    item.is_schedulable = previous.is_schedulable
    item.is_opened = previous.is_opened
    item.is_available = previous.is_available
    item.status = previous.status
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '调度开关更新失败')
  } finally {
    setTogglingSchedulable(item.account_id, false)
  }
}

async function bulkToggleSchedulable(schedulable: boolean) {
  const ids = [...selectedAccountIds.value]
  if (ids.length === 0 || bulkSchedulableLoading.value) return
  if (!schedulable && !window.confirm(`确认关闭选中的 ${ids.length} 个账号调度？`)) {
    return
  }

  bulkSchedulableLoading.value = true
  const targetIds = new Set(ids)
  const previous = items.value
    .filter(item => targetIds.has(item.account_id))
    .map(item => ({
      item,
      is_schedulable: item.is_schedulable,
      is_opened: item.is_opened,
      is_available: item.is_available
    }))
  for (const snapshot of previous) {
    applySchedulableState(snapshot.item, schedulable)
  }

  try {
    const result = await adminAPI.accounts.bulkUpdate(ids, { schedulable })
    const successIDs = new Set<number>(
      (result.success_ids?.length ? result.success_ids : result.results.filter(row => row.success).map(row => row.account_id))
    )
    for (const item of items.value) {
      if (successIDs.has(item.account_id)) {
        applySchedulableState(item, schedulable)
      }
    }
    if (result.failed > 0) {
      appStore.showWarning(`批量调度部分成功：成功 ${result.success}，失败 ${result.failed}`)
    } else {
      appStore.showSuccess(schedulable ? `已开启 ${result.success} 个账号调度` : `已关闭 ${result.success} 个账号调度`)
      clearSelection()
    }
    await fetchData()
  } catch (err: any) {
    for (const snapshot of previous) {
      snapshot.item.is_schedulable = snapshot.is_schedulable
      snapshot.item.is_opened = snapshot.is_opened
      snapshot.item.is_available = snapshot.is_available
    }
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '批量调度更新失败')
  } finally {
    bulkSchedulableLoading.value = false
  }
}

async function runProbe(item: OpsAccountHealthItem) {
  if (!item?.account_id || isProbing(item.account_id)) return
  setProbing(item.account_id, true)
  try {
    await opsAPI.runAccountHealthProbe(item.account_id, {
      model_id: probeModelDraft(item) || item.probe_model_id || undefined,
      mode: settingsForm.value.probe.mode || undefined,
      prompt: settingsForm.value.probe.prompt || undefined
    })
    appStore.showSuccess('账号探测已完成')
    await fetchData()
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '账号探测失败')
    await fetchData()
  } finally {
    setProbing(item.account_id, false)
  }
}

async function onAutoProbeChange(item: OpsAccountHealthItem, event: Event) {
  const target = event.target as HTMLInputElement | null
  const enabled = Boolean(target?.checked)
  await updateAutoProbe(item, enabled)
}

async function updateAutoProbe(item: OpsAccountHealthItem, enabled: boolean) {
  if (!item?.account_id || isTogglingAutoProbe(item.account_id)) return
  setTogglingAutoProbe(item.account_id, true)
  try {
    const state = await opsAPI.updateAccountHealthProbeAuto(item.account_id, enabled)
    item.probe_auto_disabled = state.probe_auto_disabled
    appStore.showSuccess(state.probe_auto_disabled ? '已关闭该账号自动探测' : '已开启该账号自动探测')
    await fetchData()
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '自动探测开关更新失败')
    await fetchData()
  } finally {
    setTogglingAutoProbe(item.account_id, false)
  }
}

async function loadProbeModels(item: OpsAccountHealthItem, force = false) {
  if (!item?.account_id) return
  if (!force && probeModelOptionsByAccount.value[item.account_id]?.length) return
  if (isLoadingProbeModels(item.account_id)) return

  setLoadingProbeModels(item.account_id, true)
  try {
    const models = await adminAPI.accounts.getAvailableModels(item.account_id)
    const options = normalizeProbeModelOptions(models)
    const effective = probeModelEffective(item)
    if (effective && !options.some(option => option.value === effective)) {
      options.unshift({ value: effective, label: effective })
    }
    probeModelOptionsByAccount.value = {
      ...probeModelOptionsByAccount.value,
      [item.account_id]: options
    }
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '模型列表加载失败')
  } finally {
    setLoadingProbeModels(item.account_id, false)
  }
}

async function saveProbeModel(item: OpsAccountHealthItem) {
  if (!item?.account_id || isSavingProbeModel(item.account_id)) return
  setSavingProbeModel(item.account_id, true)
  try {
    const state = await opsAPI.updateAccountHealthProbeModel(item.account_id, probeModelDraft(item))
    item.probe_model_id = state.probe_model_id || ''
    item.probe_model_effective = state.probe_model_effective
    setProbeModelDraft(item, item.probe_model_id || '')
    appStore.showSuccess(item.probe_model_id ? '账号探测模型已保存' : '已改为继承全局探测模型')
    await fetchData()
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '探测模型保存失败')
  } finally {
    setSavingProbeModel(item.account_id, false)
  }
}

function normalizeProbeModelOptions(models: ClaudeModel[]): ProbeModelOption[] {
  const seen = new Set<string>()
  const options: ProbeModelOption[] = []
  for (const model of models || []) {
    const value = String(model?.id || '').trim()
    if (!value || seen.has(value)) continue
    seen.add(value)
    options.push({ value, label: String(model.display_name || value) })
  }
  return options
}

function probeModelOptions(item: OpsAccountHealthItem): ProbeModelOption[] {
  const options = probeModelOptionsByAccount.value[item.account_id] || []
  const effective = probeModelEffective(item)
  if (!effective || options.some(option => option.value === effective)) return options
  return [{ value: effective, label: effective }, ...options]
}

function probeModelDraft(item: OpsAccountHealthItem): string {
  if (!item?.account_id) return ''
  if (Object.prototype.hasOwnProperty.call(probeModelDraftByAccount.value, item.account_id)) {
    return probeModelDraftByAccount.value[item.account_id] || ''
  }
  return item.probe_model_id || ''
}

function setProbeModelDraft(item: OpsAccountHealthItem, value: string) {
  if (!item?.account_id) return
  probeModelDraftByAccount.value = {
    ...probeModelDraftByAccount.value,
    [item.account_id]: String(value || '').trim()
  }
}

function onProbeModelDraftInput(item: OpsAccountHealthItem, event: Event) {
  const target = event.target as HTMLInputElement | HTMLSelectElement | null
  setProbeModelDraft(item, target?.value || '')
}

function probeModelEffective(item: OpsAccountHealthItem): string {
  return String(item.probe_model_effective || item.probe_model_id || settingsForm.value.probe.model_id || DEFAULT_PROBE_MODEL).trim()
}

function probeModelText(item: OpsAccountHealthItem): string {
  const effective = probeModelEffective(item)
  if (!item.probe_model_id) return `${effective} · 继承`
  return effective
}

function isProbing(accountID: number): boolean {
  return probingAccounts.value.has(accountID)
}

function isTogglingAutoProbe(accountID: number): boolean {
  return togglingAutoProbeAccounts.value.has(accountID)
}

function isLoadingProbeModels(accountID: number): boolean {
  return loadingModelAccounts.value.has(accountID)
}

function isSavingProbeModel(accountID: number): boolean {
  return savingModelAccounts.value.has(accountID)
}

function setProbing(accountID: number, probing: boolean) {
  const next = new Set(probingAccounts.value)
  if (probing) {
    next.add(accountID)
  } else {
    next.delete(accountID)
  }
  probingAccounts.value = next
}

function setTogglingAutoProbe(accountID: number, toggling: boolean) {
  const next = new Set(togglingAutoProbeAccounts.value)
  if (toggling) {
    next.add(accountID)
  } else {
    next.delete(accountID)
  }
  togglingAutoProbeAccounts.value = next
}

function setLoadingProbeModels(accountID: number, loadingModels: boolean) {
  const next = new Set(loadingModelAccounts.value)
  if (loadingModels) {
    next.add(accountID)
  } else {
    next.delete(accountID)
  }
  loadingModelAccounts.value = next
}

function setSavingProbeModel(accountID: number, saving: boolean) {
  const next = new Set(savingModelAccounts.value)
  if (saving) {
    next.add(accountID)
  } else {
    next.delete(accountID)
  }
  savingModelAccounts.value = next
}

function applySettingsToForm(settings: OpsAccountHealthSettings) {
  settingsForm.value = cloneSettings(settings)
}

function defaultAccountHealthSettings(): OpsAccountHealthSettings {
  return {
    enabled: true,
    mode: 'smart',
    burst: {
      enabled: true,
      window_minutes: 1,
      min_requests: 10,
      error_rate_percent: 50,
      upstream_error_rate_percent: 25,
      cooldown_minutes: 1,
      bypass_digest: true
    },
    degrade: {
      enabled: true,
      window_minutes: 10,
      min_requests: 20,
      success_rate_min_percent: 90,
      error_rate_percent: 20,
      upstream_error_rate_percent: 10,
      cooldown_minutes: 10
    },
    recovery: {
      enabled: true,
      window_minutes: 30,
      min_requests: 10,
      success_rate_min_percent: 98,
      notify_opened_accounts: true,
      notify_closed_accounts: true,
      cooldown_minutes: 30
    },
    probe: {
      enabled: true,
      interval_minutes: 30,
      max_per_run: 2,
      timeout_seconds: 20,
      model_id: DEFAULT_PROBE_MODEL,
      mode: 'default',
      prompt: ''
    },
    notification: {
      enterprise_wechat_enabled: false,
      enterprise_wechat_webhook_url: '',
      mention_all_on_immediate: false
    },
    rate_limit_per_hour: 12
  }
}

function cloneSettings(settings: OpsAccountHealthSettings): OpsAccountHealthSettings {
  const defaults = defaultAccountHealthSettings()
  const raw = JSON.parse(JSON.stringify(settings || defaults))
  const cloned = {
    ...defaults,
    ...raw,
    burst: { ...defaults.burst, ...(raw.burst || {}) },
    degrade: { ...defaults.degrade, ...(raw.degrade || {}) },
    recovery: { ...defaults.recovery, ...(raw.recovery || {}) },
    probe: { ...defaults.probe, ...(raw.probe || {}) },
    notification: { ...defaults.notification, ...(raw.notification || {}) }
  }
  if (!String(cloned.probe.model_id || '').trim()) {
    cloned.probe.model_id = DEFAULT_PROBE_MODEL
  }
  return cloned
}

function statFor(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): OpsAccountHealthWindowStats | undefined {
  return item.windows?.[window]
}

function primaryMetricWindow(item: OpsAccountHealthItem): OpsAccountHealthWindow | null {
  const tenMin = statFor(item, '10m')
  if (tenMin && tenMin.request_count > 0) return '10m'
  const thirtyMin = statFor(item, '30m')
  if (thirtyMin && thirtyMin.request_count > 0) return '30m'
  const oneHour = statFor(item, '1h')
  if (oneHour && oneHour.request_count > 0) return '1h'
  return null
}

function primaryStat(item: OpsAccountHealthItem): OpsAccountHealthWindowStats | undefined {
  const window = primaryMetricWindow(item)
  if (window) return statFor(item, window)
  return statFor(item, '10m')
}

function hasTraffic(item: OpsAccountHealthItem): boolean {
  return windowOrder.some(window => (statFor(item, window)?.request_count ?? 0) > 0)
}

function isAutoProbeEnabled(item: OpsAccountHealthItem): boolean {
  return !item.probe_auto_disabled
}

function autoProbeToggleTitle(item: OpsAccountHealthItem): string {
  if (isTogglingAutoProbe(item.account_id)) return '正在更新自动探测开关'
  return isAutoProbeEnabled(item) ? '关闭该账号的后台自动探测' : '开启该账号的后台自动探测'
}

function autoProbeToggleClass(item: OpsAccountHealthItem): string {
  if (isTogglingAutoProbe(item.account_id)) {
    return 'border-gray-200 bg-gray-50 text-gray-400 dark:border-dark-700 dark:bg-dark-700/50 dark:text-gray-500'
  }
  if (isAutoProbeEnabled(item)) {
    return 'border-emerald-200 bg-emerald-50 text-emerald-700 hover:border-emerald-300 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300'
  }
  return 'border-gray-200 bg-gray-50 text-gray-500 hover:border-gray-300 dark:border-dark-700 dark:bg-dark-700/60 dark:text-gray-300'
}

function probeSamples(item: OpsAccountHealthItem): OpsAccountHealthSample[] {
  const raw = item.probe?.recent?.length ? item.probe.recent : []
  const samples = raw.length > 0 ? raw : syntheticProbeSamples(item)
  return sortSamplesByTime(samples).slice(-60)
}

function syntheticProbeSamples(item: OpsAccountHealthItem): OpsAccountHealthSample[] {
  const probe = item.probe
  if (!probe?.checked_at) return []
  return [{
    kind: probe.status === 'success' ? 'success' : 'error',
    created_at: probe.checked_at,
    model: probe.model_id,
    duration_ms: probe.latency_ms ? Number(probe.latency_ms) : null,
    message: probe.error_message || '主动探测'
  }]
}

function sortSamplesByTime(samples: OpsAccountHealthSample[]): OpsAccountHealthSample[] {
  return [...samples].sort((a, b) => sampleTimestamp(a) - sampleTimestamp(b))
}

function sampleTimestamp(sample: OpsAccountHealthSample): number {
  const timestamp = new Date(sample.created_at).getTime()
  return Number.isFinite(timestamp) ? timestamp : 0
}

function probeWindowSummary(item: OpsAccountHealthItem, window?: OpsAccountHealthWindow): ProbeWindowSummary {
  let samples = probeSamples(item)
  if (window) {
    const cutoff = Date.now() - probeWindowMinutes[window] * 60_000
    samples = samples.filter(sample => sampleTimestamp(sample) >= cutoff)
  }
  return summarizeProbeSamples(samples)
}

function primaryProbeSummary(item: OpsAccountHealthItem): ProbeWindowSummary {
  const tenMin = probeWindowSummary(item, '10m')
  if (tenMin.count > 0) return tenMin
  const thirtyMin = probeWindowSummary(item, '30m')
  if (thirtyMin.count > 0) return thirtyMin
  const oneHour = probeWindowSummary(item, '1h')
  if (oneHour.count > 0) return oneHour
  return probeWindowSummary(item)
}

function headlineProbeSummary(item: OpsAccountHealthItem): ProbeWindowSummary {
  const oneHour = probeWindowSummary(item, '1h')
  return oneHour.count > 0 ? oneHour : probeWindowSummary(item)
}

function summarizeProbeSamples(samples: OpsAccountHealthSample[]): ProbeWindowSummary {
  const count = samples.length
  let successCount = 0
  let latencySum = 0
  let latencyCount = 0
  for (const sample of samples) {
    if (sample.kind === 'success') successCount++
    if (typeof sample.duration_ms === 'number' && Number.isFinite(sample.duration_ms)) {
      latencySum += sample.duration_ms
      latencyCount++
    }
  }
  const errorCount = Math.max(0, count - successCount)
  return {
    count,
    successCount,
    errorCount,
    successRatePercent: count > 0 ? (successCount / count) * 100 : 0,
    errorRatePercent: count > 0 ? (errorCount / count) * 100 : 0,
    avgDurationMs: latencyCount > 0 ? latencySum / latencyCount : null
  }
}

function probeMetricClass(summary: ProbeWindowSummary, item: OpsAccountHealthItem): string {
  if (summary.count > 0) {
    if (summary.successRatePercent >= 98) return 'text-emerald-600 dark:text-emerald-300'
    if (summary.successRatePercent >= 90) return 'text-amber-600 dark:text-amber-300'
    return 'text-red-600 dark:text-red-300'
  }
  if (item.probe?.status === 'success') return 'text-emerald-600 dark:text-emerald-300'
  if (item.probe?.status === 'failed') return 'text-red-600 dark:text-red-300'
  return 'text-gray-400 dark:text-gray-500'
}

function probeMetricBarClass(summary: ProbeWindowSummary, item: OpsAccountHealthItem): string {
  if (summary.count > 0) {
    if (summary.successRatePercent >= 98) return 'bg-emerald-500'
    if (summary.successRatePercent >= 90) return 'bg-amber-500'
    return 'bg-red-500'
  }
  return probeBarClass(item)
}

function boundedPercent(value: number | null | undefined): number {
  if (!Number.isFinite(value as number)) return 0
  return Math.max(0, Math.min(100, Number(value)))
}

function formatPercent(value: number | null | undefined): string {
  if (!Number.isFinite(value as number)) return '0.0%'
  return `${Number(value).toFixed(1)}%`
}

function metricValueText(stat?: OpsAccountHealthWindowStats | null, field: keyof OpsAccountHealthWindowStats = 'success_rate_percent'): string {
  if (!stat || stat.request_count <= 0) return '暂无'
  const value = stat[field]
  return typeof value === 'number' ? formatPercent(value) : '暂无'
}

function metricValueClass(stat?: OpsAccountHealthWindowStats | null): string {
  if (!stat || stat.request_count <= 0) return 'text-gray-400 dark:text-gray-500'
  return successTextClass(stat)
}

function metricCountText(stat?: OpsAccountHealthWindowStats | null): string {
  if (!stat || stat.request_count <= 0) return '0 次'
  return `${stat.request_count} 次`
}

function primaryMetricText(item: OpsAccountHealthItem): string {
  const stat = primaryStat(item)
  if (stat && stat.request_count > 0) return formatPercent(stat.success_rate_percent)
  const probeSummary = primaryProbeSummary(item)
  if (probeSummary.count > 0) return formatPercent(probeSummary.successRatePercent)
  if (item.probe?.status === 'success') return '100.0%'
  if (item.probe?.status === 'failed') return '0.0%'
  return '待探测'
}

function primaryMetricLabel(item: OpsAccountHealthItem): string {
  const window = primaryMetricWindow(item)
  if (window) return `成功率 · ${window}`
  const probeSummary = primaryProbeSummary(item)
  if (probeSummary.count > 0 || item.probe?.checked_at) return '成功率 · 主动探测'
  return '成功率'
}

function primaryMetricHint(item: OpsAccountHealthItem): string {
  const window = primaryMetricWindow(item)
  const stat = primaryStat(item)
  if (window && stat && stat.request_count > 0) return `${window} · ${stat.request_count} 次请求样本`
  const probeSummary = primaryProbeSummary(item)
  if (probeSummary.count > 0) return `${probeSummary.count} 次探测样本`
  if (item.probe?.checked_at) return `最近探测 ${formatTime(item.probe.checked_at)}`
  return item.is_opened ? '等待请求进入' : '关闭账号需主动探测'
}

function primaryMetricClass(item: OpsAccountHealthItem): string {
  const stat = primaryStat(item)
  if (stat && stat.request_count > 0) return successTextClass(stat)
  return probeMetricClass(primaryProbeSummary(item), item)
}

function latencyText(item: OpsAccountHealthItem): string {
  const stat = primaryStat(item)
  if (stat?.avg_duration_ms && stat.request_count > 0) return `${Math.round(stat.avg_duration_ms)} ms`
  const probeSummary = primaryProbeSummary(item)
  if (probeSummary.avgDurationMs !== null) return `${Math.round(probeSummary.avgDurationMs)} ms`
  if (item.probe?.avg_latency_ms) return `${Math.round(item.probe.avg_latency_ms)} ms`
  if (item.probe?.latency_ms) return `${item.probe.latency_ms} ms`
  return '暂无'
}

function latencyHint(item: OpsAccountHealthItem): string {
  const window = primaryMetricWindow(item)
  const stat = primaryStat(item)
  if (window && stat && stat.request_count > 0) return `${window} · ${stat.request_count} 次请求平均`
  return probeText(item) || '等待主动探测'
}

function firstTokenStatFor(item: OpsAccountHealthItem, window: FirstTokenWindow): OpsAccountHealthFirstTokenStats | undefined {
  const stat = item.first_token_windows?.[window]
  if (stat) return stat
  if (window === '5m') return item.first_token_5m ?? undefined
  return undefined
}

function firstTokenAvgMs(item: OpsAccountHealthItem, window: FirstTokenWindow): number | null {
  const stat = firstTokenStatFor(item, window)
  const avg = stat?.avg_ms
  if (!item.is_opened || !stat || stat.sample_count <= 0 || typeof avg !== 'number' || !Number.isFinite(avg)) {
    return null
  }
  return avg
}

function firstTokenMetricText(item: OpsAccountHealthItem): string {
  const avg = firstTokenAvgMs(item, '5m')
  if (avg !== null) {
    return `${Math.round(avg)} ms`
  }
  return '暂无'
}

function firstTokenMetricHint(item: OpsAccountHealthItem): string {
  if (!item.is_opened) return '关闭账号不统计'
  const count = firstTokenStatFor(item, '5m')?.sample_count ?? 0
  if (count > 0) return `5m · ${count} 次首 Token样本`
  return hasTraffic(item) ? '5m 暂无首 Token样本' : '等待请求进入'
}

function firstTokenMetricClass(item: OpsAccountHealthItem): string {
  const avg = firstTokenAvgMs(item, '5m')
  return firstTokenTextClass(avg)
}

function windowFirstTokenText(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const avg = firstTokenAvgMs(item, window)
  if (avg !== null) return `${Math.round(avg)}ms`
  return '暂无'
}

function windowFirstTokenClass(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  return firstTokenTextClass(firstTokenAvgMs(item, window))
}

function firstTokenTextClass(avg: number | null): string {
  if (avg === null) return 'text-gray-400 dark:text-gray-500'
  if (avg <= 800) return 'text-emerald-600 dark:text-emerald-300'
  if (avg <= 2000) return 'text-amber-600 dark:text-amber-300'
  return 'text-red-600 dark:text-red-300'
}

function headlineHealthLabel(item: OpsAccountHealthItem): string {
  return hasTraffic(item) ? '健康度 · 1h' : '健康度 · 主动探测'
}

function headlineHealthText(item: OpsAccountHealthItem): string {
  const stat = statFor(item, '1h')
  if (stat && stat.request_count > 0) return formatPercent(stat.success_rate_percent)
  const probeSummary = headlineProbeSummary(item)
  if (probeSummary.count > 0) return formatPercent(probeSummary.successRatePercent)
  if (item.probe?.status === 'success') return '100.0%'
  if (item.probe?.status === 'failed') return '0.0%'
  return '待探测'
}

function headlineHealthClass(item: OpsAccountHealthItem): string {
  const stat = statFor(item, '1h')
  if (stat && stat.request_count > 0) return successTextClass(stat)
  return probeMetricClass(headlineProbeSummary(item), item)
}

function headlineHealthPercent(item: OpsAccountHealthItem): number {
  const stat = statFor(item, '1h')
  if (stat && stat.request_count > 0) return boundedPercent(stat.success_rate_percent)
  const probeSummary = headlineProbeSummary(item)
  if (probeSummary.count > 0) return boundedPercent(probeSummary.successRatePercent)
  return probeBarPercent(item)
}

function headlineHealthBarClass(item: OpsAccountHealthItem): string {
  const stat = statFor(item, '1h')
  if (stat && stat.request_count > 0) return windowStatBarClass(stat)
  return probeMetricBarClass(headlineProbeSummary(item), item)
}

function headlineHealthLeftMeta(item: OpsAccountHealthItem): string {
  const stat = statFor(item, '1h')
  if (stat && stat.request_count > 0) return `${stat.request_count} 次请求样本`
  const probeSummary = headlineProbeSummary(item)
  if (probeSummary.count > 0) return `${probeSummary.count} 次探测样本`
  if (item.probe?.checked_at) return probeStatusLabel(item)
  return '等待主动探测'
}

function headlineHealthRightMeta(item: OpsAccountHealthItem): string {
  const stat = statFor(item, '1h')
  if (stat && stat.request_count > 0) {
    return `错 ${formatPercent(stat.error_rate_percent)} · 上 ${formatPercent(stat.upstream_error_rate_percent)}`
  }
  const probeSummary = headlineProbeSummary(item)
  if (probeSummary.count > 0) {
    return `错 ${formatPercent(probeSummary.errorRatePercent)} · 失败 ${probeSummary.errorCount}/${probeSummary.count}`
  }
  return probeText(item) || '点击探测获取健康度'
}

function windowCountText(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return metricCountText(stat)
  const probeSummary = probeWindowSummary(item, window)
  if (probeSummary.count > 0) return `${probeSummary.count} 次`
  if (item.probe?.checked_at) return '1 次'
  return '0 次'
}

function windowMetricText(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return metricValueText(stat)
  const probeSummary = probeWindowSummary(item, window)
  if (probeSummary.count > 0) return formatPercent(probeSummary.successRatePercent)
  if (item.probe?.status === 'success') return '100.0%'
  if (item.probe?.status === 'failed') return '0.0%'
  return '暂无'
}

function windowMetricClass(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return metricValueClass(stat)
  return probeMetricClass(probeWindowSummary(item, window), item)
}

function windowBarPercent(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): number {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return boundedPercent(stat.success_rate_percent)
  const probeSummary = probeWindowSummary(item, window)
  if (probeSummary.count > 0) return boundedPercent(probeSummary.successRatePercent)
  return probeBarPercent(item)
}

function windowBarClass(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return windowStatBarClass(stat)
  return probeMetricBarClass(probeWindowSummary(item, window), item)
}

function windowLeftMeta(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return `错 ${metricValueText(stat, 'error_rate_percent')}`
  const probeSummary = probeWindowSummary(item, window)
  if (probeSummary.count > 0) return `错 ${formatPercent(probeSummary.errorRatePercent)}`
  if (item.probe?.checked_at) return `探 ${probeStatusLabel(item).replace('探测', '')}`
  return '错 暂无'
}

function windowRightMeta(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): string {
  const stat = statFor(item, window)
  if (stat && stat.request_count > 0) return `上 ${metricValueText(stat, 'upstream_error_rate_percent')}`
  const probeSummary = probeWindowSummary(item, window)
  if (probeSummary.count > 0) {
    return probeSummary.avgDurationMs !== null ? `延 ${Math.round(probeSummary.avgDurationMs)}ms` : `失 ${probeSummary.errorCount} 次`
  }
  if (item.probe?.checked_at) return item.probe.latency_ms ? `延 ${item.probe.latency_ms}ms` : '延 暂无'
  return '上 暂无'
}

function probeBarPercent(item: OpsAccountHealthItem): number {
  if (item.probe?.status === 'success') return 100
  if (item.probe?.status === 'failed') return 100
  return 0
}

function probeBarClass(item: OpsAccountHealthItem): string {
  if (item.probe?.status === 'success') return 'bg-emerald-500'
  if (item.probe?.status === 'failed') return 'bg-red-500'
  return 'bg-gray-300 dark:bg-gray-600'
}

function formatTime(raw?: string | null): string {
  if (!raw) return '-'
  return formatLocalTime(new Date(raw))
}

function formatLocalTime(date: Date | null): string {
  if (!date || Number.isNaN(date.getTime())) return '-'
  return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function successTextClass(stat?: OpsAccountHealthWindowStats | null): string {
  const value = stat?.success_rate_percent ?? 0
  if (!stat || stat.request_count === 0) return 'text-gray-400 dark:text-gray-500'
  if (value >= 98) return 'text-emerald-600 dark:text-emerald-300'
  if (value >= 90) return 'text-amber-600 dark:text-amber-300'
  return 'text-red-600 dark:text-red-300'
}

function windowStatBarClass(stat?: OpsAccountHealthWindowStats): string {
  if (!stat || stat.request_count === 0) return 'bg-gray-300 dark:bg-gray-600'
  if (stat.success_rate_percent >= 98) return 'bg-emerald-500'
  if (stat.success_rate_percent >= 90) return 'bg-amber-500'
  return 'bg-red-500'
}

function probeStatusLabel(item: OpsAccountHealthItem): string {
  if (item.probe?.status === 'success') return '探测成功'
  if (item.probe?.status === 'failed') return '探测失败'
  return '已探测'
}

function probeStatusClass(item: OpsAccountHealthItem): string {
  if (item.probe?.status === 'success') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (item.probe?.status === 'failed') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  return 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
}

function recentSamplesForDisplay(item: OpsAccountHealthItem): Array<OpsAccountHealthSample | null> {
  const source = hasTraffic(item) ? sortSamplesByTime(item.recent ?? []) : probeSamples(item)
  const samples: Array<OpsAccountHealthSample | null> = source.slice(-60)
  while (samples.length < 60) {
    samples.unshift(null)
  }
  return samples
}

function recentTimelineTitle(item: OpsAccountHealthItem): string {
  if (hasTraffic(item) && item.recent.length > 0) return `最近 ${item.recent.length} 次记录`
  const probeCount = probeSamples(item).length
  if (probeCount > 0) return `最近 ${probeCount} 次探测`
  return '最近 0 次记录'
}

function sampleClass(sample: OpsAccountHealthSample | null): string {
  if (!sample) return 'bg-gray-200 dark:bg-dark-700'
  if (sample.kind === 'success') return 'bg-emerald-500'
  if (sample.status_code === 429 || sample.status_code === 529) return 'bg-amber-500'
  return 'bg-red-500'
}

function sampleTitle(sample: OpsAccountHealthSample | null): string {
  if (!sample) return '无数据'
  const status = sample.status_code ? ` status=${sample.status_code}` : ''
  const model = sample.model ? ` model=${sample.model}` : ''
  const duration = sample.duration_ms ? ` ${sample.duration_ms}ms` : ''
  const message = sample.message ? ` ${sample.message}` : ''
  return `${sample.kind}${status}${model}${duration}${message}`
}

function severityRank(severity?: string): number {
  switch ((severity || '').toUpperCase()) {
    case 'P1': return 0
    case 'P2': return 1
    default: return 2
  }
}

function actionRank(action?: string): number {
  switch (action) {
    case 'close_now': return 0
    case 'can_open': return 1
    case 'watch': return 2
    case 'needs_probe': return 3
    case 'keep_closed': return 4
    default: return 5
  }
}

function accountBorderClass(item: OpsAccountHealthItem): string {
  if (item.recommendation.notify_mode === 'immediate' || item.recommendation.severity === 'P1') {
    return 'border-red-300 dark:border-red-900/60'
  }
  if (item.recommendation.severity === 'P2') {
    return 'border-amber-300 dark:border-amber-900/60'
  }
  return 'border-gray-200 dark:border-dark-700'
}

function recommendationPanelClass(item: OpsAccountHealthItem): string {
  if (item.recommendation.notify_mode === 'immediate' || item.recommendation.action === 'close_now') {
    return 'border-red-200 bg-red-50 dark:border-red-900/50 dark:bg-red-900/15'
  }
  if (item.recommendation.action === 'can_open' || item.recommendation.recovery_ready) {
    return 'border-emerald-200 bg-emerald-50 dark:border-emerald-900/50 dark:bg-emerald-900/15'
  }
  if (item.recommendation.severity === 'P2') {
    return 'border-amber-200 bg-amber-50 dark:border-amber-900/50 dark:bg-amber-900/15'
  }
  return 'border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-700/40'
}

function severityClass(severity?: string): string {
  switch ((severity || '').toUpperCase()) {
    case 'P1': return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'P2': return 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300'
    default: return 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
  }
}

function notifyClass(mode?: string): string {
  switch (mode) {
    case 'immediate': return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'digest': return 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
    default: return 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
  }
}

function notifyLabel(mode?: string): string {
  switch (mode) {
    case 'immediate': return '立即通知'
    case 'digest': return '汇总通知'
    default: return '不通知'
  }
}

function actionLabel(action?: string): string {
  switch (action) {
    case 'keep_open': return '保持开启'
    case 'watch': return '继续观察'
    case 'close_now': return '建议关闭'
    case 'can_open': return '可尝试打开'
    case 'needs_probe': return '需要探测'
    case 'keep_closed': return '保持关闭'
    default: return '不可用'
  }
}

function recommendationIcon(item: OpsAccountHealthItem): 'exclamationTriangle' | 'checkCircle' | 'lightbulb' | 'clock' {
  if (item.recommendation.action === 'close_now') return 'exclamationTriangle'
  if (item.recommendation.action === 'can_open' || item.recommendation.recovery_ready) return 'checkCircle'
  if (item.recommendation.action === 'needs_probe') return 'lightbulb'
  return 'clock'
}

function recommendationIconClass(item: OpsAccountHealthItem): string {
  if (item.recommendation.action === 'close_now') return 'text-red-500'
  if (item.recommendation.action === 'can_open' || item.recommendation.recovery_ready) return 'text-emerald-500'
  if (item.recommendation.severity === 'P2') return 'text-amber-500'
  return 'text-slate-400'
}

function platformInitial(platform?: string): string {
  return (platform || '?').slice(0, 1).toUpperCase()
}

function platformAvatarClass(item: OpsAccountHealthItem): string {
  if (item.recommendation.action === 'close_now') return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  if (item.recommendation.action === 'can_open') return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
  if (item.is_opened) return 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
  return 'bg-slate-100 text-slate-600 dark:bg-slate-700 dark:text-slate-300'
}

function cooldownText(item: OpsAccountHealthItem): string {
  if (item.rate_limit_remaining_sec) return `限流剩余 ${item.rate_limit_remaining_sec}s`
  if (item.overload_remaining_sec) return `过载剩余 ${item.overload_remaining_sec}s`
  if (item.temp_unschedulable_until) return `暂停到 ${formatTime(item.temp_unschedulable_until)}`
  return ''
}

function probeText(item: OpsAccountHealthItem): string {
  const probe = item.probe
  if (!probe?.checked_at) return ''
  const status = probe.status === 'success' ? '成功' : '失败'
  const latency = probe.latency_ms ? ` ${probe.latency_ms}ms` : ''
  const model = probe.model_id ? ` ${probe.model_id}` : ''
  const error = probe.status === 'success' || !probe.error_message ? '' : ` ${probe.error_message}`
  return `探测${status} ${formatTime(probe.checked_at)}${model}${latency}${error}`
}

function accountIDFromRoute(): number | null {
  const raw = route.query.account_id
  const value = Array.isArray(raw) ? raw[0] : raw
  const parsed = Number(value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
}

function syncSelectedAccountFromRoute() {
  const accountID = accountIDFromRoute()
  if (accountID) {
    selectedAccountID.value = accountID
  }
}

function openDetail(item: OpsAccountHealthItem) {
  selectedAccountID.value = item.account_id
  router.replace({
    path: route.path,
    query: { ...route.query, account_id: String(item.account_id) }
  }).catch(() => {})
}

function closeDetail() {
  selectedAccountID.value = null
  const nextQuery = { ...route.query }
  delete nextQuery.account_id
  router.replace({ path: route.path, query: nextQuery }).catch(() => {})
}

function goToAccount(item: OpsAccountHealthItem) {
  router.push({ path: '/admin/accounts', query: { account_id: String(item.account_id) } }).catch(() => {})
}

watch(
  () => route.query.account_id,
  () => syncSelectedAccountFromRoute(),
  { immediate: true }
)

onMounted(async () => {
  adminSettingsStore.fetch()
  await Promise.all([fetchData(), loadHealthGroups()])
  autoRefreshTimer = setInterval(fetchData, autoRefreshMs)
})

onUnmounted(() => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
})
</script>
