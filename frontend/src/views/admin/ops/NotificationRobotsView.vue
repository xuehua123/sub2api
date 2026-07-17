<template>
  <AppLayout>
    <div class="space-y-5 pb-10">
      <section class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div class="min-w-0">
            <div class="flex items-center gap-3">
              <span class="flex h-10 w-10 items-center justify-center rounded-lg bg-teal-50 text-teal-600 dark:bg-teal-900/30 dark:text-teal-300">
                <Icon name="bell" size="md" />
              </span>
              <div class="min-w-0">
                <h1 class="text-xl font-semibold text-gray-900 dark:text-white">通知机器人</h1>
                <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">企业微信通知、上游余额探测和账号健康提醒统一配置</p>
              </div>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button type="button" class="btn btn-secondary h-9" :disabled="loading" @click="loadAll">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span>刷新</span>
            </button>
            <button type="button" class="btn btn-primary h-9" :disabled="saving || loading" @click="saveAll">
              <Icon name="check" size="sm" />
              <span>{{ saving ? '保存中' : '保存配置' }}</span>
            </button>
          </div>
        </div>
      </section>

      <section v-if="errorMessage" class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-200">
        {{ errorMessage }}
      </section>

      <section class="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <div v-for="card in summaryCards" :key="card.label" class="rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ card.label }}</div>
          <div class="mt-2 text-2xl font-semibold" :class="card.className">{{ card.value }}</div>
        </div>
      </section>

      <section class="grid grid-cols-1 gap-4 xl:grid-cols-12">
        <div class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 xl:col-span-7">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">上游余额探测</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">智能模式会先试可用接口，成功后记住方式；频率和每轮数量都可控。</p>
            </div>
            <label class="inline-flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
              <input v-model="balanceSettings.enabled" type="checkbox" class="checkbox" />
              启用
            </label>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2">
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">默认低余额阈值 USD</span>
              <input v-model.number="balanceSettings.default_threshold_usd" type="number" min="0" step="0.01" class="input" />
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">通知限速/小时</span>
              <input v-model.number="balanceSettings.rate_limit_per_hour" type="number" min="0" max="1000" class="input" />
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">探测间隔分钟</span>
              <input v-model.number="balanceSettings.probe.interval_minutes" type="number" min="1" max="1440" class="input" />
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">每轮最多探测</span>
              <input v-model.number="balanceSettings.probe.max_per_run" type="number" min="1" max="20" class="input" />
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">单账号超时秒</span>
              <input v-model.number="balanceSettings.probe.timeout_seconds" type="number" min="1" max="60" class="input" />
            </label>
            <label class="flex items-center gap-2 rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-700">
              <input v-model="balanceSettings.probe.only_schedulable" type="checkbox" class="checkbox" />
              只探测当前可调度账号
            </label>
          </div>

          <div class="mt-4">
            <div class="text-xs font-medium text-gray-500 dark:text-gray-400">智能探测顺序</div>
            <div class="mt-2 flex flex-wrap gap-2">
              <button
                v-for="method in methodOptions.filter((item) => item.value !== 'auto' && item.value !== 'disabled')"
                :key="method.value"
                type="button"
                class="rounded-full border px-3 py-1 text-xs font-semibold transition"
                :class="balanceSettings.probe.method_order.includes(method.value) ? 'border-teal-300 bg-teal-100 text-teal-700 dark:border-teal-800 dark:bg-teal-900/40 dark:text-teal-200' : 'border-gray-200 bg-gray-50 text-gray-500 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-400'"
                @click="toggleMethodOrder(method.value)"
              >
                {{ method.label }}
              </button>
            </div>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 xl:col-span-5">
          <div class="flex items-center justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">企业微信机器人</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">余额提醒和账号健康提醒可以共用一个机器人地址。</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="testingRobot" @click="testBalanceRobot">
              <Icon name="mail" size="xs" />
              <span>{{ testingRobot ? '测试中' : '测试' }}</span>
            </button>
          </div>
          <div class="mt-4 space-y-3">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
              <input v-model="balanceSettings.notification.enterprise_wechat_enabled" type="checkbox" class="checkbox" />
              余额低于阈值时推送
            </label>
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
              <input v-model="healthSettings.notification.enterprise_wechat_enabled" type="checkbox" class="checkbox" />
              账号健康异常时推送
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">余额机器人 Webhook</span>
              <input v-model.trim="balanceSettings.notification.enterprise_wechat_webhook_url" type="text" class="input" autocomplete="off" spellcheck="false" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">账号健康机器人 Webhook</span>
              <input v-model.trim="healthSettings.notification.enterprise_wechat_webhook_url" type="text" class="input" autocomplete="off" spellcheck="false" placeholder="留空则不修改" />
            </label>
            <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <input v-model="balanceSettings.notification.mention_all_on_low_balance" type="checkbox" class="checkbox" />
                低余额 @所有人
              </label>
              <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
                <input v-model="healthSettings.notification.mention_all_on_immediate" type="checkbox" class="checkbox" />
                紧急健康告警 @所有人
              </label>
            </div>
          </div>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">账号健康整合控制</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">这里是快捷控制，账号健康页面仍然保留完整视图。</p>
          </div>
          <label class="inline-flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-200">
            <input v-model="healthSettings.enabled" type="checkbox" class="checkbox" />
            启用健康通知规则
          </label>
        </div>
        <div class="mt-4 grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-5">
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">通知范围</span>
            <select v-model="healthSettings.mode" class="input">
              <option value="smart">智能</option>
              <option value="opened_only">只看已打开账号</option>
              <option value="all">全部账号</option>
            </select>
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">主动探测间隔分钟</span>
            <input v-model.number="healthSettings.probe.interval_minutes" type="number" min="1" max="1440" class="input" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">每轮主动探测</span>
            <input v-model.number="healthSettings.probe.max_per_run" type="number" min="1" max="20" class="input" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">探测超时秒</span>
            <input v-model.number="healthSettings.probe.timeout_seconds" type="number" min="1" max="120" class="input" />
          </label>
          <label class="space-y-1">
            <span class="text-xs font-medium text-gray-500 dark:text-gray-400">健康通知限速/小时</span>
            <input v-model.number="healthSettings.rate_limit_per_hour" type="number" min="0" max="1000" class="input" />
          </label>
        </div>
      </section>

      <section class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-200 bg-gray-50/70 p-4 dark:border-dark-700 dark:bg-dark-900/30">
          <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-base font-semibold text-gray-900 dark:text-white">账号余额控制台</h2>
                <span class="rounded-full bg-white px-2 py-0.5 text-xs font-medium text-gray-500 ring-1 ring-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:ring-dark-700">
                  共 {{ balanceResponse?.total ?? 0 }} 个账号
                </span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">单账号指定查询方式、阈值和开关；异常账号优先浮现。</p>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary h-9" :disabled="loading" @click="applyBalanceFilters">
                <Icon name="search" size="sm" />
                <span>筛选</span>
              </button>
              <button type="button" class="btn btn-secondary h-9" :disabled="loading" @click="resetBalanceFilters">
                <Icon name="refresh" size="sm" />
                <span>重置</span>
              </button>
            </div>
          </div>

          <div class="mt-4 grid grid-cols-1 gap-2 md:grid-cols-[minmax(16rem,1fr)_repeat(3,minmax(8rem,10rem))]">
            <input v-model.trim="filters.q" type="text" class="input h-9" placeholder="搜索账号、平台或错误" @keyup.enter="applyBalanceFilters" />
            <select v-model="filters.platform" class="input h-9 min-w-[8rem]" @change="applyBalanceFilters">
              <option value="">全部平台</option>
              <option v-for="platform in platformOptions" :key="platform.value" :value="platform.value">{{ platform.label }}</option>
            </select>
            <select v-model="filters.method" class="input h-9 min-w-[9rem]" @change="applyBalanceFilters">
              <option value="">全部方式</option>
              <option v-for="method in methodOptions" :key="method.value" :value="method.value">{{ method.label }}</option>
            </select>
            <select v-model="filters.probe_status" class="input h-9 min-w-[8rem]" @change="applyBalanceFilters">
              <option value="">全部状态</option>
              <option v-for="status in probeStatusOptions" :key="status.value" :value="status.value">{{ status.label }}</option>
            </select>
          </div>

          <div class="mt-3 flex flex-wrap gap-2">
            <label class="inline-flex h-8 items-center gap-2 rounded-full border border-gray-200 bg-white px-3 text-xs font-medium text-gray-600 transition hover:border-amber-300 hover:text-amber-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              <input v-model="filters.only_low" type="checkbox" class="checkbox" @change="applyBalanceFilters" />
              只看低余额
            </label>
            <label class="inline-flex h-8 items-center gap-2 rounded-full border border-gray-200 bg-white px-3 text-xs font-medium text-gray-600 transition hover:border-red-300 hover:text-red-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              <input v-model="filters.only_failed" type="checkbox" class="checkbox" @change="applyBalanceFilters" />
              只看异常
            </label>
            <label class="inline-flex h-8 items-center gap-2 rounded-full border border-gray-200 bg-white px-3 text-xs font-medium text-gray-600 transition hover:border-sky-300 hover:text-sky-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              <input v-model="filters.only_due" type="checkbox" class="checkbox" @change="applyBalanceFilters" />
              只看待检查
            </label>
            <label class="inline-flex h-8 items-center gap-2 rounded-full border border-gray-200 bg-white px-3 text-xs font-medium text-gray-600 transition hover:border-emerald-300 hover:text-emerald-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-300">
              <input v-model="filters.only_schedulable" type="checkbox" class="checkbox" @change="applyBalanceFilters" />
              只看可调度
            </label>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[1180px] table-fixed divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <colgroup>
              <col class="w-[30%]" />
              <col class="w-[14%]" />
              <col class="w-[23%]" />
              <col class="w-[10%]" />
              <col class="w-[13%]" />
              <col class="w-[10%]" />
            </colgroup>
            <thead class="bg-white text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              <tr>
                <th class="px-4 py-2.5 text-left">
                  <button type="button" :class="sortHeaderClass('account_name')" @click="toggleBalanceSort('account_name')">
                    <span>账号</span>
                    <span>{{ sortHeaderMark('account_name') }}</span>
                  </button>
                </th>
                <th class="px-4 py-2.5 text-left">
                  <button type="button" :class="sortHeaderClass('balance_usd')" @click="toggleBalanceSort('balance_usd')">
                    <span>余额/状态</span>
                    <span>{{ sortHeaderMark('balance_usd') }}</span>
                  </button>
                </th>
                <th class="px-4 py-2.5 text-left">
                  <button type="button" :class="sortHeaderClass('method')" @click="toggleBalanceSort('method')">
                    <span>查询方式</span>
                    <span>{{ sortHeaderMark('method') }}</span>
                  </button>
                </th>
                <th class="px-4 py-2.5 text-left">
                  <button type="button" :class="sortHeaderClass('threshold_usd')" @click="toggleBalanceSort('threshold_usd')">
                    <span>阈值</span>
                    <span>{{ sortHeaderMark('threshold_usd') }}</span>
                  </button>
                </th>
                <th class="px-4 py-2.5 text-left">
                  <button type="button" :class="sortHeaderClass('checked_at')" @click="toggleBalanceSort('checked_at')">
                    <span>最近检查</span>
                    <span>{{ sortHeaderMark('checked_at') }}</span>
                  </button>
                </th>
                <th class="px-4 py-2.5 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in balanceItems" :key="item.account_id" class="align-middle transition hover:bg-gray-50 dark:hover:bg-dark-700/50" :class="balanceRowClass(item)">
                <td class="px-4 py-3">
                  <div class="min-w-0">
                    <div class="truncate font-medium text-gray-900 dark:text-white" :title="item.account_name">{{ item.account_name }}</div>
                    <div class="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-gray-500 dark:text-gray-400">
                      <span>#{{ item.account_id }}</span>
                      <span class="text-gray-300 dark:text-dark-500">/</span>
                      <span>{{ item.platform }}</span>
                      <span class="text-gray-300 dark:text-dark-500">/</span>
                      <span>{{ item.type }}</span>
                    </div>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <div class="flex min-w-0 flex-col gap-1">
                    <span class="text-base font-semibold tabular-nums" :class="balanceTextClass(item.balance_probe)">
                      {{ formatBalance(item.balance_probe) }}
                    </span>
                    <div class="flex items-center gap-2">
                      <span class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-semibold" :class="probeStatusClass(item.balance_probe.status)">
                        <span class="h-1.5 w-1.5 rounded-full bg-current"></span>
                        {{ probeStatusLabel(item.balance_probe.status) }}
                      </span>
                      <span v-if="item.balance_probe.total_used_usd != null" class="truncate text-xs text-gray-500 dark:text-gray-400">
                        已用 ${{ formatNumber(item.balance_probe.total_used_usd) }}
                      </span>
                    </div>
                  </div>
                </td>
                <td class="px-4 py-3">
                  <select class="input h-8 w-full text-xs" :value="item.balance_probe.method" @change="onItemMethodChange(item, $event)">
                    <option v-for="method in methodOptions" :key="method.value" :value="method.value">{{ method.label }}</option>
                  </select>
                  <div class="mt-1 flex min-h-[1rem] items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
                    <span v-if="item.balance_probe.detected_method">命中 {{ methodLabel(item.balance_probe.detected_method) }}</span>
                    <span v-else>等待命中方式</span>
                  </div>
                  <div v-if="item.balance_probe.error" class="mt-1 truncate text-xs text-red-500" :title="item.balance_probe.error">
                    {{ item.balance_probe.error }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <input
                    class="input h-8 w-full max-w-[6.5rem] text-xs"
                    type="number"
                    min="0"
                    step="0.01"
                    :value="item.balance_probe.threshold_usd ?? ''"
                    placeholder="默认"
                    @change="onItemThresholdChange(item, $event)"
                  />
                </td>
                <td class="px-4 py-3">
                  <div class="text-xs tabular-nums text-gray-600 dark:text-gray-300">{{ formatTime(item.balance_probe.checked_at) }}</div>
                </td>
                <td class="px-4 py-3">
                  <div class="flex items-center justify-end gap-2">
                    <label class="inline-flex h-8 items-center gap-1 rounded-md border border-gray-200 px-2 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                      <input :checked="item.balance_probe.enabled" type="checkbox" class="checkbox" @change="onItemEnabledChange(item, $event)" />
                      启用
                    </label>
                    <button type="button" class="btn btn-secondary btn-sm w-[4.75rem] justify-center" :disabled="probingIds.has(item.account_id)" @click="probeItem(item)">
                      <Icon name="beaker" size="xs" />
                      <span>{{ probingIds.has(item.account_id) ? '探测中' : '探测' }}</span>
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="!loading && balanceItems.length === 0" class="p-8 text-center text-sm text-gray-500 dark:text-gray-400">
          没有匹配的账号
        </div>

        <div class="flex flex-col gap-3 border-t border-gray-200 p-4 dark:border-dark-700 sm:flex-row sm:items-center sm:justify-between">
          <div class="text-sm text-gray-500 dark:text-gray-400">共 {{ balanceResponse?.total ?? 0 }} 个账号</div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="filters.page <= 1 || loading" @click="changePage(filters.page - 1)">上一页</button>
            <span class="text-sm text-gray-600 dark:text-gray-300">第 {{ filters.page }} 页</span>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="isLastPage || loading" @click="changePage(filters.page + 1)">下一页</button>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import {
  getAccountBalanceMonitor,
  getAccountHealth,
  runAccountBalanceProbe,
  testAccountBalanceEnterpriseWeChat,
  updateAccountBalanceProbeConfig,
  updateAccountBalanceSettings,
  updateAccountHealthSettings,
  type OpsAccountBalanceAccountItem,
  type OpsAccountBalanceListResponse,
  type OpsAccountBalanceProbeMethod,
  type OpsAccountBalanceProbeState,
  type OpsAccountBalanceSettings,
  type OpsAccountHealthSettings
} from '@/api/admin/ops'

const appStore = useAppStore()

const methodOptions: Array<{ value: OpsAccountBalanceProbeMethod; label: string }> = [
  { value: 'auto', label: '智能' },
  { value: 'upstream_management', label: '上游账户' },
  { value: 'sub2api_usage', label: 'Sub2API' },
  { value: 'openai_billing', label: 'OpenAI Billing' },
  { value: 'disabled', label: '禁用' }
]

const platformOptions = [
  { value: 'openai', label: 'OpenAI' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'openrouter', label: 'OpenRouter' }
]

const probeStatusOptions = [
  { value: 'ok', label: '正常' },
  { value: 'failed', label: '失败' },
  { value: 'unsupported', label: '不支持' },
  { value: 'skipped', label: '跳过' },
  { value: 'unknown', label: '未知' }
]

const loading = ref(false)
const saving = ref(false)
const testingRobot = ref(false)
const errorMessage = ref('')
const balanceResponse = ref<OpsAccountBalanceListResponse | null>(null)
const probingIds = ref(new Set<number>())

const balanceSettings = reactive<OpsAccountBalanceSettings>(defaultBalanceSettings())
const healthSettings = reactive<OpsAccountHealthSettings>(defaultHealthSettings())

const filters = reactive({
  page: 1,
  page_size: 20,
  q: '',
  platform: '',
  probe_status: '',
  method: '',
  only_low: false,
  only_failed: false,
  only_due: false,
  only_schedulable: false,
  sort_by: 'account_name',
  sort_order: 'asc' as 'asc' | 'desc'
})

const balanceItems = computed(() => balanceResponse.value?.items ?? [])
const isLastPage = computed(() => {
  const total = balanceResponse.value?.total ?? 0
  return filters.page * filters.page_size >= total
})

const summaryCards = computed(() => {
  const summary = balanceResponse.value?.summary
  return [
    { label: '账号总数', value: summary?.total_accounts ?? 0, className: 'text-gray-900 dark:text-white' },
    { label: '已识别余额', value: summary?.known_balance_count ?? 0, className: 'text-teal-600 dark:text-teal-300' },
    { label: '低余额', value: summary?.low_balance_count ?? 0, className: 'text-amber-600 dark:text-amber-300' },
    { label: '探测失败', value: (summary?.failed_count ?? 0) + (summary?.unsupported_count ?? 0), className: 'text-red-600 dark:text-red-300' }
  ]
})

onMounted(() => {
  void loadAll()
})

async function loadAll() {
  loading.value = true
  errorMessage.value = ''
  try {
    await Promise.all([loadBalance(), loadHealthSettings()])
  } catch (error) {
    errorMessage.value = errorMessageOf(error, '加载通知配置失败')
  } finally {
    loading.value = false
  }
}

async function loadBalance() {
  const data = await getAccountBalanceMonitor({
    page: filters.page,
    page_size: filters.page_size,
    platform: filters.platform || undefined,
    probe_status: filters.probe_status || undefined,
    q: filters.q || undefined,
    method: filters.method || undefined,
    only_low: filters.only_low || undefined,
    only_failed: filters.only_failed || undefined,
    only_due: filters.only_due || undefined,
    only_schedulable: filters.only_schedulable || undefined,
    sort_by: filters.sort_by,
    sort_order: filters.sort_order
  })
  balanceResponse.value = data
  applyBalanceSettings(data.settings)
}

async function loadHealthSettings() {
  const data = await getAccountHealth({ recent_limit: 1 })
  applyHealthSettings(data.settings)
}

async function saveAll() {
  saving.value = true
  errorMessage.value = ''
  try {
    const validationError = validateNotificationRobotSettings()
    if (validationError) {
      errorMessage.value = validationError
      appStore.showError(validationError)
      return
    }
    const [balance, health] = await Promise.all([
      updateAccountBalanceSettings(toPlain(balanceSettings)),
      updateAccountHealthSettings(toPlain(healthSettings))
    ])
    applyBalanceSettings(balance)
    applyHealthSettings(health)
    appStore.showSuccess('通知配置已保存')
  } catch (error) {
    errorMessage.value = errorMessageOf(error, '保存通知配置失败')
    appStore.showError(errorMessage.value)
  } finally {
    saving.value = false
  }
}

async function testBalanceRobot() {
  testingRobot.value = true
  try {
    await testAccountBalanceEnterpriseWeChat(toPlain(balanceSettings))
    appStore.showSuccess('测试消息已发送')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '测试发送失败'))
  } finally {
    testingRobot.value = false
  }
}

