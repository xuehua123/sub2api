package service

import (
	"context"
	"time"
)

func (e *SubscriptionEntitlement) IsActiveAt(now time.Time) bool {
	return entitlementActiveAt(e, now)
}

func (e *SubscriptionEntitlement) IsWindowActivated() bool {
	return e != nil && (e.DailyWindowStart != nil || e.WeeklyWindowStart != nil || e.MonthlyWindowStart != nil)
}

func (e *SubscriptionEntitlement) HasOneTimeDailyQuota() bool {
	if e == nil || e.StartsAt.IsZero() || e.ExpiresAt.IsZero() {
		return false
	}
	return !e.ExpiresAt.After(e.StartsAt.AddDate(0, 0, 1))
}

func (e *SubscriptionEntitlement) NeedsDailyResetAt(now time.Time) bool {
	if e == nil || e.DailyWindowStart == nil || e.HasOneTimeDailyQuota() {
		return false
	}
	return needsWindowResetAt(e.DailyWindowStart, e.StartsAt, 24*time.Hour, now)
}

func (e *SubscriptionEntitlement) NeedsWeeklyResetAt(now time.Time) bool {
	if e == nil {
		return false
	}
	return needsWindowResetAt(e.WeeklyWindowStart, e.StartsAt, 7*24*time.Hour, now)
}

func (e *SubscriptionEntitlement) NeedsMonthlyResetAt(now time.Time) bool {
	if e == nil {
		return false
	}
	return needsWindowResetAt(e.MonthlyWindowStart, e.StartsAt, monthlyCycleDuration, now)
}

func (e *SubscriptionEntitlement) CheckDailyLimit(additionalCost float64) bool {
	if e == nil || e.DailyLimitUSD == nil || *e.DailyLimitUSD <= 0 {
		return true
	}
	return e.DailyUsageUSD+additionalCost <= *e.DailyLimitUSD
}

func (e *SubscriptionEntitlement) CheckWeeklyLimit(additionalCost float64) bool {
	if e == nil || e.WeeklyLimitUSD == nil || *e.WeeklyLimitUSD <= 0 {
		return true
	}
	return e.WeeklyUsageUSD+additionalCost <= *e.WeeklyLimitUSD
}

func (e *SubscriptionEntitlement) CheckMonthlyLimit(additionalCost float64) bool {
	if e == nil || e.MonthlyLimitUSD == nil || *e.MonthlyLimitUSD <= 0 {
		return true
	}
	return e.MonthlyUsageUSD+additionalCost <= *e.MonthlyLimitUSD
}

func (e *SubscriptionEntitlement) CheckAllLimits(additionalCost float64) (daily, weekly, monthly bool) {
	return e.CheckDailyLimit(additionalCost), e.CheckWeeklyLimit(additionalCost), e.CheckMonthlyLimit(additionalCost)
}

func (s *SubscriptionEntitlementService) CheckAndActivateWindow(ctx context.Context, ent *SubscriptionEntitlement, now time.Time) error {
	if ent == nil || ent.IsWindowActivated() {
		return nil
	}
	windowStart := ent.StartsAt
	if windowStart.IsZero() {
		windowStart = now
	}
	if err := s.entitlementRepo.ResetUsage(ctx, ent.ID, true, true, true, windowStart); err != nil {
		return err
	}
	ent.DailyWindowStart = &windowStart
	ent.WeeklyWindowStart = &windowStart
	ent.MonthlyWindowStart = &windowStart
	ent.DailyUsageUSD = 0
	ent.WeeklyUsageUSD = 0
	ent.MonthlyUsageUSD = 0
	return nil
}

func (s *SubscriptionEntitlementService) CheckAndResetWindows(ctx context.Context, ent *SubscriptionEntitlement, now time.Time) error {
	if ent == nil {
		return nil
	}
	if now.IsZero() {
		now = s.inputNow(time.Time{})
	}
	if err := s.CheckAndActivateWindow(ctx, ent, now); err != nil {
		return err
	}
	if ent.NeedsDailyResetAt(now) {
		windowStart := resolvedWindowResetStart(ent.DailyWindowStart, ent.StartsAt, 24*time.Hour, now)
		if err := s.entitlementRepo.ResetUsage(ctx, ent.ID, true, false, false, windowStart); err != nil {
			return err
		}
		ent.DailyWindowStart = &windowStart
		ent.DailyUsageUSD = 0
	}
	if ent.NeedsWeeklyResetAt(now) {
		windowStart := resolvedWindowResetStart(ent.WeeklyWindowStart, ent.StartsAt, 7*24*time.Hour, now)
		if err := s.entitlementRepo.ResetUsage(ctx, ent.ID, false, true, false, windowStart); err != nil {
			return err
		}
		ent.WeeklyWindowStart = &windowStart
		ent.WeeklyUsageUSD = 0
	}
	if ent.NeedsMonthlyResetAt(now) {
		windowStart := resolvedWindowResetStart(ent.MonthlyWindowStart, ent.StartsAt, monthlyCycleDuration, now)
		if err := s.entitlementRepo.ResetUsage(ctx, ent.ID, false, false, true, windowStart); err != nil {
			return err
		}
		ent.MonthlyWindowStart = &windowStart
		ent.MonthlyUsageUSD = 0
	}
	return nil
}

func (s *SubscriptionEntitlementService) ValidateAndCheckLimits(ent *SubscriptionEntitlement, additionalCost float64, now time.Time) (bool, error) {
	if ent == nil {
		return false, ErrSubscriptionEntitlementNotFound
	}
	if now.IsZero() {
		now = s.inputNow(time.Time{})
	}
	if ent.Status != SubscriptionStatusActive {
		return false, ErrSubscriptionEntitlementInactive
	}
	if now.Before(ent.StartsAt) || !now.Before(ent.ExpiresAt) {
		return false, ErrSubscriptionEntitlementExpired
	}

	needsMaintenance := !ent.IsWindowActivated()
	if ent.NeedsDailyResetAt(now) {
		ent.DailyUsageUSD = 0
		needsMaintenance = true
	}
	if ent.NeedsWeeklyResetAt(now) {
		ent.WeeklyUsageUSD = 0
		needsMaintenance = true
	}
	if ent.NeedsMonthlyResetAt(now) {
		ent.MonthlyUsageUSD = 0
		needsMaintenance = true
	}
	if additionalCost < 0 {
		return needsMaintenance, ErrSubscriptionEntitlementInvalidUsage
	}
	daily, weekly, monthly := ent.CheckAllLimits(additionalCost)
	if !daily || !weekly || !monthly {
		return needsMaintenance, ErrSubscriptionEntitlementQuotaExceeded
	}
	return needsMaintenance, nil
}

func (s *SubscriptionEntitlementService) ApplyEntitlementUsage(ctx context.Context, entitlementID int64, costUSD float64, now time.Time) (*EntitlementUsageApplyResult, error) {
	if now.IsZero() {
		now = s.inputNow(time.Time{})
	}
	return s.entitlementRepo.ApplyEntitlementUsage(ctx, entitlementID, costUSD, now)
}
