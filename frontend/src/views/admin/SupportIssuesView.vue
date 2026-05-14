<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-7xl flex-col gap-5 px-4 py-5 sm:px-6 lg:px-8">
      <header class="flex flex-col gap-2 border-b border-gray-200 pb-4 dark:border-dark-700">
        <p class="text-sm font-medium text-primary-600 dark:text-primary-400">{{ t('issueCenter.admin.kicker') }}</p>
        <h1 class="text-2xl font-semibold tracking-normal text-gray-900 dark:text-white">
          {{ t('issueCenter.admin.title') }}
        </h1>
        <p class="max-w-3xl text-sm text-gray-600 dark:text-gray-400">
          {{ t('issueCenter.admin.description') }}
        </p>
      </header>

      <section class="rounded-lg border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
        <form class="space-y-4" @submit.prevent="applyFilters">
          <div class="grid gap-3 lg:grid-cols-[minmax(0,1fr)_auto_auto] lg:items-end">
            <label class="block">
              <span class="input-label">{{ t('issueCenter.search.label') }}</span>
              <input
                v-model.trim="filters.q"
                class="input"
                type="search"
                name="q"
                :placeholder="t('issueCenter.admin.searchPlaceholder')"
                data-testid="admin-issue-search-input"
              />
            </label>
            <button class="btn btn-secondary" type="submit" :disabled="loading">
              {{ t('common.search') }}
            </button>
            <button class="btn btn-secondary" type="button" @click="clearFilters">
              {{ t('issueCenter.search.clear') }}
            </button>
          </div>

          <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-6">
            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.status') }}</span>
              <select v-model="filters.status" class="input" data-testid="admin-issue-status-filter">
                <option value="pending">{{ t('issueCenter.admin.pending') }}</option>
                <option value="all">{{ t('common.all') }}</option>
                <option v-for="status in statuses" :key="status" :value="status">
                  {{ t(`issueCenter.status.${status}`) }}
                </option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.category') }}</span>
              <select v-model="filters.category" class="input" data-testid="admin-issue-category-filter">
                <option value="">{{ t('common.all') }}</option>
                <option v-for="category in categories" :key="category" :value="category">
                  {{ t(`issueCenter.category.${category}`) }}
                </option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.severity') }}</span>
              <select v-model="filters.severity" class="input" data-testid="admin-issue-severity-filter">
                <option value="">{{ t('common.all') }}</option>
                <option v-for="severity in severities" :key="severity" :value="severity">
                  {{ t(`issueCenter.severity.${severity}`) }}
                </option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('issueCenter.fields.hasImage') }}</span>
              <select v-model="hasImageFilter" class="input" data-testid="admin-issue-has-image-filter">
                <option value="">{{ t('common.all') }}</option>
                <option value="true">{{ t('common.yes') }}</option>
                <option value="false">{{ t('common.no') }}</option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('issueCenter.admin.visibility') }}</span>
              <select v-model="hiddenFilter" class="input" data-testid="admin-issue-hidden-filter">
                <option value="">{{ t('common.all') }}</option>
                <option value="false">{{ t('issueCenter.admin.visibleOnly') }}</option>
                <option value="true">{{ t('issueCenter.admin.hiddenOnly') }}</option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('issueCenter.admin.sortBy') }}</span>
              <select v-model="sortMode" class="input" data-testid="admin-issue-sort-filter">
                <option value="last_comment_at">{{ t('issueCenter.feed.active') }}</option>
                <option value="created_at">{{ t('issueCenter.feed.latest') }}</option>
                <option value="view_count">{{ t('issueCenter.feed.popular') }}</option>
                <option value="comment_count">{{ t('issueCenter.feed.replied') }}</option>
                <option value="occurred_at">{{ t('issueCenter.admin.occurredSort') }}</option>
              </select>
            </label>
          </div>
        </form>

        <div class="mt-4 rounded-md border border-dashed border-gray-300 bg-gray-50 px-3 py-2 text-xs leading-5 text-gray-600 dark:border-dark-600 dark:bg-dark-900/60 dark:text-gray-400">
          {{ t('issueCenter.admin.searchSyntaxIntro') }}
          <code class="code">id:</code>
          <code class="code">status:</code>
          <code class="code">category:</code>
          <code class="code">severity:</code>
          <code class="code">model:</code>
          <code class="code">client:</code>
          <code class="code">code:</code>
          <code class="code">error:</code>
          <code class="code">email:</code>
          <code class="code">lang:</code>
          <code class="code">has:image</code>
          <code class="code">time:</code>
          <code class="code">key:</code>
          <code class="code">"{{ t('issueCenter.search.exactPhrase') }}"</code>
          <span class="ml-1 font-medium text-amber-700 dark:text-amber-300">
            {{ t('issueCenter.admin.keySearchWarning') }}
          </span>
        </div>
      </section>

      <section class="min-h-[280px]">
        <div v-if="loading" class="flex items-center justify-center py-16">
          <LoadingSpinner />
        </div>
        <div
          v-else-if="errorMessage"
          class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
          data-testid="admin-issues-error"
        >
          {{ errorMessage }}
        </div>
        <div
          v-else-if="issues.length === 0"
          class="rounded-lg border border-gray-200 bg-white p-8 text-center dark:border-dark-700 dark:bg-dark-800"
          data-testid="admin-issues-empty"
        >
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.admin.emptyTitle') }}</h2>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.admin.emptyDescription') }}</p>
        </div>
        <div v-else class="space-y-3" data-testid="admin-issues-list">
          <article
            v-for="issue in issues"
            :key="issue.id"
            class="cursor-pointer rounded-lg border border-gray-200 bg-white p-4 shadow-sm transition hover:border-primary-300 hover:shadow-md dark:border-dark-700 dark:bg-dark-800 dark:hover:border-primary-700"
            data-testid="admin-issue-list-item"
            @click="router.push(`/admin/issues/${issue.id}`)"
          >
            <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-mono text-xs font-semibold text-gray-500 dark:text-gray-400">{{ issue.public_id }}</span>
                  <span :class="statusBadgeClass(issue.status)">{{ t(`issueCenter.status.${issue.status}`) }}</span>
                  <span v-if="issue.pinned_at" class="badge badge-primary">{{ t('issueCenter.detail.pinned') }}</span>
                  <span v-if="issue.hidden_at" class="badge badge-warning">{{ t('issueCenter.admin.hidden') }}</span>
                  <span :class="severityBadgeClass(issue.severity)">{{ t(`issueCenter.severity.${issue.severity}`) }}</span>
                  <span class="badge badge-gray">{{ t(`issueCenter.category.${issue.category}`) }}</span>
                </div>
                <h2 class="mt-2 break-words text-base font-semibold text-gray-900 dark:text-white">
                  {{ issue.title }}
                </h2>
                <div class="mt-2 grid gap-1 text-xs text-gray-500 dark:text-gray-400 md:grid-cols-2 xl:grid-cols-3">
                  <span>{{ t('issueCenter.fields.email') }}: {{ issue.account_email }}</span>
                  <span>{{ t('issueCenter.admin.maskedEmail') }}: {{ issue.account_email_masked }}</span>
                  <span>{{ t('issueCenter.fields.apiKeySuffix') }}: {{ issue.api_key_suffix || '-' }}</span>
                  <span>{{ t('issueCenter.fields.model') }}: {{ issue.model_name || '-' }}</span>
                  <span>{{ t('issueCenter.fields.client') }}: {{ issue.client_name || '-' }}</span>
                  <span>{{ t('issueCenter.fields.httpStatus') }}: {{ issue.http_status || '-' }}</span>
                  <span>{{ t('issueCenter.fields.errorCode') }}: {{ issue.error_code || '-' }}</span>
                  <span>{{ t('issueCenter.list.views', { count: issue.view_count }) }}</span>
                  <span>{{ t('issueCenter.list.lastActivity') }}: {{ formatDateTime(issue.last_comment_at || issue.updated_at) }}</span>
                </div>
              </div>
              <div class="grid grid-cols-3 gap-2 text-center text-xs text-gray-500 dark:text-gray-400 sm:w-80">
                <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900">
                  <div class="font-semibold text-gray-900 dark:text-white">{{ issue.comment_count }}</div>
                  <div>{{ t('issueCenter.admin.publicComments') }}</div>
                </div>
                <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900">
                  <div class="font-semibold text-gray-900 dark:text-white">{{ issue.attachment_count }}</div>
                  <div>{{ t('issueCenter.admin.attachments') }}</div>
                </div>
                <div class="rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900">
                  <div class="font-semibold text-gray-900 dark:text-white">{{ issue.hidden_comment_count }}</div>
                  <div>{{ t('issueCenter.admin.hiddenComments') }}</div>
                </div>
              </div>
            </div>
          </article>
        </div>
      </section>

      <Pagination
        v-if="pagination.total > 0"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import { adminIssuesAPI } from '@/api/admin/issues'
