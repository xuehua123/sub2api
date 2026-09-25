<template>
  <BaseDialog
    :show="show"
    :title="t('userBusiness.fxTitle')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <p class="text-xs text-gray-500">{{ t('userBusiness.fxImmutable') }}</p>
      <form class="flex flex-wrap items-end gap-3" @submit.prevent="save">
        <label class="text-xs text-gray-500"
          >{{ t('userBusiness.date')
          }}<input
            v-model="form.date"
            class="input mt-1"
            type="date"
            :min="start"
            :max="end"
            required
        /></label>
        <label class="text-xs text-gray-500"
          >{{ t('userBusiness.fxRate')
          }}<input
            v-model.number="form.usd_cny"
            class="input mt-1 w-32"
            type="number"
            min="0.000001"
            max="100"
            step="0.000001"
            required
        /></label>
        <label class="min-w-48 flex-1 text-xs text-gray-500"
          >{{ t('userBusiness.fxSource')
          }}<input
            v-model.trim="form.source"
            class="input mt-1"
            maxlength="200"
            required
        /></label>
        <button class="btn btn-primary" :disabled="saving">
          {{ t('userBusiness.fxRecord') }}
        </button>
      </form>
      <p v-if="error" role="alert" class="text-sm text-red-600">
        {{ t('userBusiness.fxSaveError') }}
      </p>
      <p v-if="loading" role="status">{{ t('userBusiness.loading') }}</p>
      <div v-else class="max-h-80 overflow-auto">
        <table class="w-full text-sm">
          <thead
            class="sticky top-0 bg-gray-50 text-left text-xs text-gray-500 dark:bg-dark-800"
          >
            <tr>
              <th class="p-2">{{ t('userBusiness.date') }}</th>
              <th class="p-2">{{ t('userBusiness.fxRate') }}</th>
              <th class="p-2">{{ t('userBusiness.fxSource') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="row in rows"
              :key="row.date"
              class="border-t dark:border-dark-700"
            >
              <td class="p-2">
                <button class="text-primary-600" @click="form.date = row.date">
                  {{ row.date }}
                </button>
              </td>
              <td class="p-2">
                {{ row.usd_cny ?? t('userBusiness.fxMissing') }}
              </td>
              <td class="p-2">{{ row.source ?? '—' }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { ref, reactive, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import {
  getBusinessFX,
  saveBusinessFX,
  type BusinessFX,
} from '@/api/admin/userBusiness'
const props = defineProps<{ show: boolean; start: string; end: string }>()
const emit = defineEmits<{ close: []; saved: [] }>()
const { t } = useI18n()
const rows = ref<BusinessFX[]>([]),
  loading = ref(false),
  saving = ref(false),
  error = ref(false)
const form = reactive({ date: '', usd_cny: 0, source: '' })
let generation = 0,
  controller: AbortController | undefined
async function load() {
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  if (!props.show) return
  loading.value = true
  error.value = false
  try {
    const data = await getBusinessFX(props.start, props.end, controller.signal)
    if (current === generation) rows.value = data
  } catch {
    if (current === generation) error.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}
async function save() {
  saving.value = true
  error.value = false
  try {
    await saveBusinessFX({ ...form })
    emit('saved')
    await load()
  } catch {
    error.value = true
  } finally {
    saving.value = false
  }
}
watch(
  () => [props.show, props.start, props.end],
  () => {
    form.date = props.start
    void load()
  },
)
onBeforeUnmount(() => {
  generation++
  controller?.abort()
})
</script>
