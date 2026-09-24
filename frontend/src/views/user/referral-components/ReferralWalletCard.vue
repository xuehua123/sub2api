<template>
  <section
    data-test="referral-wallet-card"
    class="flex h-full flex-col rounded-lg border border-[var(--ppx-line)] bg-[var(--ppx-panel)] p-7  "
  >
    <div class="flex gap-3">
      <span class="flex h-11 w-11 items-center justify-center rounded-md bg-[var(--ppx-tint)] text-[var(--ppx-accent)]">
        <ReferralIcon name="wallet" :size="22" />
      </span>
      <div>
        <p class="text-[13px] font-medium text-[var(--ppx-muted)]">{{ t('referral.walletCardEyebrow') }}</p>
        <h2 class="mt-0.5 text-[20px] font-semibold tracking-normal text-[var(--ppx-ink)] ">
          {{ walletTitle }}
        </h2>
        <p class="mt-1 text-[14px] leading-relaxed text-[var(--ppx-muted)] ">
          {{ walletDesc }}
        </p>
      </div>
    </div>

    <button
      type="button"
      class="mt-6 w-full rounded-md bg-[var(--ppx-soft)] px-5 py-5 text-left transition hover:bg-[var(--ppx-tint)]  "
      @click="$emit('openBucket', 'available', t('referral.availableCommission'))"
    >
      <p class="text-[13px] font-medium text-[var(--ppx-muted)]">{{ t('referral.availableCommission') }}</p>
      <p class="mt-1 text-[36px] font-semibold leading-none tracking-normal tabular-nums text-[var(--ppx-ink)] ">
        <span class="text-[20px] font-medium text-[var(--ppx-muted)]">¥</span>{{ formatMoney(available) }}
      </p>
      <p class="mt-2 inline-flex items-center gap-1 text-[12px] font-medium text-[var(--ppx-accent)]">
        {{ t('referral.clickToViewDetail') }}
        <ReferralIcon name="arrow" :size="12" />
      </p>
    </button>

    <div class="mt-3 grid grid-cols-3 gap-2">
      <button
        type="button"
        class="rounded-md bg-[var(--ppx-soft)] px-3 py-3 text-left transition hover:bg-[var(--ppx-tint)]  "
        @click="$emit('openBucket', 'processing', t('referral.processingCommission'))"
      >
        <p class="text-[11px] font-medium text-[var(--ppx-muted)]">{{ t('referral.processingCommission') }}</p>
        <p class="mt-1 text-[15px] font-semibold tabular-nums text-[var(--ppx-ink)] ">
          ¥{{ formatMoney(processing) }}
        </p>
      </button>
      <button
        type="button"
        class="rounded-md bg-[var(--ppx-soft)] px-3 py-3 text-left transition hover:bg-[var(--ppx-tint)]  "
        @click="$emit('openBucket', 'settled', t('referral.withdrawnCommission'))"
      >
        <p class="text-[11px] font-medium text-[var(--ppx-muted)]">{{ t('referral.withdrawnCommission') }}</p>
        <p class="mt-1 text-[15px] font-semibold tabular-nums text-[var(--ppx-ink)] ">
          ¥{{ formatMoney(withdrawn) }}
        </p>
      </button>
      <button
        type="button"
        class="rounded-md bg-[var(--ppx-soft)] px-3 py-3 text-left transition hover:bg-[var(--ppx-tint)]  "
        @click="$emit('openBucket', 'all', t('referral.totalCommission'))"
      >
        <p class="text-[11px] font-medium text-[var(--ppx-muted)]">{{ t('referral.totalCommission') }}</p>
        <p class="mt-1 text-[15px] font-semibold tabular-nums text-[var(--ppx-ink)] ">
          ¥{{ formatMoney(total) }}
        </p>
      </button>
    </div>

    <div class="mt-auto space-y-2.5 pt-6">
      <div
        v-if="creditConversionEnabled"
        data-test="credit-conversion-rate-hint"
        :data-conversion-rate="conversionMultiplierText"
        class="rounded-md bg-[var(--ppx-tint)] px-4 py-3.5"
      >
        <div class="flex items-start gap-2.5">
          <span class="mt-0.5 flex h-8 w-8 items-center justify-center rounded-full bg-[var(--ppx-tint)] text-[var(--ppx-accent)]">
            <ReferralIcon name="convert" :size="16" />
          </span>
          <div class="min-w-0 flex-1">
            <p class="text-[12px] font-medium text-[var(--ppx-accent)]">{{ t('referral.creditConversionRateLabel') }}</p>
            <p class="mt-0.5 text-[15px] font-semibold text-[var(--ppx-ink)] ">
              {{ conversionRateLabel }}
            </p>
            <p class="mt-0.5 text-[12px] text-[var(--ppx-muted)] ">
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
          class="inline-flex h-12 flex-1 items-center justify-center gap-2 rounded-full bg-[var(--ppx-accent)] text-[var(--ppx-accent-ink)] text-[15px] font-medium transition hover:bg-black active:scale-[0.98]  "
          @click="$emit('scrollWithdraw')"
        >
          <ReferralIcon name="wallet" :size="16" />
          {{ t('referral.withdrawal') }}
        </button>
        <button
          v-if="creditConversionEnabled"
          type="button"
          data-test="open-convert-credit"
          class="inline-flex h-12 flex-1 items-center justify-center gap-2 rounded-full bg-[var(--ppx-accent)] text-[var(--ppx-accent-ink)] text-[15px] font-medium transition hover:opacity-90 active:scale-[0.98] disabled:opacity-40"
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

