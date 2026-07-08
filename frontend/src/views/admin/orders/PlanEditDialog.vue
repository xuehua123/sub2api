<template>
  <BaseDialog :show="show" :title="plan ? t('payment.admin.editPlan') : t('payment.admin.createPlan')" width="wide" @close="emit('close')">
    <form id="plan-form" @submit.prevent="handleSavePlan" class="space-y-4">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.planName') }} <span class="text-red-500">*</span></label>
          <input v-model="planForm.name" type="text" class="input" required />
        </div>
        <div>
          <label class="input-label">{{ t('payment.admin.accessScope') }} <span class="text-red-500">*</span></label>
          <Select v-model="planForm.access_scope" :options="accessScopeOptions" class="w-full" />
        </div>
      </div>

      <div v-if="planForm.access_scope === 'explicit'" class="space-y-2">
        <div class="flex items-center justify-between gap-3">
          <label class="input-label mb-0">{{ t('payment.admin.authorizedGroups') }} <span class="text-red-500">*</span></label>
          <button
            type="button"
            class="rounded-md px-2 py-1 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-900/20"
            :disabled="subscriptionGroups.length === 0"
            @click="toggleAllSubscriptionGroups"
          >
            {{ allSubscriptionGroupsSelected ? t('payment.admin.clearAuthorizedGroups') : t('common.selectAll') }}
          </button>
        </div>
        <div class="grid max-h-48 grid-cols-1 gap-2 overflow-y-auto rounded-lg border border-gray-200 p-2 dark:border-dark-600 sm:grid-cols-2">
          <label
            v-for="group in subscriptionGroups"
            :key="group.id"
            class="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 hover:bg-gray-50 dark:hover:bg-dark-700"
          >
            <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :checked="planForm.group_ids.includes(group.id)" @change="toggleGroupID(group.id)" />
            <GroupBadge :name="group.name" :platform="group.platform" :rate-multiplier="group.rate_multiplier" />
          </label>
        </div>
      </div>

      <div v-else-if="planForm.access_scope === 'platform_subscription_groups'" class="space-y-2">
        <label class="input-label">{{ t('payment.admin.allowedPlatforms') }} <span class="text-red-500">*</span></label>
        <div class="flex flex-wrap gap-2">
          <label
            v-for="platform in platformOptions"
            :key="platform"
            class="flex cursor-pointer items-center gap-2 rounded-md border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
            :class="planForm.allowed_platforms.includes(platform) ? 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300' : 'text-gray-700 dark:text-gray-300'"
          >
            <input type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" :checked="planForm.allowed_platforms.includes(platform)" @change="togglePlatform(platform)" />
            <span :class="platformTextClass(platform)">{{ platform }}</span>
          </label>
        </div>
      </div>

      <div v-if="selectedGroupInfos.length > 0" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
        <div class="mb-2 flex flex-wrap items-center gap-2">
          <GroupBadge
            v-for="group in selectedGroupInfos.slice(0, 6)"
            :key="group.id"
            :name="group.name"
            :platform="group.platform"
            :rate-multiplier="group.rate_multiplier"
          />
          <span v-if="selectedGroupInfos.length > 6" class="text-xs text-gray-500 dark:text-gray-400">+{{ selectedGroupInfos.length - 6 }}</span>
        </div>
        <div class="grid grid-cols-3 gap-2 text-xs">
          <div><span class="text-gray-500">{{ t('payment.admin.dailyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ displayPlanLimit(planForm.daily_limit_usd) }}</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.weeklyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ displayPlanLimit(planForm.weekly_limit_usd) }}</span></div>
          <div><span class="text-gray-500">{{ t('payment.admin.monthlyLimit') }}:</span> <span class="ml-1 font-medium text-gray-700 dark:text-gray-300">{{ displayPlanLimit(planForm.monthly_limit_usd) }}</span></div>
        </div>
      </div>

      <div><label class="input-label">{{ t('payment.admin.planDescription') }} <span class="text-red-500">*</span></label><textarea v-model="planForm.description" rows="2" class="input" required></textarea></div>
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label">{{ t('payment.admin.price') }} <span class="text-red-500">*</span></label>
          <input v-model.number="planForm.price" type="number" step="0.01" min="0.01" class="input" required />
          <p v-if="subscriptionCnyPreview" class="mt-1 text-xs font-medium text-primary-600 dark:text-primary-400">
            {{ t('payment.admin.subscriptionCnyPayPreview', { amount: subscriptionCnyPreview.amount }) }}
            <span v-if="subscriptionCnyPreview.feeRate > 0">
              {{ t('payment.admin.subscriptionCnyPayPreviewWithFee', { feeRate: subscriptionCnyPreview.feeRate, total: subscriptionCnyPreview.total }) }}
            </span>
          </p>
        </div>
        <div><label class="input-label">{{ t('payment.admin.originalPrice') }}</label><input :value="nullableInputValue(planForm.original_price)" type="number" step="0.01" min="0" class="input" @input="planForm.original_price = parseNullableInput($event)" /></div>
      </div>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.validityDays') }} <span class="text-red-500">*</span></label><input v-model.number="planForm.validity_days" type="number" min="1" class="input" required /></div>
        <div><label class="input-label">{{ t('payment.admin.validityUnit') }} <span class="text-red-500">*</span></label><Select v-model="planForm.validity_unit" :options="validityUnitOptions" /></div>
      </div>
      <p v-if="invalidValidityUnit" class="text-xs text-amber-600 dark:text-amber-400">
        {{ t('payment.admin.invalidValidityUnitHint', { unit: invalidValidityUnit }) }}
      </p>
      <div class="grid grid-cols-2 gap-4">
        <div><label class="input-label">{{ t('payment.admin.overagePolicy') }}</label><Select v-model="planForm.overage_policy" :options="overagePolicyOptions" /></div>
        <div><label class="input-label">{{ t('payment.admin.sortOrder') }}</label><input v-model.number="planForm.sort_order" type="number" min="0" class="input" /></div>
      </div>
      <div class="grid grid-cols-3 gap-4">
        <div><label class="input-label">{{ t('payment.admin.dailyLimit') }}</label><input :value="nullableInputValue(planForm.daily_limit_usd)" type="number" step="0.000001" min="0.000001" class="input" @input="planForm.daily_limit_usd = parseNullableInput($event)" /></div>
        <div><label class="input-label">{{ t('payment.admin.weeklyLimit') }}</label><input :value="nullableInputValue(planForm.weekly_limit_usd)" type="number" step="0.000001" min="0.000001" class="input" @input="planForm.weekly_limit_usd = parseNullableInput($event)" /></div>
        <div><label class="input-label">{{ t('payment.admin.monthlyLimit') }}</label><input :value="nullableInputValue(planForm.monthly_limit_usd)" type="number" step="0.000001" min="0.000001" class="input" @input="planForm.monthly_limit_usd = parseNullableInput($event)" /></div>
      </div>
      <p v-if="limitPeriodError" class="text-xs text-amber-600 dark:text-amber-400">
        {{ limitPeriodError }}
      </p>
      <div>
        <label class="input-label">{{ t('payment.admin.features') }}</label>
        <textarea v-model="planFeaturesText" rows="3" class="input" :placeholder="t('payment.admin.featuresPlaceholder')"></textarea>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('payment.admin.featuresHint') }}</p>
      </div>
      <div class="flex items-center gap-3">
        <label class="text-sm text-gray-700 dark:text-gray-300">{{ t('payment.admin.forSale') }}</label>
        <button
          type="button"
          :class="[
            'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            planForm.for_sale ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
          ]"
          @click="planForm.for_sale = !planForm.for_sale"
        >
          <span :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            planForm.for_sale ? 'translate-x-5' : 'translate-x-0'
          ]" />
        </button>
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" @click="emit('close')" class="btn btn-secondary">{{ t('common.cancel') }}</button>
        <button type="submit" form="plan-form" :disabled="saving" class="btn btn-primary">{{ saving ? t('common.saving') : t('common.save') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type { AdminPaymentConfig } from '@/api/admin/payment'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatPaymentAmount } from '@/components/payment/currency'
import type { CreateSubscriptionPlanRequest, PlanAccessScope, PlanOveragePolicy, SubscriptionPlan } from '@/types/payment'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import { platformTextClass } from '@/utils/platformColors'
import { parsePlanValidityUnit } from '@/utils/subscriptionTime'

const props = defineProps<{
  show: boolean
  plan: SubscriptionPlan | null
  groups: AdminGroup[]
  paymentConfig?: AdminPaymentConfig | null
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

interface PlanFormState {
  name: string
  description: string
  price: number
  original_price: number | null
  validity_days: number
  validity_unit: string
  access_scope: PlanAccessScope
  group_ids: number[]
  allowed_platforms: string[]
  daily_limit_usd: number | null
  weekly_limit_usd: number | null
  monthly_limit_usd: number | null
  overage_policy: PlanOveragePolicy
  sort_order: number
  for_sale: boolean
}

const saving = ref(false)
const planForm = reactive<PlanFormState>(defaultPlanForm())
const planFeaturesText = ref('')
const invalidValidityUnit = ref<string | null>(null)

function defaultPlanForm(): PlanFormState {
  return {
    name: '',
    description: '',
    price: 0,
    original_price: null,
    validity_days: 30,
    validity_unit: 'day',
    access_scope: 'explicit',
    group_ids: [],
    allowed_platforms: [],
    daily_limit_usd: null,
    weekly_limit_usd: null,
    monthly_limit_usd: null,
    overage_policy: 'block',
    sort_order: 0,
    for_sale: true,
  }
}

const accessScopeOptions = computed(() => [
  { value: 'explicit', label: t('payment.admin.explicitGroups') },
  { value: 'platform_subscription_groups', label: t('payment.admin.platformSubscriptionGroups') },
  { value: 'all_subscription_groups', label: t('payment.admin.allSubscriptionGroups') },
])

const overagePolicyOptions = computed(() => [
  { value: 'block', label: t('payment.admin.overageBlock') },
  { value: 'balance_fallback', label: t('payment.admin.overageBalanceFallback') },
])

const validityUnitOptions = computed(() => {
  const options = [
    { value: 'day', label: t('payment.admin.days') },
    { value: 'week', label: t('payment.admin.weeks') },
    { value: 'month', label: t('payment.admin.months') },
    { value: 'year', label: t('payment.years') },
  ]

  if (invalidValidityUnit.value) {
    return [
      {
        value: invalidValidityUnit.value,
        label: t('payment.admin.invalidValidityUnitOption', { unit: invalidValidityUnit.value }),
        disabled: true,
      },
      ...options,
    ]
  }

  return options
})

const subscriptionGroups = computed(() =>
  props.groups
    .filter(g =>
      g.status === 'active' &&
      (g.subscription_enabled ?? g.subscription_type === 'subscription')
    )
    .slice()
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0) || a.id - b.id),
)

