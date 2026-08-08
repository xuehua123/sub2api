package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

func syntheticEntitlementIDFromSubscriptionID(subscriptionID int64) (int64, bool) {
	if subscriptionID >= 0 {
		return 0, false
	}
	if subscriptionID == -1<<63 {
		return 0, false
	}
	return -subscriptionID, true
}

func (s *SubscriptionService) adminAdjustEntitlement(ctx context.Context, entitlementID int64, days int) (*UserSubscription, error) {
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return nil, err
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	if days < -MaxValidityDays {
		days = -MaxValidityDays
	}

	now := entitlementSvc.inputNow(time.Time{})
	ent, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if ent.Status == SubscriptionStatusRevoked || ent.Status == SubscriptionStatusSuspended {
		return nil, ErrSubscriptionEntitlementInactive
	}

	expired := ent.Status == SubscriptionStatusExpired || !ent.ExpiresAt.After(now)
	if expired && days < 0 {
		return nil, ErrAdjustWouldExpire
	}

	startsAt := ent.StartsAt
	base := ent.ExpiresAt
	status := ent.Status
	if expired {
		startsAt = now
		base = now
		status = SubscriptionStatusActive
	}
	expiresAt := base.AddDate(0, 0, days)
	if expiresAt.After(MaxExpiresAt) {
		expiresAt = MaxExpiresAt
	}
	if !expiresAt.After(now) {
		return nil, ErrAdjustWouldExpire
	}

	var updateErr error
	if expired {
		updateErr = entitlementSvc.entitlementRepo.ExtendWithFulfillment(
			ctx,
			entitlementID,
			startsAt,
			expiresAt,
			status,
			ent.Notes,
			SubscriptionEntitlementSourceRef{},
			nil,
			true,
			timezone.StartOfDay(now),
			now,
		)
	} else {
		updateErr = entitlementSvc.entitlementRepo.UpdateTerm(ctx, entitlementID, startsAt, expiresAt, status, ent.Notes)
	}
	if updateErr != nil {
		return nil, updateErr
	}
	refreshed, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	s.invalidateEntitlementLegacyAlias(refreshed)
	return adminSubscriptionFromEntitlement(refreshed), nil
}

func (s *SubscriptionService) adminResetEntitlementQuota(ctx context.Context, entitlementID int64, resetDaily, resetWeekly, resetMonthly bool) (*UserSubscription, error) {
	refreshed, err := s.resetEntitlementQuotaUsage(ctx, entitlementID, resetDaily, resetWeekly, resetMonthly)
	if err != nil {
		return nil, err
	}
	return adminSubscriptionFromEntitlement(refreshed), nil
}

func (s *SubscriptionService) resetEntitlementQuotaUsage(ctx context.Context, entitlementID int64, resetDaily, resetWeekly, resetMonthly bool) (*SubscriptionEntitlement, error) {
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return nil, err
	}
	if !resetDaily && !resetWeekly && !resetMonthly {
		return nil, ErrInvalidInput
	}
	now := entitlementSvc.inputNow(time.Time{})
	if err := entitlementSvc.entitlementRepo.ResetUsage(ctx, entitlementID, resetDaily, resetWeekly, resetMonthly, timezone.StartOfDay(now), now); err != nil {
		return nil, err
	}
	refreshed, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	s.invalidateEntitlementLegacyAlias(refreshed)
	return refreshed, nil
}

func (s *SubscriptionService) adminRevokeEntitlement(ctx context.Context, entitlementID int64) error {
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return err
	}
	ent, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return err
	}
	if ent.Status == SubscriptionStatusRevoked {
		return nil
	}

	now := time.Now()
	startsAt := ent.StartsAt
	if startsAt.After(now) {
		startsAt = now
	}
	expiresAt := ent.ExpiresAt
	if expiresAt.After(now) {
		expiresAt = now
	}
	notes := appendSubscriptionNotes(ent.Notes, "revoked by admin")
	if err := entitlementSvc.entitlementRepo.UpdateTerm(ctx, entitlementID, startsAt, expiresAt, SubscriptionStatusRevoked, notes); err != nil {
		return err
	}
	s.invalidateEntitlementLegacyAlias(ent)
	return nil
}

func (s *SubscriptionService) adminRestoreEntitlement(ctx context.Context, entitlementID int64) (*UserSubscription, error) {
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return nil, err
	}
	ent, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if ent.Status != SubscriptionStatusRevoked {
		return nil, ErrSubscriptionNotRevoked
	}

	restoredStatus := SubscriptionStatusActive
	if !ent.ExpiresAt.After(time.Now()) {
		restoredStatus = SubscriptionStatusExpired
	}
	notes := appendSubscriptionNotes(ent.Notes, "restored by admin")
	if err := entitlementSvc.entitlementRepo.UpdateTerm(ctx, entitlementID, ent.StartsAt, ent.ExpiresAt, restoredStatus, notes); err != nil {
		return nil, err
	}
	refreshed, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	s.invalidateEntitlementLegacyAlias(refreshed)
	return adminSubscriptionFromEntitlement(refreshed), nil
}

