package service

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func validateActiveLinkedLegacySubscription(
	ctx context.Context,
	repo UserSubscriptionRepository,
	ent *SubscriptionEntitlement,
) error {
	if ent == nil {
		return ErrSubscriptionEntitlementNotFound
	}
	if ent.LegacySubscriptionID == nil {
		return nil
	}
	if *ent.LegacySubscriptionID <= 0 || repo == nil {
		return ErrSubscriptionEntitlementAliasUnavailable
	}

	alias, err := repo.GetByIDIncludeDeletedForUpdate(ctx, *ent.LegacySubscriptionID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return fmt.Errorf("linked legacy subscription %d is unavailable: %w", *ent.LegacySubscriptionID, ErrSubscriptionEntitlementAliasUnavailable)
		}
		return fmt.Errorf("lock linked legacy subscription %d: %w", *ent.LegacySubscriptionID, err)
	}
	if alias == nil || alias.ID <= 0 || alias.ID != *ent.LegacySubscriptionID {
		return ErrSubscriptionEntitlementAliasUnavailable
	}
	if alias.UserID != ent.UserID {
		return fmt.Errorf("linked legacy subscription %d belongs to another user: %w", alias.ID, ErrSubscriptionEntitlementNotFound)
	}
	if alias.DeletedAt != nil {
		return fmt.Errorf("linked legacy subscription %d is revoked: %w", alias.ID, ErrSubscriptionEntitlementAliasUnavailable)
	}
	return nil
}

func syncLinkedLegacySubscriptionLifecycle(
	ctx context.Context,
	repo UserSubscriptionRepository,
	ent *SubscriptionEntitlement,
) error {
	if ent == nil || ent.LegacySubscriptionID == nil {
		return nil
	}
	if *ent.LegacySubscriptionID <= 0 || repo == nil {
		return ErrSubscriptionEntitlementAliasUnavailable
	}

	alias, err := repo.GetByIDIncludeDeletedForUpdate(ctx, *ent.LegacySubscriptionID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return fmt.Errorf("linked legacy subscription %d is unavailable: %w", *ent.LegacySubscriptionID, ErrSubscriptionEntitlementAliasUnavailable)
		}
		return fmt.Errorf("lock linked legacy subscription %d: %w", *ent.LegacySubscriptionID, err)
	}
	if alias == nil || alias.ID <= 0 || alias.ID != *ent.LegacySubscriptionID {
		return ErrSubscriptionEntitlementAliasUnavailable
	}
	if alias.UserID != ent.UserID {
		return fmt.Errorf("linked legacy subscription %d belongs to another user: %w", alias.ID, ErrSubscriptionEntitlementNotFound)
	}
	lifecycle := linkedLegacySubscriptionLifecycleState(ent)
	if alias.DeletedAt != nil {
		if _, err := repo.RestoreWithLifecycle(ctx, alias.ID, lifecycle); err != nil {
			return fmt.Errorf("restore linked legacy subscription %d: %w", alias.ID, err)
		}
		return nil
	}

	alias.StartsAt = lifecycle.StartsAt
	alias.ExpiresAt = lifecycle.ExpiresAt
	alias.Status = lifecycle.Status
	alias.DailyWindowStart = lifecycle.DailyWindowStart
	alias.WeeklyWindowStart = lifecycle.WeeklyWindowStart
	alias.MonthlyWindowStart = lifecycle.MonthlyWindowStart
	alias.DailyUsageUSD = lifecycle.DailyUsageUSD
	alias.WeeklyUsageUSD = lifecycle.WeeklyUsageUSD
	alias.MonthlyUsageUSD = lifecycle.MonthlyUsageUSD
	if err := repo.Update(ctx, alias); err != nil {
		return fmt.Errorf("sync linked legacy subscription %d: %w", alias.ID, err)
	}
	return nil
}

func linkedLegacySubscriptionLifecycleState(ent *SubscriptionEntitlement) UserSubscriptionLifecycleState {
	return UserSubscriptionLifecycleState{
		StartsAt:           ent.StartsAt,
		ExpiresAt:          ent.ExpiresAt,
		Status:             ent.Status,
		DailyWindowStart:   cloneTimePtrForAdminEntitlement(ent.DailyWindowStart),
		WeeklyWindowStart:  cloneTimePtrForAdminEntitlement(ent.WeeklyWindowStart),
		MonthlyWindowStart: cloneTimePtrForAdminEntitlement(ent.MonthlyWindowStart),
		DailyUsageUSD:      ent.DailyUsageUSD,
		WeeklyUsageUSD:     ent.WeeklyUsageUSD,
		MonthlyUsageUSD:    ent.MonthlyUsageUSD,
	}
}

func deleteLinkedLegacySubscription(
	ctx context.Context,
	repo UserSubscriptionRepository,
	ent *SubscriptionEntitlement,
) error {
	if ent == nil || ent.LegacySubscriptionID == nil || *ent.LegacySubscriptionID <= 0 {
		return nil
	}
	if repo == nil {
		return ErrSubscriptionEntitlementAliasUnavailable
	}

	alias, err := repo.GetByIDIncludeDeletedForUpdate(ctx, *ent.LegacySubscriptionID)
	if err != nil {
		if errors.Is(err, ErrSubscriptionNotFound) {
			return nil
		}
		return fmt.Errorf("lock linked legacy subscription %d before revoke: %w", *ent.LegacySubscriptionID, err)
	}
	if alias.UserID != ent.UserID {
		return fmt.Errorf("linked legacy subscription %d belongs to another user: %w", alias.ID, ErrSubscriptionEntitlementNotFound)
	}
	if alias.DeletedAt != nil {
		return nil
	}
	if err := repo.Delete(ctx, alias.ID); err != nil {
		return fmt.Errorf("revoke linked legacy subscription %d: %w", alias.ID, err)
	}
	return nil
}

func (s *SubscriptionEntitlementService) revokeEntitlementAndLinkedAliasLocked(
	ctx context.Context,
	ent *SubscriptionEntitlement,
	now time.Time,
	note string,
) error {
	if ent == nil {
		return ErrSubscriptionEntitlementNotFound
	}
	if ent.Status == SubscriptionStatusRevoked {
		return deleteLinkedLegacySubscription(ctx, s.legacySubscriptionRepo, ent)
	}
	if err := syncLinkedLegacySubscriptionLifecycle(ctx, s.legacySubscriptionRepo, ent); err != nil {
		return err
	}

	startsAt := ent.StartsAt
	if startsAt.After(now) {
		startsAt = now
	}
	expiresAt := ent.ExpiresAt
	if expiresAt.After(now) {
		expiresAt = now
	}
	notes := appendSubscriptionNotes(ent.Notes, note)
	if err := s.entitlementRepo.UpdateTerm(ctx, ent.ID, startsAt, expiresAt, SubscriptionStatusRevoked, notes); err != nil {
		return err
	}
	return deleteLinkedLegacySubscription(ctx, s.legacySubscriptionRepo, ent)
}
