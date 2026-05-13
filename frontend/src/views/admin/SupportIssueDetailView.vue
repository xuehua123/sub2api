<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
      <button class="btn btn-secondary w-fit" type="button" @click="router.push('/admin/issues')">
        {{ t('issueCenter.admin.backToList') }}
      </button>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner />
      </div>
      <div
        v-else-if="errorMessage"
        class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
        data-testid="admin-issue-error"
      >
        {{ errorMessage }}
      </div>

      <template v-else-if="issue">
        <article class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 lg:flex-row lg:items-start lg:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-mono text-xs font-semibold text-gray-500 dark:text-gray-400">{{ issue.public_id }}</span>
                <span :class="statusBadgeClass(issue.status)">{{ t(`issueCenter.status.${issue.status}`) }}</span>
                <span :class="severityBadgeClass(issue.severity)">{{ t(`issueCenter.severity.${issue.severity}`) }}</span>
                <span class="badge badge-gray">{{ t(`issueCenter.category.${issue.category}`) }}</span>
              </div>
              <h1 class="mt-2 break-words text-2xl font-semibold tracking-normal text-gray-900 dark:text-white">
                {{ issue.title }}
              </h1>
            </div>
          </div>

          <section class="mt-5">
            <h2 class="section-heading">{{ t('issueCenter.admin.diagnostics') }}</h2>
            <div class="mt-3 grid gap-3 text-sm md:grid-cols-2 xl:grid-cols-3">
              <InfoRow :label="t('issueCenter.fields.email')" :value="issue.account_email || '-'" />
              <InfoRow :label="t('issueCenter.admin.normalizedEmail')" :value="issue.account_email_normalized || '-'" />
              <InfoRow :label="t('issueCenter.admin.maskedEmail')" :value="issue.account_email_masked || '-'" />
              <InfoRow :label="t('issueCenter.fields.apiKeySuffix')" :value="issue.api_key_suffix || '-'" />
              <InfoRow :label="t('issueCenter.admin.createdByUserID')" :value="formatNullableNumber(issue.created_by_user_id)" />
              <InfoRow :label="t('issueCenter.admin.resolvedByUserID')" :value="formatNullableNumber(issue.resolved_by_user_id)" />
              <InfoRow :label="t('issueCenter.admin.hiddenComments')" :value="String(issue.hidden_comment_count)" />
              <InfoRow :label="t('issueCenter.fields.occurredAt')" :value="formatDateTime(issue.occurred_at)" />
              <InfoRow :label="t('issueCenter.fields.screenshotLanguage')" :value="t(`issueCenter.language.${issue.screenshot_language}`)" />
              <InfoRow :label="t('issueCenter.fields.model')" :value="issue.model_name || '-'" />
              <InfoRow :label="t('issueCenter.fields.client')" :value="issue.client_name || '-'" />
              <InfoRow :label="t('issueCenter.fields.httpStatus')" :value="issue.http_status ? String(issue.http_status) : '-'" />
              <InfoRow :label="t('issueCenter.fields.errorCode')" :value="issue.error_code || '-'" />
              <InfoRow :label="t('issueCenter.admin.resolvedAt')" :value="formatDateTime(issue.resolved_at)" />
              <InfoRow :label="t('issueCenter.admin.lockedAt')" :value="formatDateTime(issue.locked_at)" />
            </div>
          </section>

          <section class="mt-6">
            <h2 class="section-heading">{{ t('issueCenter.fields.description') }}</h2>
            <p class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-gray-700 dark:text-gray-300">
              {{ issue.description }}
            </p>
          </section>

          <section class="mt-6">
            <h2 class="section-heading">{{ t('issueCenter.fields.screenshotText') }}</h2>
            <pre class="mt-2 whitespace-pre-wrap rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm leading-6 text-gray-700 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-300">{{ issue.screenshot_text }}</pre>
          </section>
        </article>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <h2 class="section-heading">{{ t('issueCenter.admin.actions') }}</h2>
          <div class="mt-4 grid gap-4 lg:grid-cols-2">
            <form class="space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-900" data-testid="admin-update-status-form" @submit.prevent="updateStatus">
              <label class="block">
                <span class="input-label">{{ t('issueCenter.admin.nextStatus') }}</span>
                <select v-model="statusForm.status" class="input" data-testid="admin-status-select">
                  <option v-for="status in statuses" :key="status" :value="status">
                    {{ t(`issueCenter.status.${status}`) }}
                  </option>
                </select>
              </label>
              <label class="block">
                <span class="input-label">{{ t('issueCenter.admin.reason') }}</span>
                <textarea v-model.trim="statusForm.reason" class="input min-h-[80px]" data-testid="admin-status-reason"></textarea>
              </label>
              <button class="btn btn-primary" type="submit" :disabled="actionLoading" data-testid="admin-update-status-button">
                {{ t('issueCenter.admin.updateStatus') }}
              </button>
            </form>

            <form class="space-y-3 rounded-lg bg-gray-50 p-3 dark:bg-dark-900" data-testid="admin-reopen-form" @submit.prevent="reopenIssue">
              <label class="block">
                <span class="input-label">{{ t('issueCenter.admin.reopenReason') }}</span>
                <textarea v-model.trim="reopenReason" class="input min-h-[80px]" data-testid="admin-reopen-reason"></textarea>
              </label>
              <button class="btn btn-secondary" type="submit" :disabled="actionLoading" data-testid="admin-reopen-button">
                {{ t('issueCenter.admin.reopen') }}
              </button>
            </form>
          </div>
          <p v-if="actionError" class="mt-3 text-sm text-red-600 dark:text-red-400" data-testid="admin-action-error">
            {{ actionError }}
          </p>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <h2 class="section-heading">{{ t('issueCenter.detail.comments') }}</h2>
          <form class="mt-4 space-y-3 border-b border-gray-200 pb-4 dark:border-dark-700" data-testid="admin-comment-form" @submit.prevent="submitAdminComment">
            <label class="block">
              <span class="input-label">{{ t('issueCenter.detail.commentLabel') }}</span>
              <textarea
                v-model.trim="commentContent"
                class="input min-h-[110px]"
                data-testid="admin-comment-input"
                :disabled="commenting"
              ></textarea>
            </label>
            <p v-if="commentError" class="text-sm text-red-600 dark:text-red-400">{{ commentError }}</p>
            <button class="btn btn-primary" type="submit" :disabled="commenting || !commentContent" data-testid="admin-comment-submit">
              {{ commenting ? t('common.submitting') : t('issueCenter.detail.submitComment') }}
            </button>
          </form>

          <div v-if="issue.comments?.length" class="mt-4 space-y-3">
            <article
              v-for="comment in issue.comments"
              :key="comment.id"
              class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900"
            >
              <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                <span class="badge badge-gray">{{ t(`issueCenter.roles.${comment.author_role}`) }}</span>
                <span>{{ t('issueCenter.admin.authorUserID') }}: {{ formatNullableNumber(comment.author_user_id) }}</span>
                <span>{{ formatDateTime(comment.created_at) }}</span>
                <span v-if="comment.hidden_at" class="badge badge-warning">{{ t('issueCenter.admin.hidden') }}</span>
              </div>
              <p class="mt-2 whitespace-pre-wrap break-words text-sm leading-6 text-gray-700 dark:text-gray-300">
                {{ comment.content }}
              </p>
              <div v-if="comment.hidden_at" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('issueCenter.admin.hiddenByUserID') }}: {{ formatNullableNumber(comment.hidden_by_user_id) }}
                · {{ t('issueCenter.admin.hideReason') }}: {{ comment.hide_reason || '-' }}
              </div>
              <form v-else class="mt-3 flex flex-col gap-2 sm:flex-row sm:items-end" data-testid="admin-hide-comment-form" @submit.prevent="hideComment(comment.id)">
                <label class="min-w-0 flex-1">
                  <span class="input-label">{{ t('issueCenter.admin.hideReason') }}</span>
                  <input
                    v-model.trim="commentHideReasons[comment.id]"
                    class="input"
                    data-testid="admin-hide-comment-reason"
                    type="text"
                  />
                </label>
                <button class="btn btn-danger" type="submit" data-testid="admin-hide-comment-button">
                  {{ t('issueCenter.admin.hideComment') }}
                </button>
              </form>
            </article>
          </div>
          <p v-else class="mt-3 text-sm text-gray-500 dark:text-gray-400">{{ t('issueCenter.detail.noComments') }}</p>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <h2 class="section-heading">{{ t('issueCenter.detail.attachments') }}</h2>
          <div v-if="issue.attachments?.length" class="mt-3 grid gap-3 lg:grid-cols-2">
            <article
              v-for="attachment in issue.attachments"
              :key="attachment.id"
              class="overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-900"
            >
              <a :href="attachmentPreviewURL(attachment)" target="_blank" rel="noopener noreferrer" class="block">
                <img
                  :src="attachmentPreviewURL(attachment)"
                  :alt="attachment.file_name"
                  class="h-56 w-full object-contain"
                  data-testid="admin-attachment-preview"
                />
              </a>
              <div class="space-y-2 border-t border-gray-200 p-3 text-xs text-gray-600 dark:border-dark-600 dark:text-gray-400">
                <div class="flex flex-wrap gap-x-3 gap-y-1">
                  <span>{{ attachment.file_name }}</span>
                  <span>{{ attachment.mime_type }}</span>
                  <span>{{ formatBytes(attachment.size_bytes) }}</span>
                  <span>{{ attachment.visibility }}</span>
                </div>
                <div class="break-all font-mono" data-testid="admin-attachment-file-path">
                  {{ t('issueCenter.admin.filePath') }}: {{ attachment.file_path || '-' }}
                </div>
                <div v-if="attachment.hidden_at">
                  {{ t('issueCenter.admin.hiddenByUserID') }}: {{ formatNullableNumber(attachment.hidden_by_user_id) }}
                  · {{ formatDateTime(attachment.hidden_at) }}
                </div>
                <form v-else class="flex flex-col gap-2 sm:flex-row sm:items-end" data-testid="admin-hide-attachment-form" @submit.prevent="hideAttachment(attachment.id)">
                  <label class="min-w-0 flex-1">
                    <span class="input-label">{{ t('issueCenter.admin.hideReason') }}</span>
                    <input
                      v-model.trim="attachmentHideReasons[attachment.id]"
                      class="input"
                      data-testid="admin-hide-attachment-reason"
                      type="text"
                    />
                  </label>
                  <button class="btn btn-danger" type="submit" data-testid="admin-hide-attachment-button">
                    {{ t('issueCenter.admin.hideAttachment') }}
                  </button>
                </form>
              </div>
            </article>
          </div>
          <p v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('issueCenter.detail.noAttachments') }}</p>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <h2 class="section-heading">{{ t('issueCenter.admin.events') }}</h2>
          <div v-if="events.length" class="mt-3 space-y-3" data-testid="admin-issue-events">
            <article
              v-for="event in events"
              :key="event.id"
              class="rounded-lg border border-gray-200 bg-gray-50 p-3 text-sm dark:border-dark-600 dark:bg-dark-900"
            >
              <div class="flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                <span class="font-semibold text-gray-900 dark:text-white">{{ event.event_type }}</span>
                <span>{{ formatDateTime(event.created_at) }}</span>
                <span>{{ t('issueCenter.admin.actorUserID') }}: {{ formatNullableNumber(event.actor_user_id) }}</span>
              </div>
              <div class="mt-2 text-xs text-gray-600 dark:text-gray-400">
                {{ event.from_status || '-' }} → {{ event.to_status || '-' }}
              </div>
              <pre v-if="event.metadata" class="mt-2 whitespace-pre-wrap rounded-md bg-white p-2 text-xs text-gray-700 dark:bg-dark-800 dark:text-gray-300">{{ formatMetadata(event.metadata) }}</pre>
            </article>
          </div>
          <p v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('issueCenter.admin.noEvents') }}</p>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { issuesAPI } from '@/api/issues'
