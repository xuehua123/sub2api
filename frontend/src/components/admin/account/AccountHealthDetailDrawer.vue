<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="fixed inset-0 z-[80] flex justify-end bg-black/40"
      @click.self="$emit('close')"
    >
      <aside class="flex h-full w-full max-w-xl flex-col bg-white shadow-2xl dark:bg-dark-900">
        <header class="flex items-start justify-between gap-3 border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <div class="min-w-0">
            <h2 class="truncate text-base font-semibold text-gray-900 dark:text-white">
              {{ item?.account_name || `账号 #${accountId}` }}
            </h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ item?.platform || '—' }} · {{ item?.group_name || '未分组' }}
              <span v-if="item && !item.is_opened" class="ml-1 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] text-gray-600 dark:bg-dark-700 dark:text-gray-300">已关闭</span>
              <span v-if="generatedAt" class="ml-2">数据 {{ relativeGeneratedAt }}</span>
            </p>
          </div>
          <button
            type="button"
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-800 dark:hover:text-gray-200"
            @click="$emit('close')"
          >
            <Icon name="x" size="sm" />
          </button>
        </header>

        <div class="flex-1 space-y-4 overflow-y-auto px-5 py-4">
          <div v-if="loading && !item" class="space-y-3">
            <div class="h-20 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-800"></div>
            <div class="h-40 animate-pulse rounded-lg bg-gray-100 dark:bg-dark-800"></div>
          </div>

          <template v-else-if="item">
            <!-- Timeline first: closed accounts must still show probe history -->
            <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
              <div class="mb-2 flex items-center justify-between gap-2">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
                  近 {{ timelineSamples.length }} 次（请求 + 探测）
                </h3>
                <span class="text-[10px] text-gray-400">
                  绿请求 · 蓝探测成功 · 紫探测失败 · 黄429/529 · 红错误
                </span>
              </div>
              <div class="grid grid-cols-[repeat(60,minmax(2px,1fr))] gap-0.5">
                <span
                  v-for="(sample, idx) in paddedTimeline"
                  :key="idx"
                  class="h-4 rounded-sm"
                  :class="sampleClass(sample)"
                  :title="sampleTitle(sample)"
                />
              </div>
              <p v-if="timelineSamples.length === 0" class="mt-2 text-xs text-gray-400">
                暂无请求与探测样本。可对本账号立即探测，或在「全局探测设置」开启自动探测。
              </p>
            </section>

            <!-- Per-account probe actions (global settings live in the shared dialog) -->
            <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
              <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
                <h3 class="text-sm font-semibold text-gray-900 dark:text-white">本账号探测</h3>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    class="rounded-md border border-gray-200 px-2 py-1 text-xs text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800"
                    @click="$emit('open-global-probe-settings')"
                  >
                    全局探测设置
                  </button>
                  <button
                    type="button"
                    class="rounded-md bg-sky-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-sky-700 disabled:opacity-50"
                    :disabled="probing || loading"
                    @click="$emit('probe')"
                  >
                    {{ probing ? '探测中…' : '立即探测' }}
                  </button>
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-4 text-xs">
                <label class="flex items-center gap-2 text-gray-600 dark:text-gray-300">
                  <span>本账号自动探测</span>
                  <button
                    type="button"
                    class="relative inline-flex h-6 w-11 items-center rounded-full transition"
                    :class="item.probe_auto_disabled ? 'bg-gray-300 dark:bg-dark-600' : 'bg-emerald-500'"
                    :disabled="togglingProbe"
                    @click="toggleProbeAuto"
                  >
                    <span
                      class="inline-block h-4 w-4 transform rounded-full bg-white shadow transition"
                      :class="item.probe_auto_disabled ? 'translate-x-1' : 'translate-x-6'"
                    />
                  </button>
                  <span class="text-gray-400">{{ item.probe_auto_disabled ? '已停' : '开启' }}</span>
                </label>
                <span class="text-gray-500 dark:text-gray-400">
                  {{ probeStatusLabel(item) }}
                  <span v-if="item.probe?.checked_at" class="text-gray-400"> · {{ formatDateTime(item.probe.checked_at) }}</span>
                </span>
              </div>
              <p v-if="item.probe?.error_message" class="mt-2 text-xs text-red-600 dark:text-red-400">
                {{ item.probe.error_message }}
              </p>
              <p class="mt-2 text-[11px] text-gray-400">
                全局间隔、模型、是否启用自动探测等统一配置请点「全局探测设置」。此处只控制当前账号开关与立即探测。
              </p>
            </section>

            <!-- 1m trend -->
            <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
              <h3 class="mb-2 text-sm font-semibold text-gray-900 dark:text-white">最近 1m 成功率趋势</h3>
              <div class="flex items-baseline gap-2">
                <span class="text-2xl font-bold" :class="trendClass(item.success_rate_trend_1m)">
                  {{ trendArrow(item.success_rate_trend_1m) || '·' }}
                  {{ trendDirectionLabel(item.success_rate_trend_1m?.direction) }}
                </span>
                <span
                  v-if="item.success_rate_trend_1m && item.success_rate_trend_1m.direction !== 'unknown'"
                  class="text-sm text-gray-500"
                >
                  Δ {{ formatPercent(item.success_rate_trend_1m.delta_percent) }}
                </span>
              </div>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                当前 60s：{{ formatPercent(item.success_rate_trend_1m?.current_success_rate_percent) }}
                / {{ formatCount(item.success_rate_trend_1m?.current_request_count) }} 次
                · 前 60s：{{ formatPercent(item.success_rate_trend_1m?.previous_success_rate_percent) }}
                / {{ formatCount(item.success_rate_trend_1m?.previous_request_count) }} 次
              </p>
            </section>

            <!-- Window matrix -->
            <section class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
              <h3 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">时间窗质量（全量）</h3>
              <div class="overflow-x-auto">
                <table class="min-w-full text-left text-xs">
                  <thead class="text-gray-400">
                    <tr>
                      <th class="pb-2 pr-3 font-medium">窗口</th>
                      <th class="pb-2 pr-3 font-medium text-right">请求</th>
                      <th class="pb-2 pr-3 font-medium text-right">成功</th>
                      <th class="pb-2 pr-3 font-medium text-right">失败</th>
                      <th class="pb-2 pr-3 font-medium text-right">成功%</th>
                      <th class="pb-2 pr-3 font-medium text-right">429</th>
                      <th class="pb-2 pr-3 font-medium text-right">529</th>
                      <th class="pb-2 pr-3 font-medium text-right">均延迟</th>
                      <th class="pb-2 font-medium text-right">首Token</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr
                      v-for="window in allWindows"
                      :key="window"
                      class="border-t border-gray-100 dark:border-dark-700"
                    >
                      <td class="py-2 pr-3 font-semibold text-gray-700 dark:text-gray-200">{{ window }}</td>
                      <td class="py-2 pr-3 text-right tabular-nums">{{ formatCount(windowStats(item, window)?.request_count) }}</td>
                      <td class="py-2 pr-3 text-right tabular-nums">{{ formatCount(windowStats(item, window)?.success_count) }}</td>
                      <td class="py-2 pr-3 text-right tabular-nums">{{ formatCount(windowStats(item, window)?.error_count) }}</td>
                      <td
                        class="py-2 pr-3 text-right tabular-nums font-medium"
                        :class="successClass(windowStats(item, window)?.success_rate_percent)"
                      >
                        {{
                          (windowStats(item, window)?.request_count || 0) > 0
                            ? formatPercent(windowStats(item, window)?.success_rate_percent)
                            : '—'
                        }}
                      </td>
                      <td class="py-2 pr-3 text-right tabular-nums">{{ formatCount(windowStats(item, window)?.status_429_count) }}</td>
                      <td class="py-2 pr-3 text-right tabular-nums">{{ formatCount(windowStats(item, window)?.status_529_count) }}</td>
                      <td class="py-2 pr-3 text-right tabular-nums">{{ formatMs(windowStats(item, window)?.avg_duration_ms ?? null) }}</td>
                      <td class="py-2 text-right tabular-nums">{{ formatMs(firstTokenStats(item, window)?.avg_ms ?? null) }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </section>

            <section v-if="item.recommendation" class="rounded-xl border border-dashed border-gray-200 p-4 dark:border-dark-700">
              <h3 class="mb-1 text-sm font-semibold text-gray-900 dark:text-white">系统建议（参考）</h3>
              <p class="text-sm text-gray-700 dark:text-gray-200">
                {{ item.recommendation.title || actionLabel(item.recommendation.action) }}
                <span v-if="item.recommendation.severity" class="ml-1 text-xs text-gray-400">{{ item.recommendation.severity }}</span>
              </p>
              <p v-if="item.recommendation.reason" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ item.recommendation.reason }}
              </p>
            </section>
          </template>

          <p v-else class="text-sm text-gray-400">暂无该账号健康数据，请先刷新列表。</p>
        </div>
      </aside>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { OpsAccountHealthItem, OpsAccountHealthSettings } from '@/api/admin/ops'