const walletTitle = computed(() => {
  if (props.withdrawEnabled) return t('referral.walletCardTitle')
  if (props.creditConversionEnabled) return t('referral.walletCardTitleConvertOnly')
  return t('referral.walletCardTitleBalanceOnly')
})

const walletDesc = computed(() => {
  if (ratePct.value && props.withdrawEnabled && props.creditConversionEnabled) {
    return t('referral.walletCardDescWithRate', { pct: ratePct.value })
  }
  if (ratePct.value && props.withdrawEnabled) {
    return t('referral.walletCardDescWithRateWithdrawOnly', { pct: ratePct.value })
  }
  if (ratePct.value && props.creditConversionEnabled) {
    return t('referral.walletCardDescWithRateConvertOnly', { pct: ratePct.value })
  }
  if (props.withdrawEnabled && props.creditConversionEnabled) return t('referral.walletCardDesc')
  if (props.withdrawEnabled) return t('referral.walletCardDescWithdrawOnly')
  if (props.creditConversionEnabled) return t('referral.walletCardDescConvertOnly')
  return t('referral.walletCardDescDisabled')
})

const conversionMultiplier = computed(() => {
  const rate = Number(props.creditConversionRate || 1)
  return rate > 0 ? rate : 1
})

function formatRateDisplay(rate: number): string {
  if (rate % 1 === 0) return String(rate)
  return rate.toFixed(8).replace(/\.?0+$/, '')
}

function roundMoney8(value: number): number {
  return Math.round(value * 1e8) / 1e8
}

const conversionMultiplierText = computed(() => formatRateDisplay(conversionMultiplier.value))

const conversionRateLabel = computed(() => {
  const m = conversionMultiplier.value
  if (m === 1) return t('referral.creditConversionRateOneToOne')
  return t('referral.creditConversionRateMulti', { rate: conversionMultiplierText.value })
})

const conversionExample = computed(() => {
  const m = conversionMultiplier.value
  const sample = 100
  const out = roundMoney8(sample * m)
  const outText = out % 1 === 0 ? String(out) : out.toFixed(8).replace(/\.?0+$/, '')
  return t('referral.creditConversionExample', { in: sample, out: outText })
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
