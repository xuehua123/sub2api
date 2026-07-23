<template>
  <section
    data-test="referral-share-card"
    class="flex h-full flex-col rounded-[28px] border border-black/[0.06] bg-white p-7 dark:border-white/10 dark:bg-[#1c1c1e]"
  >
    <div class="flex items-start justify-between gap-3">
      <div class="flex gap-3">
        <span class="flex h-11 w-11 items-center justify-center rounded-[14px] bg-[#0071e3]/10 text-[#0071e3]">
          <ReferralIcon name="share" :size="22" />
        </span>
        <div>
          <p class="text-[13px] font-medium text-[#86868b]">{{ t('referral.shareCardEyebrow') }}</p>
          <h2 class="mt-0.5 text-[22px] font-semibold tracking-tight text-[#1d1d1f] dark:text-white">
            {{ shareTitle }}
          </h2>
          <p class="mt-1 text-[14px] leading-relaxed text-[#6e6e73] dark:text-[#a1a1a6]">
            {{ shareDesc }}
          </p>
        </div>
      </div>
      <span class="inline-flex items-center gap-1 rounded-full bg-[#f5f5f7] px-2.5 py-1 text-[12px] font-medium text-[#1d1d1f] dark:bg-white/10 dark:text-white">
        <ReferralIcon name="users" :size="13" />
        {{ inviteCount }}
      </span>
    </div>

    <div class="mt-6 rounded-[20px] bg-[#f5f5f7] px-5 py-6 text-center dark:bg-[#2c2c2e]">
      <p class="inline-flex items-center gap-1.5 text-[12px] font-medium text-[#86868b]">
        <ReferralIcon name="link" :size="13" />
        {{ t('referral.defaultCode') }}
      </p>
      <p class="mt-2 font-mono text-[34px] font-semibold tracking-[0.16em] text-[#1d1d1f] dark:text-white">
        {{ code || '—' }}
      </p>
    </div>

    <div class="mt-4">
      <p class="mb-1.5 text-[12px] font-medium text-[#86868b]">{{ t('referral.inviteLink') }}</p>
      <div class="flex items-center gap-2 rounded-[14px] border border-black/[0.06] bg-[#fafafa] px-3 py-2.5 dark:border-white/10 dark:bg-[#2c2c2e]">
        <ReferralIcon name="link" :size="15" class="text-[#86868b]" />
        <input
          class="min-w-0 flex-1 bg-transparent text-[13px] text-[#1d1d1f] outline-none dark:text-white"
          :value="inviteLink"
          readonly
        />
      </div>
    </div>

    <div class="mt-auto flex flex-col gap-2.5 pt-6 sm:flex-row">
      <button
        type="button"
        data-test="copy-invite-link"
        class="inline-flex h-12 flex-1 items-center justify-center gap-2 rounded-full bg-[#0071e3] text-[15px] font-medium text-white transition hover:bg-[#0077ed] active:scale-[0.98] disabled:opacity-40"
        :disabled="!inviteLink"
        @click="$emit('copy', inviteLink)"
      >
        <ReferralIcon name="share" :size="16" />
        {{ t('referral.copyInviteLink') }}
      </button>
      <button
        type="button"
        data-test="copy-invite-code"
        class="inline-flex h-12 flex-1 items-center justify-center gap-2 rounded-full bg-[#f5f5f7] text-[15px] font-medium text-[#1d1d1f] transition hover:bg-[#e8e8ed] active:scale-[0.98] disabled:opacity-40 dark:bg-white/10 dark:text-white dark:hover:bg-white/15"
        :disabled="!code"
        @click="$emit('copy', code)"
      >
        <ReferralIcon name="link" :size="16" />
        {{ t('referral.copyCode') }}
      </button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ReferralIcon from './ReferralIcons.vue'

const props = defineProps<{
  code: string
  inviteLink: string
  inviteCount: number
  level1Rate?: number
  level1Enabled?: boolean
  rewardMode?: string
}>()

defineEmits<{
  copy: [text: string]
}>()

const { t } = useI18n()

const ratePct = computed(() => {
  if (props.level1Enabled === false) return ''
  const rate = Number(props.level1Rate || 0)
  if (rate <= 0) return ''
  const pct = rate * 100
  return pct % 1 === 0 ? String(pct) : pct.toFixed(1)
})

const shareTitle = computed(() =>
  ratePct.value ? t('referral.shareCardTitle') : t('referral.shareCardTitleNoCommission')
)

const shareDesc = computed(() => {
  // Only promise commission when L1 is on and rate > 0 (same gate as reward booking).
  if (!ratePct.value) return t('referral.shareCardDescNoCommission')
  if (props.rewardMode === 'every_paid_order') {
    return t('referral.shareCardDescWithRateEvery', { pct: ratePct.value })
  }
  return t('referral.shareCardDescWithRateFirst', { pct: ratePct.value })
})
</script>
