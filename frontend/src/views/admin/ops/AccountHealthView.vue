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
                <h1 class="truncate text-xl font-semibold text-gray-900 dark:text-white">账号健康监控</h1>
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
              <input
                v-model.trim="groupIdInput"
                type="number"
                min="1"
                class="input h-9 min-w-[8rem]"
                placeholder="group_id"
                @keyup.enter="fetchData"
              />
            </label>
            <button type="button" class="btn btn-secondary h-9 whitespace-nowrap" :disabled="loading" @click="fetchData">
              <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
              <span>刷新</span>
            </button>
            <button type="button" class="btn btn-secondary h-9 whitespace-nowrap" @click="settingsOpen = !settingsOpen">
              <Icon name="cog" size="sm" />
              <span>规则</span>
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

      <section v-if="settingsOpen" class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex items-center gap-2">
            <Icon name="bell" size="md" class="text-sky-500" />
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">智能通知规则</h2>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingRuntime || savingSettings" @click="loadRuntimeSettings">
              <Icon name="refresh" size="xs" :class="loadingRuntime ? 'animate-spin' : ''" />
              <span>重载</span>
            </button>
            <button type="button" class="btn btn-primary btn-sm" :disabled="savingSettings || !settingsLoaded" @click="saveSettings">
              <Icon name="check" size="xs" />
              <span>{{ savingSettings ? '保存中' : '保存' }}</span>
            </button>
          </div>
        </div>

        <div class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-5">
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
            <div class="flex items-center justify-between gap-3">
              <label class="text-sm font-medium text-gray-800 dark:text-gray-100">总开关</label>
              <input v-model="settingsForm.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
            </div>
            <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">通知模式</label>
            <select v-model="settingsForm.mode" class="input mt-1 h-9">
              <option value="smart">智能</option>
              <option value="opened_only">只通知打开账号</option>
              <option value="all">全部账号</option>
            </select>
            <label class="mt-3 block text-xs font-medium text-gray-500 dark:text-gray-400">每小时上限</label>
            <input v-model.number="settingsForm.rate_limit_per_hour" type="number" min="0" max="1000" class="input mt-1 h-9" />
            <label class="mt-3 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
              <input v-model="settingsForm.notification.enterprise_wechat_enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-sky-600 focus:ring-sky-500" />
              企业微信
            </label>
            <input
              v-if="settingsForm.notification.enterprise_wechat_enabled"
              v-model.trim="settingsForm.notification.enterprise_wechat_webhook_url"
              type="password"
              class="input mt-2 h-9"
              placeholder="https://qyapi.weixin.qq.com/..."
            />
            <div
              v-if="settingsForm.notification.enterprise_wechat_enabled && settingsForm.notification.enterprise_wechat_webhook_url === WEBHOOK_MASK"
              class="mt-1 text-[11px] font-medium text-emerald-600 dark:text-emerald-300"
            >
              webhook 已配置
            </div>
            <label v-if="settingsForm.notification.enterprise_wechat_enabled" class="mt-2 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
              <input v-model="settingsForm.notification.mention_all_on_immediate" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-red-600 focus:ring-red-500" />
              立即通知 @所有人
            </label>
          </div>

          <div class="rounded-lg border border-red-200 p-3 dark:border-red-900/50">
            <div class="flex items-center justify-between gap-3">
              <label class="text-sm font-medium text-gray-800 dark:text-gray-100">1 分钟突增</label>
              <input v-model="settingsForm.burst.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-red-600 focus:ring-red-500" />
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2">
              <NumberField v-model="settingsForm.burst.min_requests" label="最少请求" />
              <NumberField v-model="settingsForm.burst.error_rate_percent" label="错误率 %" />
              <NumberField v-model="settingsForm.burst.upstream_error_rate_percent" label="上游错误 %" />
              <NumberField v-model="settingsForm.burst.cooldown_minutes" label="冷却分钟" />
            </div>
            <label class="mt-3 flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
              <input v-model="settingsForm.burst.bypass_digest" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-red-600 focus:ring-red-500" />
              立即通知
            </label>
          </div>

          <div class="rounded-lg border border-amber-200 p-3 dark:border-amber-900/50">
            <div class="flex items-center justify-between gap-3">
              <label class="text-sm font-medium text-gray-800 dark:text-gray-100">降级关闭</label>
              <input v-model="settingsForm.degrade.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-amber-600 focus:ring-amber-500" />
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2">
              <NumberField v-model="settingsForm.degrade.window_minutes" label="窗口分钟" />
              <NumberField v-model="settingsForm.degrade.min_requests" label="最少请求" />
              <NumberField v-model="settingsForm.degrade.success_rate_min_percent" label="成功率 >=" />
              <NumberField v-model="settingsForm.degrade.error_rate_percent" label="错误率 >=" />
              <NumberField v-model="settingsForm.degrade.upstream_error_rate_percent" label="上游错误 >=" />
              <NumberField v-model="settingsForm.degrade.cooldown_minutes" label="冷却分钟" />
            </div>
          </div>

          <div class="rounded-lg border border-emerald-200 p-3 dark:border-emerald-900/50">
            <div class="flex items-center justify-between gap-3">
              <label class="text-sm font-medium text-gray-800 dark:text-gray-100">恢复打开</label>
              <input v-model="settingsForm.recovery.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-emerald-600 focus:ring-emerald-500" />
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2">
              <NumberField v-model="settingsForm.recovery.window_minutes" label="窗口分钟" />
              <NumberField v-model="settingsForm.recovery.min_requests" label="最少请求" />
              <NumberField v-model="settingsForm.recovery.success_rate_min_percent" label="成功率 >=" />
              <NumberField v-model="settingsForm.recovery.cooldown_minutes" label="冷却分钟" />
            </div>
            <div class="mt-3 grid grid-cols-1 gap-2 text-xs text-gray-600 dark:text-gray-300">
              <label class="flex items-center gap-2">
                <input v-model="settingsForm.recovery.notify_opened_accounts" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-emerald-600 focus:ring-emerald-500" />
                打开账号恢复通知
              </label>
              <label class="flex items-center gap-2">
                <input v-model="settingsForm.recovery.notify_closed_accounts" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-emerald-600 focus:ring-emerald-500" />
                关闭账号恢复通知
              </label>
            </div>
          </div>

          <div class="rounded-lg border border-sky-200 p-3 dark:border-sky-900/50">
            <div class="flex items-center justify-between gap-3">
              <label class="text-sm font-medium text-gray-800 dark:text-gray-100">关闭账号探测</label>
              <input v-model="settingsForm.probe.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-sky-600 focus:ring-sky-500" />
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2">
              <NumberField v-model="settingsForm.probe.interval_minutes" label="间隔分钟" />
              <NumberField v-model="settingsForm.probe.max_per_run" label="每轮最多" />
              <NumberField v-model="settingsForm.probe.timeout_seconds" label="超时秒" />
              <label class="block min-w-0">
                <span class="block truncate text-[11px] font-medium text-gray-500 dark:text-gray-400">模型</span>
                <input v-model.trim="settingsForm.probe.model_id" type="text" class="input mt-1 h-8 text-sm" placeholder="默认" />
              </label>
            </div>
          </div>
        </div>
      </section>

      <section v-if="loading && !response" class="rounded-lg border border-gray-200 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        加载中...
      </section>

      <section v-else-if="!response?.enabled" class="rounded-lg border border-gray-200 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        运维监控未启用
      </section>

      <section v-else-if="sortedItems.length === 0" class="rounded-lg border border-gray-200 bg-white p-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400">
        暂无账号健康数据
      </section>

      <section v-else class="space-y-3">
        <article
          v-for="item in sortedItems"
          :key="item.account_id"
          class="rounded-lg border bg-white p-4 shadow-sm dark:bg-dark-800"
          :class="accountBorderClass(item)"
        >
          <div class="grid grid-cols-1 gap-4 2xl:grid-cols-[minmax(18rem,1.4fr)_minmax(20rem,1.4fr)_minmax(18rem,1fr)]">
            <div class="min-w-0">
              <div class="flex items-start gap-3">
                <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg text-sm font-bold" :class="platformAvatarClass(item)">
                  {{ platformInitial(item.platform) }}
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</h2>
                    <span class="text-xs text-gray-400">#{{ item.account_id }}</span>
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>{{ item.platform || '-' }}</span>
                    <span>{{ item.group_name || '-' }}</span>
                    <span>group {{ item.group_id }}</span>
                  </div>
                  <div class="mt-3 flex flex-wrap gap-2">
                    <StatusBadge :text="item.is_opened ? '账号已打开' : '账号已关闭'" :kind="item.is_opened ? 'success' : 'muted'" />
                    <StatusBadge :text="item.is_available ? '可调度' : '不可调度'" :kind="item.is_available ? 'success' : 'warning'" />
                    <StatusBadge v-if="item.is_rate_limited" text="限流中" kind="warning" />
                    <StatusBadge v-if="item.is_overloaded" text="过载冷却" kind="warning" />
                    <StatusBadge v-if="item.is_temp_unschedulable" text="临时暂停" kind="warning" />
                    <StatusBadge v-if="item.has_error" text="错误状态" kind="danger" />
                  </div>
                </div>
              </div>

              <div class="mt-4 grid grid-cols-2 gap-2">
                <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                  <div class="text-xs text-gray-500 dark:text-gray-400">主成功率</div>
                  <div class="mt-1 text-3xl font-semibold" :class="successTextClass(primaryStat(item))">
                    {{ formatPercent(primaryStat(item)?.success_rate_percent) }}
                  </div>
                </div>
                <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                  <div class="text-xs text-gray-500 dark:text-gray-400">上游错误</div>
                  <div class="mt-1 text-3xl font-semibold" :class="errorTextClass(primaryStat(item)?.upstream_error_rate_percent)">
                    {{ formatPercent(primaryStat(item)?.upstream_error_rate_percent) }}
                  </div>
                </div>
              </div>
            </div>

            <div class="min-w-0">
              <div class="grid grid-cols-2 gap-2 md:grid-cols-4">
                <div v-for="window in windowOrder" :key="window" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
                  <div class="flex items-center justify-between gap-2">
                    <span class="text-xs font-semibold text-gray-500 dark:text-gray-400">{{ window }}</span>
                    <span class="text-xs text-gray-400">{{ statFor(item, window)?.request_count ?? 0 }} 次</span>
                  </div>
                  <div class="mt-2 text-lg font-semibold" :class="successTextClass(statFor(item, window))">
                    {{ formatPercent(statFor(item, window)?.success_rate_percent) }}
                  </div>
                  <div class="mt-2 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
                    <div class="h-full rounded-full" :class="windowBarClass(statFor(item, window))" :style="{ width: `${boundedPercent(statFor(item, window)?.success_rate_percent)}%` }"></div>
                  </div>
                  <div class="mt-2 flex justify-between text-[11px] text-gray-500 dark:text-gray-400">
                    <span>错 {{ formatPercent(statFor(item, window)?.error_rate_percent) }}</span>
                    <span>上 {{ formatPercent(statFor(item, window)?.upstream_error_rate_percent) }}</span>
                  </div>
                </div>
              </div>

              <div class="mt-4">
                <div class="flex items-center justify-between gap-3">
                  <div class="text-xs font-semibold text-gray-500 dark:text-gray-400">最近 {{ item.recent.length || 0 }} 次记录</div>
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
            </div>

            <div class="min-w-0 rounded-lg border p-3" :class="recommendationPanelClass(item)">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="severityClass(item.recommendation.severity)">
                      {{ item.recommendation.severity || 'P3' }}
                    </span>
                    <span class="rounded-md px-2 py-0.5 text-xs font-semibold" :class="notifyClass(item.recommendation.notify_mode)">
                      {{ notifyLabel(item.recommendation.notify_mode) }}
                    </span>
                  </div>
                  <h3 class="mt-3 text-lg font-semibold text-gray-900 dark:text-white">{{ item.recommendation.title }}</h3>
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
              <div v-if="probeText(item)" class="mt-3 rounded-lg bg-white/60 px-3 py-2 text-xs text-gray-600 dark:bg-dark-900/30 dark:text-gray-300">
                {{ probeText(item) }}
              </div>
            </div>
          </div>
        </article>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, onUnmounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAdminSettingsStore } from '@/stores'
