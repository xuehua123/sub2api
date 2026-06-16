import type { UserSubscription } from '@/types'
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
