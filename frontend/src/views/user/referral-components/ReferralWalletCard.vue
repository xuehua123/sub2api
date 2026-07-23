<template>
  <section
    data-test="referral-wallet-card"
    class="flex h-full flex-col rounded-[28px] border border-black/[0.06] bg-white p-7 dark:border-white/10 dark:bg-[#1c1c1e]"
  >
    <div class="flex gap-3">
      <span class="flex h-11 w-11 items-center justify-center rounded-[14px] bg-[#0071e3]/10 text-[#0071e3]">
        <ReferralIcon name="wallet" :size="22" />
      </span>
      <div>
        <p class="text-[13px] font-medium text-[#86868b]">{{ t('referral.walletCardEyebrow') }}</p>
        <h2 class="mt-0.5 text-[22px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
          {{ t('referral.walletCardTitle') }}
        </h2>
        <p class="mt-1 text-[14px] leading-relaxed text-[#6e6e73] dark:text-[#a1a1a6]">
          <template v-if="ratePct">{{ t('referral.walletCardDescWithRate', { pct: ratePct }) }}</template>
          <template v-else>{{ t('referral.walletCardDesc') }}</template>
        </p>
      </div>
    </div>

    <button
      type="button"
      class="mt-6 w-full rounded-[20px] bg-[#f5f5f7] px-5 py-5 text-left transition hover:bg-[#ebebed] dark:bg-[#2c2c2e] dark:hover:bg-[#3a3a3c]"
      @click="$emit('openBucket', 'available', t('referral.availableCommission'))"
    >
      <p class="text-[13px] font-medium text-[#86868b]">{{ t('referral.availableCommission') }}</p>
      <p class="mt-1 text-[44px] font-semibold leading-none tracking-tight tabular-nums text-[#1d1d1f] dark:text-white">
        <span class="text-[22px] font-medium text-[#86868b]">¥</span>{{ formatMoney(available) }}
      </p>
      <p class="mt-2 inline-flex items-center gap-1 text-[12px] font-medium text-[#0071e3]">
        {{ t('referral.clickToViewDetail') }}
        <ReferralIcon name="arrow" :size="12" />
      </p>
    </button>

    <div class="mt-3 grid grid-cols-3 gap-2">
      <button
        type="button"
        class="rounded-[16px] bg-[#f5f5f7] px-3 py-3 text-left transition hover:bg-[#ebebed] dark:bg-[#2c2c2e] dark:hover:bg-[#3a3a3c]"
        @click="$emit('openBucket', 'processing', t('referral.processingCommission'))"
      >
        <p class="text-[11px] font-medium text-[#86868b]">{{ t('referral.processingCommission') }}</p>
        <p class="mt-1 text-[15px] font-semibold tabular-nums text-[#1d1d1f] dark:text-white">
          ¥{{ formatMoney(processing) }}
        </p>
      </button>
      <button
        type="button"
        class="rounded-[16px] bg-[#f5f5f7] px-3 py-3 text-left transition hover:bg-[#ebebed] dark:bg-[#2c2c2e] dark:hover:bg-[#3a3a3c]"
        @click="$emit('openBucket', 'settled', t('referral.withdrawnCommission'))"
      >
        <p class="text-[11px] font-medium text-[#86868b]">{{ t('referral.withdrawnCommission') }}</p>
        <p class="mt-1 text-[15px] font-semibold tabular-nums text-[#1d1d1f] dark:text-white">
          ¥{{ formatMoney(withdrawn) }}
        </p>
      </button>
      <button
        type="button"
        class="rounded-[16px] bg-[#f5f5f7] px-3 py-3 text-left transition hover:bg-[#ebebed] dark:bg-[#2c2c2e] dark:hover:bg-[#3a3a3c]"
        @click="$emit('openBucket', 'available', t('referral.totalCommission'))"
      >
        <p class="text-[11px] font-medium text-[#86868b]">{{ t('referral.totalCommission') }}</p>
        <p class="mt-1 text-[15px] font-semibold tabular-nums text-[#1d1d1f] dark:text-white">
          ¥{{ formatMoney(total) }}
        </p>
      </button>
    </div>

    <div class="mt-auto space-y-2.5 pt-6">
      <div
        v-if="creditConversionEnabled"
        data-test="credit-conversion-rate-hint"
        :data-conversion-rate="conversionMultiplierText"
        class="rounded-[16px] bg-[#0071e3]/[0.08] px-4 py-3.5"
      >
        <div class="flex items-start gap-2.5">
          <span class="mt-0.5 flex h-8 w-8 items-center justify-center rounded-full bg-[#0071e3]/15 text-[#0071e3]">
            <ReferralIcon name="convert" :size="16" />
          </span>
          <div class="min-w-0 flex-1">
            <p class="text-[12px] font-medium text-[#0071e3]">{{ t('referral.creditConversionRateLabel') }}</p>
            <p class="mt-0.5 text-[15px] font-semibold text-[#1d1d1f] dark:text-white">
              {{ conversionRateLabel }}
            </p>
            <p class="mt-0.5 text-[12px] text-[#6e6e73] dark:text-[#a1a1a6]">
              {{ conversionExample }}
            </p>
          </div>
        </div>
      </div>

      <div class="flex flex-col gap-2.5 sm:flex-row">
        <button
          v-if="withdrawEnabled"
          type="button"
          data-test="scroll-to-withdraw"
          class="inline-flex h-12 flex-1 items-center justify-center gap-2 rounded-full bg-[#1d1d1f] text-[15px] font-medium text-white transition hover:bg-black active:scale-[0.98] dark:bg-white dark:text-black"
          @click="$emit('scrollWithdraw')"
        >
          <ReferralIcon name="wallet" :size="16" />
          {{ t('referral.withdrawal') }}
        </button>
        <button
          v-if="creditConversionEnabled"
          type="button"
          data-test="open-convert-credit"
          class="inline-flex h-12 flex-1 items-center justify-center gap-2 rounded-full bg-[#0071e3] text-[15px] font-medium text-white transition hover:bg-[#0077ed] active:scale-[0.98] disabled:opacity-40"
          :disabled="!available"
          @click="$emit('openConvert')"
        >
          <ReferralIcon name="convert" :size="16" />
          {{ convertButtonLabel }}
        </button>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ReferralIcon from './ReferralIcons.vue'

