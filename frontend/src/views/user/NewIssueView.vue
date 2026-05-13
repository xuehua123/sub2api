<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-7xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
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
          <div class="flex flex-col gap-1">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.new.basicInfoTitle') }}</h2>
            <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.new.basicInfoHint') }}</p>
          </div>

          <div class="mt-4 grid gap-4 lg:grid-cols-[minmax(0,1fr)_260px]">
            <label class="block min-w-0">
              <span class="input-label">{{ t('issueCenter.fields.title') }}</span>
              <input
                v-model.trim="form.title"
                class="input"
                required
                data-testid="new-issue-title"
                :placeholder="t('issueCenter.new.titlePlaceholder')"
              />
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.titleHint') }}</span>
            </label>

            <label class="block">
              <span class="input-label">{{ t('issueCenter.new.scenarioLabel') }}</span>
              <select v-model="form.scenario" class="input" required data-testid="new-issue-scenario" @change="applyScenarioDefaults">
                <option v-for="scenario in scenarios" :key="scenario" :value="scenario">
                  {{ t(`issueCenter.scenario.${scenario}`) }}
                </option>
              </select>
            </label>
          </div>

          <div class="mt-4 grid gap-4 md:grid-cols-2 xl:grid-cols-5">
            <label class="block xl:col-span-2">
              <span class="input-label">{{ t('issueCenter.fields.email') }}</span>
              <input v-model.trim="form.account_email" class="input bg-gray-50 dark:bg-dark-900" type="email" required readonly data-testid="new-issue-email" />
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.emailAutoHint') }}</span>
            </label>

            <label class="block xl:col-span-2">
              <span class="input-label">{{ t('issueCenter.fields.occurredAt') }}</span>
              <div class="flex gap-2">
                <input v-model="occurredAtLocal" class="input min-w-0" type="datetime-local" step="60" required data-testid="new-issue-occurred-at" />
                <button class="btn btn-secondary shrink-0 px-3" type="button" data-testid="new-issue-use-current-time" @click="useCurrentTime">
                  {{ t('common.now') }}
                </button>
              </div>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.occurredAtHint') }}</span>
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
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-1">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.new.problemDetailsTitle') }}</h2>
            <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.new.problemDetailsHint') }}</p>
          </div>

          <div class="mt-4 grid gap-4 lg:grid-cols-2">
            <label class="block min-w-0">
              <span class="input-label">{{ t('issueCenter.new.errorSummaryLabel') }}</span>
              <textarea
                v-model.trim="form.error_summary"
                class="input min-h-[130px] resize-y leading-6"
                required
                data-testid="new-issue-error-summary"
                :placeholder="t('issueCenter.new.errorSummaryPlaceholder')"
                @paste="handlePaste"
              ></textarea>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.pasteImageHint') }}</span>
            </label>

            <label class="block min-w-0">
              <span class="input-label">{{ t('issueCenter.new.descriptionLabel') }}</span>
              <textarea
                v-model.trim="form.description"
                class="input min-h-[130px] resize-y leading-6"
                required
                data-testid="new-issue-description"
                :placeholder="t('issueCenter.new.descriptionPlaceholder')"
                @paste="handlePaste"
              ></textarea>
              <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.pasteImageHint') }}</span>
            </label>
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-1">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.new.screenshotEvidenceTitle') }}</h2>
            <p class="text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.new.screenshotEvidenceHint') }}</p>
          </div>

          <div class="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
            <div class="grid gap-4 lg:grid-cols-2 xl:grid-cols-1">
              <label class="block min-w-0">
                <span class="input-label">{{ t('issueCenter.new.screenshotTextLabel') }}</span>
                <textarea
                  v-model.trim="form.screenshot_text"
                  class="input min-h-[130px] resize-y leading-6"
                  required
                  data-testid="new-issue-screenshot-text"
                  :placeholder="t('issueCenter.new.screenshotTextPlaceholder')"
                  @paste="handlePaste"
                ></textarea>
                <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.pasteImageHint') }}</span>
              </label>

              <label class="block min-w-0">
                <span class="input-label">{{ t('issueCenter.new.screenshotMeaningLabel') }}</span>
                <textarea
                  v-model.trim="form.screenshot_meaning"
                  class="input min-h-[130px] resize-y leading-6"
                  data-testid="new-issue-screenshot-meaning"
                  :placeholder="t('issueCenter.new.screenshotMeaningPlaceholder')"
                  @paste="handlePaste"
                ></textarea>
                <span class="mt-1 block text-xs text-gray-500 dark:text-gray-400">{{ t('issueCenter.new.pasteImageHint') }}</span>
              </label>
            </div>

            <div
              class="flex min-h-[280px] flex-col rounded-lg border border-dashed p-4 transition"
              :class="dragActive ? 'border-primary-400 bg-primary-50 dark:bg-primary-950/30' : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-900/50'"
              data-testid="issue-dropzone"
              @dragenter.prevent="handleDragEnter"
              @dragover.prevent
              @dragleave.prevent="handleDragLeave"
              @drop.prevent="handleDrop"
            >
              <div class="flex flex-1 flex-col justify-center text-center">
                <div class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ dragActive ? t('issueCenter.new.dropActive') : t('issueCenter.new.dropTitle') }}
                </div>
                <p class="mx-auto mt-2 max-w-xs text-xs leading-5 text-gray-500 dark:text-gray-400">
                  {{ t('issueCenter.new.dropHint') }}
                </p>
                <label class="btn btn-secondary mx-auto mt-4 w-fit cursor-pointer">
                  {{ uploading ? t('issueCenter.new.uploading') : t('common.chooseFile') }}
                  <input
                    class="sr-only"
                    type="file"
                    accept="image/png,image/jpeg,image/webp,image/gif"
                    multiple
                    data-testid="issue-file-input"
                    @change="handleFileChange"
                  />
                </label>
              </div>

              <p v-if="uploadError" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ uploadError }}</p>

              <div v-if="uploadedAttachments.length" class="mt-4 border-t border-gray-200 pt-3 dark:border-dark-600">
                <div class="mb-2 text-xs font-medium uppercase tracking-normal text-gray-500 dark:text-gray-400">
                  {{ t('issueCenter.new.uploadedScreenshots') }}
                </div>
                <div class="space-y-2">
                  <div v-for="attachment in uploadedAttachments" :key="attachment.id" class="flex items-center gap-3 text-xs">
                    <img
                      v-if="attachment.preview_url"
                      :src="attachment.preview_url"
                      :alt="attachment.file_name"
                      class="h-12 w-14 shrink-0 rounded border border-gray-200 bg-white object-contain dark:border-dark-600 dark:bg-dark-800"
                    />
                    <div class="min-w-0">
                      <div class="truncate font-medium text-gray-900 dark:text-white">{{ attachment.file_name }}</div>
                      <div class="mt-0.5 text-gray-500 dark:text-gray-400">{{ attachment.mime_type }} · {{ formatBytes(attachment.size_bytes) }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div class="max-w-3xl">
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.new.diagnosticsTitle') }}</h2>
              <p class="mt-1 text-sm leading-5 text-gray-600 dark:text-gray-400">{{ t('issueCenter.new.diagnosticsHint') }}</p>
            </div>
            <button class="btn btn-secondary w-fit" type="button" :disabled="usageLogsLoading" data-testid="load-usage-logs" @click="loadRecentUsageLogs">
              {{ usageLogsLoading ? t('common.processing') : t('issueCenter.new.loadLogs') }}
            </button>
          </div>

          <div class="mt-4 grid gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
            <div class="space-y-4">
              <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
                <label class="block">
                  <span class="input-label">{{ t('issueCenter.new.logRangeStart') }}</span>
                  <input v-model="usageLogStartLocal" class="input" type="datetime-local" step="60" data-testid="usage-log-range-start" />
                </label>
                <label class="block">
                  <span class="input-label">{{ t('issueCenter.new.logRangeEnd') }}</span>
                  <input v-model="usageLogEndLocal" class="input" type="datetime-local" step="60" data-testid="usage-log-range-end" />
                </label>
              </div>

              <details class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900/60">
                <summary class="cursor-pointer text-sm font-medium text-gray-800 dark:text-gray-100">
                  {{ t('issueCenter.new.manualDiagnosticsTitle') }}
                </summary>
                <div class="mt-4 space-y-4">
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
                </div>
              </details>
            </div>

            <div class="min-w-0 space-y-3">
              <p v-if="usageLogError" class="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">{{ usageLogError }}</p>

              <div v-if="usageLogs.length" class="space-y-3" data-testid="usage-log-list">
                <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <span class="text-xs font-medium text-gray-500 dark:text-gray-400">
                    {{ t('issueCenter.new.selectedLogsCount', { count: selectedUsageLogs.length }) }}
                  </span>
                  <div class="flex gap-2">
                    <button class="btn btn-secondary px-3 py-1 text-xs" type="button" data-testid="select-all-usage-logs" @click="selectAllUsageLogs">
                      {{ t('issueCenter.new.selectAllLogs') }}
                    </button>
                    <button class="btn btn-secondary px-3 py-1 text-xs" type="button" data-testid="clear-usage-logs" @click="clearSelectedUsageLogs">
                      {{ t('issueCenter.new.clearSelectedLogs') }}
                    </button>
                  </div>
                </div>

                <div class="max-h-72 space-y-2 overflow-auto rounded-lg border border-gray-200 p-2 dark:border-dark-600">
                  <label
                    v-for="log in usageLogs"
                    :key="log.id"
                    class="grid cursor-pointer gap-2 rounded-md px-3 py-2 text-xs transition sm:grid-cols-[auto_minmax(0,1fr)_minmax(120px,auto)] sm:items-center hover:bg-gray-50 dark:hover:bg-dark-700"
                    :class="selectedUsageLogIDs.includes(String(log.id)) ? 'bg-primary-50 text-primary-900 dark:bg-primary-950/30 dark:text-primary-100' : 'text-gray-700 dark:text-gray-300'"
                  >
                    <input
                      v-model="selectedUsageLogIDs"
                      class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 sm:mt-0"
                      type="checkbox"
                      :value="String(log.id)"
                      data-testid="usage-log-checkbox"
                      @change="applySelectedUsageLogFields"
                    />
                    <span class="min-w-0">
                      <span class="block truncate font-medium">{{ usageLogOptionLabel(log) }}</span>
                      <span class="mt-0.5 block truncate text-gray-500 dark:text-gray-400">
                        {{ log.inbound_endpoint || log.user_agent || t('common.notAvailable') }}
                      </span>
                    </span>
                    <span class="font-mono text-gray-500 dark:text-gray-400">#{{ log.id }}</span>
                  </label>
                </div>
              </div>

              <p v-else-if="usageLogsLoaded" class="rounded-md bg-gray-50 px-3 py-2 text-sm text-gray-600 dark:bg-dark-900 dark:text-gray-400">
                {{ t('issueCenter.new.noLogs') }}
              </p>

              <pre v-if="selectedUsageLogs.length" class="max-h-44 overflow-auto whitespace-pre-wrap rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs leading-5 text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300" data-testid="selected-usage-log">{{ formatSelectedUsageLogsDiagnostic() }}</pre>
            </div>
          </div>
        </section>

        <section
          v-if="suggesting || suggestionError || suggestions.length"
          class="rounded-lg border border-primary-200 bg-primary-50/70 p-4 shadow-sm dark:border-primary-900/60 dark:bg-primary-950/20"
          data-testid="similar-issues-panel"
        >
          <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h2 class="text-sm font-semibold text-primary-950 dark:text-primary-100">{{ t('issueCenter.new.similarTitle') }}</h2>
              <p class="mt-1 text-xs leading-5 text-primary-800 dark:text-primary-200">
                {{ suggestions.length ? t('issueCenter.new.similarFound', { count: suggestions.length }) : t('issueCenter.new.similarHint') }}
              </p>
            </div>
            <span v-if="suggesting" class="rounded-full bg-white px-3 py-1 text-xs font-medium text-primary-700 dark:bg-dark-900 dark:text-primary-300">
              {{ t('issueCenter.new.similarChecking') }}
            </span>
          </div>
          <p v-if="suggestionError" class="mt-3 text-sm text-red-700 dark:text-red-300">{{ suggestionError }}</p>
          <div v-if="suggestions.length" class="mt-3 grid gap-2 md:grid-cols-2 xl:grid-cols-3" data-testid="suggestions-list">
            <router-link
              v-for="suggestion in suggestions"
              :key="suggestion.id"
              :to="`/issues/${suggestion.id}`"
              class="block rounded-lg border border-primary-200 bg-white p-3 text-sm transition hover:border-primary-400 dark:border-primary-900/70 dark:bg-dark-900"
            >
              <span class="font-mono text-xs text-primary-700 dark:text-primary-300">{{ suggestion.public_id }}</span>
              <span class="ml-2 font-medium text-gray-900 dark:text-white">{{ suggestion.title }}</span>
            </router-link>
          </div>
          <p v-if="suggestions.length" class="mt-2 text-xs text-primary-800 dark:text-primary-200">{{ t('issueCenter.new.similarContinue') }}</p>
        </section>

        <section
          class="rounded-lg border p-4 shadow-sm"
          :class="diagnosticCompleteness.ready ? 'border-emerald-200 bg-emerald-50/70 dark:border-emerald-900/60 dark:bg-emerald-950/20' : 'border-amber-200 bg-amber-50/80 dark:border-amber-900/60 dark:bg-amber-950/20'"
          data-testid="diagnostic-completeness"
        >
          <div class="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <h2 class="text-sm font-semibold" :class="diagnosticCompleteness.ready ? 'text-emerald-900 dark:text-emerald-100' : 'text-amber-900 dark:text-amber-100'">
                {{ t('issueCenter.new.completenessTitle') }}
              </h2>
              <p class="mt-1 text-sm" :class="diagnosticCompleteness.ready ? 'text-emerald-800 dark:text-emerald-200' : 'text-amber-800 dark:text-amber-200'">
                {{ diagnosticCompleteness.message }}
              </p>
            </div>
            <span class="rounded-full px-3 py-1 text-xs font-medium" :class="diagnosticCompleteness.ready ? 'bg-white text-emerald-700 dark:bg-dark-900 dark:text-emerald-300' : 'bg-white text-amber-700 dark:bg-dark-900 dark:text-amber-300'">
              {{ diagnosticCompleteness.ready ? t('issueCenter.new.completenessReady') : t('issueCenter.new.completenessNeedsInfo') }}
            </span>
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
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import { issuesAPI } from '@/api/issues'
import { usageAPI } from '@/api/usage'
import { useAuthStore } from '@/stores/auth'
import type {
  CreateSupportIssueRequest,
  PublicSupportIssue,
  SupportIssueCategory,
  SupportIssueScreenshotLanguage,
  SupportIssueSeverity,
  UploadedSupportIssueAttachment,
  UsageLog
} from '@/types'

type UploadedAttachmentPreview = UploadedSupportIssueAttachment & { preview_url?: string }
type UsageLogWithDiagnostics = UsageLog & {
  http_status?: number | null
  status_code?: number | null
  error_code?: string | number | null
  error_message?: string | null
}
type IssueScenario = 'error' | 'slow_timeout' | 'payment' | 'model_unavailable' | 'login_key' | 'other'

const maxFileBytes = 5 * 1024 * 1024
const maxAttachments = 5
const allowedMimeTypes = ['image/png', 'image/jpeg', 'image/webp', 'image/gif']
const categories: SupportIssueCategory[] = ['login', 'payment', 'api_call', 'model_unavailable', 'api_key', 'balance', 'subscription', 'channel', 'other']
const severities: SupportIssueSeverity[] = ['blocked', 'partial', 'intermittent', 'question']
const languages: SupportIssueScreenshotLanguage[] = ['zh', 'en', 'mixed', 'unknown']
const scenarios: IssueScenario[] = ['error', 'slow_timeout', 'payment', 'model_unavailable', 'login_key', 'other']

const { t } = useI18n()
const router = useRouter()
const authStore = useAuthStore()

const form = reactive({
  title: '',
  scenario: 'error' as IssueScenario,
  error_summary: '',
  description: '',
  screenshot_text: '',
  screenshot_meaning: '',
  account_email: '',
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
const lastSuggestionFingerprint = ref('')
let suggestionTimer: ReturnType<typeof setTimeout> | null = null
const dragActive = ref(false)
const dragDepth = ref(0)
const usageLogs = ref<UsageLogWithDiagnostics[]>([])
const usageLogsLoaded = ref(false)
const usageLogsLoading = ref(false)
const usageLogError = ref('')
const usageLogStartLocal = ref('')
const usageLogEndLocal = ref('')
const selectedUsageLogIDs = ref<string[]>([])
const selectedUsageLogs = computed(() => {
  const selected = new Set(selectedUsageLogIDs.value)
  return usageLogs.value.filter((log) => selected.has(String(log.id)))
})
const manualSlowEvidenceReady = computed(() => {
  return Boolean(parseUsageLogRange() && (form.model_name.trim() || form.client_name.trim()) && (form.description.trim().length >= 20 || form.screenshot_text.trim().length >= 10))
})
const slowEvidenceReady = computed(() => {
  if (form.scenario !== 'slow_timeout') return true
  return selectedUsageLogs.value.length > 0 || manualSlowEvidenceReady.value
})
const lowQualityMessage = computed(() => {
  return supportIssueLowQualityMessage([form.title, form.error_summary, form.description, form.screenshot_text].join(' '))
})
const diagnosticCompleteness = computed(() => {
  if (form.scenario === 'slow_timeout' && !slowEvidenceReady.value) {
    return {
      ready: false,
      message: t('issueCenter.new.completenessSlowNeedsLogs'),
    }
  }
  if (lowQualityMessage.value && selectedUsageLogs.value.length === 0) {
    return {
      ready: false,
      message: lowQualityMessage.value,
    }
  }
  if (selectedUsageLogs.value.length > 0) {
    return {
      ready: true,
      message: t('issueCenter.new.completenessWithLogs', { count: selectedUsageLogs.value.length }),
    }
  }
  return {
    ready: true,
    message: t('issueCenter.new.completenessBasicReady'),
  }
})

function buildRequest(): CreateSupportIssueRequest {
  const description = buildDescriptionContent()
  const screenshotText = buildScreenshotText()
  return {
    title: form.title,
    description,
    account_email: form.account_email,
    occurred_at: new Date(occurredAtLocal.value).toISOString(),
    screenshot_text: screenshotText,
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
  if (!form.title || !form.error_summary || !form.description || !form.screenshot_text || !form.account_email || !occurredAtLocal.value || !form.screenshot_language || !form.category || !form.severity) {
    formError.value = t('issueCenter.new.requiredError')
    return false
  }
  const qualityMessage = lowQualityMessage.value
  if (qualityMessage && selectedUsageLogs.value.length === 0) {
    formError.value = qualityMessage
    return false
  }
  if (!slowEvidenceReady.value) {
    formError.value = t('issueCenter.new.slowEvidenceError')
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

function applyCurrentUserEmail() {
  const email = authStore.user?.email?.trim()
  if (email) {
    form.account_email = email
  }
}

async function initializeDefaults() {
  const now = new Date()
  const logStart = new Date(now)
  logStart.setHours(logStart.getHours() - 2)
  occurredAtLocal.value = formatDateTimeLocalMinute(now)
  usageLogStartLocal.value = formatDateTimeLocalMinute(logStart)
  usageLogEndLocal.value = formatDateTimeLocalMinute(now)
  applyCurrentUserEmail()
  if (!form.account_email && authStore.token) {
    try {
      await authStore.refreshUser()
      applyCurrentUserEmail()
    } catch {
      // The route already requires auth; validation will surface a missing email if refresh fails.
    }
  }
}

function applyScenarioDefaults() {
  switch (form.scenario) {
    case 'slow_timeout':
      form.category = 'api_call'
      form.severity = form.severity === 'question' ? 'intermittent' : form.severity
      break
    case 'payment':
      form.category = 'payment'
      break
    case 'model_unavailable':
      form.category = 'model_unavailable'
      form.severity = form.severity === 'question' ? 'partial' : form.severity
      break
    case 'login_key':
      form.category = 'api_key'
      break
    case 'error':
      form.category = 'api_call'
      break
    default:
      form.category = 'other'
      break
  }
}

function handleDragEnter() {
  dragDepth.value += 1
  dragActive.value = true
}

function handleDragLeave() {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
  if (dragDepth.value === 0) {
    dragActive.value = false
  }
}

async function handleDrop(event: DragEvent) {
  dragDepth.value = 0
  dragActive.value = false
  await uploadFiles(Array.from(event.dataTransfer?.files ?? []))
}

async function handlePaste(event: ClipboardEvent) {
  const files = clipboardFiles(event.clipboardData)
  if (files.length === 0) return
  event.preventDefault()
  await uploadFiles(files)
}

async function handleFileChange(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ''
  await uploadFiles(files)
}

function clipboardFiles(data: DataTransfer | null): File[] {
  if (!data) return []
  const files: File[] = []
  for (const item of Array.from(data.items ?? [])) {
    if (item.kind === 'file' && allowedMimeTypes.includes(item.type)) {
      const file = item.getAsFile()
      if (file) files.push(file)
    }
  }
  if (files.length > 0) return files
  return Array.from(data.files ?? []).filter((file) => allowedMimeTypes.includes(file.type))
}

async function uploadFiles(files: File[]) {
  if (files.length === 0) return

  uploadError.value = ''
  const availableSlots = maxAttachments - uploadedAttachments.value.length
  if (availableSlots <= 0) {
    uploadError.value = t('issueCenter.new.tooManyAttachments')
    return
  }
  if (files.length > availableSlots) {
    uploadError.value = t('issueCenter.new.tooManyAttachments')
  }

  uploading.value = true
  try {
    for (const file of files.slice(0, availableSlots)) {
      const validationError = validateUploadFile(file)
      if (validationError) {
        uploadError.value = validationError
        continue
      }
      const previewURL = URL.createObjectURL(file)
      try {
        const attachment = await issuesAPI.uploadAttachment(file)
        uploadedAttachments.value.push({ ...attachment, preview_url: previewURL })
      } catch (error) {
        URL.revokeObjectURL(previewURL)
        uploadError.value = getErrorMessage(error, t('issueCenter.new.uploadFailed'))
      }
    }
  } finally {
    uploading.value = false
  }
}

function validateUploadFile(file: File): string {
  if (!allowedMimeTypes.includes(file.type)) {
    return t('issueCenter.new.invalidFileType')
  }
  if (file.size > maxFileBytes) {
    return t('issueCenter.new.fileTooLarge')
  }
  return ''
}

async function loadRecentUsageLogs() {
  const range = parseUsageLogRange()
  if (!range) {
    usageLogError.value = t('issueCenter.new.logRangeError')
    return
  }

  usageLogsLoading.value = true
  usageLogError.value = ''
  try {
    const response = await usageAPI.query({
      start_date: formatLocalDate(range.start),
      end_date: formatLocalDate(range.end),
      page: 1,
      page_size: 50,
      sort_by: 'created_at',
      sort_order: 'desc',
    })
    usageLogs.value = (response.items as UsageLogWithDiagnostics[]).filter((log) => usageLogInRange(log, range.start, range.end))
    selectedUsageLogIDs.value = usageLogs.value.map((log) => String(log.id))
    applySelectedUsageLogFields()
    scheduleSimilarLookup()
    usageLogsLoaded.value = true
  } catch (error) {
    usageLogError.value = getErrorMessage(error, t('issueCenter.new.logsFailed'))
  } finally {
    usageLogsLoading.value = false
  }
}

function parseUsageLogRange(): { start: Date; end: Date } | null {
  const start = new Date(usageLogStartLocal.value)
  const end = new Date(usageLogEndLocal.value)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime()) || start > end) {
    return null
  }
  return { start, end }
}

function usageLogInRange(log: UsageLogWithDiagnostics, start: Date, end: Date): boolean {
  const createdAt = new Date(log.created_at)
  if (Number.isNaN(createdAt.getTime())) {
    return false
  }
  return createdAt >= start && createdAt <= end
}

function selectAllUsageLogs() {
  selectedUsageLogIDs.value = usageLogs.value.map((log) => String(log.id))
  applySelectedUsageLogFields()
  scheduleSimilarLookup()
}

function clearSelectedUsageLogs() {
  selectedUsageLogIDs.value = []
  scheduleSimilarLookup()
}

function applySelectedUsageLogFields() {
  const log = selectedUsageLogs.value[0]
  if (!log) {
    return
  }
  if (log.model) {
    form.model_name = trimRunes(log.model, 120)
  }
  const client = log.inbound_endpoint || log.user_agent || ''
  if (client) {
    form.client_name = trimRunes(client, 120)
  }
  const status = log.http_status ?? log.status_code ?? null
  if (typeof status === 'number' && status >= 100 && status <= 599) {
    httpStatus.value = status
  }
  if (log.error_code !== undefined && log.error_code !== null && String(log.error_code).trim()) {
    form.error_code = trimRunes(String(log.error_code).trim(), 120)
  }
  const occurredAt = new Date(log.created_at)
  if (!Number.isNaN(occurredAt.getTime())) {
    occurredAtLocal.value = formatDateTimeLocalMinute(occurredAt)
  }
  scheduleSimilarLookup()
}

function buildDescriptionContent(): string {
  const parts = [
    `${t('issueCenter.new.scenarioLabel')}: ${t(`issueCenter.scenario.${form.scenario}`)}`,
    `${t('issueCenter.new.errorSummaryLabel')}: ${form.error_summary.trim()}`,
    `${t('issueCenter.new.descriptionLabel')}:\n${form.description.trim()}`,
  ]
  if (form.screenshot_meaning.trim()) {
    parts.push(`${t('issueCenter.new.screenshotMeaningLabel')}:\n${form.screenshot_meaning.trim()}`)
  }
  if (selectedUsageLogs.value.length > 0) {
    parts.push(`${t('issueCenter.new.selectedLogPayloadTitle')} (${selectedUsageLogs.value.length})\n${formatSelectedUsageLogsDiagnostic()}`)
  }
  return parts.join('\n\n')
}

function buildScreenshotText(): string {
  const parts = [form.screenshot_text.trim()]
  if (form.screenshot_meaning.trim()) {
    parts.push(`${t('issueCenter.new.screenshotMeaningLabel')}: ${form.screenshot_meaning.trim()}`)
  }
  return parts.join('\n\n')
}

function formatSelectedUsageLogsDiagnostic(): string {
  return selectedUsageLogs.value
    .map((log, index) => `${t('issueCenter.new.logNumber', { number: index + 1 })}\n${formatUsageLogDiagnostic(log)}`)
    .join('\n\n')
}

function formatUsageLogDiagnostic(log: UsageLogWithDiagnostics): string {
  const parts = [
    `${t('issueCenter.new.logId')}: ${log.id}`,
    `${t('issueCenter.new.logTime')}: ${formatDateTime(log.created_at)}`,
    `${t('issueCenter.fields.model')}: ${log.model || '-'}`,
    `${t('issueCenter.new.logApiKey')}: ${log.api_key?.name || '-'}`,
    `${t('issueCenter.new.logEndpoint')}: ${log.inbound_endpoint || '-'}`,
    `${t('issueCenter.new.logRequestType')}: ${log.request_type || '-'}`,
    `${t('issueCenter.fields.httpStatus')}: ${log.http_status ?? log.status_code ?? '-'}`,
    `${t('issueCenter.fields.errorCode')}: ${log.error_code ?? '-'}`,
    `${t('issueCenter.new.logDuration')}: ${formatDuration(log.duration_ms)}`,
    `${t('issueCenter.new.logTokens')}: ${log.input_tokens + log.output_tokens + log.cache_creation_tokens + log.cache_read_tokens}`,
  ]
  if (log.error_message) {
    parts.push(`${t('issueCenter.new.logErrorMessage')}: ${log.error_message}`)
  }
  return parts.join('\n')
}

function usageLogOptionLabel(log: UsageLogWithDiagnostics): string {
  const status = log.http_status ?? log.status_code
  const statusLabel = typeof status === 'number' ? `HTTP ${status}` : t('common.notAvailable')
  return `${formatDateTime(log.created_at)} · ${log.model || '-'} · ${statusLabel}`
}

function trimRunes(value: string, limit: number): string {
  const runes = Array.from(value.trim())
  if (runes.length <= limit) return value.trim()
  return runes.slice(0, limit).join('')
}

function supportIssueLowQualityMessage(value: string): string {
  const normalized = value
    .toLowerCase()
    .replace(/\s+/g, '')
    .replace(/[，。,.!?！？、]/g, '')
  if (!normalized) return ''

  const genericPhrases = ['太慢了', '很慢', '看截图', '看图', '报错', '不能用', '用不了', '打不开', '有问题', 'error', 'slow', 'failed']
  const isOnlyGeneric = genericPhrases.some((phrase) => normalized === phrase || normalized === `${phrase}${phrase}`)
  if (isOnlyGeneric || Array.from(normalized).length < 12) {
    return t('issueCenter.new.lowQualityError')
  }
  return ''
}

function formatLocalDate(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

function formatDateTimeLocalMinute(date: Date): string {
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  const hours = String(date.getHours()).padStart(2, '0')
  const minutes = String(date.getMinutes()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}T${hours}:${minutes}`
}

function useCurrentTime() {
  occurredAtLocal.value = formatDateTimeLocalMinute(new Date())
}

function formatDateTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatDuration(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '-'
  if (value < 1000) return `${Math.round(value)}ms`
  return `${(value / 1000).toFixed(2)}s`
}

function canSuggestSimilar(): boolean {
  return Boolean(
    form.title.trim() &&
    form.error_summary.trim() &&
    form.description.trim() &&
    form.screenshot_text.trim() &&
    form.account_email.trim() &&
    occurredAtLocal.value &&
    form.screenshot_language &&
    form.category &&
    form.severity &&
    !Number.isNaN(new Date(occurredAtLocal.value).getTime()) &&
    (httpStatus.value === null || (httpStatus.value >= 100 && httpStatus.value <= 599))
  )
}

function suggestionFingerprint(): string {
  return JSON.stringify({
    title: form.title.trim(),
    scenario: form.scenario,
    error: form.error_summary.trim(),
    description: form.description.trim(),
    screenshot: form.screenshot_text.trim(),
    meaning: form.screenshot_meaning.trim(),
    language: form.screenshot_language,
    category: form.category,
    severity: form.severity,
    model: form.model_name.trim(),
    client: form.client_name.trim(),
    status: httpStatus.value,
    errorCode: form.error_code.trim(),
    logs: selectedUsageLogIDs.value,
  })
}

function scheduleSimilarLookup() {
  if (suggestionTimer) {
    clearTimeout(suggestionTimer)
  }
  if (!canSuggestSimilar()) {
    suggestions.value = []
    suggestionError.value = ''
    lastSuggestionFingerprint.value = ''
    return
  }
  suggestionTimer = setTimeout(() => {
    void findSimilar(false)
  }, 700)
}

async function findSimilar(showErrors: boolean): Promise<PublicSupportIssue[]> {
  if (!canSuggestSimilar()) return []
  const fingerprint = suggestionFingerprint()
  if (fingerprint === lastSuggestionFingerprint.value) {
    return suggestions.value
  }
  suggesting.value = true
  suggestionError.value = ''
  try {
    const items = await issuesAPI.searchSuggestions(buildRequest())
    suggestions.value = items
    lastSuggestionFingerprint.value = fingerprint
    return items
  } catch (error) {
    if (showErrors) {
      suggestionError.value = getErrorMessage(error, t('issueCenter.new.suggestionsFailed'))
    }
    return []
  } finally {
    suggesting.value = false
  }
}

async function submitIssue() {
  if (!validateForm()) return
  submitting.value = true
  formError.value = ''
  try {
    await findSimilar(false)
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

onMounted(() => {
  void initializeDefaults()
})

watch(
  () => [
    form.title,
    form.scenario,
    form.error_summary,
    form.description,
    form.screenshot_text,
    form.screenshot_meaning,
    form.screenshot_language,
    form.category,
    form.severity,
    form.model_name,
    form.client_name,
    form.error_code,
    String(httpStatus.value ?? ''),
    selectedUsageLogIDs.value.join(','),
  ],
  () => scheduleSimilarLookup()
)

onBeforeUnmount(() => {
  if (suggestionTimer) {
    clearTimeout(suggestionTimer)
  }
  for (const attachment of uploadedAttachments.value) {
    if (attachment.preview_url) URL.revokeObjectURL(attachment.preview_url)
  }
})
</script>
