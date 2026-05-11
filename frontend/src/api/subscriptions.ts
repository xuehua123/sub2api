/**
 * User Subscription API
 * API for regular users to view their own subscriptions and progress
 */

import { apiClient } from './client'
import type {
  AdvanceMonthlyCycleResult,
  SubscriptionGroupPreference,
  SubscriptionProgress,
  UserSubscription
} from '@/types'

/**
 * Subscription summary for user dashboard
 */
export interface SubscriptionSummary {
  active_count: number
  subscriptions: Array<{
    id: number
    group_name: string
    status: string
    daily_progress: number | null
    weekly_progress: number | null
    monthly_progress: number | null
    expires_at: string | null
    days_remaining: number | null
  }>
}

/**
 * Get list of current user's subscriptions
 */
export async function getMySubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions')
  return response.data
}

/**
 * Get current user's active subscriptions
 */
export async function getActiveSubscriptions(): Promise<UserSubscription[]> {
  const response = await apiClient.get<UserSubscription[]>('/subscriptions/active')
  return response.data
}

/**
 * Get progress for all user's active subscriptions
 */
export async function getSubscriptionsProgress(): Promise<SubscriptionProgress[]> {
  const response = await apiClient.get<SubscriptionProgress[]>('/subscriptions/progress')
  return response.data
}

/**
 * Get subscription summary for dashboard display
 */
export async function getSubscriptionSummary(): Promise<SubscriptionSummary> {
  const response = await apiClient.get<SubscriptionSummary>('/subscriptions/summary')
  return response.data
}

/**
 * Get progress for a specific subscription
 */
export async function getSubscriptionProgress(
  subscriptionId: number
): Promise<SubscriptionProgress> {
  const response = await apiClient.get<SubscriptionProgress>(
    `/subscriptions/${subscriptionId}/progress`
  )
  return response.data
}

export async function getGroupPreferences(): Promise<SubscriptionGroupPreference[]> {
  const response = await apiClient.get<SubscriptionGroupPreference[]>('/subscriptions/group-preferences')
  return response.data
}

export async function saveGroupPreferences(
  preferences: SubscriptionGroupPreference[]
): Promise<SubscriptionGroupPreference[]> {
  const response = await apiClient.put<SubscriptionGroupPreference[]>('/subscriptions/group-preferences', {
    preferences
  })
  return response.data
}

export async function advanceMonthlyCycle(
  subscriptionId: number
): Promise<AdvanceMonthlyCycleResult> {
  const response = await apiClient.post<AdvanceMonthlyCycleResult>(
    `/subscriptions/${subscriptionId}/advance-monthly-cycle`
  )
  return response.data
}

export default {
  getMySubscriptions,
  getActiveSubscriptions,
  getSubscriptionsProgress,
  getSubscriptionSummary,
  getSubscriptionProgress,
  getGroupPreferences,
  saveGroupPreferences,
  advanceMonthlyCycle
}