import Icon from '@/components/icons/Icon.vue'
import {
  ACCOUNT_HEALTH_ALL_WINDOWS,
  actionLabel,
  firstTokenStats,
  formatCount,
  formatMs,
  formatPercent,
  mergedTimelineSamples,
  paddedTimelineSamples,
  probeStatusLabel,
  sampleClass,
  sampleTitle,
  successClass,
  trendArrow,
  trendClass,
  windowStats
} from '@/utils/accountHealthDisplay'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  show: boolean
  accountId: number | null
  item?: OpsAccountHealthItem | null
  settings?: OpsAccountHealthSettings | null
  generatedAt?: string | null
  loading?: boolean
  probing?: boolean
  savingSettings?: boolean
  togglingProbe?: boolean
}>()

const emit = defineEmits<{
  close: []
  probe: []
  'update-probe-auto': [enabled: boolean]
  'open-global-probe-settings': []
}>()

const allWindows = ACCOUNT_HEALTH_ALL_WINDOWS

const timelineSamples = computed(() => mergedTimelineSamples(props.item, 60))
const paddedTimeline = computed(() => paddedTimelineSamples(props.item, 60))

const relativeGeneratedAt = computed(() => {
  if (!props.generatedAt) return ''
  const ts = new Date(props.generatedAt).getTime()
  if (!Number.isFinite(ts)) return ''
  const sec = Math.max(0, Math.round((Date.now() - ts) / 1000))
  if (sec < 5) return '刚刚'
  if (sec < 60) return `${sec}s 前`
  return `${Math.floor(sec / 60)}m 前`
})

function trendDirectionLabel(direction?: string) {
  switch (direction) {
    case 'up':
      return '上升'
    case 'down':
      return '下降'
    case 'flat':
      return '持平'
    default:
      return '未知'
  }
}

function toggleProbeAuto() {
  if (!props.item) return
  emit('update-probe-auto', !!props.item.probe_auto_disabled)
}
</script>
