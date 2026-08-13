<template>
  <div class="relative inline-block">
    <span
      ref="triggerEl"
      :class="[
        'inline-flex cursor-help items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium transition-colors',
        effectivePlatform
          ? platformBadgeClass(effectivePlatform)
          : 'border-gray-200 bg-gray-50 text-gray-700 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300',
      ]"
      @mouseenter="onEnter"
      @mouseleave="onLeave"
      @focusin="onEnter"
      @focusout="onLeave"
      tabindex="0"
    >
      <PlatformIcon
        v-if="effectivePlatform"
        :platform="effectivePlatform as GroupPlatform"
        size="xs"
      />
      <span
        v-if="showPlatform && model.platform"
        class="rounded bg-gray-200/60 px-1 text-[10px] uppercase text-gray-600 dark:bg-dark-700 dark:text-gray-400"
      >
        {{ model.platform }}
      </span>
      {{ model.name }}
    </span>

    <!-- Teleport to body so the popover is not clipped by card/overflow-hidden
         ancestors. Fixed-position coords are computed from the trigger's
         bounding rect; re-measured on enter / scroll / resize. -->
    <Teleport to="body">
      <div
        v-show="show"
        ref="popoverEl"
        role="tooltip"
        class="pointer-events-none fixed z-[99999] w-80 max-w-[min(22rem,calc(100vw-1rem))] rounded-lg border bg-white text-xs shadow-xl dark:bg-dark-800"
        :class="[popoverBorderClass]"
        :style="popoverStyle"
      >
        <!-- Header：平台主题色背景，含模型名 + 平台徽章 -->
        <div
          class="flex items-center justify-between gap-2 rounded-t-lg border-b px-3 py-2"
          :class="[popoverHeaderClass, popoverBorderClass]"
        >
          <span class="truncate font-semibold">{{ model.name }}</span>
          <span
            v-if="model.platform"
            class="flex-shrink-0 rounded bg-white/70 px-1.5 py-0.5 text-[10px] uppercase tracking-wide dark:bg-dark-900/60"
          >
            {{ model.platform }}
          </span>
        </div>

        <div class="space-y-3 p-3">
          <div
            v-for="(priceEntry, entryIndex) in displayPricingEntries"
            :key="priceEntry.key"
            :class="entryIndex > 0 ? ['border-t pt-3', popoverBorderClass] : ''"
          >
            <div
              v-if="priceEntry.name"
              class="mb-2 truncate font-semibold text-gray-700 dark:text-gray-200"
            >
              {{ priceEntry.name }}
            </div>
            <div v-if="!priceEntry.pricing" class="text-gray-500 dark:text-gray-400">
              {{ noPricingLabel }}
            </div>

            <div v-else class="space-y-2 text-gray-700 dark:text-gray-300">
            <div class="flex justify-between">
              <span class="text-gray-500 dark:text-gray-400">{{ t(prefixKey('billingMode')) }}</span>
              <span>{{ billingModeLabel(priceEntry.pricing.billing_mode) }}</span>
            </div>

            <template v-if="priceEntry.pricing.billing_mode === BILLING_MODE_TOKEN">
              <PricingRow
                :label="t(prefixKey('inputPrice'))"
                :value="priceEntry.pricing.input_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                :label="t(prefixKey('outputPrice'))"
                :value="priceEntry.pricing.output_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
				v-if="priceEntry.pricing.cache_write_5m_price == null && priceEntry.pricing.cache_write_1h_price == null"
                :label="t(prefixKey('cacheWritePrice'))"
                :value="priceEntry.pricing.cache_write_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
			  <PricingRow
				v-if="priceEntry.pricing.cache_write_5m_price != null"
				:label="t(prefixKey('cacheWrite5mPrice'))"
				:value="priceEntry.pricing.cache_write_5m_price"
				:unit="t(prefixKey('unitPerMillion'))"
				:scale="perMillionScale"
			  />
			  <PricingRow
				v-if="priceEntry.pricing.cache_write_1h_price != null"
				:label="t(prefixKey('cacheWrite1hPrice'))"
				:value="priceEntry.pricing.cache_write_1h_price"
				:unit="t(prefixKey('unitPerMillion'))"
				:scale="perMillionScale"
			  />
              <PricingRow
                :label="t(prefixKey('cacheReadPrice'))"
                :value="priceEntry.pricing.cache_read_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                v-if="priceEntry.pricing.image_input_price != null"
                :label="t(prefixKey('imageInputPrice'))"
                :value="priceEntry.pricing.image_input_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
              <PricingRow
                v-if="priceEntry.pricing.image_output_price != null"
                :label="t(prefixKey('imageOutputPrice'))"
                :value="priceEntry.pricing.image_output_price"
                :unit="t(prefixKey('unitPerMillion'))"
                :scale="perMillionScale"
              />
            </template>

            <PricingRow
              v-if="
                (priceEntry.pricing.billing_mode === BILLING_MODE_PER_REQUEST ||
                  priceEntry.pricing.billing_mode === BILLING_MODE_VIDEO) &&
                priceEntry.pricing.per_request_price != null
              "
              :label="
                t(
                  prefixKey(
                    priceEntry.pricing.billing_mode === BILLING_MODE_VIDEO
                      ? 'perSecondPrice'
                      : 'perRequestPrice',
                  ),
                )
              "
              :value="priceEntry.pricing.per_request_price"
              :unit="
                t(
                  prefixKey(
                    priceEntry.pricing.billing_mode === BILLING_MODE_VIDEO
                      ? 'unitPerSecond'
                      : 'unitPerRequest',
                  ),
                )
              "
              :scale="1"
            />

            <PricingRow
              v-if="
                priceEntry.pricing.billing_mode === BILLING_MODE_IMAGE &&
                priceEntry.pricing.image_output_price != null
              "
              :label="t(prefixKey('imageOutputPrice'))"
              :value="priceEntry.pricing.image_output_price"
              :unit="t(prefixKey('unitPerRequest'))"
              :scale="1"
            />

            <div
              v-if="priceEntry.pricing.intervals && priceEntry.pricing.intervals.length > 0"
              class="mt-2 border-t pt-2"
              :class="[popoverBorderClass]"
            >
              <div class="mb-1 font-medium text-gray-600 dark:text-gray-400">
                {{ t(prefixKey('intervals')) }}
              </div>
              <div class="space-y-1">
                <div
                  v-for="(iv, idx) in priceEntry.pricing.intervals"
                  :key="idx"
                  class="flex justify-between text-[11px]"
                >
                  <span class="text-gray-500 dark:text-gray-400">
                    {{ intervalLabel(iv) }}
                  </span>
                  <span>{{ formatInterval(iv, priceEntry.pricing.billing_mode) }}</span>
                </div>
              </div>
            </div>
          </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import PricingRow from './PricingRow.vue'