import {
  opsAPI,
  type OpsAccountHealthItem,
  type OpsAccountHealthResponse,
  type OpsAccountHealthSettings,
  type OpsAccountHealthSample,
  type OpsAccountHealthWindow,
  type OpsAccountHealthWindowStats
} from '@/api/admin/ops'

const WEBHOOK_MASK = '__configured__'

const NumberField = defineComponent({
  name: 'NumberField',
  props: {
    modelValue: { type: Number, required: true },
    label: { type: String, required: true }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () => h('label', { class: 'block min-w-0' }, [
      h('span', { class: 'block truncate text-[11px] font-medium text-gray-500 dark:text-gray-400' }, props.label),
      h('input', {
        value: props.modelValue,
        type: 'number',
        min: 0,
        class: 'input mt-1 h-8 text-sm',
        onInput: (event: Event) => {
          const value = Number((event.target as HTMLInputElement).value)
          emit('update:modelValue', Number.isFinite(value) ? value : 0)
        }
      })
    ])
  }
})

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

const response = ref<OpsAccountHealthResponse | null>(null)
const loading = ref(false)
const loadingRuntime = ref(false)
const savingSettings = ref(false)
const errorMessage = ref('')
const platformFilter = ref('')
const groupIdInput = ref('')
const settingsOpen = ref(false)
const lastUpdated = ref<Date | null>(null)
const autoRefreshMs = 45_000
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
let applyingSettings = false