const allSubscriptionGroupsSelected = computed(() => (
  subscriptionGroups.value.length > 0 &&
  subscriptionGroups.value.every(group => planForm.group_ids.includes(group.id))
))

const platformOptions = computed(() => {
  const seen = new Set<string>()
  const platforms: string[] = []
  for (const group of subscriptionGroups.value) {
    if (!group.platform || seen.has(group.platform)) continue
    seen.add(group.platform)
    platforms.push(group.platform)
  }
  return platforms
})

const selectedGroupInfos = computed(() => {
  if (planForm.access_scope === 'all_subscription_groups') {
    return subscriptionGroups.value
  }
  if (planForm.access_scope === 'platform_subscription_groups') {
    const platforms = new Set(planForm.allowed_platforms)
    return subscriptionGroups.value.filter(group => platforms.has(group.platform))
  }
  const selected = new Set(planForm.group_ids)
  return subscriptionGroups.value.filter(group => selected.has(group.id))
})

const effectiveValidityDays = computed(() => planValidityDays(planForm.validity_days, planForm.validity_unit))

const limitPeriodError = computed(() => {
  if (planForm.weekly_limit_usd != null && effectiveValidityDays.value < 7) {
    return t('payment.admin.weeklyLimitPeriodInvalid', { days: effectiveValidityDays.value })
  }
  if (planForm.monthly_limit_usd != null && effectiveValidityDays.value < 30) {
    return t('payment.admin.monthlyLimitPeriodInvalid', { days: effectiveValidityDays.value })
  }
  return ''
})

