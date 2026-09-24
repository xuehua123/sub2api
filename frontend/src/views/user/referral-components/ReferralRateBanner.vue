<template>
  <section
    data-test="referral-rate-banner"
    class="referral-rate-band"
  >
    <div class="grid lg:grid-cols-[1.4fr_1fr]">
      <div class="relative px-7 py-9 sm:px-10 sm:py-11">


        <div class="inline-flex items-center gap-1.5 rounded-full bg-[var(--ppx-tint)] px-3 py-1 text-[12px] font-semibold text-[var(--ppx-accent)]">
          <ReferralIcon name="gift" :size="14" />
          {{ t('referral.rateBanner.live', '邀请有礼进行中') }}
        </div>

        <h2 class="relative mt-4 max-w-xl text-[24px] font-semibold leading-[1.15] tracking-normal text-[var(--ppx-ink)] sm:text-[26px] ">
          <template v-if="hasRate">
            {{ t('referral.rateBanner.headlineBefore') }}
            <span class="text-[var(--ppx-accent)]">{{ ratePct }}%</span>
            {{ t('referral.rateBanner.headlineAfter') }}
          </template>
          <template v-else-if="!level1On">
            {{ t('referral.rateBanner.titleDisabled') }}
          </template>
          <template v-else>
            {{ t('referral.rateBanner.titleNoRate') }}
          </template>
        </h2>

        <p class="relative mt-3 max-w-lg text-[15px] leading-relaxed text-[var(--ppx-muted)] ">
          {{ subtitle }}
        </p>

        <div class="relative mt-6 flex flex-wrap gap-x-5 gap-y-2 text-[13px] font-medium text-[var(--ppx-ink)] ">
          <span class="inline-flex items-center gap-1.5">
            <ReferralIcon name="check" :size="16" class="text-[var(--ppx-accent)]" />
            {{ t('referral.rateBanner.bulletPermanent') }}
          </span>
          <span class="inline-flex items-center gap-1.5">
            <ReferralIcon name="check" :size="16" class="text-[var(--ppx-accent)]" />
            {{ rechargeBullet }}
          </span>
          <span v-if="cashoutChip" class="inline-flex items-center gap-1.5">
            <ReferralIcon name="check" :size="16" class="text-[var(--ppx-accent)]" />
            {{ cashoutChip }}
          </span>
        </div>
      </div>

      <div class="border-t border-[var(--ppx-line)] bg-[var(--ppx-soft)] px-7 py-9   sm:px-10 sm:py-11 lg:border-l lg:border-t-0">
        <div class="flex items-center gap-2 text-[13px] font-medium text-[var(--ppx-muted)]">
          <ReferralIcon name="percent" :size="16" />
          {{ t('referral.rateBanner.rateLabel') }}
        </div>
        <p
          data-test="referral-rate-pct"
          class="mt-1 text-[48px] font-semibold leading-none tracking-normal text-[var(--ppx-ink)] "
        >
          {{ ratePct }}<span class="text-[28px] font-medium text-[var(--ppx-muted)]">%</span>
        </p>
        <p class="mt-2 text-[13px] text-[var(--ppx-muted)]" data-test="referral-rate-scope">
          {{ rechargeScopeLabel }}
        </p>

        <div v-if="hasRate" class="mt-7 space-y-0" data-test="referral-rate-examples">
          <div
            v-for="ex in examples"
            :key="ex.pay"
            class="flex items-center justify-between border-t border-[var(--ppx-line)] py-3 first:border-t-0 first:pt-0 "
          >
            <span class="inline-flex items-center gap-2 text-[14px] text-[var(--ppx-muted)] ">
              <ReferralIcon name="users" :size="15" class="opacity-60" />
              {{ rechargePayLabel }} ¥{{ ex.pay }}
            </span>
            <span class="inline-flex items-center gap-1 text-[15px] font-semibold tabular-nums text-[var(--ppx-accent)]">
              <ReferralIcon name="arrow" :size="14" />
              {{ t('referral.rateBanner.youEarn') }} +¥{{ ex.earn }}
            </span>
          </div>
        </div>
        <p v-else class="mt-7 text-[13px] text-[var(--ppx-muted)]" data-test="referral-rate-pending">
          {{ ratePendingLabel }}
        </p>

        <p class="mt-5 text-[12px] leading-relaxed text-[var(--ppx-muted)]">
          {{ footnote }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ReferralIcon from './ReferralIcons.vue'

const props = defineProps<{
  level1Rate?: number
  level1Enabled?: boolean
  rewardMode?: string
  settlementDelayDays?: number
  withdrawEnabled?: boolean
  creditConversionEnabled?: boolean
  creditConversionRate?: number
}>()

const { t } = useI18n()

const level1On = computed(() => props.level1Enabled !== false)
const isEveryPaid = computed(() => props.rewardMode === 'every_paid_order')

const hasRate = computed(
  () => level1On.value && Number(props.level1Rate || 0) > 0
)
const rateFraction = computed(() => (hasRate.value ? Number(props.level1Rate) : 0))

const ratePct = computed(() => {
  if (!hasRate.value) return '—'
  const pct = rateFraction.value * 100
  return pct % 1 === 0 ? String(pct) : pct.toFixed(1)
})

const rechargeScopeLabel = computed(() => {
  if (!hasRate.value) return t('referral.rateBanner.perRechargeDisabled')
  return isEveryPaid.value
    ? t('referral.rateBanner.perRechargeEvery')
    : t('referral.rateBanner.perRechargeFirst')
})

const rechargePayLabel = computed(() =>
  isEveryPaid.value
    ? t('referral.rateBanner.friendPaysEvery')
    : t('referral.rateBanner.friendPaysFirst')
)

const rechargeBullet = computed(() => {
  if (!level1On.value) return t('referral.rateBanner.bulletDisabled')
  // rate=0 also means no booking — do not claim auto-credit.
  if (!hasRate.value) return t('referral.rateBanner.bulletNoRate')
  return isEveryPaid.value
    ? t('referral.rateBanner.bulletAutoEvery')
    : t('referral.rateBanner.bulletAutoFirst')
})

const ratePendingLabel = computed(() =>
  !level1On.value
    ? t('referral.rateBanner.rateDisabled')
    : t('referral.rateBanner.ratePending')
)

const subtitle = computed(() => {
  const days = Number(props.settlementDelayDays || 0)
  if (!level1On.value) {
    return t('referral.rateBanner.subtitleDisabled')
  }
  if (hasRate.value && days > 0) {
    return isEveryPaid.value
      ? t('referral.rateBanner.subtitleEveryWithDelay', { pct: ratePct.value, days })
      : t('referral.rateBanner.subtitleFirstWithDelay', { pct: ratePct.value, days })
  }
  if (hasRate.value) {
    return isEveryPaid.value
      ? t('referral.rateBanner.subtitleEvery', { pct: ratePct.value })
      : t('referral.rateBanner.subtitleFirst', { pct: ratePct.value })
  }
  // L1 on but rate 0 / unset — never fall back to "recharge and earn".
  return t('referral.rateBanner.subtitleNoRate')
})

const cashoutChip = computed(() => {
  if (props.withdrawEnabled && props.creditConversionEnabled) {
    return t('referral.rateBanner.bulletCashAndCredit')
  }
  if (props.withdrawEnabled) return t('referral.rateBanner.bulletWithdraw')
  if (props.creditConversionEnabled) return t('referral.rateBanner.bulletCredit')
  return ''
})

const examples = computed(() => {
  if (!hasRate.value) return [] as { pay: string; earn: string }[]
  const rate = rateFraction.value
  return [100, 500, 1000].map((pay) => ({
    pay: String(pay),
    earn: (pay * rate).toFixed(pay * rate % 1 === 0 ? 0 : 1)
  }))
})

const footnote = computed(() => {
  const days = Number(props.settlementDelayDays || 0)
  if (days > 0) return t('referral.rateBanner.footnoteDelay', { days })
  return t('referral.rateBanner.footnote')
})
</script>
