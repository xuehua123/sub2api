<template>
  <PricePortalLayout :section-label="t('accountPages.orders')">
    <main id="price-main" class="account-workspace">
      <AccountPageHeader section="orders" />
      <!-- Filters -->
      <section class="account-order-filters">
        <div class="flex flex-wrap items-center gap-3">
          <label class="account-filter-label"
            >{{ t('accountPages.status')
            }}<Select
              v-model="currentFilter"
              :options="statusFilters"
              class="w-48"
              @change="handlePageChange(1)"
          /></label>
          <label class="account-filter-label"
            >{{ t('accountPages.orderType')
            }}<select
              v-model="orderType"
              class="input"
              data-testid="order-type-filter"
              @change="handlePageChange(1)"
            >
              <option value="">{{ t('accountPages.all') }}</option>
              <option value="balance">{{ t('accountPages.balance') }}</option>
              <option value="subscription">{{ t('accountPages.plan') }}</option>
            </select></label
          >
          <div class="flex flex-1 items-center justify-end gap-2">
            <button
              @click="fetchOrders"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon
                name="refresh"
                size="md"
                :class="loading ? 'animate-spin' : ''"
              />
            </button>
            <button class="btn btn-primary" @click="router.push('/purchase')">
              {{ t('payment.result.backToRecharge') }}
            </button>
          </div>
        </div>
      </section>

      <!-- Table -->
      <div v-if="loading" class="account-empty" role="status">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="loadError" role="alert" class="account-empty">
        <p>{{ loadError }}</p>
        <button class="btn btn-secondary" @click="fetchOrders">
          {{ t('accountPages.retry') }}
        </button>
      </div>
      <div v-else-if="!orders.length" class="account-empty">
        <h2>{{ t('accountPages.noOrders') }}</h2>
        <p>
          {{
            t(
              currentFilter || orderType
                ? 'accountPages.noFilteredOrders'
                : 'accountPages.noOrdersHint',
            )
          }}
        </p>
      </div>
      <template v-else>
        <UserOrderItem
          v-for="row in orders"
          :key="row.id"
          :order="row"
          @copy="copyOrderNumber"
          @navigate="router.push($event)"
        >
          <template #actions>
            <div class="flex items-center gap-2">
              <button
                v-if="row.status === 'PENDING'"
                :disabled="actionLoading"
                @click="handleCancel(row.id)"
                class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-yellow-600 hover:bg-yellow-50 dark:text-yellow-400 dark:hover:bg-yellow-900/20"
              >
                <Icon name="x" size="sm" />
                <span>{{ t('payment.orders.cancel') }}</span>
              </button>

            </div>
          </template>
        </UserOrderItem>
      </template>

      <!-- Pagination -->
      <Pagination
        class="account-pagination"
        v-if="pagination.total > 0 && !loadError"
        :page="pagination.page"
        :total="pagination.total"
        :page-size="pagination.page_size"
        @update:page="handlePageChange"
        @update:pageSize="handlePageSizeChange"
      />
    </main>

    <!-- Cancel Confirm Dialog -->
    <BaseDialog
      :show="!!cancelTargetId"
      :title="t('payment.orders.cancel')"
      width="narrow"
      :close-on-escape="!actionLoading"
      :show-close-button="!actionLoading"
      @close="!actionLoading && (cancelTargetId = null)"
    >
      <p class="text-sm text-gray-600 dark:text-gray-300">
        {{ t('payment.confirmCancel') }}
      </p>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button
            class="btn btn-secondary"
            :disabled="actionLoading"
            @click="cancelTargetId = null"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            class="btn btn-danger"
            :disabled="actionLoading"
            @click="confirmCancel"
          >
            {{
              actionLoading
                ? t('common.processing')
                : t('payment.orders.cancel')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

  </PricePortalLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import type { PaymentOrder } from '@/types/payment'
import PricePortalLayout from '@/components/model-price/PricePortalLayout.vue'
import AccountPageHeader from '@/components/account-center/AccountPageHeader.vue'
import UserOrderItem from '@/components/account-center/UserOrderItem.vue'
import '@/components/account-center/account-center.css'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const loading = ref(false)
const actionLoading = ref(false)
const orders = ref<PaymentOrder[]>([])
const currentFilter = ref('')
const orderType = ref('')
const loadError = ref('')
let requestVersion = 0
const cancelTargetId = ref<number | null>(null)
const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const statusFilters = computed(() => [
  { value: '', label: t('common.all') },
  { value: 'PENDING', label: t('payment.status.pending') },
  { value: 'PAID', label: t('accountPages.statusPaid') },
  { value: 'RECHARGING', label: t('accountPages.statusProcessing') },
  { value: 'COMPLETED', label: t('accountPages.statusCompleted') },
  { value: 'EXPIRED', label: t('payment.status.expired') },
  { value: 'CANCELLED', label: t('payment.status.cancelled') },
  { value: 'REFUND_REQUESTED', label: t('payment.status.refund_requested') },
  { value: 'REFUNDING', label: t('payment.status.refunding') },
  { value: 'REFUND_PENDING', label: t('payment.status.refund_pending') },
  {
    value: 'PARTIALLY_REFUNDED',
    label: t('payment.status.partially_refunded'),
  },
  { value: 'REFUND_FAILED', label: t('payment.status.refund_failed') },
  { value: 'FAILED', label: t('payment.status.failed') },
  { value: 'REFUNDED', label: t('payment.status.refunded') },
])

async function fetchOrders() {
  const version = ++requestVersion
  loading.value = true
  loadError.value = ''
  try {
    const res = await paymentAPI.getMyOrders({
      page: pagination.page,
      page_size: pagination.page_size,
      status: currentFilter.value || undefined,
      ...(orderType.value ? { order_type: orderType.value } : {}),
    })
    if (version !== requestVersion) return
    orders.value = res.data.items || []
    pagination.total = res.data.total || 0
  } catch (err: unknown) {
    if (version !== requestVersion) return
    loadError.value = extractI18nErrorMessage(
      err,
      t,
      'payment.errors',
      t('common.error'),
    )
    appStore.showError(loadError.value)
  } finally {
    if (version === requestVersion) loading.value = false
  }
}

function handlePageChange(page: number) {
  pagination.page = page
  fetchOrders()
}
function handlePageSizeChange(size: number) {
  pagination.page_size = size
  pagination.page = 1
  fetchOrders()
}

function handleCancel(orderId: number) {
  cancelTargetId.value = orderId
}

async function confirmCancel() {
  if (!cancelTargetId.value || actionLoading.value) return
  actionLoading.value = true
  try {
    await paymentAPI.cancelOrder(cancelTargetId.value)
    appStore.showSuccess(t('common.success'))
    cancelTargetId.value = null
    await fetchOrders()
  } catch (err: unknown) {
    appStore.showError(
      extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')),
    )
  } finally {
    actionLoading.value = false
  }
}

async function copyOrderNumber(value: string) {
  try {
    await navigator.clipboard.writeText(value)
    appStore.showSuccess(t('common.copied'))
  } catch {
    appStore.showError(t('common.error'))
  }
}
onMounted(() => {
  fetchOrders()
})
onUnmounted(() => {
  requestVersion++
})
</script>