function roundCnyAmount(value: number): number {
  return Math.round(value * 100) / 100
}

function ceilCnyAmount(value: number): number {
  return Math.ceil(value * 100) / 100
}

const subscriptionCnyPreview = computed(() => {
  const price = Number(planForm.price) || 0
  const rate = Number(props.paymentConfig?.subscription_usd_to_cny_rate) || 0
  if (price <= 0 || rate <= 0) return null

  const amount = roundCnyAmount(price * rate)
  const feeRate = Number(props.paymentConfig?.recharge_fee_rate) || 0
  const fee = feeRate > 0 ? ceilCnyAmount((amount * feeRate) / 100) : 0
  const total = feeRate > 0 ? roundCnyAmount(amount + fee) : amount

  return {
    amount: formatPaymentAmount(amount, 'CNY'),
    feeRate,
    total: formatPaymentAmount(total, 'CNY'),
  }
})

// Reset form when dialog opens
watch(() => props.show, (visible) => {
  if (!visible) return
  if (props.plan) {
    const parsedValidityUnit = parsePlanValidityUnit(props.plan.validity_unit)
    invalidValidityUnit.value = parsedValidityUnit ? null : (props.plan.validity_unit?.trim() || null)
    Object.assign(planForm, {
      name: props.plan.name,
      description: props.plan.description,
      price: props.plan.price,
      original_price: props.plan.original_price ?? null,
      validity_days: props.plan.validity_days,
      validity_unit: parsedValidityUnit ?? invalidValidityUnit.value ?? 'day',
      access_scope: props.plan.access_scope ?? 'explicit',
      group_ids: initialPlanGroupIDs(props.plan),
      allowed_platforms: initialAllowedPlatforms(props.plan),
      daily_limit_usd: props.plan.daily_limit_usd ?? null,
      weekly_limit_usd: props.plan.weekly_limit_usd ?? null,
      monthly_limit_usd: props.plan.monthly_limit_usd ?? null,
      overage_policy: props.plan.overage_policy ?? 'block',
      sort_order: props.plan.sort_order || 0,
      for_sale: props.plan.for_sale,
    })
    planFeaturesText.value = (props.plan.features || []).join('\n')
  } else {
    Object.assign(planForm, defaultPlanForm())
    planFeaturesText.value = ''
    invalidValidityUnit.value = null
  }
})