function applyBalanceSettings(settings: OpsAccountBalanceSettings) {
  Object.assign(balanceSettings, defaultBalanceSettings(), settings)
  balanceSettings.probe = { ...defaultBalanceSettings().probe, ...(settings.probe ?? {}) }
  balanceSettings.notification = { ...defaultBalanceSettings().notification, ...(settings.notification ?? {}) }
}

function applyHealthSettings(settings: OpsAccountHealthSettings) {
  Object.assign(healthSettings, defaultHealthSettings(), settings)
  healthSettings.probe = { ...defaultHealthSettings().probe, ...(settings.probe ?? {}) }
  healthSettings.notification = { ...defaultHealthSettings().notification, ...(settings.notification ?? {}) }
}

function toggleMethodOrder(method: OpsAccountBalanceProbeMethod | string) {
  const list = balanceSettings.probe.method_order
  const index = list.indexOf(method)
  if (index >= 0) {
    list.splice(index, 1)
    return
  }
  list.push(method)
}

async function updateItemMethod(item: OpsAccountBalanceAccountItem, method: string) {
  await patchItem(item, { method })
}

async function updateItemEnabled(item: OpsAccountBalanceAccountItem, enabled: boolean) {
  await patchItem(item, { enabled })
}

async function updateItemThreshold(item: OpsAccountBalanceAccountItem, raw: string) {
  const trimmed = raw.trim()
  if (trimmed === '') {
    await patchItem(item, { use_default_threshold: true })
    return
  }
  const value = Number(trimmed)
  if (!Number.isFinite(value) || value < 0) {
    appStore.showError('阈值必须是非负数字')
    return
  }
  await patchItem(item, { threshold_usd: value })
}

