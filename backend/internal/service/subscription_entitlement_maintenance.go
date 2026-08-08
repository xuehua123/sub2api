package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type subscriptionEntitlementTermCASRepository interface {
	CompareAndSwapTerm(
		ctx context.Context,
		id int64,
		expectedUpdatedAt time.Time,
		startsAt time.Time,
		expiresAt time.Time,
		status string,
		notes string,
	) (updatedAt time.Time, swapped bool, err error)
}

type SubscriptionEntitlementRefundAdjustment struct {
	Snapshot  *SubscriptionEntitlement
	Revoked   bool
	UpdatedAt time.Time
}

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
	_, ok := e.automaticDailyWindowStartAt(now)
	return ok
}

func (e *SubscriptionEntitlement) automaticDailyWindowStartAt(now time.Time) (time.Time, bool) {
	if e == nil || e.DailyWindowStart == nil || e.HasOneTimeDailyQuota() {
		return time.Time{}, false
	}
	today := timezone.StartOfDay(now)
	if !today.After(timezone.StartOfDay(*e.DailyWindowStart)) {
		return time.Time{}, false
	}
	return today, true
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
	periodicStart := ent.StartsAt
	if periodicStart.IsZero() {
		periodicStart = now
	}
	dailyStart := timezone.StartOfDay(now)
	if err := s.entitlementRepo.ActivateWindows(ctx, ent.ID, dailyStart, periodicStart); err != nil {
		return err
	}
	return s.refreshEntitlementSnapshot(ctx, ent)
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
	maintained := false
	if windowStart, ok := ent.automaticDailyWindowStartAt(now); ok {
		if err := s.entitlementRepo.ResetDailyUsage(ctx, ent.ID, ent.DailyWindowStart, windowStart); err != nil {
			return err
		}
		maintained = true
	}
	if ent.NeedsWeeklyResetAt(now) {
		windowStart := resolvedWindowResetStart(ent.WeeklyWindowStart, ent.StartsAt, 7*24*time.Hour, now)
		if err := s.entitlementRepo.ResetWeeklyUsage(ctx, ent.ID, ent.WeeklyWindowStart, windowStart); err != nil {
			return err
		}
		maintained = true
	}
	if ent.NeedsMonthlyResetAt(now) {
		windowStart := resolvedWindowResetStart(ent.MonthlyWindowStart, ent.StartsAt, monthlyCycleDuration, now)
		if err := s.entitlementRepo.ResetMonthlyUsage(ctx, ent.ID, ent.MonthlyWindowStart, windowStart); err != nil {
			return err
		}
		maintained = true
	}
	if maintained {
		return s.refreshEntitlementSnapshot(ctx, ent)
	}
	return nil
}

