<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { clearConnectivityCache, loadConnectivityCache, saveConnectivityCache } from '@/features/connectivity/cache'
import { parseConnectivityProbeConfig } from '@/features/connectivity/config'
import { runConnectivityTest } from '@/features/connectivity/runner'
import type {
  ConnectivityEndpointResult,
  ConnectivityGrade,
  ConnectivityProbeConfig,
  ConnectivityRunResult,
  ConnectivityTestEndpoint,
} from '@/features/connectivity/types'

interface FallbackEndpoint {
  name: string
  apiURL: string
  isDefault: boolean
}

const props = defineProps<{
  show: boolean
  fallbackEndpoints: FallbackEndpoint[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

type Phase = 'idle' | 'loading' | 'running' | 'complete' | 'incomplete' | 'cancelled' | 'rate_limited' | 'error'
type RowStatus = 'untested' | 'testing' | 'incomplete' | 'cancelled' | 'rate_limited' | 'graded'

interface DisplayRow {
  name: string
  apiURL: string
  isDefault: boolean
  status: RowStatus
  grade?: ConnectivityGrade
  clientIP?: string | null
  recommended: boolean
}

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const phase = ref<Phase>('idle')
const config = ref<ConnectivityProbeConfig | null>(null)
const runResult = ref<ConnectivityRunResult | null>(null)
const cachedGrades = ref(new Map<string, ConnectivityGrade>())
const cachedTestedAt = ref<number | null>(null)
let runController: AbortController | null = null
let removeRunListeners: (() => void) | null = null
let runGeneration = 0

const rows = computed<DisplayRow[]>(() => {
  if (runResult.value) {
    return runResult.value.endpoints.map(resultToRow)
  }
  if (config.value) {
    return config.value.endpoints.map((endpoint) => endpointToRow(endpoint, phase.value === 'running' ? 'testing' : 'untested'))
  }
  return props.fallbackEndpoints.map((endpoint) => {
    const grade = cachedGrades.value.get(connectivityCacheURLKey(endpoint.apiURL))
    return {
      name: endpoint.name,
      apiURL: endpoint.apiURL,
      isDefault: endpoint.isDefault,
      status: grade ? 'graded' : 'untested',
      grade,
      recommended: false,
    }
  })
})

const isBusy = computed(() => phase.value === 'loading' || phase.value === 'running')
const hasDisplayedResults = computed(() => runResult.value !== null || cachedGrades.value.size > 0)
const testedAt = computed(() => runResult.value?.testedAt ?? cachedTestedAt.value)
const phaseAnnouncement = computed(() => {
  if (phase.value === 'loading') return t('keys.connectivity.loadingConfig')
  if (phase.value === 'running') return t('keys.connectivity.testing')
  if (phase.value === 'complete') return t('keys.connectivity.completeAnnouncement')
  if (phase.value === 'rate_limited') return t('keys.connectivity.rateLimitedMessage')
  if (phase.value === 'cancelled' || phase.value === 'incomplete') {
    return t('keys.connectivity.incompleteMessage')
  }
  if (phase.value === 'error') return t('keys.connectivity.configUnavailable')
  return ''
})

const exitIPSummary = computed(() => {
  if (!config.value?.clientIPEnabled || !runResult.value) return null
  const results = runResult.value.endpoints
  if (
    results.length === 0
    || results.some((item) => item.status !== 'graded' || !item.clientIP)
  ) return { mode: 'unknown' as const }
  const addresses = results.map((item) => item.clientIP as string)
  const unique = [...new Set(addresses)]
  if (unique.length === 1) {
    return { mode: 'common' as const, address: unique[0] }
  }
  return { mode: 'split' as const }
})

watch(() => props.show, (show) => {
  if (show) {
    restoreCachedGrades()
  } else {
    cancelRun()
  }
}, { immediate: true })

async function startTest() {
  cancelRun()
  const generation = ++runGeneration
  clearConnectivityCache()
  cachedGrades.value = new Map()
  cachedTestedAt.value = null
  runResult.value = null
  config.value = null
  phase.value = 'loading'

  const settings = await appStore.fetchPublicSettings(true)
  if (!props.show || generation !== runGeneration) return
  const nextConfig = settings ? parseConnectivityProbeConfig(settings) : null
  if (!nextConfig) {
    phase.value = 'error'
    return
  }
  if (document.visibilityState === 'hidden') {
    phase.value = 'cancelled'
    return
  }

  config.value = nextConfig
  phase.value = 'running'
  const controller = new AbortController()
  runController = controller
  const cleanupListeners = installRunCancellation(controller)
  removeRunListeners = cleanupListeners
  try {
    const result = await runConnectivityTest(nextConfig, controller.signal)
    if (generation !== runGeneration) return
    runResult.value = result
    phase.value = result.status
    if (result.status === 'complete') saveConnectivityCache(result)
  } catch {
    if (generation === runGeneration) phase.value = controller.signal.aborted ? 'cancelled' : 'error'
  } finally {
    cleanupListeners()
    if (generation === runGeneration) {
      removeRunListeners = null
      runController = null
    }
  }
}

function cancelRun(options: { preserveActiveResult?: boolean } = {}) {
  const preserveResult = options.preserveActiveResult && phase.value === 'running' && runController !== null
  if (!preserveResult) runGeneration++
  runController?.abort()
  runController = null
  removeRunListeners?.()
  removeRunListeners = null
  if (phase.value === 'loading' || phase.value === 'running') phase.value = 'cancelled'
}

function closeDialog() {
  cancelRun()
  emit('close')
}

function handlePrimaryAction() {
  if (isBusy.value) {
    cancelRun({ preserveActiveResult: true })
    return
  }
  void startTest()
}

function installRunCancellation(controller: AbortController): () => void {
  const cancel = () => controller.abort()
  const onVisibility = () => {
    if (document.visibilityState === 'hidden') cancel()
  }
  document.addEventListener('visibilitychange', onVisibility)
  window.addEventListener('offline', cancel)
  window.addEventListener('online', cancel)
  const connection = (navigator as Navigator & { connection?: EventTarget }).connection
  connection?.addEventListener('change', cancel)
  return () => {
    document.removeEventListener('visibilitychange', onVisibility)
    window.removeEventListener('offline', cancel)
    window.removeEventListener('online', cancel)
    connection?.removeEventListener('change', cancel)
  }
}

function restoreCachedGrades() {
  const settings = appStore.cachedPublicSettings
  const thresholds = settings?.connectivity_grade_thresholds
  if (!settings?.connectivity_test_enabled || !thresholds?.grading_version) return
  const cached = loadConnectivityCache(thresholds.grading_version)
  cachedGrades.value = new Map(cached.map((item) => [connectivityCacheURLKey(item.url), item.grade]))
  cachedTestedAt.value = cached[0]?.tested_at ?? null
}

function connectivityCacheURLKey(rawURL: string): string {
  try {
    const url = new URL(rawURL)
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) return rawURL
    return `${url.origin}${url.pathname.replace(/\/+$/, '')}`
  } catch {
    return rawURL
  }
}

async function copyURL(url: string) {
  await copyToClipboard(url, t('keys.endpoints.copied'))
}

function endpointToRow(endpoint: ConnectivityTestEndpoint, status: RowStatus): DisplayRow {
  return {
    name: connectivityEndpointName(endpoint),
    apiURL: endpoint.api_url,
    isDefault: endpoint.is_default,
    status,
    recommended: false,
  }
}

function resultToRow(result: ConnectivityEndpointResult): DisplayRow {
  return {
    name: connectivityEndpointName(result.endpoint),
    apiURL: result.endpoint.api_url,
    isDefault: result.endpoint.is_default,
    status: result.status,
    grade: result.grade,
    clientIP: result.clientIP,
    recommended: runResult.value?.recommendedAPIURL === result.endpoint.api_url,
  }
}

function connectivityEndpointName(endpoint: ConnectivityTestEndpoint): string {
  return endpoint.is_default ? t('keys.endpoints.title') : endpoint.name
}

function statusLabel(row: DisplayRow): string {
  if (row.status === 'graded' && row.grade) return t(`keys.connectivity.grades.${row.grade}`)
  if (row.status === 'testing') return t('keys.connectivity.testing')
  if (row.status === 'rate_limited') return t('keys.connectivity.rateLimited')
  if (row.status === 'cancelled' || row.status === 'incomplete') return t('keys.connectivity.incomplete')
  return t('keys.connectivity.notTested')
}

function statusIcon(row: DisplayRow): 'checkCircle' | 'infoCircle' | 'exclamationTriangle' | 'xCircle' | 'clock' {
  if (row.status === 'graded') {
    if (row.grade === 'excellent' || row.grade === 'good') return 'checkCircle'
    if (row.grade === 'fair') return 'exclamationTriangle'
    return 'xCircle'
  }
  if (row.status === 'testing') return 'clock'
  return 'infoCircle'
}

function statusClass(row: DisplayRow): string {
  if (row.grade === 'excellent') return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300'
  if (row.grade === 'good') return 'bg-sky-50 text-sky-700 dark:bg-sky-950/40 dark:text-sky-300'
  if (row.grade === 'fair') return 'bg-amber-50 text-amber-800 dark:bg-amber-950/40 dark:text-amber-300'
  if (row.grade === 'not_recommended') return 'bg-red-50 text-red-700 dark:bg-red-950/40 dark:text-red-300'
  return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300'
}

function resultMessage(row: DisplayRow): string | null {
  if (!row.grade) return null
  return t(`keys.connectivity.messages.${row.grade}`)
}

function formatTestedAt(value: number): string {
  return new Intl.DateTimeFormat(undefined, { hour: '2-digit', minute: '2-digit' }).format(new Date(value))
}

onBeforeUnmount(cancelRun)
</script>

<template>
  <BaseDialog
    :show="show"
    :title="t('keys.connectivity.title')"
    width="wide"
    :close-on-escape="!isBusy"
    @close="closeDialog"
  >
    <div class="space-y-4">
      <p class="text-sm leading-6 text-gray-600 dark:text-dark-300">
        {{ t('keys.connectivity.disclaimer') }}
      </p>
      <p
        data-testid="connectivity-status-announcement"
        class="sr-only"
        role="status"
        aria-live="polite"
        aria-atomic="true"
      >
        {{ phaseAnnouncement }}
      </p>

      <div
        v-if="exitIPSummary"
        data-testid="connectivity-exit-summary"
        class="border-l-2 border-sky-400 bg-sky-50 px-3 py-2.5 text-sm text-sky-900 dark:border-sky-500 dark:bg-sky-950/30 dark:text-sky-200"
      >
        <template v-if="exitIPSummary.mode === 'common'">
          {{ t('keys.connectivity.exitIP') }}
          <code class="ml-1 font-mono">{{ exitIPSummary.address }}</code>
        </template>
        <template v-else-if="exitIPSummary.mode === 'split'">
          {{ t('keys.connectivity.splitExit') }}
        </template>
        <template v-else>{{ t('keys.connectivity.unknownExit') }}</template>
        <p class="mt-1 text-xs leading-5 text-sky-700 dark:text-sky-300">
          {{ t('keys.connectivity.exitIPHint') }}
        </p>
      </div>

      <div
        v-if="phase === 'loading'"
        data-testid="connectivity-loading"
        class="flex items-center gap-2 border-l-2 border-sky-400 bg-sky-50 px-3 py-2.5 text-sm text-sky-800 dark:border-sky-500 dark:bg-sky-950/30 dark:text-sky-200"
      >
        <Icon name="clock" size="sm" class="animate-pulse motion-reduce:animate-none" aria-hidden="true" />
        {{ t('keys.connectivity.loadingConfig') }}
      </div>
      <div v-else-if="phase === 'error'" class="border-l-2 border-red-500 bg-red-50 px-3 py-2.5 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
        {{ t('keys.connectivity.configUnavailable') }}
      </div>
      <div v-else-if="phase === 'rate_limited'" class="border-l-2 border-amber-500 bg-amber-50 px-3 py-2.5 text-sm text-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
        {{ t('keys.connectivity.rateLimitedMessage') }}
      </div>
      <div v-else-if="phase === 'cancelled' || phase === 'incomplete'" class="border-l-2 border-gray-400 bg-gray-50 px-3 py-2.5 text-sm text-gray-700 dark:bg-dark-800 dark:text-dark-300">
        {{ t('keys.connectivity.incompleteMessage') }}
      </div>

      <div
        class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700"
        :aria-busy="isBusy"
      >
        <div v-for="row in rows" :key="row.apiURL" class="grid gap-2 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-sm font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <span v-if="row.isDefault" class="text-xs text-primary-600 dark:text-primary-400">{{ t('keys.endpoints.default') }}</span>
              <span v-if="row.recommended" class="inline-flex items-center gap-1 text-xs font-semibold text-emerald-700 dark:text-emerald-300">
                <Icon name="checkCircle" size="xs" aria-hidden="true" />
                {{ t('keys.connectivity.recommended') }}
              </span>
            </div>
            <div class="mt-1 flex min-w-0 items-center gap-1.5">
              <code class="min-w-0 break-all font-mono text-xs text-gray-500 dark:text-dark-400">{{ row.apiURL }}</code>
              <button
                type="button"
                data-testid="connectivity-copy-url"
                class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-primary-600 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 focus-visible:ring-offset-2 dark:hover:bg-dark-700 dark:hover:text-primary-400 dark:focus-visible:ring-offset-dark-900"
                :title="t('keys.endpoints.clickToCopy')"
                :aria-label="t('keys.endpoints.clickToCopy')"
                @click="copyURL(row.apiURL)"
              >
                <Icon name="copy" size="xs" aria-hidden="true" />
              </button>
            </div>
            <p v-if="resultMessage(row)" class="mt-1.5 text-xs leading-5 text-gray-600 dark:text-dark-300">{{ resultMessage(row) }}</p>
            <p v-if="config?.clientIPEnabled && exitIPSummary?.mode !== 'common' && row.status === 'graded'" class="mt-1 font-mono text-[11px] text-gray-500 dark:text-dark-400">
              {{ row.clientIP || t('keys.connectivity.unknownExit') }}
            </p>
          </div>
          <span class="inline-flex h-7 w-fit items-center gap-1.5 rounded px-2 text-xs font-semibold" :class="statusClass(row)">
            <Icon
              :name="statusIcon(row)"
              size="xs"
              :class="row.status === 'testing' ? 'animate-pulse motion-reduce:animate-none' : ''"
              aria-hidden="true"
            />
            {{ statusLabel(row) }}
          </span>
        </div>
      </div>

      <p v-if="testedAt" class="text-right text-xs text-gray-500 dark:text-dark-400">
        {{ t('keys.connectivity.testedAt', { time: formatTestedAt(testedAt) }) }}
      </p>
    </div>

    <template #footer>
      <button type="button" class="btn btn-secondary" @click="closeDialog">{{ t('common.close') }}</button>
      <button
        type="button"
        class="btn"
        :class="isBusy ? 'btn-secondary' : 'btn-primary'"
        data-test="start-connectivity-test"
        @click="handlePrimaryAction"
      >
        <Icon
          :name="isBusy ? 'x' : hasDisplayedResults ? 'refresh' : 'bolt'"
          size="sm"
          aria-hidden="true"
        />
        {{ isBusy ? t('common.cancel') : hasDisplayedResults ? t('keys.connectivity.retry') : t('keys.connectivity.start') }}
      </button>
    </template>
  </BaseDialog>
</template>