import { formatScaled } from '@/utils/pricing'
import {
  BILLING_MODE_TOKEN,
  BILLING_MODE_PER_REQUEST,
  BILLING_MODE_IMAGE,
  BILLING_MODE_VIDEO,
  type BillingMode
} from '@/constants/channel'
// 复用 api/channels.ts 的用户侧最小形态 DTO。
// admin 侧 ChannelModelPricing 字段更多，但结构上是用户 DTO 的超集，admin 视图传入可直接通过结构化子类型检查。
import type {
  UserAvailableGroup,
  UserPricingInterval,
  UserSupportedModel,
  UserSupportedModelPricing
} from '@/api/channels'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'
import { platformBadgeClass, platformBorderClass, platformBadgeLightClass } from '@/utils/platformColors'

const props = withDefaults(
  defineProps<{
    model: UserSupportedModel
    groups?: UserAvailableGroup[]
    /** i18n 前缀：管理端传 `admin.availableChannels.pricing`，用户端传 `availableChannels.pricing`。 */
    pricingKeyPrefix?: string
    noPricingLabel?: string
    showPlatform?: boolean
    /**
     * 当 model.platform 缺失（如 admin 聚合场景）时，用父行的平台作为兜底着色。
     * 仅用于视觉，不影响业务逻辑。
     */
    platformHint?: string
  }>(),
  {
    pricingKeyPrefix: 'availableChannels.pricing',
    noPricingLabel: '',
    showPlatform: true,
    platformHint: ''
  }
)

const effectivePlatform = computed<string>(() => props.model.platform || props.platformHint || '')

interface DisplayPricingEntry {
  key: string
  name: string
  pricing: UserSupportedModelPricing | null
}

/**
 * 新响应按当前 section 的可见分组逐项展示最终生效价；旧响应没有
 * pricing_by_group 时继续使用原 pricing，保证滚动升级期间兼容。
 */
const displayPricingEntries = computed<DisplayPricingEntry[]>(() => {
  const pricingByGroup = props.model.pricing_by_group
  if (!pricingByGroup || !props.groups?.length) {
    return [{ key: 'default', name: '', pricing: props.model.pricing }]
  }
  const entries = props.groups
    .filter((group) => Object.prototype.hasOwnProperty.call(pricingByGroup, String(group.id)))
    .map((group) => ({
      key: String(group.id),
      name: group.name,
      pricing: pricingByGroup[String(group.id)] ?? null
    }))
  return entries.length > 0
    ? entries
    : [{ key: 'default', name: '', pricing: props.model.pricing }]
})

const { t } = useI18n()

/** 按 token 定价展示时的换算单位：每百万 token。 */
const perMillionScale = 1_000_000