func (s *SubscriptionEntitlementService) refreshEntitlementSnapshot(ctx context.Context, ent *SubscriptionEntitlement) error {
	refreshed, err := s.entitlementRepo.GetByID(ctx, ent.ID)
	if err != nil {
		return err
	}
	*ent = *refreshed
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

func (s *SubscriptionEntitlementService) GetRefundSnapshot(ctx context.Context, entitlementID int64, now time.Time) (*SubscriptionEntitlement, error) {
	if s == nil || s.entitlementRepo == nil || entitlementID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if now.IsZero() {
		now = s.inputNow(time.Time{})
	}
	ent, err := s.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if !entitlementActiveAt(ent, now) {
		return nil, ErrSubscriptionEntitlementExpired
	}
	return cloneSubscriptionEntitlementForRefund(ent), nil
}

func (s *SubscriptionEntitlementService) ShortenForRefund(ctx context.Context, entitlementID int64, days int, now time.Time) (*SubscriptionEntitlementRefundAdjustment, error) {
	if s == nil || s.entitlementRepo == nil || entitlementID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if days <= 0 {
		return &SubscriptionEntitlementRefundAdjustment{}, nil
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	if now.IsZero() {
		now = s.inputNow(time.Time{})
	}
	ent, err := s.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if !entitlementActiveAt(ent, now) {
		return nil, ErrSubscriptionEntitlementExpired
	}

	expiresAt := ent.ExpiresAt.AddDate(0, 0, -days)
	status := SubscriptionStatusActive
	revoked := false
	if !expiresAt.After(now) {
		expiresAt = now
		status = SubscriptionStatusExpired
		revoked = true
	}
	casRepo, ok := s.entitlementRepo.(subscriptionEntitlementTermCASRepository)
	if !ok {
		return nil, ErrSubscriptionEntitlementTermConflict
	}
	updatedAt, swapped, err := casRepo.CompareAndSwapTerm(
		ctx,
		entitlementID,
		ent.UpdatedAt,
		ent.StartsAt,
		expiresAt,
		status,
		ent.Notes,
	)
	if err != nil {
		return nil, err
	}
	if !swapped {
		return nil, ErrSubscriptionEntitlementTermConflict
	}
	return &SubscriptionEntitlementRefundAdjustment{
		Snapshot:  cloneSubscriptionEntitlementForRefund(ent),
		Revoked:   revoked,
		UpdatedAt: updatedAt,
	}, nil
}

func (s *SubscriptionEntitlementService) RestoreRefundSnapshot(ctx context.Context, snapshot *SubscriptionEntitlement, expectedUpdatedAt time.Time) error {
	if s == nil || s.entitlementRepo == nil || snapshot == nil || snapshot.ID <= 0 {
		return ErrSubscriptionEntitlementNotFound
	}
	if expectedUpdatedAt.IsZero() {
		return ErrSubscriptionEntitlementTermConflict
	}
	casRepo, ok := s.entitlementRepo.(subscriptionEntitlementTermCASRepository)
	if !ok {
		return ErrSubscriptionEntitlementTermConflict
	}
	_, swapped, err := casRepo.CompareAndSwapTerm(
		ctx,
		snapshot.ID,
		expectedUpdatedAt,
		snapshot.StartsAt,
		snapshot.ExpiresAt,
		snapshot.Status,
		snapshot.Notes,
	)
	if err != nil {
		return err
	}
	if !swapped {
		return ErrSubscriptionEntitlementTermConflict
	}
	return nil
}

func cloneSubscriptionEntitlementForRefund(ent *SubscriptionEntitlement) *SubscriptionEntitlement {
	if ent == nil {
		return nil
	}
	cp := *ent
	cp.PlanID = cloneInt64Ptr(ent.PlanID)
	cp.LegacySubscriptionID = cloneInt64Ptr(ent.LegacySubscriptionID)
	cp.PrimaryGroupID = cloneInt64Ptr(ent.PrimaryGroupID)
	cp.DailyWindowStart = cloneEntitlementTimePtr(ent.DailyWindowStart)
	cp.WeeklyWindowStart = cloneEntitlementTimePtr(ent.WeeklyWindowStart)
	cp.MonthlyWindowStart = cloneEntitlementTimePtr(ent.MonthlyWindowStart)
	cp.DailyLimitUSD = cloneFloat64Ptr(ent.DailyLimitUSD)
	cp.WeeklyLimitUSD = cloneFloat64Ptr(ent.WeeklyLimitUSD)
	cp.MonthlyLimitUSD = cloneFloat64Ptr(ent.MonthlyLimitUSD)
	cp.PlanSnapshot = copyMap(ent.PlanSnapshot)
	cp.SourceID = cloneInt64Ptr(ent.SourceID)
	cp.SourceExternalID = cloneStringPtr(ent.SourceExternalID)
	cp.SourceRedeemCodeID = cloneInt64Ptr(ent.SourceRedeemCodeID)
	cp.AssignedBy = cloneInt64Ptr(ent.AssignedBy)
	cp.Groups = append([]Group(nil), ent.Groups...)
	cp.GroupGrants = append([]SubscriptionEntitlementGroupGrant(nil), ent.GroupGrants...)
	return &cp
}

func cloneEntitlementTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