function onItemMethodChange(item: OpsAccountBalanceAccountItem, event: Event) {
  const target = event.target as HTMLSelectElement | null
  if (!target) return
  void updateItemMethod(item, target.value)
}

function onItemThresholdChange(item: OpsAccountBalanceAccountItem, event: Event) {
  const target = event.target as HTMLInputElement | null
  if (!target) return
  void updateItemThreshold(item, target.value)
}

function onItemEnabledChange(item: OpsAccountBalanceAccountItem, event: Event) {
  const target = event.target as HTMLInputElement | null
  if (!target) return
  void updateItemEnabled(item, target.checked)
}

async function patchItem(item: OpsAccountBalanceAccountItem, payload: { method?: string; enabled?: boolean; threshold_usd?: number; use_default_threshold?: boolean }) {
  try {
    const state = await updateAccountBalanceProbeConfig(item.account_id, payload)
    item.balance_probe = state
    appStore.showSuccess('账号余额配置已更新')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '更新账号配置失败'))
  }
}

async function probeItem(item: OpsAccountBalanceAccountItem) {
  const next = new Set(probingIds.value)
  next.add(item.account_id)
  probingIds.value = next
  try {
    const result = await runAccountBalanceProbe(item.account_id, { force: true })
    item.balance_probe = result.state
    appStore.showSuccess('余额探测完成')
  } catch (error) {
    appStore.showError(errorMessageOf(error, '余额探测失败'))
  } finally {
    const done = new Set(probingIds.value)
    done.delete(item.account_id)
    probingIds.value = done
  }
}

