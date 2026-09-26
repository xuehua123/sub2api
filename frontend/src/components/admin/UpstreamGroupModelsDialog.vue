<template>
  <BaseDialog :show="!!group" :title="group?.name || ''" width="wide" @close="close">
    <template v-if="group">
      <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
        <a v-if="website" :href="website" target="_blank" rel="noopener noreferrer" class="inline-flex min-w-0 items-center gap-2 text-sm text-primary-600" :title="t('upstreamWorkspace.website')">
          <span class="break-all">{{ group.connection_name }}</span><Icon name="externalLink" size="sm" class="shrink-0" />
        </a>
        <div class="flex items-center gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="loading || syncing" @click="refresh">
            <Icon name="refresh" size="sm" :class="syncing ? 'animate-spin' : ''" />{{ t('upstreamWorkspace.refreshModels') }}
          </button>
          <button class="btn btn-secondary btn-sm" :disabled="!filtered.length" :title="t('upstreamWorkspace.copyModels')" :aria-label="t('upstreamWorkspace.copyModels')" @click="copyModels">
            <Icon :name="copied ? 'check' : 'copy'" size="sm" />
          </button>
        </div>
      </div>
      <div v-if="snapshot" class="mb-4 flex flex-wrap items-center gap-x-4 gap-y-1 border-y border-gray-200 py-3 text-xs text-gray-500 dark:border-dark-700">
        <span>{{ t('upstreamWorkspace.modelCoverage.' + snapshot.coverage) }}</span>
        <span>{{ t('upstreamWorkspace.modelStatus.' + snapshot.status) }}</span>
        <span v-if="snapshot.observed_at">{{ formatDateTime(snapshot.observed_at) }}</span>
      </div>
      <p v-if="error" role="alert" class="mb-3 text-sm text-red-600">{{ error }}</p>
      <button v-if="pollStopped" class="btn btn-secondary btn-sm mb-3" @click="retryProgress">
        <Icon name="refresh" size="sm" />{{ t('upstreamWorkspace.retryModelProgress') }}
      </button>
      <p v-if="snapshot?.error_code" role="status" class="mb-3 text-sm text-amber-700 dark:text-amber-400">{{ t('upstreamWorkspace.modelErrors.' + snapshot.error_code) }}</p>
      <p v-if="snapshot?.coverage === 'bound_keys' && snapshot.source_count" class="mb-3 text-xs text-gray-500" role="status">
        {{ t('upstreamWorkspace.modelSourceProgress', { ready: snapshot.ready_source_count || 0, total: snapshot.source_count, pending: snapshot.pending_source_count || 0, failed: snapshot.failed_source_count || 0, stale: snapshot.stale_source_count || 0 }) }}
      </p>
      <input v-model="search" class="input mb-3" :placeholder="t('upstreamWorkspace.searchModels')" :aria-label="t('upstreamWorkspace.searchModels')" />
      <div v-if="loading" class="py-10 text-center text-gray-500">{{ t('upstreamWorkspace.busy') }}</div>
      <template v-else-if="snapshot">
        <p class="mb-2 text-xs text-gray-500">{{ t('upstreamWorkspace.modelCount', { count: filtered.length }) }}</p>
        <ul class="max-h-80 overflow-y-auto divide-y divide-gray-100 dark:divide-dark-700">
          <li v-for="model in visibleModels" :key="model" class="flex items-start justify-between gap-3 py-2.5">
            <code class="min-w-0 break-all text-sm text-gray-800 dark:text-gray-200">{{ model }}</code>
            <button class="shrink-0 p-1 text-gray-400 hover:text-primary-600" :title="t('upstreamWorkspace.copyModel')" :aria-label="t('upstreamWorkspace.copyModel') + ' ' + model" @click="copyModels(model)"><Icon name="copy" size="sm" /></button>
          </li>
        </ul>
        <p v-if="!filtered.length" class="py-8 text-center text-sm text-gray-500">{{ t(snapshot.observed_at ? 'upstreamWorkspace.noMatchingModels' : 'upstreamWorkspace.modelsNotFetched') }}</p>
        <Pagination v-if="filtered.length > 50" :page="page" :page-size="50" :total="filtered.length" :show-page-size-selector="false" @update:page="page = $event" />
      </template>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import Icon from '@/components/icons/Icon.vue'
