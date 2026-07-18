<template>
  <section class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <label class="input-label mb-0">{{ t('admin.upstreamConnections.binding.title') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.upstreamConnections.binding.description') }}
        </p>
      </div>
      <RouterLink
        to="/admin/upstream-connections"
        class="text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
      >
        {{ t('admin.upstreamConnections.binding.manage') }}
      </RouterLink>
    </div>

    <div class="mt-3 max-w-2xl">
      <select
        :value="selectedConnectionId ?? ''"
        class="input"
        :disabled="loading || disabled"
        data-testid="upstream-connection-binding-select"
        @change="onSelectionChange"
      >
        <option value="">{{ t('admin.upstreamConnections.binding.none') }}</option>
        <option v-for="connection in connections" :key="connection.id" :value="connection.id">
          {{ connection.name }} · {{ providerLabel(connection.provider) }} · {{ statusLabel(connection.status) }}
        </option>
      </select>
      <p v-if="loading" class="input-hint">{{ t('common.loading') }}</p>
      <p v-else-if="connections.length === 0" class="input-hint">
        {{ t('admin.upstreamConnections.binding.empty') }}
      </p>
      <p v-else class="input-hint">{{ t('admin.upstreamConnections.binding.observeOnly') }}</p>

      <div
        v-if="binding"
        class="mt-3 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-600 dark:text-gray-300"
      >
        <span class="badge" :class="binding.status === 'ready' ? 'badge-success' : 'badge-warning'">
          {{ statusLabel(binding.status) }}
        </span>
        <span v-if="binding.remote_group_name">
          {{ t('admin.upstreamConnections.binding.group') }}: {{ binding.remote_group_name }}
        </span>
        <span v-if="binding.observed_multiplier !== null">
          {{ t('admin.upstreamConnections.binding.observedMultiplier') }}: {{ binding.observed_multiplier }}x
        </span>
        <span v-if="binding.last_error" class="text-amber-700 dark:text-amber-300">{{ binding.last_error }}</span>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { UpstreamAccountBinding, UpstreamConnection } from '@/api/admin/upstreamConnections'

const props = withDefaults(defineProps<{
  modelValue: number | null
  accountId?: number | null
  show?: boolean
  disabled?: boolean
}>(), {
  accountId: null,
  show: true,
  disabled: false
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
  'loading-change': [value: boolean]
}>()

const { t } = useI18n()
const appStore = useAppStore()
const connections = ref<UpstreamConnection[]>([])
const binding = ref<UpstreamAccountBinding | null>(null)
const selectedConnectionId = ref<number | null>(props.modelValue ?? null)
const originalConnectionId = ref<number | null>(null)
const originalAccountId = ref<number | null>(null)
const loading = ref(false)
const ready = ref(false)
let loadGeneration = 0
let activeLoad: Promise<void> | null = null

function isNotFound(error: unknown): boolean {
  const candidate = error as { status?: number; response?: { status?: number } }
  return candidate?.status === 404 || candidate?.response?.status === 404
}

async function load(): Promise<void> {
  if (!props.show) return
  const generation = ++loadGeneration
  loading.value = true
  ready.value = false
  binding.value = null
  selectedConnectionId.value = null
  originalConnectionId.value = null
  originalAccountId.value = props.accountId ?? null
  emit('loading-change', true)
  emit('update:modelValue', null)
  try {
    const items = await adminAPI.upstreamConnections.listAll()
    if (generation !== loadGeneration) return
    connections.value = items
    if (props.accountId) {
      try {
        const current = await adminAPI.upstreamConnections.getAccountBinding(props.accountId)
        if (generation !== loadGeneration) return
        binding.value = current
        selectedConnectionId.value = current.connection_id
        originalConnectionId.value = current.connection_id
        emit('update:modelValue', current.connection_id)
      } catch (error: unknown) {
        if (generation !== loadGeneration) return
        if (!isNotFound(error)) throw error
        selectedConnectionId.value = null
        emit('update:modelValue', null)
      }
    } else {
      selectedConnectionId.value = null
      emit('update:modelValue', null)
    }
    ready.value = true
  } finally {
    if (generation === loadGeneration) {
      loading.value = false
      emit('loading-change', false)
    }
  }
}

function startLoad(): void {
  const pending = load()
  activeLoad = pending
  void pending
    .catch(() => appStore.showError(t('admin.upstreamConnections.binding.loadFailed')))
    .finally(() => {
      if (activeLoad === pending) activeLoad = null
    })
}

function onSelectionChange(event: Event): void {
  const raw = (event.target as HTMLSelectElement).value
  selectedConnectionId.value = raw ? Number(raw) : null
  emit('update:modelValue', selectedConnectionId.value)
}

function providerLabel(provider: string): string {
  return t(`admin.upstreamConnections.providers.${provider}`, provider)
}

function statusLabel(status: string): string {
  return t(`admin.upstreamConnections.statuses.${status}`, status)
}

async function apply(accountId: number): Promise<UpstreamAccountBinding | null> {
  if (activeLoad) await activeLoad
  if (!ready.value) throw new Error('upstream connection binding state is not ready')
  if (props.accountId != null && (accountId !== props.accountId || originalAccountId.value !== props.accountId)) {
    throw new Error('upstream connection binding account changed')
  }
  const selected = selectedConnectionId.value
  if (accountId === originalAccountId.value && selected === originalConnectionId.value) return binding.value
  if (selected) {
    const next = await adminAPI.upstreamConnections.bindAccount(selected, accountId)
    binding.value = next
    selectedConnectionId.value = selected
    emit('update:modelValue', selected)
    originalConnectionId.value = selected
    originalAccountId.value = accountId
    return next
  }
  if (originalConnectionId.value) {
    await adminAPI.upstreamConnections.unbindAccount(originalConnectionId.value, accountId)
  }
  binding.value = null
  selectedConnectionId.value = null
  emit('update:modelValue', null)
  originalConnectionId.value = null
  originalAccountId.value = accountId
  return null
}

watch(
  () => [props.show, props.accountId] as const,
  ([show]) => {
    if (show) {
      startLoad()
    } else {
      loadGeneration++
      activeLoad = null
      loading.value = false
      ready.value = false
      emit('loading-change', false)
    }
  },
  { immediate: true }
)

watch(
  () => props.modelValue,
  (value) => {
    if (!loading.value) selectedConnectionId.value = value ?? null
  }
)

defineExpose({ apply, load: startLoad })
</script>