watch(() => planForm.access_scope, (scope) => {
  if (scope !== 'explicit') {
    planForm.group_ids = []
  }
  if (scope !== 'platform_subscription_groups') {
    planForm.allowed_platforms = []
  }
})

watch(() => planForm.validity_unit, (value) => {
  if (!invalidValidityUnit.value) return
  if (value !== invalidValidityUnit.value && parsePlanValidityUnit(value)) {
    invalidValidityUnit.value = null
  }
})

function initialPlanGroupIDs(plan: SubscriptionPlan): number[] {
  if (plan.group_ids && plan.group_ids.length > 0) {
    return [...plan.group_ids]
  }
  return plan.group_id ? [plan.group_id] : []
}

function initialAllowedPlatforms(plan: SubscriptionPlan): string[] {
  if (plan.allowed_platforms && plan.allowed_platforms.length > 0) {
    return [...plan.allowed_platforms]
  }
  if (plan.access_scope === 'platform_subscription_groups' && plan.groups) {
    return Array.from(new Set(plan.groups.map(group => group.platform).filter(Boolean)))
  }
  return []
}

function toggleGroupID(groupID: number) {
  if (planForm.group_ids.includes(groupID)) {
    planForm.group_ids = planForm.group_ids.filter(id => id !== groupID)
  } else {
    planForm.group_ids = [...planForm.group_ids, groupID]
  }
}

function toggleAllSubscriptionGroups() {
  if (allSubscriptionGroupsSelected.value) {
    planForm.group_ids = []
    return
  }
  planForm.group_ids = subscriptionGroups.value.map(group => group.id)
}

