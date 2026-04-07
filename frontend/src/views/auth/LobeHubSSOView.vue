<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary-100 text-primary-600 dark:bg-primary-500/15 dark:text-primary-300">
          <Icon name="refresh" size="lg" :class="isLoading ? 'animate-spin' : ''" />
        </div>
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.lobehub.title') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ isLoading ? t('auth.lobehub.continuing') : t('auth.lobehub.retryHint') }}
        </p>
      </div>

      <div
        class="rounded-2xl border border-primary-100 bg-primary-50/80 p-4 text-sm text-primary-800 dark:border-primary-500/20 dark:bg-primary-500/10 dark:text-primary-200"
      >
        {{ t('auth.lobehub.ssoDescription') }}
      </div>

      <div
        v-if="errorMessage"
        class="rounded-2xl border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300"
      >
        {{ errorMessage }}
      </div>

      <button
        type="button"
        class="btn btn-primary w-full"
        :disabled="isLoading"
        @click="continueFlow"
      >
        <Icon name="login" size="md" class="mr-2" />
        {{ t('auth.lobehub.retry') }}
      </button>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import { createLobeHubOIDCWebSession } from '@/api/lobehub'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'

import {
  buildLobeHubContinuationPayload,
  buildLobeHubSelectKeyRouteQuery,
  isLobeHubDefaultKeyRequiredError,
  parseLobeHubContinuationQuery
} from './lobehubFlow'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const isLoading = ref(false)
const errorMessage = ref('')

const continuationQuery = computed(() => parseLobeHubContinuationQuery(route.query))

function validateContinuationRequest(): boolean {
  if (!continuationQuery.value.returnURL) {
    errorMessage.value = t('auth.lobehub.invalidRequest')
    return false
  }

  if (continuationQuery.value.mode === 'oidc' && !continuationQuery.value.resumeToken) {
    errorMessage.value = t('auth.lobehub.invalidRequest')
    return false
  }

  return true
}

async function continueFlow(): Promise<void> {
  if (!validateContinuationRequest()) {
    appStore.showError(errorMessage.value)
    return
  }

  isLoading.value = true
  errorMessage.value = ''

  try {
    const response = await createLobeHubOIDCWebSession(
      buildLobeHubContinuationPayload(route.query)
    )
    window.location.assign(response.continue_url)
  } catch (error) {
    if (isLobeHubDefaultKeyRequiredError(error)) {
      await router.replace({
        name: 'LobeHubSelectKey',
        query: buildLobeHubSelectKeyRouteQuery(route.query)
      })
      return
    }

    errorMessage.value = t('auth.lobehub.continueFailed')
    appStore.showError(errorMessage.value)
  } finally {
    isLoading.value = false
  }
}

onMounted(() => {
  continueFlow()
})
</script>