const windowOrder: OpsAccountHealthWindow[] = ['1m', '10m', '30m', '1h']

const settingsForm = ref<OpsAccountHealthSettings>(defaultAccountHealthSettings())
const settingsLoaded = ref(false)
const settingsDirty = ref(false)

const groupId = computed(() => {
  const parsed = Number(groupIdInput.value)
  return Number.isFinite(parsed) && parsed > 0 ? parsed : null
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

const summaryCards = computed(() => {
  const total = items.value.length
  const opened = items.value.filter(item => item.is_opened).length
  const closeNow = items.value.filter(item => item.recommendation.action === 'close_now').length
  const canOpen = items.value.filter(item => item.recommendation.action === 'can_open').length
  const immediate = items.value.filter(item => item.recommendation.notify_mode === 'immediate').length
  const available = items.value.filter(item => item.is_available).length
  return [
    { label: '账号', value: total, className: 'text-gray-900 dark:text-white' },
    { label: '已打开', value: opened, className: 'text-sky-600 dark:text-sky-300' },
    { label: '可调度', value: available, className: 'text-emerald-600 dark:text-emerald-300' },
    { label: '建议关闭', value: closeNow, className: closeNow ? 'text-red-600 dark:text-red-300' : 'text-gray-900 dark:text-white' },
    { label: '可恢复', value: canOpen, className: canOpen ? 'text-emerald-600 dark:text-emerald-300' : 'text-gray-900 dark:text-white' },
    { label: '立即通知', value: immediate, className: immediate ? 'text-red-600 dark:text-red-300' : 'text-gray-900 dark:text-white' }
  ]
})

watch(
  () => response.value?.settings,
  (settings) => {
    if (settings && !settingsLoaded.value && !settingsDirty.value) {
      applySettingsToForm(settings)
    }
  },
  { immediate: true }
)

watch(
  settingsForm,
  () => {
    if (!applyingSettings) {
      settingsDirty.value = true
    }
  },
  { deep: true, flush: 'sync' }
)

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

async function loadRuntimeSettings() {
  loadingRuntime.value = true
  try {
    const runtime = await opsAPI.getAlertRuntimeSettings()
    applySettingsToForm(runtime.account_health || response.value?.settings || defaultAccountHealthSettings())
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '规则加载失败')
  } finally {
    loadingRuntime.value = false
  }
}

async function saveSettings() {
  if (!settingsLoaded.value) {
    await loadRuntimeSettings()
  }
  if (!settingsLoaded.value) return

  savingSettings.value = true
  try {
    const saved = await opsAPI.updateAccountHealthSettings(cloneSettings(settingsForm.value))
    applySettingsToForm(saved)
    appStore.showSuccess('账号健康规则已保存')
    await fetchData()
  } catch (err: any) {
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || '规则保存失败')
  } finally {
    savingSettings.value = false
  }
}

