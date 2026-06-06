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

        <div class="mt-4 grid grid-cols-1 gap-4 xl:grid-cols-6">
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

          <div class="rounded-lg border border-sky-200 p-3 dark:border-sky-900/50 xl:col-span-2">
            <div class="flex items-center justify-between gap-3">
              <div>
                <label class="text-sm font-medium text-gray-800 dark:text-gray-100">关闭账号自动探测</label>
                <p class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">按频率自动探测关闭账号，手动探测也使用这里的请求方式</p>
              </div>
              <input v-model="settingsForm.probe.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-sky-600 focus:ring-sky-500" />
            </div>
            <div class="mt-3 grid grid-cols-2 gap-2">
              <NumberField v-model="settingsForm.probe.interval_minutes" label="间隔分钟" />
              <NumberField v-model="settingsForm.probe.max_per_run" label="每轮最多" />
              <NumberField v-model="settingsForm.probe.timeout_seconds" label="超时秒" />
              <label class="block min-w-0">
                <span class="block truncate text-[11px] font-medium text-gray-500 dark:text-gray-400">请求模式</span>
                <select v-model="settingsForm.probe.mode" class="input mt-1 h-8 text-sm">
                  <option value="default">默认连接测试</option>
                  <option value="compact">OpenAI compact</option>
                </select>
              </label>
              <label class="block min-w-0">
                <span class="block truncate text-[11px] font-medium text-gray-500 dark:text-gray-400">模型</span>
                <input v-model.trim="settingsForm.probe.model_id" type="text" class="input mt-1 h-8 text-sm" placeholder="默认" />
              </label>
            </div>
            <label class="mt-3 block min-w-0">
              <span class="block truncate text-[11px] font-medium text-gray-500 dark:text-gray-400">探测 prompt</span>
              <textarea
                v-model.trim="settingsForm.probe.prompt"
                rows="2"
                class="input mt-1 text-sm"
                placeholder="留空使用默认探测请求"
              ></textarea>
            </label>
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

      <section v-else class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        <article
          v-for="item in sortedItems"
          :key="item.account_id"
          class="overflow-hidden rounded-lg border bg-white shadow-sm dark:bg-dark-800"
          :class="accountBorderClass(item)"
        >
          <div class="p-4 sm:p-5">
            <div class="flex items-start justify-between gap-3">
              <div class="flex min-w-0 items-start gap-3">
                <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-lg text-base font-bold" :class="platformAvatarClass(item)">
                  {{ platformInitial(item.platform) }}
                </div>
                <div class="min-w-0">
                  <div class="flex min-w-0 flex-wrap items-center gap-2">
                    <h2 class="min-w-0 truncate text-lg font-semibold text-gray-900 dark:text-white">{{ item.account_name || `#${item.account_id}` }}</h2>
                    <span class="shrink-0 text-xs text-gray-400">#{{ item.account_id }}</span>
                  </div>
                  <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                    <span>{{ item.platform || '-' }}</span>
                    <span>{{ item.group_name || '-' }}</span>
                    <span>group {{ item.group_id }}</span>
                  </div>
                </div>
              </div>
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

            <div class="mt-3 flex flex-wrap gap-2">
              <StatusBadge :text="item.is_opened ? '账号已打开' : '账号已关闭'" :kind="item.is_opened ? 'success' : 'muted'" />
              <StatusBadge :text="item.is_available ? '可调度' : '不可调度'" :kind="item.is_available ? 'success' : 'warning'" />
              <StatusBadge v-if="!hasTraffic(item)" text="暂无流量" kind="muted" />
              <StatusBadge v-if="item.is_rate_limited" text="限流中" kind="warning" />
              <StatusBadge v-if="item.is_overloaded" text="过载冷却" kind="warning" />
              <StatusBadge v-if="item.is_temp_unschedulable" text="临时暂停" kind="warning" />
              <StatusBadge v-if="item.has_error" text="错误状态" kind="danger" />
            </div>

            <div class="mt-5 grid grid-cols-1 gap-3 sm:grid-cols-3">
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ primaryMetricLabel(item) }}</div>
                <div class="mt-1 truncate text-3xl font-semibold" :class="primaryMetricClass(item)">
                  {{ primaryMetricText(item) }}
                </div>
                <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ primaryMetricHint(item) }}</div>
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">延迟</div>
                <div class="mt-1 truncate text-3xl font-semibold text-gray-900 dark:text-white">
                  {{ latencyText(item) }}
                </div>
                <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ latencyHint(item) }}</div>
              </div>
              <div class="rounded-lg bg-gray-50 p-3 dark:bg-dark-700/60">
                <div class="text-xs font-medium text-gray-500 dark:text-gray-400">首 Token · 5m</div>
                <div class="mt-1 truncate text-3xl font-semibold" :class="firstTokenMetricClass(item)">
                  {{ firstTokenMetricText(item) }}
                </div>
                <div class="mt-1 truncate text-xs text-gray-500 dark:text-gray-400">{{ firstTokenMetricHint(item) }}</div>
              </div>
            </div>

            <div class="mt-5 border-t border-gray-200 pt-4 dark:border-dark-700">
              <div class="flex items-end justify-between gap-3">
                <div class="text-sm text-gray-500 dark:text-gray-400">{{ headlineHealthLabel(item) }}</div>
                <div class="text-4xl font-semibold" :class="headlineHealthClass(item)">
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
              <div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-4">
                <div v-for="window in windowOrder" :key="window" class="rounded-lg border border-gray-200 p-3 dark:border-dark-700">
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

            <div class="mt-5">
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

            <div class="mt-5 rounded-lg border p-3" :class="recommendationPanelClass(item)">
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
  type OpsAccountHealthFirstTokenStats,
  type OpsAccountHealthResponse,
  type OpsAccountHealthSettings,
  type OpsAccountHealthSample,
  type OpsAccountHealthWindow,
  type OpsAccountHealthWindowStats
} from '@/api/admin/ops'

const WEBHOOK_MASK = '__configured__'

type FirstTokenWindow = OpsAccountHealthWindow | '5m'

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
const probingAccounts = ref<Set<number>>(new Set())
const autoRefreshMs = 45_000
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null
let applyingSettings = false

const windowOrder: OpsAccountHealthWindow[] = ['1m', '10m', '30m', '1h']
const probeWindowMinutes: Record<OpsAccountHealthWindow, number> = {
  '1m': 1,
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

async function runProbe(item: OpsAccountHealthItem) {
  if (!item?.account_id || isProbing(item.account_id)) return
  setProbing(item.account_id, true)
  try {
    await opsAPI.runAccountHealthProbe(item.account_id, {
      model_id: settingsForm.value.probe.model_id || undefined,
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

function isProbing(accountID: number): boolean {
  return probingAccounts.value.has(accountID)
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
      enabled: true,
      interval_minutes: 30,
      max_per_run: 2,
      timeout_seconds: 20,
      model_id: '',
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
