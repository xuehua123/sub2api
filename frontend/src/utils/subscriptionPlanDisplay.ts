import type { UserEntitlement, UserSubscription } from '@/types'
import type { PlanAccessScope, PlanOveragePolicy, SubscriptionPlan } from '@/types/payment'

type PlanGroupCarrier = {
  group_id?: number | null
  group_ids?: Array<number | null | undefined> | null
  groups?: Array<{ id?: number | null }> | null
}

export type RawSubscriptionPlan = Omit<
  SubscriptionPlan,
  'access_scope' | 'allowed_platforms' | 'features' | 'group_id' | 'group_ids' | 'groups' | 'overage_policy'
> & {
  access_scope?: PlanAccessScope | null
  allowed_platforms?: string[] | null
  features?: string | string[] | null
  group_id?: number | null
  group_ids?: number[] | null
  groups?: SubscriptionPlan['groups'] | null
  overage_policy?: PlanOveragePolicy | null
}

export function normalizeSubscriptionPlan(plan: RawSubscriptionPlan | SubscriptionPlan): SubscriptionPlan {
  const groupIDs = subscriptionPlanGroupIDs(plan)

  return {
    ...plan,
    access_scope: plan.access_scope || 'explicit',
    allowed_platforms: plan.allowed_platforms || [],
    features: normalizePlanFeatures(plan.features),
    group_id: typeof plan.group_id === 'number' && plan.group_id > 0
      ? plan.group_id
      : (groupIDs[0] || 0),
    group_ids: groupIDs,
    groups: plan.groups || [],
    overage_policy: plan.overage_policy || 'block'
  } as SubscriptionPlan
}

export function normalizeSubscriptionPlans(
  plans: Array<RawSubscriptionPlan | SubscriptionPlan>
): SubscriptionPlan[] {
  return plans.map((plan) => normalizeSubscriptionPlan(plan))
}

function normalizePlanFeatures(features: string | string[] | null | undefined): string[] {
  if (Array.isArray(features)) return features
  if (typeof features !== 'string') return []
  return features.split('\n').map((feature) => feature.trim()).filter(Boolean)
}

export function subscriptionPlanDisplayName(plan: SubscriptionPlan | null | undefined): string {
  if (!plan) return ''
  return plan.product_name || plan.name || `Plan #${plan.id}`
}

export function subscriptionPlanGroupIDs(plan: PlanGroupCarrier | null | undefined): number[] {
  if (!plan) return []
  const ids = [
    plan.group_id,
    ...(plan.group_ids || []),
    ...(plan.groups || []).map((group) => group.id)
  ].filter((id): id is number => typeof id === 'number' && id > 0)

  return [...new Set(ids)]
}

// V2 entitlements without a legacy subscription alias still need to appear in
// compact subscription surfaces. These records are display-only.
export function activeSubscriptionDisplayRecords(
  activeSubscriptions: readonly UserSubscription[] | null | undefined,
  entitlements: readonly UserEntitlement[] | null | undefined,
  now = new Date()
): UserSubscription[] {
  const records = [...(activeSubscriptions ?? [])]
  const entitlementIDs = new Set(
    records
      .map((subscription) => subscription.entitlement_id)
      .filter((id): id is number => typeof id === 'number' && id > 0)
  )
  const legacySubscriptionIDs = new Set(
    records
      .map((subscription) => subscription.id)
      .filter((id) => id > 0)
  )

  for (const entitlement of entitlements ?? []) {
    if (!isActiveDisplayEntitlement(entitlement, now)) continue
    if (entitlementIDs.has(entitlement.id)) continue
    if (
      entitlement.legacy_subscription_id != null &&
      legacySubscriptionIDs.has(entitlement.legacy_subscription_id)
    ) {
      continue
    }

    records.push(entitlementDisplaySubscription(entitlement))
    entitlementIDs.add(entitlement.id)
  }

  return records
}

function isActiveDisplayEntitlement(entitlement: UserEntitlement, now: Date): boolean {
  if (entitlement.status !== 'active') return false
  const startsAt = Date.parse(entitlement.starts_at)
  if (!Number.isNaN(startsAt) && startsAt > now.getTime()) return false
  const expiresAt = Date.parse(entitlement.expires_at)
  return Number.isNaN(expiresAt) || expiresAt > now.getTime()
}

function entitlementDisplaySubscription(entitlement: UserEntitlement): UserSubscription {
  const primaryGroupID = entitlement.groups.find((group) => group.id > 0)?.id ?? 0

  return {
    id: -entitlement.id,
    user_id: 0,
    group_id: primaryGroupID,
    status: 'active',
    starts_at: entitlement.starts_at,
    expires_at: entitlement.expires_at,
    daily_usage_usd: entitlement.daily_usage_usd,
    weekly_usage_usd: entitlement.weekly_usage_usd,
    monthly_usage_usd: entitlement.monthly_usage_usd,
    daily_limit_usd: entitlement.daily_limit_usd,
    weekly_limit_usd: entitlement.weekly_limit_usd,
    monthly_limit_usd: entitlement.monthly_limit_usd,
    daily_window_start: entitlement.daily_window_start,
    weekly_window_start: entitlement.weekly_window_start,
    monthly_window_start: entitlement.monthly_window_start,
    created_at: entitlement.starts_at,
    updated_at: entitlement.starts_at,
    entitlement_only: true,
    entitlement_id: entitlement.id,
    plan_id: entitlement.plan_id,
    plan_name: entitlement.plan_name || entitlement.name,
    groups: [...entitlement.groups],
    overage_policy: entitlement.overage_policy,
  }
}

export function subscriptionMatchesPlan(
  subscription: UserSubscription,
  plan: SubscriptionPlan | null | undefined
): boolean {
  if (!plan) return false
  if (subscription.plan_id) return subscription.plan_id === plan.id
  return subscriptionPlanGroupIDs(plan).includes(subscription.group_id)
}

export function planForSubscription(
  subscription: UserSubscription,
  plans: SubscriptionPlan[]
): SubscriptionPlan | null {
  if (subscription.plan_id) {
    const exactPlan = plans.find((plan) => plan.id === subscription.plan_id)
    if (exactPlan) return exactPlan
  }

  const fallbackPlans = plans.filter((plan) => subscriptionPlanGroupIDs(plan).includes(subscription.group_id))
  return fallbackPlans.length === 1 ? fallbackPlans[0] : null
}

export function subscriptionDisplayName(
  subscription: UserSubscription,
  plans: SubscriptionPlan[]
): string {
  if (subscription.plan_name?.trim()) return subscription.plan_name.trim()
  const plan = planForSubscription(subscription, plans)
  if (plan) return subscriptionPlanDisplayName(plan)
  return subscription.group?.name || `Group #${subscription.group_id}`
}
