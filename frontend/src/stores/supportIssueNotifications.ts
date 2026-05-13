import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { issuesAPI } from '@/api/issues'
import type { SupportIssueNotificationSummary } from '@/types'

const REFRESH_INTERVAL_MS = 60 * 1000

export const useSupportIssueNotificationStore = defineStore('supportIssueNotifications', () => {
  const summary = ref<SupportIssueNotificationSummary>({
    unread_count: 0,
    needs_info_count: 0,
    resolved_unread_count: 0,
    latest_activity_at: null,
  })
  const loaded = ref(false)
  const loading = ref(false)
  let refreshTimer: ReturnType<typeof setInterval> | null = null

  const unreadCount = computed(() => summary.value.unread_count)
  const hasUnread = computed(() => unreadCount.value > 0)

  async function refresh(): Promise<void> {
    if (loading.value) return
    loading.value = true
    try {
      summary.value = await issuesAPI.notifications()
      loaded.value = true
    } finally {
      loading.value = false
    }
  }

  function reset(): void {
    summary.value = {
      unread_count: 0,
      needs_info_count: 0,
      resolved_unread_count: 0,
      latest_activity_at: null,
    }
    loaded.value = false
  }

  function start(): void {
    if (refreshTimer) return
    refresh().catch(() => {
      reset()
    })
    refreshTimer = setInterval(() => {
      refresh().catch(() => {
        reset()
      })
    }, REFRESH_INTERVAL_MS)
  }

  function stop(): void {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
    reset()
  }

  return {
    summary,
    loaded,
    loading,
    unreadCount,
    hasUnread,
    refresh,
    start,
    stop,
  }
})
