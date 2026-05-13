<template>
  <AppLayout>
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-4 px-4 py-5 sm:px-6 lg:px-8">
      <header class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 lg:flex-row lg:items-center lg:justify-between">
        <div class="min-w-0">
          <h1 class="text-2xl font-semibold tracking-normal text-gray-900 dark:text-white">
            {{ t('issueCenter.title') }}
          </h1>
          <p class="mt-1 max-w-3xl text-sm text-gray-600 dark:text-gray-400">
            {{ t('issueCenter.description') }}
          </p>
        </div>
        <button class="btn btn-primary w-full sm:w-auto" data-testid="new-issue-button" @click="goNewIssue">
          {{ t('issueCenter.newIssue') }}
        </button>
      </header>

      <section class="space-y-3">
        <div
          class="inline-flex max-w-full gap-1 overflow-x-auto rounded-lg border border-gray-200 bg-white p-1 shadow-sm dark:border-dark-700 dark:bg-dark-800"
          data-testid="issue-feed-tabs"
        >
          <button
            v-for="mode in feedModes"
            :key="mode.value"
            type="button"
            :class="feedMode === mode.value ? 'feed-tab-active' : 'feed-tab'"
            @click="setFeedMode(mode.value)"
          >
            {{ t(mode.labelKey) }}
          </button>
        </div>

        <form class="rounded-lg border border-gray-200 bg-white p-3 shadow-sm dark:border-dark-700 dark:bg-dark-800" @submit.prevent="applyFilters">
          <div class="flex flex-col gap-2 md:flex-row">
            <input
              v-model.trim="filters.q"
              class="input min-h-11 flex-1"
              type="search"
              name="q"
              :placeholder="t('issueCenter.search.placeholder')"
              data-testid="issue-search-input"
            />
            <div class="grid grid-cols-2 gap-2 sm:flex">
              <button class="btn btn-primary" type="submit" :disabled="loading">
                {{ t('common.search') }}
              </button>
              <button class="btn btn-secondary" type="button" @click="clearFilters">
                {{ t('issueCenter.search.clear') }}
              </button>
            </div>
          </div>

          <div class="mt-3 flex gap-2 overflow-x-auto pb-1" data-testid="issue-category-shortcuts">
            <button
              type="button"
              :class="filters.category === '' ? 'category-pill-active' : 'category-pill'"
              @click="setCategory('')"
            >
              {{ t('common.all') }}
            </button>
            <button
              v-for="category in categories"
              :key="category"
              type="button"
              :class="filters.category === category ? 'category-pill-active' : 'category-pill'"
              @click="setCategory(category)"
            >
              {{ t(`issueCenter.category.${category}`) }}
            </button>
          </div>

          <details class="mt-2 rounded-md border border-gray-200 bg-gray-50/70 px-3 py-2 dark:border-dark-700 dark:bg-dark-900/40">
            <summary class="cursor-pointer select-none text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('issueCenter.search.moreFilters') }}
              <span v-if="activeFilterCount" class="ml-1 text-primary-600 dark:text-primary-400">{{ activeFilterCount }}</span>
            </summary>

            <div class="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
              <label class="block">
                <span class="input-label">{{ t('issueCenter.fields.status') }}</span>
                <select v-model="filters.status" class="input" data-testid="issue-status-filter">
                  <option value="">{{ t('common.all') }}</option>
                  <option v-for="status in statuses" :key="status" :value="status">
                    {{ t(`issueCenter.status.${status}`) }}
                  </option>
                </select>
              </label>
              <label class="block">
                <span class="input-label">{{ t('issueCenter.fields.category') }}</span>
                <select v-model="filters.category" class="input" data-testid="issue-category-filter">
                  <option value="">{{ t('common.all') }}</option>
                  <option v-for="category in categories" :key="category" :value="category">
                    {{ t(`issueCenter.category.${category}`) }}
                  </option>
                </select>
              </label>
              <label class="block">
                <span class="input-label">{{ t('issueCenter.fields.severity') }}</span>
                <select v-model="filters.severity" class="input" data-testid="issue-severity-filter">
                  <option value="">{{ t('common.all') }}</option>
                  <option v-for="severity in severities" :key="severity" :value="severity">
                    {{ t(`issueCenter.severity.${severity}`) }}
                  </option>
                </select>
              </label>
              <label class="block">
                <span class="input-label">{{ t('issueCenter.fields.hasImage') }}</span>
                <select v-model="hasImageFilter" class="input" data-testid="issue-has-image-filter">
                  <option value="">{{ t('common.all') }}</option>
                  <option value="true">{{ t('common.yes') }}</option>
                  <option value="false">{{ t('common.no') }}</option>
                </select>
              </label>
            </div>
          </details>

          <details class="mt-2 rounded-md border border-dashed border-gray-200 px-3 py-2 text-xs leading-5 text-gray-500 dark:border-dark-700 dark:text-gray-400">
            <summary class="cursor-pointer select-none font-medium">{{ t('issueCenter.search.syntaxIntro') }}</summary>
            <div class="mt-2 flex flex-wrap gap-1.5">
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
              <code class="code">"{{ t('issueCenter.search.exactPhrase') }}"</code>
            </div>
          </details>
        </form>
      </section>

      <section class="min-h-[260px]">
        <div v-if="loading" class="flex items-center justify-center py-16">
          <LoadingSpinner />
        </div>
        <div
          v-else-if="errorMessage"
          class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300"
          data-testid="issues-error"
        >
          {{ errorMessage }}
        </div>
        <div
          v-else-if="issues.length === 0"
          class="rounded-lg border border-gray-200 bg-white px-5 py-10 text-center shadow-sm dark:border-dark-700 dark:bg-dark-800"
          data-testid="issues-empty"
        >
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('issueCenter.empty.title') }}</h2>
          <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">{{ t('issueCenter.empty.description') }}</p>
          <button class="btn btn-primary mt-5" @click="goNewIssue">{{ t('issueCenter.newIssue') }}</button>
        </div>
        <div v-else class="overflow-hidden rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-800" data-testid="issues-list">
          <article
            v-for="issue in issues"
            :key="issue.id"
            class="cursor-pointer border-b border-gray-100 px-4 py-3 transition last:border-b-0 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60"
            data-testid="issue-list-item"
            @click="router.push(`/issues/${issue.id}`)"
          >
            <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="font-mono text-xs font-semibold text-gray-500 dark:text-gray-400">{{ issue.public_id }}</span>
                  <span :class="statusBadgeClass(issue.status)">{{ t(`issueCenter.status.${issue.status}`) }}</span>
                  <span v-if="issue.pinned_at" class="badge badge-primary">{{ t('issueCenter.detail.pinned') }}</span>
                  <span :class="severityBadgeClass(issue.severity)">{{ t(`issueCenter.severity.${issue.severity}`) }}</span>
                  <span class="badge badge-gray">{{ t(`issueCenter.category.${issue.category}`) }}</span>
                </div>
                <h2 class="mt-1 break-words text-base font-semibold text-gray-900 dark:text-white">
                  {{ issue.title }}
                </h2>
                <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                  <span>{{ issue.account_email_masked }}</span>
                  <span>{{ formatDateTime(issue.occurred_at) }}</span>
                  <span>{{ t(`issueCenter.language.${issue.screenshot_language}`) }}</span>
                  <span v-if="issue.model_name">{{ issue.model_name }}</span>
                  <span v-if="issue.client_name">{{ issue.client_name }}</span>
                  <span v-if="issue.http_status">HTTP {{ issue.http_status }}</span>
                  <span v-if="issue.error_code">{{ issue.error_code }}</span>
                </div>
              </div>
              <div class="grid grid-cols-4 gap-2 text-center text-xs text-gray-500 dark:text-gray-400 lg:w-72">
                <div class="issue-stat">
                  <strong>{{ issue.comment_count }}</strong>
                  <span>{{ t('issueCenter.list.commentLabel') }}</span>
                </div>
                <div class="issue-stat">
                  <strong>{{ issue.view_count }}</strong>
                  <span>{{ t('issueCenter.list.viewLabel') }}</span>
                </div>
                <div class="issue-stat">
                  <strong>{{ issue.attachment_count }}</strong>
                  <span>{{ t('issueCenter.list.attachmentLabel') }}</span>
                </div>
                <div class="issue-stat">
                  <strong>{{ formatRelative(issue.last_comment_at || issue.updated_at) }}</strong>
                  <span>{{ t('issueCenter.list.activeLabel') }}</span>
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
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Pagination from '@/components/common/Pagination.vue'
import { issuesAPI } from '@/api/issues'
import { useAuthStore } from '@/stores/auth'
import type {
  PublicSupportIssue,
  SupportIssueCategory,
  SupportIssueSeverity,
  SupportIssueStatus
} from '@/types'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()

