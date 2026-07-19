<template>
  <div class="flex h-8 min-w-[7rem] items-center gap-2">
    <span
      v-if="connection"
      class="min-w-0 truncate text-sm font-semibold"
      :class="balanceClass"
      :title="connection.last_error || connection.name"
      data-testid="upstream-connection-balance"
    >
      {{ balanceText }}
    </span>
    <span v-else class="text-xs text-gray-400 dark:text-dark-400">
      {{ emptyStateText }}
    </span>
    <button
      v-if="connection"
      type="button"
      class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
      :disabled="refreshing"
      :aria-label="t('admin.accounts.upstreamConnectionBalance.refresh')"
      :title="t('admin.accounts.upstreamConnectionBalance.refresh')"
      data-testid="upstream-connection-balance-refresh"
      @click="$emit('refresh')"
    >
      <Icon name="refresh" size="xs" :class="{ 'animate-spin': refreshing }" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { UpstreamConnection } from '@/api/admin/upstreamConnections'

const props = defineProps<{
  connection?: UpstreamConnection | null
  loading?: boolean
  loadFailed?: boolean
  refreshing?: boolean
}>()

defineEmits<{
  (event: 'refresh'): void
}>()

const { t } = useI18n()

const emptyStateText = computed(() => {
  if (props.loading) return t('admin.accounts.upstreamConnectionBalance.loading')
  if (props.loadFailed) return t('admin.accounts.upstreamConnectionBalance.loadFailed')
  return t('admin.accounts.upstreamConnectionBalance.unbound')
})

const formatAmount = (amount: number, currency: string) => {
  const value = amount.toFixed(2)
  const unit = currency.trim().toUpperCase() || 'USD'
  if (unit === 'USD') return `$${value}`
  if (unit === 'CNY') return `¥${value}`
  return `${unit} ${value}`
}

const balanceText = computed(() => {
  const connection = props.connection
  if (!connection) return ''
  if (connection.wallet_unlimited) return t('admin.accounts.upstreamConnectionBalance.unlimited')
  if (typeof connection.wallet_amount === 'number' && Number.isFinite(connection.wallet_amount)) {
    return formatAmount(connection.wallet_amount, connection.wallet_currency)
  }
  if (typeof connection.wallet_usd === 'number' && Number.isFinite(connection.wallet_usd)) {
    return formatAmount(connection.wallet_usd, 'USD')
  }
  return t('admin.accounts.upstreamConnectionBalance.unknown')
})

const balanceClass = computed(() => {
  const connection = props.connection
  if (!connection) return 'text-gray-500 dark:text-gray-400'
  if (connection.last_error || connection.status === 'auth_error' || connection.status === 'needs_input' || connection.status === 'error') {
    return 'text-red-600 dark:text-red-400'
  }
  if (!connection.wallet_unlimited && connection.wallet_usd != null && connection.wallet_usd < 50) {
    return 'text-amber-600 dark:text-amber-300'
  }
  if (connection.wallet_unlimited || connection.wallet_amount != null || connection.wallet_usd != null) {
    return 'text-emerald-600 dark:text-emerald-300'
  }
  return 'text-gray-500 dark:text-gray-400'
})
</script>
