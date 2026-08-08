package service

import (
	"context"
	"time"
)

func (s *SubscriptionEntitlementService) ListUserEntitlements(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error) {
	if s == nil || s.entitlementRepo == nil || userID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	ents, err := s.entitlementRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]SubscriptionEntitlement, 0, len(ents))
	for i := range ents {
		if ents[i].Status == SubscriptionStatusRevoked {
			continue
		}
		s.attachEntitlementEconomics(ctx, &ents[i])
		out = append(out, ents[i])
	}
	return out, nil
}

func (s *SubscriptionEntitlementService) ListActiveUserEntitlements(ctx context.Context, userID int64, now time.Time) ([]SubscriptionEntitlement, error) {
	ents, err := s.ListActiveBindingsByUser(ctx, userID, now)
	if err != nil {
		return nil, err
	}
	for i := range ents {
		s.attachEntitlementEconomics(ctx, &ents[i])
	}
	return ents, nil
}

func (s *SubscriptionEntitlementService) GetUserEntitlementByID(ctx context.Context, userID, entitlementID int64) (*SubscriptionEntitlement, error) {
	if s == nil || s.entitlementRepo == nil || userID <= 0 || entitlementID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	ent, err := s.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if ent.UserID != userID {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if ent.Status == SubscriptionStatusRevoked {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	s.attachEntitlementEconomics(ctx, ent)
	return ent, nil
}

func (s *SubscriptionEntitlementService) RevokeUserEntitlement(ctx context.Context, userID, entitlementID int64, now time.Time) error {
	if s == nil || s.entitlementRepo == nil || userID <= 0 || entitlementID <= 0 {
		return ErrSubscriptionEntitlementNotFound
	}
	if now.IsZero() {
		now = s.inputNow(time.Time{})
	}
	var invalidationTarget *SubscriptionEntitlement
	if err := s.withLockedEntitlement(ctx, entitlementID, userID, func(txCtx context.Context, ent *SubscriptionEntitlement) error {
		invalidationTarget = ent
		return s.revokeEntitlementAndLinkedAliasLocked(txCtx, ent, now, "revoked by user")
	}); err != nil {
		return err
	}
	s.invalidateLinkedLegacyAlias(invalidationTarget)
	return nil
}
