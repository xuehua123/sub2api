<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
      <header class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p class="text-sm font-medium text-primary-600 dark:text-primary-400">{{ t('issueCenter.kicker') }}</p>
          <h1 class="mt-1 text-2xl font-semibold tracking-normal text-gray-900 dark:text-white">
            {{ t('issueCenter.new.title') }}
          </h1>
          <p class="mt-1 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
            {{ t('issueCenter.new.description') }}
          </p>
        </div>
        <button class="btn btn-secondary w-fit" type="button" @click="router.push('/issues')">
          {{ t('issueCenter.detail.backToList') }}
        </button>
      </header>

      <form class="space-y-5" @submit.prevent="submitIssue">
        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="grid gap-4 md:grid-cols-2">
            <label class="block md:col-span-2">
              <span class="input-label">{{ t('issueCenter.fields.title') }}</span>
              <input v-model.trim="form.title" class="input" required data-testid="new-issue-title" />
            </label>

            <label class="block md:col-span-2">
              <span class="input-label">{{ t('issueCenter.fields.description') }}</span>
              <textarea v-model.trim="form.description" class="input min-h-[130px]" required data-testid="new-issue-description"></textarea>
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.email') }}</span>
              <input v-model.trim="form.account_email" class="input" type="email" required data-testid="new-issue-email" />
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.occurredAt') }}</span>
              <input v-model="occurredAtLocal" class="input" type="datetime-local" required data-testid="new-issue-occurred-at" />
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.screenshotLanguage') }}</span>
              <select v-model="form.screenshot_language" class="input" required data-testid="new-issue-language">
                <option v-for="language in languages" :key="language" :value="language">
                  {{ t(`issueCenter.language.${language}`) }}
                </option>
              </select>
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.category') }}</span>
              <select v-model="form.category" class="input" required data-testid="new-issue-category">
                <option v-for="category in categories" :key="category" :value="category">
                  {{ t(`issueCenter.category.${category}`) }}
                </option>
              </select>
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.severity') }}</span>
              <select v-model="form.severity" class="input" required data-testid="new-issue-severity">
                <option v-for="severity in severities" :key="severity" :value="severity">
                  {{ t(`issueCenter.severity.${severity}`) }}
                </option>
              </select>
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.httpStatus') }}</span>
              <input v-model.number="httpStatus" class="input" type="number" min="100" max="599" data-testid="new-issue-http-status" />
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.model') }}</span>
              <input v-model.trim="form.model_name" class="input" data-testid="new-issue-model" />
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.client') }}</span>
              <input v-model.trim="form.client_name" class="input" data-testid="new-issue-client" />
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.errorCode') }}</span>
              <input v-model.trim="form.error_code" class="input" data-testid="new-issue-error-code" />
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.apiKeySuffix') }}</span>
              <input v-model.trim="form.api_key_suffix" class="input" maxlength="16" data-testid="new-issue-key-suffix" />
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.keySuffixHint') }}</span>
            </label>

            <label class="block md:col-span-2">
              <span class="input-label">{{ t('issueCenter.fields.screenshotText') }}</span>
              <textarea v-model.trim="form.screenshot_text" class="input min-h-[120px]" required data-testid="new-issue-screenshot-text"></textarea>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.screenshotTextHint') }}</span>
            </label>
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.new.uploadTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.new.uploadHint') }}</p>
            </div>
            <label class="btn btn-secondary w-fit cursor-pointer">
              {{ uploading ? t('issueCenter.new.uploading') : t('common.chooseFile') }}
              <input class="sr-only" type="file" accept="image/png,image/jpeg,image/webp,image/gif" data-testid="issue-file-input" @change="handleFileChange" />
            </label>
          </div>
          <p v-if="uploadError" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ uploadError }}</p>
          <div v-if="uploadedAttachments.length" class="mt-4 grid gap-3 sm:grid-cols-2">
            <div
              v-for="attachment in uploadedAttachments"
              :key="attachment.id"
              class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900"
            >
              <img v-if="attachment.preview_url" :src="attachment.preview_url" :alt="attachment.file_name" class="mb-2 h-32 w-full rounded object-contain" />
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ attachment.file_name }}</div>
              <div class="text-xs text-gray-500 dark:text-gray-400">{{ attachment.mime_type }} · {{ formatBytes(attachment.size_bytes) }}</div>
            </div>
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.new.similarTitle') }}</h2>
              <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.new.similarHint') }}</p>
            </div>
            <button class="btn btn-secondary w-fit" type="button" :disabled="suggesting" data-testid="suggestions-button" @click="findSimilar(true)">
              {{ suggesting ? t('common.processing') : t('issueCenter.new.findSimilar') }}
            </button>
          </div>
          <p v-if="suggestionError" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ suggestionError }}</p>
          <div v-if="suggestions.length" class="mt-4 space-y-2" data-testid="suggestions-list">
            <router-link
              v-for="suggestion in suggestions"
              :key="suggestion.id"
              :to="`/issues/${suggestion.id}`"
              class="block rounded-lg border border-gray-200 p-3 text-sm transition hover:border-primary-300 dark:border-dark-600"
            >
              <span class="font-mono text-xs text-gray-500">{{ suggestion.public_id }}</span>
              <span class="ml-2 font-medium text-gray-900 dark:text-white">{{ suggestion.title }}</span>
            </router-link>
          </div>
        </section>

        <p v-if="formError" class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300" data-testid="new-issue-error">
          {{ formError }}
        </p>

        <div class="flex flex-col-reverse gap-3 border-t border-gray-200 pt-4 dark:border-dark-700 sm:flex-row sm:justify-end">
          <button class="btn btn-secondary" type="button" @click="router.push('/issues')">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="submit" :disabled="submitting" data-testid="submit-new-issue">
            {{ submitting ? t('common.submitting') : t('issueCenter.new.submit') }}
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { issuesAPI } from '@/api/issues'
import type {
  CreateSupportIssueRequest,
  PublicSupportIssue,
  SupportIssueCategory,
  SupportIssueScreenshotLanguage,
  SupportIssueSeverity,
  UploadedSupportIssueAttachment
} from '@/types'