import type {
  AdminSupportIssue,
  SupportIssueCategory,
  SupportIssueSeverity,
  SupportIssueStatus
} from '@/types'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()

const statuses: SupportIssueStatus[] = ['open', 'needs_info', 'in_progress', 'resolved', 'closed']
const categories: SupportIssueCategory[] = ['login', 'payment', 'api_call', 'model_unavailable', 'api_key', 'balance', 'subscription', 'channel', 'other']
const severities: SupportIssueSeverity[] = ['blocked', 'partial', 'intermittent', 'question']

const filters = reactive({
  q: '',
  status: 'pending' as 'pending' | 'all' | SupportIssueStatus,
  category: '' as '' | SupportIssueCategory,
  severity: '' as '' | SupportIssueSeverity,
})
const hasImageFilter = ref('')
const hiddenFilter = ref('')
const sortMode = ref('last_comment_at')
const issues = ref<AdminSupportIssue[]>([])
const loading = ref(false)
const errorMessage = ref('')
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0,
})

function queryString(value: unknown): string {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function syncStateFromRoute() {
  filters.q = queryString(route.query.q)
  filters.status = (queryString(route.query.status) || 'pending') as 'pending' | 'all' | SupportIssueStatus
  filters.category = queryString(route.query.category) as '' | SupportIssueCategory
  filters.severity = queryString(route.query.severity) as '' | SupportIssueSeverity
  hasImageFilter.value = queryString(route.query.has_image)
  hiddenFilter.value = queryString(route.query.hidden)
  sortMode.value = queryString(route.query.sort_by) || 'last_comment_at'
  pagination.page = Number(queryString(route.query.page) || 1) || 1
  pagination.page_size = Number(queryString(route.query.page_size) || 20) || 20
}

function buildQuery() {
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.status !== 'pending' ? { status: filters.status } : {}),
    ...(filters.category ? { category: filters.category } : {}),
    ...(filters.severity ? { severity: filters.severity } : {}),
    ...(hasImageFilter.value ? { has_image: hasImageFilter.value } : {}),
    ...(hiddenFilter.value ? { hidden: hiddenFilter.value } : {}),
    ...(sortMode.value !== 'last_comment_at' ? { sort_by: sortMode.value } : {}),
    ...(pagination.page > 1 ? { page: String(pagination.page) } : {}),
    ...(pagination.page_size !== 20 ? { page_size: String(pagination.page_size) } : {}),
  }
}