import { adminIssuesAPI } from '@/api/admin/issues'
import type {
  AdminSupportIssue,
  AdminSupportIssueAttachment,
  SupportIssueEvent,
  SupportIssueSeverity,
  SupportIssueStatus
} from '@/types'

const InfoRow = defineComponent({
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
  },
  setup(props) {
    return () => h('div', { class: 'rounded-lg bg-gray-50 p-3 dark:bg-dark-900' }, [
      h('div', { class: 'text-xs font-medium text-gray-500 dark:text-gray-400' }, props.label),
      h('div', { class: 'mt-1 break-words text-gray-900 dark:text-white' }, props.value),
    ])
  },
})

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const statuses: SupportIssueStatus[] = ['open', 'needs_info', 'in_progress', 'resolved', 'closed']
const issue = ref<AdminSupportIssue | null>(null)
const events = ref<SupportIssueEvent[]>([])
const loading = ref(false)
const errorMessage = ref('')
const actionError = ref('')
const actionLoading = ref(false)
const commentContent = ref('')
const commentError = ref('')
const commenting = ref(false)
const reopenReason = ref('')
const statusForm = reactive({
  status: 'open' as SupportIssueStatus,
  reason: '',
})
const commentHideReasons = reactive<Record<number, string>>({})
const attachmentHideReasons = reactive<Record<number, string>>({})

