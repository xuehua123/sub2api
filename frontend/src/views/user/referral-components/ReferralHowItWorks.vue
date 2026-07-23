<template>
  <section
    data-test="referral-how-it-works"
    class="rounded-[28px] border border-black/[0.06] bg-white p-7 dark:border-white/10 dark:bg-[#1c1c1e]"
  >
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex items-center gap-2.5">
        <span class="flex h-9 w-9 items-center justify-center rounded-full bg-[#0071e3]/10 text-[#0071e3]">
          <ReferralIcon name="spark" :size="18" />
        </span>
        <h3 class="text-[17px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
          {{ t('referral.howItWorksTitle') }}
        </h3>
      </div>
      <span
        v-if="rateLabel"
        class="inline-flex items-center gap-1 rounded-full bg-[#0071e3]/10 px-3 py-1 text-[12px] font-semibold text-[#0071e3]"
      >
        <ReferralIcon name="percent" :size="13" />
        {{ rateLabel }}
      </span>
    </div>

    <ol class="mt-6 grid grid-cols-1 gap-3 md:grid-cols-3">
      <li
        v-for="(step, index) in steps"
        :key="step.key"
        class="relative rounded-[20px] bg-[#f5f5f7] px-5 py-5 dark:bg-[#2c2c2e]"
      >
        <div class="flex items-center gap-2.5">
          <span class="flex h-8 w-8 items-center justify-center rounded-full bg-[#1d1d1f] text-white dark:bg-white dark:text-black">
            <ReferralIcon :name="step.icon" :size="16" />
          </span>
          <span class="text-[12px] font-medium text-[#86868b]">0{{ index + 1 }}</span>
        </div>
        <p class="mt-3 text-[15px] font-semibold text-[#1d1d1f] dark:text-white">{{ step.title }}</p>
        <p class="mt-1 text-[13px] leading-relaxed text-[#6e6e73] dark:text-[#a1a1a6]">{{ step.desc }}</p>
      </li>
    </ol>
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

const ratePctText = computed(() => {
  if (!level1On.value) return ''
  const rate = Number(props.level1Rate || 0)
  if (rate <= 0) return ''
  const pct = rate * 100
  return pct % 1 === 0 ? String(pct) : pct.toFixed(1)
})

const rateLabel = computed(() =>
  ratePctText.value ? t('referral.rateChip', { pct: ratePctText.value }) : ''
)

function formatRateDisplay(rate: number): string {
  if (rate % 1 === 0) return String(rate)
  return rate.toFixed(8).replace(/\.?0+$/, '')
}

const convertHint = computed(() => {
  if (!props.creditConversionEnabled) return ''
  const m = Number(props.creditConversionRate || 1)
  const mText = formatRateDisplay(m)
  if (m === 1) return t('referral.howItWorks.step3Credit')
  return t('referral.howItWorks.step3CreditMulti', { rate: mText })
})

const steps = computed(() => {
  const days = Number(props.settlementDelayDays || 0)
  const pct = ratePctText.value

  // Only promise booking when L1 is on AND rate > 0 (matches reward service rate<=0 skip).
  let earnTitle = t('referral.howItWorks.step2TitleNoCommission')
  let earnDesc = t('referral.howItWorks.step2NoCommission')
  if (!level1On.value) {
    earnTitle = t('referral.howItWorks.step2TitleDisabled')
    earnDesc = t('referral.howItWorks.step2Disabled')
  } else if (pct && days > 0) {
    earnTitle = isEveryPaid.value
      ? t('referral.howItWorks.step2TitleEveryWithRate', { pct })
      : t('referral.howItWorks.step2TitleFirstWithRate', { pct })
    earnDesc = isEveryPaid.value
      ? t('referral.howItWorks.step2EveryWithRateAndDelay', { pct, days })
      : t('referral.howItWorks.step2FirstWithRateAndDelay', { pct, days })
  } else if (pct) {
    earnTitle = isEveryPaid.value
      ? t('referral.howItWorks.step2TitleEveryWithRate', { pct })
      : t('referral.howItWorks.step2TitleFirstWithRate', { pct })
    earnDesc = isEveryPaid.value
      ? t('referral.howItWorks.step2EveryWithRate', { pct })
      : t('referral.howItWorks.step2FirstWithRate', { pct })
  }

  let cashTitle = t('referral.howItWorks.step3PendingTitle')
  let cashDesc = t('referral.howItWorks.step3Pending')
  if (props.withdrawEnabled && props.creditConversionEnabled) {
    cashTitle = t('referral.howItWorks.step3Title')
    cashDesc = `${t('referral.howItWorks.step3Both')} ${convertHint.value}`.trim()
  } else if (props.withdrawEnabled) {
    cashTitle = t('referral.howItWorks.step3WithdrawTitle')
    cashDesc = t('referral.howItWorks.step3Withdraw')
  } else if (props.creditConversionEnabled) {
    cashTitle = t('referral.howItWorks.step3CreditTitle')
    cashDesc = convertHint.value || t('referral.howItWorks.step3Credit')
  }

  return [
    {
      key: 'share',
      icon: 'share' as const,
      title: t('referral.howItWorks.step1Title'),
      desc: t('referral.howItWorks.step1')
    },
    {
      key: 'earn',
      icon: 'gift' as const,
      title: earnTitle,
      desc: earnDesc
    },
    {
      key: 'cash',
      icon: 'convert' as const,
      title: cashTitle,
      desc: cashDesc
    }
  ]
})
</script>
