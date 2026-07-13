<template>
  <div class="space-y-3">
    <div class="relative">
      <label class="input-label">{{ t('admin.announcements.form.targetUsers') }}</label>
      <input
        v-model="query"
        type="search"
        autocomplete="off"
        class="input"
        :placeholder="t('admin.announcements.form.searchTargetUsers')"
        @input="searchUsers"
        @focus="showResults = results.length > 0"
      />
      <div
        v-if="showResults"
        class="absolute z-20 mt-1 max-h-64 w-full overflow-y-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
      >
        <button
          v-for="user in results"
          :key="user.id"
          type="button"
          class="flex w-full items-center justify-between gap-3 px-3 py-2 text-left hover:bg-gray-50 dark:hover:bg-dark-700"
          :disabled="selectedIDs.has(user.id)"
          @click="addUser(user)"
        >
          <span class="min-w-0">
            <span class="block truncate text-sm text-gray-900 dark:text-white">{{ user.email }}</span>
            <span class="block truncate text-xs text-gray-500 dark:text-dark-400">{{ user.username || `#${user.id}` }}</span>
          </span>
          <Icon v-if="selectedIDs.has(user.id)" name="check" size="sm" class="shrink-0 text-emerald-500" />
        </button>
      </div>
    </div>

    <div v-if="modelValue.length > 0" class="divide-y divide-gray-200 border-y border-gray-200 dark:divide-dark-700 dark:border-dark-700">
      <div v-for="userID in modelValue" :key="userID" class="flex items-center justify-between gap-3 py-2">
        <div class="min-w-0">
          <div class="truncate text-sm text-gray-900 dark:text-white">{{ userLabels.get(userID)?.email || `#${userID}` }}</div>
          <div v-if="userLabels.get(userID)?.username" class="truncate text-xs text-gray-500 dark:text-dark-400">
            {{ userLabels.get(userID)?.username }} / #{{ userID }}
          </div>
        </div>
        <button
          type="button"
          class="btn btn-secondary shrink-0"
          :title="t('common.delete')"
          @click="removeUser(userID)"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>
    </div>

    <p v-else class="text-sm text-amber-600 dark:text-amber-400">
      {{ t('admin.announcements.form.selectTargetUsers') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminUser } from '@/types'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  modelValue: number[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: number[]): void
}>()

const { t } = useI18n()
const query = ref('')
const results = ref<AdminUser[]>([])
const showResults = ref(false)
const userLabels = ref(new Map<number, AdminUser>())
const selectedIDs = computed(() => new Set(props.modelValue))
let searchTimer: ReturnType<typeof setTimeout> | null = null
let searchController: AbortController | null = null

function searchUsers() {
  if (searchTimer) clearTimeout(searchTimer)
  searchController?.abort()
  const keyword = query.value.trim()
  if (!keyword) {
    results.value = []
    showResults.value = false
    return
  }
  searchTimer = setTimeout(async () => {
    const controller = new AbortController()
    searchController = controller
    try {
      const response = await adminAPI.users.list(1, 10, { search: keyword }, { signal: controller.signal })
      if (controller.signal.aborted) return
      results.value = response.items
      showResults.value = true
    } catch {
      if (!controller.signal.aborted) results.value = []
    }
  }, 300)
}

function addUser(user: AdminUser) {
  userLabels.value.set(user.id, user)
  if (!selectedIDs.value.has(user.id)) {
    emit('update:modelValue', [...props.modelValue, user.id].sort((a, b) => a - b))
  }
  query.value = ''
  results.value = []
  showResults.value = false
}

function removeUser(userID: number) {
  emit('update:modelValue', props.modelValue.filter((id) => id !== userID))
}

watch(
  () => props.modelValue,
  async (userIDs) => {
    const missing = userIDs.filter((id) => !userLabels.value.has(id)).slice(0, 50)
    await Promise.all(missing.map(async (id) => {
      try {
        const user = await adminAPI.users.getById(id)
        userLabels.value.set(id, user)
      } catch {
        // Keep the stable user ID visible when an account was deleted.
      }
    }))
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer)
  searchController?.abort()
})
</script>
