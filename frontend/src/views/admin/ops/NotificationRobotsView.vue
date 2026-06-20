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
              <input v-model.trim="balanceSettings.notification.enterprise_wechat_webhook_url" type="password" class="input" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
            </label>
            <label class="space-y-1">
              <span class="text-xs font-medium text-gray-500 dark:text-gray-400">账号健康机器人 Webhook</span>
              <input v-model.trim="healthSettings.notification.enterprise_wechat_webhook_url" type="password" class="input" placeholder="留空则不修改" />
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

      <section class="rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 border-b border-gray-200 p-4 dark:border-dark-700 xl:flex-row xl:items-center xl:justify-between">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">账号余额控制台</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">单账号可以指定查询方式，也可以保持智能自动识别。</p>
          </div>
          <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
            <input v-model.trim="filters.q" type="text" class="input h-9 min-w-[14rem]" placeholder="搜索账号/平台/错误" @keyup.enter="loadBalance" />
            <select v-model="filters.method" class="input h-9 min-w-[9rem]" @change="loadBalance">
              <option value="">全部方式</option>
              <option v-for="method in methodOptions" :key="method.value" :value="method.value">{{ method.label }}</option>
            </select>
            <label class="inline-flex h-9 items-center gap-2 rounded-lg border border-gray-200 px-3 text-sm dark:border-dark-700">
              <input v-model="filters.only_low" type="checkbox" class="checkbox" @change="loadBalance" />
              只看低余额
            </label>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-xs uppercase tracking-wide text-gray-500 dark:bg-dark-900 dark:text-gray-400">
              <tr>
                <th class="px-4 py-3 text-left">账号</th>
                <th class="px-4 py-3 text-left">余额</th>
                <th class="px-4 py-3 text-left">状态</th>
                <th class="px-4 py-3 text-left">查询方式</th>
                <th class="px-4 py-3 text-left">阈值</th>
                <th class="px-4 py-3 text-left">最近检查</th>
                <th class="px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="item in balanceItems" :key="item.account_id" class="hover:bg-gray-50 dark:hover:bg-dark-700/60">
                <td class="px-4 py-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ item.account_name }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">#{{ item.account_id }} · {{ item.platform }} · {{ item.type }}</div>
                </td>
                <td class="px-4 py-3">
                  <div class="font-semibold" :class="balanceTextClass(item.balance_probe)">
                    {{ formatBalance(item.balance_probe) }}
                  </div>
                  <div v-if="item.balance_probe.total_used_usd != null" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    已用 ${{ formatNumber(item.balance_probe.total_used_usd) }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <span class="rounded-full px-2 py-0.5 text-xs font-semibold" :class="probeStatusClass(item.balance_probe.status)">
                    {{ probeStatusLabel(item.balance_probe.status) }}
                  </span>
                  <div v-if="item.balance_probe.error" class="mt-1 max-w-[18rem] truncate text-xs text-red-500" :title="item.balance_probe.error">
                    {{ item.balance_probe.error }}
                  </div>
                </td>
                <td class="px-4 py-3">
                  <select class="input h-8 min-w-[9rem]" :value="item.balance_probe.method" @change="onItemMethodChange(item, $event)">
                    <option v-for="method in methodOptions" :key="method.value" :value="method.value">{{ method.label }}</option>
                  </select>
                  <div v-if="item.balance_probe.detected_method" class="mt-1 text-xs text-gray-500 dark:text-gray-400">命中 {{ methodLabel(item.balance_probe.detected_method) }}</div>
                </td>
                <td class="px-4 py-3">
                  <input
                    class="input h-8 w-24"
                    type="number"
                    min="0"
                    step="0.01"
                    :value="item.balance_probe.threshold_usd ?? ''"
                    placeholder="默认"
                    @change="onItemThresholdChange(item, $event)"
                  />
                </td>
                <td class="px-4 py-3 text-xs text-gray-500 dark:text-gray-400">
                  {{ formatTime(item.balance_probe.checked_at) }}
                </td>
                <td class="px-4 py-3">
                  <div class="flex justify-end gap-2">
                    <label class="inline-flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400">
                      <input :checked="item.balance_probe.enabled" type="checkbox" class="checkbox" @change="onItemEnabledChange(item, $event)" />
                      启用
                    </label>
                    <button type="button" class="btn btn-secondary btn-sm" :disabled="probingIds.has(item.account_id)" @click="probeItem(item)">
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
  { value: 'newapi_token_usage', label: 'New API' },
  { value: 'sub2api_usage', label: 'Sub2API' },
  { value: 'openai_billing', label: 'OpenAI Billing' },
  { value: 'disabled', label: '禁用' }
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
  method: '',
  only_low: false
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
    q: filters.q || undefined,
    method: filters.method || undefined,
    only_low: filters.only_low || undefined
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

function formatBalance(state: OpsAccountBalanceProbeState) {
  if (state.unlimited) return '无限额度'
  if (state.balance_usd == null) return '未知'
  return `$${formatNumber(state.balance_usd)}`
}

function formatNumber(value: number) {
  return Number(value).toFixed(2)
}

function balanceTextClass(state: OpsAccountBalanceProbeState) {
  if (state.unlimited) return 'text-sky-600 dark:text-sky-300'
  if (state.balance_usd == null) return 'text-gray-500 dark:text-gray-400'
  const threshold = state.threshold_usd ?? balanceSettings.default_threshold_usd
  if (threshold > 0 && state.balance_usd <= threshold) return 'text-amber-600 dark:text-amber-300'
  return 'text-emerald-600 dark:text-emerald-300'
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
    return anyError.response?.data?.message || anyError.response?.data?.error || anyError.message || fallback
  }
  return fallback
}

function defaultBalanceSettings(): OpsAccountBalanceSettings {
  return {
    enabled: true,
    probe: {
      interval_minutes: 60,
      max_per_run: 2,
      timeout_seconds: 8,
      only_schedulable: true,
      method_order: ['newapi_token_usage', 'sub2api_usage', 'openai_billing']
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