const issueID = computed(() => Number(route.params.id))

async function loadIssue() {
  loading.value = true
  errorMessage.value = ''
  try {
    const [loadedIssue, loadedEvents] = await Promise.all([
      adminIssuesAPI.get(issueID.value),
      adminIssuesAPI.events(issueID.value),
    ])
    issue.value = loadedIssue
    events.value = loadedEvents
    statusForm.status = loadedIssue.status
  } catch (error) {
    errorMessage.value = getErrorMessage(error, t('issueCenter.admin.errors.loadDetailFailed'))
  } finally {
    loading.value = false
  }
}

async function updateStatus() {
  if (!issue.value) return
  if (!statusForm.reason) {
    actionError.value = t('issueCenter.admin.errors.reasonRequired')
    return
  }
  actionLoading.value = true
  actionError.value = ''
  try {
    await adminIssuesAPI.updateStatus(issue.value.id, {
      status: statusForm.status,
      reason: statusForm.reason,
    })
    statusForm.reason = ''
    await loadIssue()
  } catch (error) {
    actionError.value = getErrorMessage(error, t('issueCenter.admin.errors.updateStatusFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function reopenIssue() {
  if (!issue.value) return
  if (!reopenReason.value) {
    actionError.value = t('issueCenter.admin.errors.reasonRequired')
    return
  }
  actionLoading.value = true
  actionError.value = ''
  try {
    await adminIssuesAPI.reopen(issue.value.id, { reason: reopenReason.value })
    reopenReason.value = ''
    await loadIssue()
  } catch (error) {
    actionError.value = getErrorMessage(error, t('issueCenter.admin.errors.reopenFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function submitAdminComment() {
  if (!issue.value || !commentContent.value) return
  commenting.value = true
  commentError.value = ''
  try {
    await issuesAPI.addComment(issue.value.id, { content: commentContent.value })
    commentContent.value = ''
    await loadIssue()
  } catch (error) {
    commentError.value = getErrorMessage(error, t('issueCenter.errors.commentFailed'))
  } finally {
    commenting.value = false
  }
}

async function hideComment(commentID: number) {
  if (!issue.value) return
  const reason = commentHideReasons[commentID]
  if (!reason) {
    actionError.value = t('issueCenter.admin.errors.reasonRequired')
    return
  }
  if (!window.confirm(t('issueCenter.admin.confirmHideComment'))) return
  actionLoading.value = true
  actionError.value = ''
  try {
    await adminIssuesAPI.hideComment(issue.value.id, commentID, { reason })
    commentHideReasons[commentID] = ''
    await loadIssue()
  } catch (error) {
    actionError.value = getErrorMessage(error, t('issueCenter.admin.errors.hideCommentFailed'))
  } finally {
    actionLoading.value = false
  }
}

async function hideAttachment(attachmentID: number) {
  if (!issue.value) return
  const reason = attachmentHideReasons[attachmentID]
  if (!reason) {
    actionError.value = t('issueCenter.admin.errors.reasonRequired')
    return
  }
  if (!window.confirm(t('issueCenter.admin.confirmHideAttachment'))) return
  actionLoading.value = true
  actionError.value = ''
  try {
    await adminIssuesAPI.hideAttachment(issue.value.id, attachmentID, { reason })
    attachmentHideReasons[attachmentID] = ''
    await loadIssue()
  } catch (error) {
    actionError.value = getErrorMessage(error, t('issueCenter.admin.errors.hideAttachmentFailed'))
  } finally {
    actionLoading.value = false
  }
}

function attachmentPreviewURL(attachment: AdminSupportIssueAttachment): string {
  if (attachment.file_url && attachment.file_url !== attachment.file_path) {
    return attachment.file_url
  }
  return issuesAPI.attachmentFileURL(attachment.id)
}

function getErrorMessage(error: unknown, fallback: string): string {
  if (error && typeof error === 'object' && 'message' in error) {
    return String((error as { message?: unknown }).message || fallback)
  }
  return fallback
}

function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatNullableNumber(value?: number | null): string {
  return value == null ? '-' : String(value)
}

function formatBytes(value: number): string {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
}

function formatMetadata(metadata: Record<string, unknown>): string {
  return JSON.stringify(metadata, null, 2)
}

function statusBadgeClass(status: SupportIssueStatus): string {
  const base = 'badge'
  if (status === 'resolved') return `${base} badge-success`
  if (status === 'closed') return `${base} badge-danger`
  if (status === 'in_progress') return `${base} badge-primary`
  if (status === 'needs_info') return `${base} badge-warning`
  return `${base} badge-gray`
}

function severityBadgeClass(severity: SupportIssueSeverity): string {
  const base = 'badge'
  if (severity === 'blocked') return `${base} badge-danger`
  if (severity === 'partial') return `${base} badge-warning`
  if (severity === 'intermittent') return `${base} badge-primary`
  return `${base} badge-gray`
}

onMounted(loadIssue)
</script>

<style scoped>
.section-heading {
  @apply text-sm font-semibold uppercase tracking-normal text-gray-700 dark:text-gray-300;
}
</style>
