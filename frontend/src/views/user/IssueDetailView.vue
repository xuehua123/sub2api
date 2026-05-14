<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-5xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
      <button class="btn btn-secondary w-fit" type="button" @click="router.push('/issues')">
        {{ t('issueCenter.detail.backToList') }}
      </button>

      <div v-if="loading" class="flex items-center justify-center py-16">
        <LoadingSpinner />
      </div>
      <div
        v-else-if="errorMessage"
        class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
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
                <span v-if="issue.pinned_at" class="badge badge-primary">{{ t('issueCenter.detail.pinned') }}</span>
              </div>
              <h1 class="mt-2 break-words text-2xl font-semibold tracking-normal text-gray-900 dark:text-white">
                {{ issue.title }}
              </h1>
            </div>
            <div class="flex flex-wrap gap-2">
              <button
                v-if="authStore.isAdmin"
                class="btn btn-primary"
                type="button"
                data-testid="admin-manage-issue-button"
                @click="router.push(`/admin/issues/${issue.id}`)"
              >
                {{ t('issueCenter.detail.adminManage') }}
              </button>
              <button
                v-if="authStore.isAuthenticated"
                class="btn btn-secondary"
                type="button"
                :disabled="resolving || isLocked"
                data-testid="resolve-issue-button"
                @click="resolveIssue"
              >
                {{ resolving ? t('common.processing') : t('issueCenter.detail.markResolved') }}
              </button>
            </div>
          </div>

          <div
            v-if="issue.related_issue"
            class="mt-4 rounded-lg border border-primary-200 bg-primary-50 p-3 text-sm text-primary-800 dark:border-primary-900/60 dark:bg-primary-950/30 dark:text-primary-200"
            data-testid="related-solved-issue"
          >
            <div class="font-semibold">{{ t('issueCenter.detail.relatedSolvedIssue') }}</div>
            <RouterLink class="mt-1 inline-flex break-words underline" :to="`/issues/${issue.related_issue.id}`">
              {{ issue.related_issue.public_id }} · {{ issue.related_issue.title }}
            </RouterLink>
          </div>

          <div
            v-if="issue.solution_comment"
            class="mt-4 rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm dark:border-emerald-900/60 dark:bg-emerald-950/30"
            data-testid="solution-comment"
          >
            <div class="font-semibold text-emerald-800 dark:text-emerald-200">{{ t('issueCenter.detail.solution') }}</div>
            <p class="mt-2 whitespace-pre-wrap break-words leading-6 text-emerald-900 dark:text-emerald-100">
              {{ issue.solution_comment.content }}
            </p>
            <div class="mt-2 text-xs text-emerald-700 dark:text-emerald-300">
              {{ formatDateTime(issue.solution_comment.created_at) }}
            </div>
          </div>

          <div class="mt-5 grid gap-4 text-sm md:grid-cols-2">
            <InfoRow :label="t('issueCenter.fields.email')" :value="issue.account_email_masked" />
            <InfoRow :label="t('issueCenter.fields.occurredAt')" :value="formatDateTime(issue.occurred_at)" />
            <InfoRow :label="t('issueCenter.fields.screenshotLanguage')" :value="t(`issueCenter.language.${issue.screenshot_language}`)" />
            <InfoRow :label="t('issueCenter.fields.model')" :value="issue.model_name || '-'" />
            <InfoRow :label="t('issueCenter.fields.client')" :value="issue.client_name || '-'" />
            <InfoRow :label="t('issueCenter.fields.httpStatus')" :value="issue.http_status ? String(issue.http_status) : '-'" />
            <InfoRow :label="t('issueCenter.fields.errorCode')" :value="issue.error_code || '-'" />
            <InfoRow :label="t('issueCenter.detail.views')" :value="String(issue.view_count)" />
          </div>

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
          <h2 class="section-heading">{{ t('issueCenter.detail.attachments') }}</h2>
          <div v-if="issue.attachments?.length" class="mt-3 grid gap-3 sm:grid-cols-2">
            <a
              v-for="attachment in issue.attachments"
              :key="attachment.id"
              :href="issuesAPI.attachmentFileURL(attachment.id)"
              target="_blank"
              rel="noopener noreferrer"
              class="block overflow-hidden rounded-lg border border-gray-200 bg-gray-50 transition hover:border-primary-300 dark:border-dark-600 dark:bg-dark-900"
            >
              <img
                :src="issuesAPI.attachmentFileURL(attachment.id)"
                :alt="attachment.file_name"
                class="h-48 w-full object-contain"
              />
              <div class="border-t border-gray-200 px-3 py-2 text-xs text-gray-600 dark:border-dark-600 dark:text-gray-400">
                {{ attachment.file_name }} · {{ formatBytes(attachment.size_bytes) }}
              </div>
            </a>
          </div>
          <p v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('issueCenter.detail.noAttachments') }}</p>
        </section>

        <section class="rounded-lg border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex items-center justify-between gap-3">
            <h2 class="section-heading">{{ t('issueCenter.detail.comments') }}</h2>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ issue.comment_count }}</span>
          </div>
          <div v-if="issue.comments?.length" class="mt-4 space-y-3">
            <article
              v-for="comment in issue.comments"
              :key="comment.id"
              class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-900"
            >
              <div class="mb-2 flex flex-wrap items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                <span v-if="comment.author_display_name" class="font-medium text-gray-700 dark:text-gray-200">
                  {{ comment.author_display_name }}
                </span>
                <span class="badge badge-gray">{{ t(`issueCenter.roles.${comment.author_role}`) }}</span>
                <span v-if="comment.id === issue.solution_comment_id" class="badge badge-success">{{ t('issueCenter.detail.solution') }}</span>
                <span>{{ formatDateTime(comment.created_at) }}</span>
              </div>
              <p class="whitespace-pre-wrap break-words text-sm leading-6 text-gray-700 dark:text-gray-300">
                {{ comment.content }}
              </p>
              <RouterLink
                v-if="comment.related_issue"
                class="mt-2 inline-flex text-sm font-medium text-primary-600 underline dark:text-primary-300"
                :to="`/issues/${comment.related_issue.id}`"
              >
                {{ t('issueCenter.detail.relatedIssueLink', { publicId: comment.related_issue.public_id }) }}
              </RouterLink>
            </article>
          </div>
          <p v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('issueCenter.detail.noComments') }}</p>

          <div class="mt-5 border-t border-gray-200 pt-4 dark:border-dark-700">
            <p v-if="isLocked" class="rounded-md bg-gray-100 px-3 py-2 text-sm text-gray-600 dark:bg-dark-900 dark:text-gray-400" data-testid="locked-comment-hint">
              {{ t('issueCenter.detail.lockedHint') }}
            </p>
            <button v-else-if="!authStore.isAuthenticated" class="btn btn-secondary" type="button" @click="goLogin">
              {{ t('issueCenter.detail.loginToComment') }}
            </button>
            <form v-else class="space-y-3" @submit.prevent="submitComment">
              <label class="block">
                <span class="input-label flex items-center justify-between gap-2">
                  <span>{{ t('issueCenter.detail.commentLabel') }}</span>
                  <IssueEmojiPickerButton v-model="commentContent" target-id="issue-comment-input" />
                </span>
                <textarea
                  id="issue-comment-input"
                  v-model.trim="commentContent"
                  class="input min-h-[110px]"
                  data-testid="comment-input"
                  :disabled="commenting"
                ></textarea>
              </label>
              <p v-if="commentError" class="text-sm text-red-600 dark:text-red-400">{{ commentError }}</p>
              <button class="btn btn-primary" type="submit" :disabled="commenting || !commentContent">
                {{ commenting ? t('common.submitting') : t('issueCenter.detail.submitComment') }}
              </button>
            </form>
          </div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import IssueEmojiPickerButton from '@/components/issues/IssueEmojiPickerButton.vue'
