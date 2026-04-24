<template>
  <div class="relative space-y-3">
    <label class="block text-sm font-medium text-gray-700 dark:text-gray-300">
      {{ label }}
    </label>
    <input
      :value="query"
      :data-test="inputTestId"
      class="input"
      :placeholder="placeholder"
      @input="handleInput"
    />

    <div
      v-if="modelValue"
      class="rounded-xl bg-gray-50 px-4 py-3 text-sm text-gray-600 dark:bg-dark-800 dark:text-gray-300"
    >
      <div class="font-medium text-gray-900 dark:text-white">
        {{ modelValue.email }}
      </div>
      <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ modelValue.username || '-' }}
        <span v-if="showReferralCode" class="ml-2">{{ modelValue.referral_code }}</span>
      </div>
      <button
        type="button"
        class="mt-3 btn btn-secondary btn-sm"
        @click="clearSelection"
      >
        {{ t('common.clear', '清空') }}
      </button>
    </div>

    <div
      v-if="loading || options.length > 0"
      class="absolute left-0 right-0 top-full z-30 mt-2 max-h-72 overflow-y-auto rounded-xl border border-gray-200 bg-white p-2 shadow-xl dark:border-dark-700 dark:bg-dark-900"
    >
      <div v-if="loading" class="px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('common.loading', '加载中') }}
      </div>
      <template v-else>
        <button
          v-for="option in options"
          :key="option.user_id"
          type="button"
          data-test="account-option"
          class="flex w-full items-center justify-between rounded-lg px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-800"
          @click="selectOption(option)"
        >
          <span>{{ option.email }}</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ showReferralCode ? option.referral_code : option.username || '-' }}
          </span>
        </button>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { watch, ref, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminReferralAccountOption } from '@/types'

const { t } = useI18n()

interface Props {
  label: string
  placeholder: string
  query: string
  modelValue: AdminReferralAccountOption | null
  options: AdminReferralAccountOption[]
  loading?: boolean
  inputTestId: string
  showReferralCode?: boolean
}

const props = withDefaults(defineProps<Props>(), {
  loading: false,
  showReferralCode: false
})

const emit = defineEmits<{
  (e: 'update:query', value: string): void
  (e: 'search', value: string): void
  (e: 'select', value: AdminReferralAccountOption): void
  (e: 'clear'): void
}>()

const searchDebounceTimer = ref<ReturnType<typeof setTimeout> | null>(null)

function handleInput(event: Event) {
  const value = (event.target as HTMLInputElement).value
  emit('update:query', value)
}

function selectOption(option: AdminReferralAccountOption) {
  emit('select', option)
  emit('update:query', option.email)
}

function clearSelection() {
  emit('clear')
  emit('update:query', '')
}

watch(
  () => props.query,
  (value) => {
    if (searchDebounceTimer.value) {
      clearTimeout(searchDebounceTimer.value)
      searchDebounceTimer.value = null
    }
    if (!value.trim()) {
      emit('clear')
      return
    }
    searchDebounceTimer.value = setTimeout(() => {
      emit('search', value.trim())
    }, 300)
  }
)

onUnmounted(() => {
  if (searchDebounceTimer.value) {
    clearTimeout(searchDebounceTimer.value)
  }
})
</script>