function applySettingsToForm(settings: OpsAccountHealthSettings) {
  applyingSettings = true
  settingsForm.value = cloneSettings(settings)
  settingsLoaded.value = true
  settingsDirty.value = false
  applyingSettings = false
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
      enabled: false,
      interval_minutes: 30,
      max_per_run: 2,
      timeout_seconds: 20,
      model_id: ''
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
  return {
    ...defaults,
    ...raw,
    burst: { ...defaults.burst, ...(raw.burst || {}) },
    degrade: { ...defaults.degrade, ...(raw.degrade || {}) },
    recovery: { ...defaults.recovery, ...(raw.recovery || {}) },
    probe: { ...defaults.probe, ...(raw.probe || {}) },
    notification: { ...defaults.notification, ...(raw.notification || {}) }
  }
}

function statFor(item: OpsAccountHealthItem, window: OpsAccountHealthWindow): OpsAccountHealthWindowStats | undefined {
  return item.windows?.[window]
}

function primaryStat(item: OpsAccountHealthItem): OpsAccountHealthWindowStats | undefined {
  const tenMin = statFor(item, '10m')
  if (tenMin && tenMin.request_count > 0) return tenMin
  return statFor(item, '30m') || statFor(item, '1h') || tenMin
}

function boundedPercent(value: number | null | undefined): number {
  if (!Number.isFinite(value as number)) return 0
  return Math.max(0, Math.min(100, Number(value)))
}