func (s *SubscriptionService) adminEntitlementService() (*SubscriptionEntitlementService, error) {
	if s == nil || s.entitlementSvc == nil || s.entitlementSvc.entitlementRepo == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	return s.entitlementSvc, nil
}

func (s *SubscriptionService) invalidateEntitlementLegacyAlias(ent *SubscriptionEntitlement) {
	if s == nil || ent == nil || ent.LegacySubscriptionID == nil {
		return
	}
	groupID := entitlementAdminGroupID(ent)
	if groupID <= 0 {
		return
	}
	s.InvalidateSubCache(ent.UserID, groupID)
	s.waitSubCacheInvalidation()
}

func adminSubscriptionFromEntitlement(ent *SubscriptionEntitlement) *UserSubscription {
	if ent == nil {
		return nil
	}
	groupID := entitlementAdminGroupID(ent)
	sub := &UserSubscription{
		ID:                 -ent.ID,
		UserID:             ent.UserID,
		GroupID:            groupID,
		StartsAt:           ent.StartsAt,
		ExpiresAt:          ent.ExpiresAt,
		Status:             ent.Status,
		DailyWindowStart:   cloneTimePtrForAdminEntitlement(ent.DailyWindowStart),
		WeeklyWindowStart:  cloneTimePtrForAdminEntitlement(ent.WeeklyWindowStart),
		MonthlyWindowStart: cloneTimePtrForAdminEntitlement(ent.MonthlyWindowStart),
		DailyUsageUSD:      ent.DailyUsageUSD,
		WeeklyUsageUSD:     ent.WeeklyUsageUSD,
		MonthlyUsageUSD:    ent.MonthlyUsageUSD,
		AssignedBy:         cloneInt64PtrForAdminEntitlement(ent.AssignedBy),
		AssignedAt:         ent.AssignedAt,
		CreatedAt:          ent.CreatedAt,
		UpdatedAt:          ent.UpdatedAt,
		EntitlementOnly:    ent.LegacySubscriptionID == nil,
		EntitlementLink: &UserSubscriptionEntitlementLink{
			EntitlementID:      ent.ID,
			PlanID:             cloneInt64PtrForAdminEntitlement(ent.PlanID),
			PlanName:           cloneStringPtr(&ent.Name),
			Status:             ent.Status,
			ExpiresAt:          ent.ExpiresAt,
			DailyWindowStart:   cloneTimePtrForAdminEntitlement(ent.DailyWindowStart),
			WeeklyWindowStart:  cloneTimePtrForAdminEntitlement(ent.WeeklyWindowStart),
			MonthlyWindowStart: cloneTimePtrForAdminEntitlement(ent.MonthlyWindowStart),
			DailyLimitUSD:      cloneFloat64PtrForAdminEntitlement(ent.DailyLimitUSD),
			WeeklyLimitUSD:     cloneFloat64PtrForAdminEntitlement(ent.WeeklyLimitUSD),
			MonthlyLimitUSD:    cloneFloat64PtrForAdminEntitlement(ent.MonthlyLimitUSD),
			DailyUsageUSD:      ent.DailyUsageUSD,
			WeeklyUsageUSD:     ent.WeeklyUsageUSD,
			MonthlyUsageUSD:    ent.MonthlyUsageUSD,
			PrimaryGroupID:     cloneInt64PtrForAdminEntitlement(ent.PrimaryGroupID),
			OveragePolicy:      ent.OveragePolicy,
		},
	}
	if group := entitlementAdminGroup(ent); group != nil {
		sub.Group = group
	}
	if sub.EntitlementLink.PrimaryGroupID == nil && groupID > 0 {
		sub.EntitlementLink.PrimaryGroupID = &groupID
	}
	return sub
}

func entitlementAdminGroupID(ent *SubscriptionEntitlement) int64 {
	if ent == nil {
		return 0
	}
	if ent.PrimaryGroupID != nil && *ent.PrimaryGroupID > 0 {
		return *ent.PrimaryGroupID
	}
	if len(ent.GroupGrants) > 0 {
		best := ent.GroupGrants[0]
		for _, grant := range ent.GroupGrants[1:] {
			if grant.SortOrder < best.SortOrder || (grant.SortOrder == best.SortOrder && grant.GroupID < best.GroupID) {
				best = grant
			}
		}
		return best.GroupID
	}
	if len(ent.Groups) > 0 {
		return ent.Groups[0].ID
	}
	return 0
}

func entitlementAdminGroup(ent *SubscriptionEntitlement) *Group {
	if ent == nil {
		return nil
	}
	groupID := entitlementAdminGroupID(ent)
	for i := range ent.GroupGrants {
		if ent.GroupGrants[i].GroupID == groupID && ent.GroupGrants[i].Group != nil {
			cp := *ent.GroupGrants[i].Group
			return &cp
		}
	}
	for i := range ent.Groups {
		if ent.Groups[i].ID == groupID {
			cp := ent.Groups[i]
			return &cp
		}
	}
	return nil
}

func cloneInt64PtrForAdminEntitlement(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFloat64PtrForAdminEntitlement(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneTimePtrForAdminEntitlement(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}
