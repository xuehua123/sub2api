package service

import (
	"context"
	"time"
)

type subscriptionEntitlementMonthlyCycleStore interface {
	WithUserEntitlementMutationTx(ctx context.Context, userID int64, fn func(context.Context) error) error
	LockEntitlementMonthlyCycle(ctx context.Context, userID, entitlementID int64) (*SubscriptionEntitlementMonthlyCycleSnapshot, error)
	UpdateEntitlementMonthlyCycle(ctx context.Context, update SubscriptionEntitlementMonthlyCycleUpdate) error
	InsertEntitlementCycleResetLog(ctx context.Context, log SubscriptionEntitlementCycleResetLog) error
}

func (s *SubscriptionEntitlementService) AdvanceMonthlyCycle(ctx context.Context, userID, entitlementID int64) (*AdvanceEntitlementMonthlyCycleResult, error) {
	if s == nil || s.entitlementRepo == nil || userID <= 0 || entitlementID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	store, ok := s.entitlementRepo.(subscriptionEntitlementMonthlyCycleStore)
	if !ok {
		return nil, ErrSubscriptionEntitlementNotFound
	}

	now := s.inputNow(time.Time{}).Truncate(time.Second)
	var result *AdvanceEntitlementMonthlyCycleResult
	var invalidationTarget *SubscriptionEntitlement
	if err := store.WithUserEntitlementMutationTx(ctx, userID, func(txCtx context.Context) error {
		snapshot, err := store.LockEntitlementMonthlyCycle(txCtx, userID, entitlementID)
		if err != nil {
			return err
		}
		advanced, err := advanceEntitlementMonthlyCycleLocked(txCtx, store, snapshot, now)
		if err != nil {
			return err
		}
		updatedEntitlement, err := s.entitlementRepo.GetByID(txCtx, entitlementID)
		if err != nil {
			return err
		}
		if err := syncLinkedLegacySubscriptionLifecycle(txCtx, s.legacySubscriptionRepo, updatedEntitlement); err != nil {
			return err
		}
		invalidationTarget = updatedEntitlement
		result = advanced
		return nil
	}); err != nil {
		return nil, err
	}
	s.invalidateLinkedLegacyAlias(invalidationTarget)

	entitlement, err := s.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if entitlement.UserID != userID {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	s.attachEntitlementEconomics(ctx, entitlement)
	result.Entitlement = entitlement
	return result, nil
}

func advanceEntitlementMonthlyCycleLocked(
	ctx context.Context,
	store subscriptionEntitlementMonthlyCycleStore,
	snapshot *SubscriptionEntitlementMonthlyCycleSnapshot,
	now time.Time,
) (*AdvanceEntitlementMonthlyCycleResult, error) {
	if snapshot == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if snapshot.UserID <= 0 || snapshot.ID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	switch snapshot.Status {
	case SubscriptionStatusActive:
	case SubscriptionStatusSuspended:
		return nil, ErrSubscriptionEntitlementInactive
	default:
		return nil, ErrSubscriptionEntitlementExpired
	}
	if now.Before(snapshot.StartsAt) || !snapshot.ExpiresAt.After(now) {
		return nil, ErrSubscriptionEntitlementExpired
	}
	if snapshot.MonthlyLimitUSD == nil || *snapshot.MonthlyLimitUSD <= 0 {
		return nil, ErrMonthlyCycleNotExhausted
	}
	if !canAdvanceMonthlyCycleByUsage(snapshot.MonthlyUsageUSD, *snapshot.MonthlyLimitUSD) {
		return nil, ErrMonthlyCycleNotExhausted
	}

	resetAt := monthlyCycleResetAt(snapshot.MonthlyWindowStart, snapshot.StartsAt, now)
	if !resetAt.After(now) {
		return nil, ErrMonthlyCycleNotExhausted
	}
	if !canAdvanceMonthlyCycleByValidity(snapshot.StartsAt, snapshot.ExpiresAt, resetAt) {
		return nil, ErrMonthlyCycleNoFutureTime
	}

	remaining := resetAt.Sub(now)
	deductedSeconds := ceilDurationSeconds(remaining)
	deductedDays := int((time.Duration(deductedSeconds)*time.Second + 24*time.Hour - 1) / (24 * time.Hour))
	if deductedDays <= 0 {
		deductedDays = 1
	}
	newExpiresAt := snapshot.ExpiresAt.Add(-time.Duration(deductedSeconds) * time.Second)
	if !newExpiresAt.After(now) {
		return nil, ErrMonthlyCycleNoFutureTime
	}

	newWindowStart := now
	if err := store.UpdateEntitlementMonthlyCycle(ctx, SubscriptionEntitlementMonthlyCycleUpdate{
		EntitlementID:         snapshot.ID,
		UserID:                snapshot.UserID,
		NewExpiresAt:          newExpiresAt,
		NewMonthlyWindowStart: newWindowStart,
		NewMonthlyUsageUSD:    0,
		UpdatedAt:             now.Add(time.Millisecond),
	}); err != nil {
		return nil, err
	}
	if err := store.InsertEntitlementCycleResetLog(ctx, SubscriptionEntitlementCycleResetLog{
		UserID:                     snapshot.UserID,
		EntitlementID:              snapshot.ID,
		PlanID:                     snapshot.PlanID,
		PreviousExpiresAt:          snapshot.ExpiresAt,
		NewExpiresAt:               newExpiresAt,
		PreviousMonthlyUsageUSD:    snapshot.MonthlyUsageUSD,
		PreviousMonthlyWindowStart: snapshot.MonthlyWindowStart,
		NewMonthlyWindowStart:      newWindowStart,
		DeductedDays:               deductedDays,
		DeductedSeconds:            deductedSeconds,
		ResetMonthlyUsage:          true,
	}); err != nil {
		return nil, err
	}

	return &AdvanceEntitlementMonthlyCycleResult{
		PreviousExpiresAt:     snapshot.ExpiresAt,
		NewExpiresAt:          newExpiresAt,
		DeductedDays:          deductedDays,
		DeductedSeconds:       deductedSeconds,
		PreviousMonthlyUsage:  snapshot.MonthlyUsageUSD,
		NewMonthlyWindowStart: newWindowStart,
	}, nil
}