function changePage(page: number) {
  filters.page = Math.max(1, page)
  void loadBalance()
}

function applyBalanceFilters() {
  filters.page = 1
  void loadBalance()
}

function resetBalanceFilters() {
  filters.page = 1
  filters.q = ''
  filters.platform = ''
  filters.probe_status = ''
  filters.method = ''
  filters.only_low = false
  filters.only_failed = false
  filters.only_due = false
  filters.only_schedulable = false
  filters.sort_by = 'account_name'
  filters.sort_order = 'asc'
  void loadBalance()
}

function toggleBalanceSort(key: string) {
  if (filters.sort_by === key) {
    filters.sort_order = filters.sort_order === 'asc' ? 'desc' : 'asc'
  } else {
    filters.sort_by = key
    filters.sort_order = key === 'checked_at' || key === 'balance_usd' ? 'desc' : 'asc'
  }
  filters.page = 1
  void loadBalance()
}

function sortHeaderClass(key: string) {
  return [
    'inline-flex items-center gap-1 font-semibold transition hover:text-teal-600 dark:hover:text-teal-300',
    filters.sort_by === key ? 'text-teal-600 dark:text-teal-300' : 'text-gray-500 dark:text-gray-400'
  ]
}

function sortHeaderMark(key: string) {
  if (filters.sort_by !== key) return '↕'
  return filters.sort_order === 'asc' ? '↑' : '↓'
}

