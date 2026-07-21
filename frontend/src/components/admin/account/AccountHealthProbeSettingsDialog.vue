<template>
  <Teleport to="body">
    <div
      v-if="show"
      class="fixed inset-0 z-[90] flex items-center justify-center bg-black/40 p-4"
      @click.self="$emit('close')"
    >
      <div class="w-full max-w-lg rounded-2xl border border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900">
        <header class="flex items-center justify-between border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <div>
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">全局探测设置</h2>
            <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
              控制所有账号的自动探测节奏；单个账号可在健康详情里单独停/开。
            </p>
          </div>
          <button
            type="button"
            class="rounded-lg p-1.5 text-gray-400 hover:bg-gray-100 dark:hover:bg-dark-800"
            @click="$emit('close')"
          >
            <Icon name="x" size="sm" />
          </button>
        </header>

        <div class="space-y-4 px-5 py-4">
          <div v-if="!settings?.probe" class="rounded-lg bg-amber-50 px-3 py-2 text-sm text-amber-800 dark:bg-amber-950/40 dark:text-amber-200">
            探测设置未加载。请关闭后重试；若持续失败请确认运维监控已启用。
          </div>
          <template v-else>
            <label class="flex items-center justify-between gap-3 text-sm text-gray-700 dark:text-gray-200">
              <span>
                启用自动探测
                <span class="mt-0.5 block text-xs font-normal text-gray-400">关闭后调度器不再主动探测任何账号</span>
              </span>
              <button
                type="button"
                class="relative inline-flex h-7 w-12 items-center rounded-full transition"
                :class="draft.enabled ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-dark-600'"
                @click="draft.enabled = !draft.enabled"
              >
                <span
                  class="inline-block h-5 w-5 transform rounded-full bg-white shadow transition"
                  :class="draft.enabled ? 'translate-x-6' : 'translate-x-1'"
                />
              </button>
            </label>

            <div class="grid grid-cols-2 gap-3">
              <label class="flex flex-col gap-1 text-xs text-gray-500 dark:text-gray-400">
                探测间隔（分钟）
                <input
                  v-model.number="draft.interval_minutes"
                  type="number"
                  min="1"
                  max="1440"
                  class="input-field h-9 text-sm"
                />
              </label>
              <label class="flex flex-col gap-1 text-xs text-gray-500 dark:text-gray-400">
                每轮最多探测数
                <input
                  v-model.number="draft.max_per_run"
                  type="number"
                  min="1"
                  max="20"
                  class="input-field h-9 text-sm"
                />
              </label>
              <label class="flex flex-col gap-1 text-xs text-gray-500 dark:text-gray-400">
                超时（秒）
                <input
                  v-model.number="draft.timeout_seconds"
                  type="number"
                  min="1"
                  max="120"
                  class="input-field h-9 text-sm"
                />
              </label>
              <label class="flex flex-col gap-1 text-xs text-gray-500 dark:text-gray-400">
                模式
                <select v-model="draft.mode" class="input-field h-9 text-sm">
                  <option value="default">default</option>
                  <option value="compact">compact</option>
                </select>
              </label>
            </div>

            <label class="flex flex-col gap-1 text-xs text-gray-500 dark:text-gray-400">
              默认探测模型
              <input
                v-model="draft.model_id"
                type="text"
                class="input-field h-9 text-sm"
                placeholder="如 gpt-5.4-mini"
              />
            </label>

            <p class="rounded-lg bg-gray-50 px-3 py-2 text-[11px] leading-relaxed text-gray-500 dark:bg-dark-800 dark:text-gray-400">
              说明：自动探测约每分钟调度一轮，每轮最多探测「每轮数量」个账号，按<strong>最久未探测优先</strong>轮转。
              覆盖范围：已关闭账号、以及已打开但当前不可调度的账号。间隔是同一账号的冷却时间。
              列底部色条：绿=请求成功，蓝=探测成功，紫=探测失败。
            </p>
          </template>
        </div>

        <footer class="flex items-center justify-end gap-2 border-t border-gray-200 px-5 py-3 dark:border-dark-700">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="$emit('close')">取消</button>
          <button type="button" class="btn btn-primary" :disabled="!canSave" @click="save">
            {{ saving ? '保存中…' : '保存' }}
          </button>
        </footer>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
import type { OpsAccountHealthProbeSettings, OpsAccountHealthSettings } from '@/api/admin/ops'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
  settings?: OpsAccountHealthSettings | null
  saving?: boolean
}>()

const emit = defineEmits<{
  close: []
  save: [probe: OpsAccountHealthProbeSettings]
}>()

const draft = reactive({
  enabled: true,
  interval_minutes: 30,
  max_per_run: 2,
  timeout_seconds: 20,
  model_id: '',
  mode: 'default',
  prompt: ''
})

watch(
  () => [props.show, props.settings?.probe] as const,
  ([show, probe]) => {
    if (!show || !probe) return
    draft.enabled = probe.enabled !== false
    draft.interval_minutes = probe.interval_minutes || 30
    draft.max_per_run = probe.max_per_run || 2
    draft.timeout_seconds = probe.timeout_seconds || 20
    draft.model_id = probe.model_id || ''
    draft.mode = probe.mode || 'default'
    draft.prompt = probe.prompt || ''
  },
  { immediate: true }
)

const isValid = computed(() => {
  const interval = Number(draft.interval_minutes)
  const maxPerRun = Number(draft.max_per_run)
  const timeout = Number(draft.timeout_seconds)
  return (
    Number.isFinite(interval) &&
    interval >= 1 &&
    interval <= 1440 &&
    Number.isFinite(maxPerRun) &&
    maxPerRun >= 1 &&
    maxPerRun <= 20 &&
    Number.isFinite(timeout) &&
    timeout >= 1 &&
    timeout <= 120
  )
})

/** Never allow saving a synthetic default draft when server settings were not loaded. */
const canSave = computed(() => !!props.settings?.probe && !props.saving && isValid.value)

function save() {
  if (!canSave.value) return
  emit('save', {
    enabled: !!draft.enabled,
    interval_minutes: Math.round(Number(draft.interval_minutes)),
    max_per_run: Math.round(Number(draft.max_per_run)),
    timeout_seconds: Math.round(Number(draft.timeout_seconds)),
    model_id: draft.model_id.trim(),
    mode: draft.mode || 'default',
    prompt: draft.prompt || ''
  })
}
</script>