import { useAuthStore } from '@/stores/auth'
import { useSupportIssueNotificationStore } from '@/stores/supportIssueNotifications'
import { issuesAPI } from '@/api/issues'
import type { PublicSupportIssue, SupportIssueSeverity, SupportIssueStatus } from '@/types'

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
const authStore = useAuthStore()
const issueNotificationStore = useSupportIssueNotificationStore()

const issue = ref<PublicSupportIssue | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const commentContent = ref('')
const commentError = ref('')
const commenting = ref(false)
const resolving = ref(false)

const issueID = computed(() => Number(route.params.id))
const isLocked = computed(() => {
  if (!issue.value) return false
  return issue.value.status === 'resolved' || issue.value.status === 'closed' || Boolean(issue.value.locked_at)
})

async function loadIssue() {
  loading.value = true
  errorMessage.value = ''
  try {
    issue.value = await issuesAPI.get(issueID.value)
    if (authStore.isAuthenticated) {
      issueNotificationStore.refresh().catch(() => {})
    }
  } catch (error) {
    errorMessage.value = getErrorMessage(error, t('issueCenter.errors.loadDetailFailed'))
  } finally {
    loading.value = false
  }
}

async function submitComment() {
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

async function resolveIssue() {
  if (!issue.value) return
  resolving.value = true
  errorMessage.value = ''
  try {
    issue.value = await issuesAPI.resolve(issue.value.id)
    await loadIssue()
  } catch (error) {
    errorMessage.value = getErrorMessage(error, t('issueCenter.errors.resolveFailed'))
  } finally {
    resolving.value = false
  }
}

function goLogin() {
  router.push({ path: '/login', query: { redirect: route.fullPath } })
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

function formatBytes(value: number): string {
  if (value >= 1024 * 1024) return `${(value / 1024 / 1024).toFixed(1)} MB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${value} B`
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