function formatBalance(state: OpsAccountBalanceProbeState) {
  if (state.balance_amount != null) {
    const currency = String(state.balance_currency || 'USD').toUpperCase()
    const value = formatNumber(state.balance_amount)
    if (currency === 'USD') return `$${value}`
    if (currency === 'CNY') return `¥${value}`
    return `${currency} ${value}`
  }
  if (state.balance_usd != null) return `$${formatNumber(state.balance_usd)}`
  if (state.unlimited) return '额度未设上限'
  return '未知'
}

function formatNumber(value: number) {
  return Number(value).toFixed(2)
}

function balanceTextClass(state: OpsAccountBalanceProbeState) {
  if (state.balance_amount != null && state.balance_currency !== 'USD') return 'text-emerald-600 dark:text-emerald-300'
  if (state.balance_usd == null) return state.unlimited ? 'text-sky-600 dark:text-sky-300' : 'text-gray-500 dark:text-gray-400'
  const threshold = state.threshold_usd ?? balanceSettings.default_threshold_usd
  if (threshold > 0 && state.balance_usd <= threshold) return 'text-amber-600 dark:text-amber-300'
  return 'text-emerald-600 dark:text-emerald-300'
}

function balanceRowClass(item: OpsAccountBalanceAccountItem) {
  const state = item.balance_probe
  if (state.status === 'failed' || state.status === 'unsupported') {
    return 'bg-red-50/40 dark:bg-red-950/10'
  }
  const threshold = state.threshold_usd ?? balanceSettings.default_threshold_usd
  if (state.balance_usd != null && threshold > 0 && state.balance_usd <= threshold) {
    return 'bg-amber-50/40 dark:bg-amber-950/10'
  }
  return ''
}