const statuses: SupportIssueStatus[] = ['open', 'needs_info', 'in_progress', 'resolved', 'closed']
const categories: SupportIssueCategory[] = ['login', 'payment', 'api_call', 'model_unavailable', 'api_key', 'balance', 'subscription', 'channel', 'other']
const severities: SupportIssueSeverity[] = ['blocked', 'partial', 'intermittent', 'question']
type FeedMode = 'active' | 'mine' | 'latest' | 'popular' | 'replied' | 'hot24'
const feedModes: Array<{ value: FeedMode; labelKey: string }> = [
  { value: 'active', labelKey: 'issueCenter.feed.active' },
  { value: 'mine', labelKey: 'issueCenter.feed.mine' },
  { value: 'latest', labelKey: 'issueCenter.feed.latest' },
  { value: 'popular', labelKey: 'issueCenter.feed.popular' },
  { value: 'replied', labelKey: 'issueCenter.feed.replied' },
  { value: 'hot24', labelKey: 'issueCenter.feed.hot24' },
]

const filters = reactive({
  q: '',
  status: '' as '' | SupportIssueStatus,
  category: '' as '' | SupportIssueCategory,
  severity: '' as '' | SupportIssueSeverity,
})
const hasImageFilter = ref('')
const issues = ref<PublicSupportIssue[]>([])
const loading = ref(false)
const errorMessage = ref('')
const feedMode = ref<FeedMode>('active')
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0,
})