const props = defineProps<{
  available: number
  processing: number
  withdrawn: number
  total: number
  withdrawEnabled: boolean
  creditConversionEnabled: boolean
  /** From overview.referral_credit_conversion_rate (admin setting). */
  creditConversionRate?: number
  level1Rate?: number
}>()

defineEmits<{
  openBucket: [bucket: string, title: string]
  openConvert: []
  scrollWithdraw: []
}>()

const { t } = useI18n()

const ratePct = computed(() => {
  const rate = Number(props.level1Rate || 0)
  if (rate <= 0) return ''
  const pct = rate * 100
  return pct % 1 === 0 ? String(pct) : pct.toFixed(1)
})

const conversionMultiplier = computed(() => {
  const rate = Number(props.creditConversionRate || 1)
  return rate > 0 ? rate : 1
})

const conversionMultiplierText = computed(() => {
  const m = conversionMultiplier.value
  return m % 1 === 0 ? String(m) : m.toFixed(2)
})

const conversionRateLabel = computed(() => {
  const m = conversionMultiplier.value
  if (m === 1) return t('referral.creditConversionRateOneToOne')
  return t('referral.creditConversionRateMulti', { rate: conversionMultiplierText.value })
})

const conversionExample = computed(() => {
  const m = conversionMultiplier.value
  const sample = 100
  const out = (sample * m).toFixed(m % 1 === 0 ? 0 : 2)
  return t('referral.creditConversionExample', { in: sample, out })
})

const convertButtonLabel = computed(() => {
  const m = conversionMultiplier.value
  if (m === 1) return t('referral.convertToCredit')
  return t('referral.convertToCreditMulti', { rate: conversionMultiplierText.value })
})

function formatMoney(value: number) {
  return `${Number(value || 0).toFixed(2)}`
}
</script>