function probeStatusLabel(status: string) {
  switch (status) {
    case 'ok':
      return '正常'
    case 'failed':
      return '失败'
    case 'unsupported':
      return '不支持'
    case 'skipped':
      return '跳过'
    default:
      return '未知'
  }
}

function probeStatusClass(status: string) {
  switch (status) {
    case 'ok':
      return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'failed':
    case 'unsupported':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'skipped':
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
    default:
      return 'bg-slate-100 text-slate-600 dark:bg-slate-800 dark:text-slate-300'
  }
}

function methodLabel(method?: string) {
  if (method === 'newapi_token_usage') return 'API Key 用量（非账户余额）'
  return methodOptions.find((item) => item.value === method)?.label ?? method ?? '智能'
}

function formatTime(value?: string | null) {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  return date.toLocaleString()
}

function toPlain<T>(value: T): T {
  return JSON.parse(JSON.stringify(value))
}

function errorMessageOf(error: unknown, fallback: string) {
  if (typeof error === 'object' && error !== null) {
    const anyError = error as { response?: { data?: { message?: string; error?: string } }; message?: string }
    return humanizeOpsError(anyError.response?.data?.message || anyError.response?.data?.error || anyError.message || fallback)
  }
  return fallback
}

function humanizeOpsError(message: string) {
  if (message.includes('account_balance.notification.enterprise_wechat_webhook_url is required')) {
    return '已开启余额低额通知，请先填写余额机器人 Webhook，或关闭余额通知。'
  }
  if (message.includes('account health enterprise wechat webhook is not configured')) {
    return '已开启账号健康通知，请先填写账号健康机器人 Webhook，或关闭健康通知。'
  }
  if (message.includes('enterprise wechat webhook')) {
    return '企业微信机器人 Webhook 配置不正确，请检查地址后再保存。'
  }
  return message
}