// Popover border + header classes echo the platform theme so each card reads
// at a glance which model family it belongs to.
const popoverBorderClass = computed(() =>
  effectivePlatform.value
    ? platformBorderClass(effectivePlatform.value)
    : 'border-gray-200 dark:border-dark-600',
)
const popoverHeaderClass = computed(() =>
  effectivePlatform.value
    ? platformBadgeLightClass(effectivePlatform.value)
    : 'bg-gray-50 text-gray-700 dark:bg-dark-700/60 dark:text-gray-300',
)

function prefixKey(k: string): string {
  return `${props.pricingKeyPrefix}.${k}`
}

function billingModeLabel(mode: BillingMode): string {
  switch (mode) {
    case BILLING_MODE_TOKEN:
      return t(prefixKey('billingModeToken'))
    case BILLING_MODE_PER_REQUEST:
      return t(prefixKey('billingModePerRequest'))
    case BILLING_MODE_IMAGE:
      return t(prefixKey('billingModeImage'))
    case BILLING_MODE_VIDEO:
      return t(prefixKey('billingModeVideo'))
    default:
      return '-'
  }
}

function formatRange(min: number, max: number | null): string {
  if (max == null) return `>${formatTokenCount(min)}`
  if (min === 0) return `≤${formatTokenCount(max)}`
  return `${formatTokenCount(min)}–${formatTokenCount(max)}`
}

function formatTokenCount(value: number): string {
  if (value >= 1_000_000) return `${trimTokenCount(value / 1_000_000)}M`
  if (value >= 1_000) return `${trimTokenCount(value / 1_000)}K`
  return String(value)
}

function trimTokenCount(value: number): string {
  return String(Math.round(value * 100) / 100)
}

function formatInterval(iv: UserPricingInterval, mode: BillingMode): string {
  if (
    mode === BILLING_MODE_PER_REQUEST ||
    mode === BILLING_MODE_IMAGE ||
    mode === BILLING_MODE_VIDEO
  ) {
    return formatScaled(iv.per_request_price, 1)
  }
  const values = [
    [t(prefixKey('inputPrice')), iv.input_price],
    [t(prefixKey('outputPrice')), iv.output_price],
    [iv.cache_write_5m_price != null || iv.cache_write_1h_price != null ? t(prefixKey('cacheWrite5mPrice')) : t(prefixKey('cacheWritePrice')), iv.cache_write_5m_price ?? iv.cache_write_price],
    [t(prefixKey('cacheWrite1hPrice')), iv.cache_write_1h_price],
    [t(prefixKey('cacheReadPrice')), iv.cache_read_price],
  ] as const
  return values
    .filter(([, value]) => value != null)
    .map(([label, value]) => `${label} ${formatScaled(value ?? null, perMillionScale)}`)
    .join(' · ') || '-'
}

function intervalLabel(iv: UserPricingInterval): string {
  const range = iv.min_tokens === 0 && iv.max_tokens == null
    ? ''
    : formatRange(iv.min_tokens, iv.max_tokens)
  const label = iv.tier_label
    ? [iv.tier_label, range].filter(Boolean).join(' · ')
    : range
  return iv.requires_account_long_context
    ? `${label}${t(prefixKey('accountLongContextRequired'))}`
    : label
}

// ── Popover positioning ─────────────────────────────────────────────
// Teleport-to-body + fixed positioning avoids being clipped by
// overflow-hidden ancestors (the parent table card). We re-measure on
// hover enter, scroll, and resize. Pinning to the trigger's top-center
// with a flip when the viewport edge is near keeps it aligned without a
// full-blown positioning lib.
const show = ref(false)
const triggerEl = ref<HTMLElement | null>(null)
const popoverEl = ref<HTMLElement | null>(null)
const popoverStyle = ref<Record<string, string>>({ top: '0px', left: '0px' })

function updatePosition() {
  const trigger = triggerEl.value
  if (!trigger) return
  const rect = trigger.getBoundingClientRect()
  const margin = 8
  const popover = popoverEl.value
  const popWidth = popover?.offsetWidth ?? 320
  const popHeight = popover?.offsetHeight ?? 240
  const vw = window.innerWidth
  const vh = window.innerHeight

  let top = rect.bottom + margin
  // Flip upward if it would overflow below.
  if (top + popHeight > vh - margin) {
    top = Math.max(margin, rect.top - popHeight - margin)
  }

  let left = rect.left + rect.width / 2 - popWidth / 2
  if (left < margin) left = margin
  if (left + popWidth > vw - margin) left = vw - margin - popWidth

  popoverStyle.value = {
    top: `${Math.round(top)}px`,
    left: `${Math.round(left)}px`,
  }
}

function onEnter() {
  show.value = true
  nextTick(() => {
    updatePosition()
    window.addEventListener('scroll', updatePosition, true)
    window.addEventListener('resize', updatePosition)
  })
}

function onLeave() {
  show.value = false
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
}

onBeforeUnmount(() => {
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
})
</script>
