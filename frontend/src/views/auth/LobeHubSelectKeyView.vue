<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.lobehub.selectKeyTitle') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.lobehub.selectKeyDescription') }}
        </p>
      </div>

      <div v-if="loading" class="flex justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>

      <div v-else-if="activeKeys.length === 0" class="rounded-xl border border-gray-200 bg-gray-50 p-4 text-center dark:border-dark-700 dark:bg-dark-800">
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ t('auth.lobehub.noKeys') }}
        </p>
        <router-link :to="createKeyRoute" class="btn btn-primary mt-4">
          {{ t('keys.createKey') }}
        </router-link>
      </div>

      <div v-else class="space-y-4">
        <div class="space-y-2">
          <label
            v-for="key in activeKeys"
            :key="key.id"
            class="flex cursor-pointer items-center justify-between rounded-xl border p-3 transition-colors hover:bg-gray-50 dark:hover:bg-dark-800"
            :class="selectedKeyId === key.id ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/20' : 'border-gray-200 dark:border-dark-700'"
          >
            <span>
              <span class="block text-sm font-medium text-gray-900 dark:text-white">{{ key.name }}</span>
              <span class="block text-xs text-gray-500 dark:text-dark-400">{{ maskKey(key.key) }}</span>
            </span>
            <input v-model="selectedKeyId" type="radio" :value="key.id" class="h-4 w-4 text-primary-600" />
          </label>
        </div>

        <button
          class="btn btn-primary w-full"
          :disabled="submitting || selectedKeyId === null"
          @click="continueWithSelectedKey"
        >
          <span v-if="submitting" class="mr-2 h-4 w-4 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
          {{ submitting ? t('auth.lobehub.continuing') : t('auth.lobehub.continueWithKey') }}
        </button>
      </div>

      <transition name="fade">
        <p v-if="errorMessage" class="text-center text-sm text-red-600 dark:text-red-400">
          {{ errorMessage }}
        </p>
      </transition>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import { keysAPI, lobehubAPI } from '@/api'
import { useAppStore, useAuthStore } from '@/stores'
import type { ApiKey } from '@/types'
import {
  buildLobeHubOIDCWebSessionPayload,
  filterActiveLobeHubKeys
} from '@/utils/lobehubFlow'

const route = useRoute()
const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const loading = ref(false)
const submitting = ref(false)
const errorMessage = ref('')
const keys = ref<ApiKey[]>([])
const selectedKeyId = ref<number | null>(null)

const activeKeys = computed(() => filterActiveLobeHubKeys(keys.value))
const createKeyRoute = computed(() => ({
  path: '/keys',
  query: {
    openCreate: '1',
    redirect: route.fullPath
  }
}))

function maskKey(key: string): string {
  if (key.length <= 12) return key
  return `${key.slice(0, 8)}...${key.slice(-4)}`
}

async function loadKeys() {
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await keysAPI.list(1, 100)
    keys.value = response.items
    if (activeKeys.value.length > 0) {
      const storedDefaultId = authStore.user?.default_chat_api_key_id ?? null
      selectedKeyId.value = activeKeys.value.find((key) => key.id === storedDefaultId)?.id ?? activeKeys.value[0].id
    }
  } catch {
    errorMessage.value = t('auth.lobehub.keysLoadFailed')
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

function persistDefaultKey(apiKeyId: number) {
  if (!authStore.user) return
  const user = {
    ...authStore.user,
    default_chat_api_key_id: apiKeyId
  }
  authStore.user = user
  localStorage.setItem('auth_user', JSON.stringify(user))
}

async function continueWithSelectedKey() {
  if (selectedKeyId.value === null) {
    errorMessage.value = t('auth.lobehub.keyRequired')
    return
  }

  submitting.value = true
  errorMessage.value = ''
  try {
    const response = await lobehubAPI.createOIDCWebSession(
      buildLobeHubOIDCWebSessionPayload(route.query, selectedKeyId.value)
    )
    persistDefaultKey(selectedKeyId.value)
    window.location.assign(response.continue_url)
  } catch {
    errorMessage.value = t('auth.lobehub.continueFailed')
    appStore.showError(errorMessage.value)
  } finally {
    submitting.value = false
  }
}

onMounted(() => {
  void loadKeys()
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
