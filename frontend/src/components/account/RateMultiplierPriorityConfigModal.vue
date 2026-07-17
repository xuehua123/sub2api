<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.rateMultiplierPriorityConfigTitle')"
    width="wide"
    @close="emit('close')"
  >
    <form id="rate-multiplier-priority-config" class="space-y-6" @submit.prevent="submit">
      <p class="text-sm text-gray-500 dark:text-dark-400">
        {{ t('admin.accounts.rateMultiplierPriorityConfigDescription') }}
      </p>

      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label class="input-label" for="rate-priority-interval">
            {{ t('admin.accounts.rateMultiplierPriorityInterval') }}
          </label>
          <input
            id="rate-priority-interval"
            v-model.number="draft.interval_minutes"
            class="input"
            type="number"
            min="1"
            max="60"
            step="1"
          />
          <p class="input-hint">{{ t('admin.accounts.rateMultiplierPriorityIntervalHint') }}</p>
        </div>

        <div>
          <label class="input-label" for="rate-priority-step">
            {{ t('admin.accounts.rateMultiplierPriorityStep') }}
          </label>
          <input
            id="rate-priority-step"
            v-model.number="draft.priority_step"
            class="input"
            type="number"
            min="1"
            max="1000"
            step="1"
          />
          <p class="input-hint">{{ t('admin.accounts.rateMultiplierPriorityStepHint') }}</p>
        </div>
      </div>

      <p v-if="validationError" class="text-sm text-red-600 dark:text-red-300" role="alert">
        {{ validationError }}
      </p>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="emit('close')">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" form="rate-multiplier-priority-config" class="btn btn-primary" :disabled="saving || !!validationError">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { RateMultiplierPrioritySettings } from '@/api/admin/settings'
import BaseDialog from '@/components/common/BaseDialog.vue'

interface Props {
  show: boolean
  settings: RateMultiplierPrioritySettings
  saving?: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'save', settings: RateMultiplierPrioritySettings): void
}

const props = withDefaults(defineProps<Props>(), { saving: false })
const emit = defineEmits<Emits>()
const { t } = useI18n()

const cloneSettings = (settings: RateMultiplierPrioritySettings): RateMultiplierPrioritySettings => ({
  enabled: settings.enabled,
  interval_minutes: settings.interval_minutes,
  priority_step: settings.priority_step
})

const draft = ref<RateMultiplierPrioritySettings>(cloneSettings(props.settings))

watch(
  () => props.show,
  (show) => {
    if (show) draft.value = cloneSettings(props.settings)
  }
)

const validationError = computed(() => {
  if (!Number.isInteger(draft.value.interval_minutes) || draft.value.interval_minutes < 1 || draft.value.interval_minutes > 60) {
    return t('admin.accounts.rateMultiplierPriorityInvalidInterval')
  }
  if (!Number.isInteger(draft.value.priority_step) || draft.value.priority_step < 1 || draft.value.priority_step > 1000) {
    return t('admin.accounts.rateMultiplierPriorityInvalidStep')
  }
  return null
})

function submit() {
  if (validationError.value) return
  emit('save', cloneSettings(draft.value))
}
</script>