function formatPercent(value: number | null | undefined): string {
  if (!Number.isFinite(value as number)) return '0.0%'
  return `${Number(value).toFixed(1)}%`
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

function errorTextClass(value?: number | null): string {
  const v = value ?? 0
  if (v >= 20) return 'text-red-600 dark:text-red-300'
  if (v >= 10) return 'text-amber-600 dark:text-amber-300'
  return 'text-emerald-600 dark:text-emerald-300'
}

function windowBarClass(stat?: OpsAccountHealthWindowStats): string {
  if (!stat || stat.request_count === 0) return 'bg-gray-300 dark:bg-gray-600'
  if (stat.success_rate_percent >= 98) return 'bg-emerald-500'
  if (stat.success_rate_percent >= 90) return 'bg-amber-500'
  return 'bg-red-500'
}

function recentSamplesForDisplay(item: OpsAccountHealthItem): Array<OpsAccountHealthSample | null> {
  const samples: Array<OpsAccountHealthSample | null> = [...(item.recent ?? [])].reverse().slice(-60)
  while (samples.length < 60) {
    samples.unshift(null)
  }
  return samples
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
  return `${sample.kind}${status}${model}${duration}`
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

onMounted(async () => {
  adminSettingsStore.fetch()
  await Promise.all([fetchData(), loadRuntimeSettings()])
  autoRefreshTimer = setInterval(fetchData, autoRefreshMs)
})

onUnmounted(() => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
})
</script>
