<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-100 text-primary-600 dark:bg-primary-500/15 dark:text-primary-300">
          <Icon name="key" size="lg" />
        </div>
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.lobehub.selectKeyTitle') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.lobehub.selectKeyDescription') }}
        </p>
      </div>

      <div v-if="isLoading" class="rounded-2xl border border-primary-100 bg-primary-50/80 p-4 text-sm text-primary-800 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-200">
        {{ t('common.loading') }}
      </div>

      <div
        v-else-if="selectableKeys.length === 0"
        class="space-y-4 rounded-2xl border border-amber-200 bg-amber-50 p-4 dark:border-amber-400/20 dark:bg-amber-400/10"
      >
        <p class="text-sm text-amber-800 dark:text-amber-200">
          {{ t('auth.lobehub.noKeys') }}
        </p>
        <button type="button" class="btn btn-primary w-full" @click="openKeysPage">
          <Icon name="plus" size="md" class="mr-2" />
          {{ t('auth.lobehub.createKey') }}
        </button>
      </div>

      <div v-else class="space-y-3">
        <button
          v-for="key in selectableKeys"
          :key="key.id"
          :data-testid="`lobehub-key-option-${key.id}`"
          type="button"
          class="w-full rounded-2xl border p-4 text-left transition-colors"
          :class="selectedKeyID === key.id
            ? 'border-primary-500 bg-primary-50 dark:border-primary-400 dark:bg-primary-500/10'
            : 'border-gray-200 bg-white hover:border-primary-300 hover:bg-primary-50/50 dark:border-dark-700 dark:bg-dark-900 dark:hover:border-primary-500/60 dark:hover:bg-dark-800'"
          @click="selectedKeyID = key.id"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <p class="font-semibold text-gray-900 dark:text-white">
                {{ key.name }}
              </p>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">
                {{ maskKey(key.key) }}
              </p>
            </div>
            <div
              class="mt-0.5 h-5 w-5 rounded-full border-2 transition-colors"
              :class="selectedKeyID === key.id
                ? 'border-primary-500 bg-primary-500'
                : 'border-gray-300 dark:border-dark-500'"
            ></div>
          </div>
        </button>
      </div>

      <div
        v-if="errorMessage"
        class="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300"
      >
        {{ errorMessage }}
      </div>

      <button
        v-if="selectableKeys.length > 0"
        data-testid="lobehub-select-key-submit"
        type="button"
        class="btn btn-primary w-full"
        :disabled="isSubmitting || selectedKeyID === null"
        @click="submitSelection"
      >
        <Icon name="login" size="md" class="mr-2" />
        {{ isSubmitting ? t('common.processing') : t('auth.lobehub.continueWithKey') }}
      </button>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { keysAPI } from '@/api'
import { createLobeHubOIDCWebSession } from '@/api/lobehub'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore, useAuthStore } from '@/stores'
import type { ApiKey, User } from '@/types'

import { buildLobeHubContinuationPayload, filterSelectableLobeHubKeys } from './lobehubFlow'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const authStore = useAuthStore()

const isLoading = ref(true)
const isSubmitting = ref(false)
const errorMessage = ref('')
const keys = ref<ApiKey[]>([])
const selectedKeyID = ref<number | null>(null)

const selectableKeys = computed(() => filterSelectableLobeHubKeys(keys.value))

function maskKey(key: string): string {
  if (key.length <= 12) {
    return key
  }
  return `${key.slice(0, 8)}...${key.slice(-4)}`
}

async function loadKeys(): Promise<void> {
  isLoading.value = true
  errorMessage.value = ''

  try {
    const response = await keysAPI.list(1, 100)
    keys.value = response.items
    if (selectableKeys.value.length > 0) {
      selectedKeyID.value = selectableKeys.value[0].id
    }
  } catch (error) {
    errorMessage.value = t('auth.lobehub.keysLoadFailed')
    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}

function persistDefaultKeySelection(apiKeyID: number): void {
  if (!authStore.user) {
    return
  }

  const nextUser: User = {
    ...authStore.user,
    default_chat_api_key_id: apiKeyID
  }
  authStore.user = nextUser
  localStorage.setItem('auth_user', JSON.stringify(nextUser))
}

async function submitSelection(): Promise<void> {
  if (selectedKeyID.value === null) {
    errorMessage.value = t('auth.lobehub.keyRequired')
    return
  }

  isSubmitting.value = true
  errorMessage.value = ''

  try {
    const response = await createLobeHubOIDCWebSession(
      buildLobeHubContinuationPayload(route.query, selectedKeyID.value)
    )
    persistDefaultKeySelection(selectedKeyID.value)
    window.location.assign(response.continue_url)
  } catch (error) {
    errorMessage.value = t('auth.lobehub.continueFailed')
    appStore.showError(errorMessage.value)
  } finally {
    isSubmitting.value = false
  }
}

async function openKeysPage(): Promise<void> {
  await router.push({
    path: '/keys',
    query: {
      openCreate: '1',
      redirect: route.fullPath
    }
  })
}

onMounted(() => {
  loadKeys()
})
</script>