const activeFilterCount = computed(() => {
  return [filters.status, filters.category, filters.severity, hasImageFilter.value].filter(Boolean).length
})

function queryString(value: unknown): string {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

function syncStateFromRoute() {
  filters.q = queryString(route.query.q)
  filters.status = queryString(route.query.status) as '' | SupportIssueStatus
  filters.category = queryString(route.query.category) as '' | SupportIssueCategory
  filters.severity = queryString(route.query.severity) as '' | SupportIssueSeverity
  hasImageFilter.value = queryString(route.query.has_image)
  feedMode.value = normalizeFeedMode(queryString(route.query.tab))
  pagination.page = Number(queryString(route.query.page) || 1) || 1
  pagination.page_size = Number(queryString(route.query.page_size) || 20) || 20
}

function buildQuery() {
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.status ? { status: filters.status } : {}),
    ...(filters.category ? { category: filters.category } : {}),
    ...(filters.severity ? { severity: filters.severity } : {}),
    ...(hasImageFilter.value ? { has_image: hasImageFilter.value } : {}),
    ...(feedMode.value !== 'active' ? { tab: feedMode.value } : {}),
    ...(pagination.page > 1 ? { page: String(pagination.page) } : {}),
    ...(pagination.page_size !== 20 ? { page_size: String(pagination.page_size) } : {}),
  }
}

async function replaceRouteQuery() {
  await router.replace({ path: '/issues', query: buildQuery() })
}

function buildParams() {
  const sort = sortForFeedMode(feedMode.value)
  return {
    ...(filters.q ? { q: filters.q } : {}),
    ...(filters.status ? { status: filters.status } : {}),
    ...(filters.category ? { category: filters.category } : {}),
    ...(filters.severity ? { severity: filters.severity } : {}),
    ...(hasImageFilter.value ? { has_image: hasImageFilter.value === 'true' } : {}),
    page: pagination.page,
    page_size: pagination.page_size,
    sort_by: sort.sort_by,
    sort_order: 'desc' as const,
    ...(feedMode.value === 'hot24' ? { window: '24h' } : {}),
  }
}

async function loadIssues() {
  if (feedMode.value === 'mine' && !authStore.isAuthenticated) {
    router.push({ path: '/login', query: { redirect: route.fullPath || '/issues?tab=mine' } })
    return
  }
  loading.value = true
  errorMessage.value = ''
  try {
    const params = buildParams()
    const result = await loadIssuesForFeed(params)
    issues.value = result.items
    pagination.total = result.total
    pagination.pages = result.pages
  } catch (error) {
    errorMessage.value = getErrorMessage(error)
  } finally {
    loading.value = false
  }
}

