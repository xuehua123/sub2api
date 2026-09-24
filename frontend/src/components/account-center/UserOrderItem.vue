<template>
  <article class="account-order" :data-testid="'order-' + order.id">
    <div class="account-order-top">
      <div>
        <span class="account-eyebrow">{{
          t(
            order.order_type === 'subscription'
              ? 'accountPages.plan'
              : 'accountPages.balance',
          )
        }}</span>
        <h2>
          {{
            order.order_type === 'subscription' && order.plan_id
              ? t('accountPages.planID', { id: order.plan_id })
              : t(
                  order.order_type === 'subscription'
                    ? 'accountPages.plan'
                    : 'accountPages.balance',
                )
          }}
        </h2>
        <div class="account-order-number">
          <code>{{ order.out_trade_no || '#' + order.id }}</code
          ><button
            class="account-icon"
            :title="t('accountPages.copyOrder')"
            :aria-label="t('accountPages.copyOrder')"
            @click="$emit('copy', order.out_trade_no || String(order.id))"
          >
            <Icon name="copy" size="sm" />
          </button>
        </div>
      </div>
      <div class="account-order-amount">
        <span>{{ t('accountPages.paymentAmount') }}</span
        ><strong>{{
          formatPaymentAmount(order.pay_amount, order.currency, locale)
        }}</strong
        ><small>{{ normalizePaymentCurrency(order.currency) }}</small>
      </div>
      <div class="account-order-state">
        <span :class="['account-status', tone]">{{ label }}</span>
        <p>{{ hint }}</p>
      </div>
    </div>
    <div class="account-order-footer">
      <span>{{ formatDateTimeToMinute(order.created_at) }}</span>
      <div class="account-order-actions">
        <button
          v-if="order.status === 'COMPLETED'"
          class="btn btn-secondary btn-sm"
          @click="
            $emit(
              'navigate',
              order.order_type === 'subscription'
                ? '/subscriptions'
                : '/dashboard',
            )
          "
        >
          {{
            t(
              order.order_type === 'subscription'
                ? 'accountPages.viewSubscription'
                : 'accountPages.viewBalance',
            )
          }}</button
        ><slot name="actions" />
      </div>
    </div>
    <details class="account-order-details">
      <summary>{{ t('accountPages.orderDetails') }}</summary>
      <dl>
        <div>
          <dt>{{ t('accountPages.orderNumber') }}</dt>
          <dd>{{ order.out_trade_no || '#' + order.id }}</dd>
        </div>
        <div>
          <dt>{{ t('accountPages.paymentMethod') }}</dt>
          <dd>
            {{ t('payment.methods.' + order.payment_type, order.payment_type) }}
          </dd>
        </div>
        <div v-if="order.order_type === 'balance'">
          <dt>{{ t('accountPages.creditedAmount') }}</dt>
          <dd>{{ formatPaymentAmount(order.amount, 'USD', locale) }}</dd>
        </div>
        <div>
          <dt>{{ t('accountPages.fee') }}</dt>
          <dd>{{ order.fee_rate }}%</dd>
        </div>
        <div>
          <dt>{{ t('accountPages.placed') }}</dt>
          <dd>{{ formatDateTime(order.created_at) }}</dd>
        </div>
        <div>
          <dt>{{ t('accountPages.paid') }}</dt>
          <dd>
            {{
              order.paid_at
                ? formatDateTime(order.paid_at)
                : t('accountPages.notRecorded')
            }}
          </dd>
        </div>
        <div>
          <dt>{{ t('accountPages.fulfilled') }}</dt>
          <dd>
            {{
              order.completed_at
                ? formatDateTime(order.completed_at)
                : t('accountPages.notRecorded')
            }}
          </dd>
        </div>
        <div v-if="order.status === 'PENDING'">
          <dt>{{ t('accountPages.expires') }}</dt>
          <dd>{{ formatDateTime(order.expires_at) }}</dd>
        </div>
      </dl>
    </details>
  </article>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PaymentOrder } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import {
  formatPaymentAmount,
  normalizePaymentCurrency,
} from '@/components/payment/currency'
import { formatDateTime, formatDateTimeToMinute } from '@/utils/format'
import { orderState } from './orderState'
const props = defineProps<{ order: PaymentOrder }>()
defineEmits<{ copy: [value: string]; navigate: [path: string] }>()
const { t, locale } = useI18n()
const state = computed(() => orderState(props.order))
const label = computed(() => t(state.value.label))
const hint = computed(() => t(state.value.hint))
const tone = computed(() => state.value.tone)
</script>