async function replaceRouteQuery() {
  await router.replace({ path: '/admin/issues', query: buildQuery() })
}

function buildParams() {
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.status !== 'all' ? { status: filters.status } : {}),
    ...(filters.category ? { category: filters.category } : {}),
    ...(filters.severity ? { severity: filters.severity } : {}),
    ...(hasImageFilter.value ? { has_image: hasImageFilter.value === 'true' } : {}),
    ...(hiddenFilter.value ? { hidden: hiddenFilter.value === 'true' } : {}),
    page: pagination.page,
    page_size: pagination.page_size,
    sort_by: sortMode.value,
    sort_order: 'desc' as const,
  }
}

async function loadIssues() {
  loading.value = true
  errorMessage.value = ''
  try {
    const result = await adminIssuesAPI.list(buildParams())
    issues.value = result.items
    pagination.total = result.total
    pagination.pages = result.pages
  } catch (error) {
    errorMessage.value = getErrorMessage(error)
  } finally {
    loading.value = false
  }
}

async function applyFilters() {
  pagination.page = 1
  await replaceRouteQuery()
  await loadIssues()
}

async function clearFilters() {
  filters.q = ''
  filters.status = 'pending'
  filters.category = ''
  filters.severity = ''
  hasImageFilter.value = ''
  hiddenFilter.value = ''
  sortMode.value = 'last_comment_at'
  pagination.page = 1
  await replaceRouteQuery()
  await loadIssues()
}

async function handlePageChange(page: number) {
  pagination.page = page
  await replaceRouteQuery()
  await loadIssues()
}

async function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  await replaceRouteQuery()
  await loadIssues()
}

function getErrorMessage(error: unknown): string {
  if (error && typeof error === 'object' && 'message' in error) {
    return String((error as { message?: unknown }).message || t('issueCenter.admin.errors.loadFailed'))
  }
  return t('issueCenter.admin.errors.loadFailed')
}

function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
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

onMounted(() => {
  syncStateFromRoute()
  loadIssues()
})
</script>
