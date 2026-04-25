<template>
  <AuthLayout>
    <div class="space-y-6 text-center">
      <div>
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('auth.lobehub.title') }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('auth.lobehub.continuing') }}
        </p>
      </div>

      <div v-if="loading" class="flex justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
      </div>

      <transition name="fade">
        <div
          v-if="errorMessage"
          class="rounded-xl border border-red-200 bg-red-50 p-4 text-left dark:border-red-800/50 dark:bg-red-900/20"
        >
          <div class="flex items-start gap-3">
            <Icon name="exclamationCircle" size="md" class="mt-0.5 flex-shrink-0 text-red-500" />
            <div class="space-y-3">
              <p class="text-sm text-red-700 dark:text-red-400">
                {{ errorMessage }}
              </p>
              <router-link to="/keys" class="btn btn-primary">
                {{ t('keys.title') }}
              </router-link>
            </div>
          </div>
        </div>
      </transition>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import { lobehubAPI } from '@/api'
import { useAppStore } from '@/stores'
import {
  buildLobeHubOIDCWebSessionPayload,
  buildLobeHubSelectKeyQuery,
  isDefaultChatAPIKeyRequired,
  parseLobeHubFlowQuery
} from '@/utils/lobehubFlow'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const errorMessage = ref('')

const parsedQuery = computed(() => parseLobeHubFlowQuery(route.query))

function validateRequest(): boolean {
  const parsed = parsedQuery.value
  if (!parsed.returnURL || (parsed.mode === 'oidc' && !parsed.resumeToken)) {
    errorMessage.value = t('auth.lobehub.invalidRequest')
    return false
  }
  return true
}

async function continueToLobeHub() {
  if (!validateRequest()) {
    appStore.showError(errorMessage.value)
    return
  }

  loading.value = true
  errorMessage.value = ''
  try {
    const response = await lobehubAPI.createOIDCWebSession(
      buildLobeHubOIDCWebSessionPayload(route.query)
    )
    window.location.assign(response.continue_url)
  } catch (error) {
    if (isDefaultChatAPIKeyRequired(error)) {
      await router.replace({ name: 'LobeHubSelectKey', query: buildLobeHubSelectKeyQuery(route.query) })
      return
    }
    errorMessage.value = t('auth.lobehub.continueFailed')
    appStore.showError(errorMessage.value)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void continueToLobeHub()
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