import { getGroupModels, syncGroupModels, type CatalogGroup, type ModelSnapshot } from '@/api/admin/upstreamCatalog'
import { formatDateTime } from '@/utils/format'
import { upstreamWebsite } from '@/utils/upstreamCatalog'
const props = defineProps<{ group: CatalogGroup | null }>()
const emit = defineEmits<{ close: []; updated: [] }>()
const { t } = useI18n()
const snapshot = ref<ModelSnapshot | null>(null)
const search = ref(''), page = ref(1), loading = ref(false), syncing = ref(false), error = ref(''), copied = ref(false)
const website = computed(() => upstreamWebsite(props.group?.management_base_url || ''))
const filtered = computed(() => (snapshot.value?.models || []).filter(model => model.toLowerCase().includes(search.value.trim().toLowerCase())))
const visibleModels = computed(() => filtered.value.slice((page.value - 1) * 50, page.value * 50))
const pollStopped = ref(false)
let controller: AbortController | undefined, generation = 0
let pollController: AbortController | undefined
let pollTimer: ReturnType<typeof setTimeout> | undefined
let pollFailures = 0
function running(data: ModelSnapshot | null): boolean {
  return data?.refresh_in_progress ?? (data?.status === 'pending' || data?.status === 'syncing')
}
function stopPolling() {
  clearTimeout(pollTimer)
  pollTimer = undefined
  pollController?.abort()
  pollController = undefined
}
function schedulePoll() {
  clearTimeout(pollTimer)
  pollTimer = undefined
  if (!props.group || !running(snapshot.value) || loading.value || syncing.value || pollStopped.value || document.hidden) return
  pollTimer = setTimeout(() => void pollProgress(), Math.min(5000 * 2 ** pollFailures, 30000))
}
function acceptSnapshot(data: ModelSnapshot) {
  snapshot.value = data
  page.value = Math.min(page.value, Math.max(1, Math.ceil(filtered.value.length / 50)))
  copied.value = false
}
async function pollProgress() {
  clearTimeout(pollTimer)
  pollTimer = undefined
  if (!props.group || pollController || loading.value || syncing.value || document.hidden) return
  const current = generation
  const request = new AbortController()
  pollController = request
  try {
    const data = await getGroupModels(props.group, request.signal)
    if (current !== generation || request.signal.aborted) return
    const wasRunning = running(snapshot.value)
    acceptSnapshot(data)
    pollFailures = 0
    error.value = ''
    pollStopped.value = false
    if (wasRunning && !running(data)) emit('updated')
  } catch (cause) {
    if (current !== generation || request.signal.aborted) return
    pollFailures++
    const status = (cause as { response?: { status?: number } })?.response?.status
    if (pollFailures >= 3 || [401, 403, 404].includes(status || 0)) {
      pollStopped.value = true
      error.value = t('upstreamWorkspace.modelsPollError')
    }
  } finally {
    if (pollController === request) pollController = undefined
    if (current === generation && !request.signal.aborted) schedulePoll()
  }
}
function retryProgress() {
  pollFailures = 0
  pollStopped.value = false
  error.value = ''
  void pollProgress()
}
function visibilityChanged() {
  if (document.hidden) stopPolling()
  else schedulePoll()
}
function close() {
  generation++
  controller?.abort()
  stopPolling()
  emit('close')
}
watch(search, () => { page.value = 1; copied.value = false })
watch(() => props.group, async group => {
  controller?.abort()
  stopPolling()
  const current = ++generation
  pollFailures = 0; pollStopped.value = false
  snapshot.value = null; search.value = ''; page.value = 1; error.value = ''; copied.value = false; syncing.value = false; loading.value = false
  if (!group) return
  controller = new AbortController(); loading.value = true
  try {
    const data = await getGroupModels(group, controller.signal)
    if (current === generation) acceptSnapshot(data)
  } catch {
    if (current === generation) error.value = t('upstreamWorkspace.modelsLoadError')
  } finally { if (current === generation) { loading.value = false; schedulePoll() } }
}, { immediate: true })
async function refresh() {
  if (!props.group || syncing.value) return
  stopPolling()
  controller?.abort(); controller = new AbortController()
  const current = ++generation
  syncing.value = true; error.value = ''; copied.value = false; pollFailures = 0; pollStopped.value = false
  try {
    const data = await syncGroupModels(props.group, controller.signal)
    if (current === generation) { acceptSnapshot(data); page.value = 1; emit('updated') }
  } catch {
    if (current === generation) error.value = t('upstreamWorkspace.modelsSyncError')
  } finally { if (current === generation) { syncing.value = false; schedulePoll() } }
}
async function copyModels(model?: string | MouseEvent) {
  try { await navigator.clipboard.writeText(typeof model === 'string' ? model : filtered.value.join('\n')); copied.value = true }
  catch { error.value = t('upstreamWorkspace.copyError') }
}
onMounted(() => document.addEventListener('visibilitychange', visibilityChanged))
onBeforeUnmount(() => {
  generation++; controller?.abort(); stopPolling()
  document.removeEventListener('visibilitychange', visibilityChanged)
})
</script>