function loadIssuesForFeed(params: ReturnType<typeof buildParams>) {
  if (feedMode.value === 'mine') {
    return issuesAPI.mine(params)
  }
  if (feedMode.value === 'hot24') {
    return issuesAPI.trending(params)
  }
  return issuesAPI.list(params)
}

async function setFeedMode(mode: FeedMode) {
  if (mode === 'mine' && !authStore.isAuthenticated) {
    router.push({ path: '/login', query: { redirect: '/issues?tab=mine' } })
    return
  }
  feedMode.value = mode
  pagination.page = 1
  await replaceRouteQuery()
  await loadIssues()
}

async function setCategory(category: '' | SupportIssueCategory) {
  filters.category = category
  pagination.page = 1
  await replaceRouteQuery()
  await loadIssues()
}

function normalizeFeedMode(value: string): FeedMode {
  return feedModes.some((mode) => mode.value === value) ? value as FeedMode : 'active'
}

function sortForFeedMode(mode: FeedMode): { sort_by: string } {
  switch (mode) {
    case 'mine':
      return { sort_by: 'created_at' }
    case 'latest':
      return { sort_by: 'created_at' }
    case 'popular':
      return { sort_by: 'view_count' }
    case 'replied':
      return { sort_by: 'comment_count' }
    case 'hot24':
      return { sort_by: 'hot_24h' }
    default:
      return { sort_by: 'last_comment_at' }
  }
}

async function applyFilters() {
  pagination.page = 1
  await replaceRouteQuery()
  await loadIssues()
}

async function clearFilters() {
  filters.q = ''
  filters.status = ''
  filters.category = ''
  filters.severity = ''
  hasImageFilter.value = ''
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

function goNewIssue() {
  router.push('/issues/new')
}

function getErrorMessage(error: unknown): string {
  if (error && typeof error === 'object' && 'message' in error) {
    return String((error as { message?: unknown }).message || t('issueCenter.errors.loadFailed'))
  }
  return t('issueCenter.errors.loadFailed')
}

function formatDateTime(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatRelative(value?: string | null): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '-'
  const diffMs = Date.now() - date.getTime()
  const minute = 60 * 1000
  const hour = 60 * minute
  const day = 24 * hour
  if (diffMs < minute) return t('issueCenter.list.justNow')
  if (diffMs < hour) return t('issueCenter.list.minutesAgo', { count: Math.max(1, Math.floor(diffMs / minute)) })
  if (diffMs < day) return t('issueCenter.list.hoursAgo', { count: Math.floor(diffMs / hour) })
  if (diffMs < 7 * day) return t('issueCenter.list.daysAgo', { count: Math.floor(diffMs / day) })
  return date.toLocaleDateString()
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

<style scoped>
.feed-tab {
  @apply shrink-0 rounded-md px-3 py-2 text-sm font-medium text-gray-600 transition hover:bg-gray-100 hover:text-gray-900 dark:text-gray-300 dark:hover:bg-dark-700 dark:hover:text-white;
}

.feed-tab-active {
  @apply shrink-0 rounded-md bg-primary-500 px-3 py-2 text-sm font-semibold text-white shadow-sm shadow-primary-500/20;
}

.category-pill {
  @apply shrink-0 rounded-full border border-gray-200 px-3 py-1.5 text-sm text-gray-600 transition hover:border-primary-300 hover:text-primary-700 dark:border-dark-600 dark:text-gray-300 dark:hover:border-primary-700 dark:hover:text-primary-300;
}

.category-pill-active {
  @apply shrink-0 rounded-full border border-primary-500 bg-primary-50 px-3 py-1.5 text-sm font-medium text-primary-700 dark:border-primary-700 dark:bg-primary-950/40 dark:text-primary-200;
}

.issue-stat {
  @apply rounded-md bg-gray-50 px-2 py-1.5 dark:bg-dark-900/70;
}

.issue-stat strong {
  @apply block truncate text-sm font-semibold text-gray-900 dark:text-white;
}

.issue-stat span {
  @apply block truncate text-[11px] text-gray-500 dark:text-gray-400;
}
</style>