type UploadedAttachmentPreview = UploadedSupportIssueAttachment & { preview_url?: string }

const maxFileBytes = 5 * 1024 * 1024
const allowedMimeTypes = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
const categories: SupportIssueCategory[] = ['login', 'payment', 'api_call', 'model_unavailable', 'api_key', 'balance', 'subscription', 'channel', 'other']
const severities: SupportIssueSeverity[] = ['blocked', 'partial', 'intermittent', 'question']
const languages: SupportIssueScreenshotLanguage[] = ['zh', 'en', 'mixed', 'unknown']

const { t } = useI18n()
const router = useRouter()

const form = reactive({
  title: '',
  description: '',
  account_email: '',
  screenshot_text: '',
  screenshot_language: 'unknown' as SupportIssueScreenshotLanguage,
  category: 'other' as SupportIssueCategory,
  severity: 'question' as SupportIssueSeverity,
  model_name: '',
  client_name: '',
  error_code: '',
  api_key_suffix: '',
})
const occurredAtLocal = ref('')
const httpStatus = ref<number | null>(null)
const uploadedAttachments = ref<UploadedAttachmentPreview[]>([])
const suggestions = ref<PublicSupportIssue[]>([])
const formError = ref('')
const uploadError = ref('')
const suggestionError = ref('')
const uploading = ref(false)
const submitting = ref(false)
const suggesting = ref(false)

function buildRequest(): CreateSupportIssueRequest {
  return {
    title: form.title,
    description: form.description,
    account_email: form.account_email,
    occurred_at: new Date(occurredAtLocal.value).toISOString(),
    screenshot_text: form.screenshot_text,
    screenshot_language: form.screenshot_language,
    category: form.category,
    severity: form.severity,
    ...(form.model_name ? { model_name: form.model_name } : {}),
    ...(form.client_name ? { client_name: form.client_name } : {}),
    ...(httpStatus.value ? { http_status: httpStatus.value } : {}),
    ...(form.error_code ? { error_code: form.error_code } : {}),
    ...(form.api_key_suffix ? { api_key_suffix: form.api_key_suffix } : {}),
    attachment_ids: uploadedAttachments.value.map((item) => item.id),
  }
}

function validateForm(): boolean {
  formError.value = ''
  if (!form.title || !form.description || !form.account_email || !occurredAtLocal.value || !form.screenshot_text || !form.screenshot_language || !form.category || !form.severity) {
    formError.value = t('issueCenter.new.requiredError')
    return false
  }
  if (httpStatus.value !== null && (httpStatus.value < 100 || httpStatus.value > 599)) {
    formError.value = t('issueCenter.new.httpStatusError')
    return false
  }
  if (form.api_key_suffix && !/^[A-Za-z0-9_-]{4,16}$/.test(form.api_key_suffix)) {
    formError.value = t('issueCenter.new.keySuffixError')
    return false
  }
  if (Number.isNaN(new Date(occurredAtLocal.value).getTime())) {
    formError.value = t('issueCenter.new.occurredAtError')
    return false
  }
  return true
}

async function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return

  uploadError.value = ''
  if (!allowedMimeTypes.includes(file.type)) {
    uploadError.value = t('issueCenter.new.invalidFileType')
    return
  }
  if (file.size > maxFileBytes) {
    uploadError.value = t('issueCenter.new.fileTooLarge')
    return
  }
  if (uploadedAttachments.value.length >= 5) {
    uploadError.value = t('issueCenter.new.tooManyAttachments')
    return
  }

  uploading.value = true
  const previewURL = URL.createObjectURL(file)
  try {
    const attachment = await issuesAPI.uploadAttachment(file)
    uploadedAttachments.value.push({ ...attachment, preview_url: previewURL })
  } catch (error) {
    URL.revokeObjectURL(previewURL)
    uploadError.value = getErrorMessage(error, t('issueCenter.new.uploadFailed'))
  } finally {
    uploading.value = false
  }
}

async function findSimilar(showErrors: boolean) {
  if (!validateForm()) return
  suggesting.value = true
  suggestionError.value = ''
  try {
    suggestions.value = await issuesAPI.searchSuggestions(buildRequest())
  } catch (error) {
    if (showErrors) {
      suggestionError.value = getErrorMessage(error, t('issueCenter.new.suggestionsFailed'))
    }
  } finally {
    suggesting.value = false
  }
}

async function submitIssue() {
  if (!validateForm()) return
  submitting.value = true
  formError.value = ''
  try {
    if (suggestions.value.length === 0) {
      await findSimilar(false)
    }
    const created = await issuesAPI.create(buildRequest())
    await router.push(`/issues/${created.id}`)
  } catch (error) {
    formError.value = getErrorMessage(error, t('issueCenter.new.submitFailed'))
  } finally {
    submitting.value = false
  }
}

function getErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object' && 'message' in error) {
    return String((error as { message?: unknown }).message || fallback)
  }
  return fallback
}

function formatBytes(value: number): string {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}

onBeforeUnmount(() => {
  for (const attachment of uploadedAttachments.value) {
    if (attachment.preview_url) URL.revokeObjectURL(attachment.preview_url)
  }
})
</script>