function validateNotificationRobotSettings() {
  if (balanceSettings.notification.enterprise_wechat_enabled && !hasConfiguredWebhook(balanceSettings.notification.enterprise_wechat_webhook_url)) {
    return '已开启余额低额通知，请先填写余额机器人 Webhook，或关闭余额通知。'
  }
  if (healthSettings.notification.enterprise_wechat_enabled && !hasConfiguredWebhook(healthSettings.notification.enterprise_wechat_webhook_url)) {
    return '已开启账号健康通知，请先填写账号健康机器人 Webhook，或关闭健康通知。'
  }
  return ''
}

function hasConfiguredWebhook(value?: string) {
  const trimmed = String(value ?? '').trim()
  return trimmed !== ''
}

function defaultBalanceSettings(): OpsAccountBalanceSettings {
  return {
    enabled: true,
    probe: {
      interval_minutes: 5,
      max_per_run: 2,
      timeout_seconds: 8,
      only_schedulable: true,
      method_order: ['upstream_management', 'sub2api_usage', 'openai_billing']
    },
    notification: {
      enterprise_wechat_enabled: false,
      enterprise_wechat_webhook_url: '',
      mention_all_on_low_balance: false
    },
    default_threshold_usd: 10,
    rate_limit_per_hour: 12
  }
}

function defaultHealthSettings(): OpsAccountHealthSettings {
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
      model_id: 'gpt-5.4-mini',
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
</script>
