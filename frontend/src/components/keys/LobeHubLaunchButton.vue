<template>
  <button
    v-if="visible"
    :disabled="loading"
    class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-emerald-50 hover:text-emerald-600 disabled:cursor-not-allowed disabled:opacity-60 dark:hover:bg-emerald-900/20 dark:hover:text-emerald-400"
    @click="handleClick"
  >
    <Icon name="sparkles" size="sm" :class="loading ? 'animate-pulse' : ''" />
    <span class="text-xs">
      {{ loading ? t('keys.openingLobeHub') : t('keys.openInLobeHub') }}
    </span>
  </button>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { createLobeHubLaunchTicket } from '@/api/lobehub'
import { useAppStore } from '@/stores/app'

interface LobeHubVisibilitySettings {
  lobehub_enabled?: boolean
  hide_lobehub_import_button?: boolean
}

const props = defineProps<{
  apiKeyId: number
  publicSettings?: LobeHubVisibilitySettings | null
}>()

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(false)

const visible = computed(() => {
  return !!props.publicSettings?.lobehub_enabled && !props.publicSettings?.hide_lobehub_import_button
})

const handleClick = async () => {
  if (loading.value) return

  loading.value = true
  try {
    const result = await createLobeHubLaunchTicket(props.apiKeyId)
    const target = new URL(result.bridge_url, window.location.origin).toString()
    window.location.assign(target)
  } catch (error) {
    console.error('Failed to open LobeHub:', error)
    appStore.showError(t('keys.failedToOpenLobeHub'))
  } finally {
    loading.value = false
  }
}
</script>
