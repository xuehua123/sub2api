package handler

import "github.com/Wei-Shaw/sub2api/internal/service"

func calculateEntitlementRemaining(entitlement *service.SubscriptionEntitlement) float64 {
	if entitlement == nil {
		return -1
	}
	var remainingValues []float64
	if entitlement.DailyLimitUSD != nil && *entitlement.DailyLimitUSD > 0 {
		remaining := *entitlement.DailyLimitUSD - entitlement.DailyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}
	if entitlement.WeeklyLimitUSD != nil && *entitlement.WeeklyLimitUSD > 0 {
		remaining := *entitlement.WeeklyLimitUSD - entitlement.WeeklyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}
	if entitlement.MonthlyLimitUSD != nil && *entitlement.MonthlyLimitUSD > 0 {
		remaining := *entitlement.MonthlyLimitUSD - entitlement.MonthlyUsageUSD
		if remaining <= 0 {
			return 0
		}
		remainingValues = append(remainingValues, remaining)
	}
	if len(remainingValues) == 0 {
		return -1
	}
	min := remainingValues[0]
	for _, v := range remainingValues[1:] {
		if v < min {
			min = v
		}
	}
	return min
}