function togglePlatform(platform: string) {
  if (planForm.allowed_platforms.includes(platform)) {
    planForm.allowed_platforms = planForm.allowed_platforms.filter(item => item !== platform)
  } else {
    planForm.allowed_platforms = [...planForm.allowed_platforms, platform]
  }
}

function nullableInputValue(value: number | null): string {
  return value == null ? '' : String(value)
}

function parseNullableInput(event: Event): number | null {
  const value = (event.target as HTMLInputElement).value.trim()
  if (!value) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function displayPlanLimit(value: number | null): string {
  return value != null ? `$${value}` : t('payment.admin.unlimited')
}

function normalizeLimit(value: number | null): number | null {
  return value != null && Number.isFinite(value) ? value : null
}

function planValidityDays(days: number, unit: string): number {
  const value = Number.isFinite(days) && days > 0 ? days : 0
  switch (parsePlanValidityUnit(unit)) {
    case 'week':
      return value * 7
    case 'month':
      return value * 30
    case 'year':
      return value * 365
    default:
      return value
  }
}

/** Build request payload with snake_case keys matching backend JSON tags */
function buildPlanPayload(): CreateSubscriptionPlanRequest | null {
  const parsedValidityUnit = parsePlanValidityUnit(planForm.validity_unit)
  if (!parsedValidityUnit) {
    return null
  }
  const features = planFeaturesText.value.split('\n').map(f => f.trim()).filter(Boolean).join('\n')
  return {
    name: planForm.name,
    description: planForm.description,
    price: planForm.price,
    original_price: planForm.original_price,
    validity_days: planForm.validity_days,
    validity_unit: parsedValidityUnit,
    access_scope: planForm.access_scope,
    group_ids: planForm.access_scope === 'explicit' ? [...planForm.group_ids] : [],
    allowed_platforms: planForm.access_scope === 'platform_subscription_groups' ? [...planForm.allowed_platforms] : [],
    daily_limit_usd: normalizeLimit(planForm.daily_limit_usd),
    weekly_limit_usd: normalizeLimit(planForm.weekly_limit_usd),
    monthly_limit_usd: normalizeLimit(planForm.monthly_limit_usd),
    overage_policy: planForm.overage_policy,
    sort_order: planForm.sort_order,
    for_sale: planForm.for_sale,
    features,
  }
}

function hasInvalidLimit(): boolean {
  return [planForm.daily_limit_usd, planForm.weekly_limit_usd, planForm.monthly_limit_usd]
    .some(value => value != null && value <= 0)
}

async function handleSavePlan() {
  if (planForm.access_scope === 'explicit' && planForm.group_ids.length === 0) {
    appStore.showError(t('payment.admin.groupIdsRequired'))
    return
  }
  if (planForm.access_scope === 'platform_subscription_groups' && planForm.allowed_platforms.length === 0) {
    appStore.showError(t('payment.admin.platformsRequired'))
    return
  }
  if (!planForm.price || planForm.price <= 0) {
    appStore.showError(t('payment.admin.priceRequired'))
    return
  }
  if (!planForm.validity_days || planForm.validity_days < 1) {
    appStore.showError(t('payment.admin.validityDaysRequired'))
    return
  }
  if (!parsePlanValidityUnit(planForm.validity_unit)) {
    appStore.showError(t('payment.admin.invalidValidityUnitMessage', { unit: planForm.validity_unit || '-' }))
    return
  }
  if (hasInvalidLimit()) {
    appStore.showError(t('payment.admin.limitRequired'))
    return
  }
  if (limitPeriodError.value) {
    appStore.showError(limitPeriodError.value)
    return
  }
  saving.value = true
  try {
    const data = buildPlanPayload()
    if (!data) {
      appStore.showError(t('payment.admin.invalidValidityUnitMessage', { unit: planForm.validity_unit || '-' }))
      return
    }
    if (props.plan) { await adminPaymentAPI.updatePlan(props.plan.id, data) }
    else { await adminPaymentAPI.createPlan(data) }
    appStore.showSuccess(t('common.saved'))
    emit('close')
    emit('saved')
  } catch (err: unknown) { appStore.showError(extractApiErrorMessage(err, t('common.error'))) }
  finally { saving.value = false }
}
</script>
