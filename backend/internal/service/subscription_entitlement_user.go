package service

import (
	"context"
	"time"
)

func (s *SubscriptionEntitlementService) ListUserEntitlements(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error) {
	if s == nil || s.entitlementRepo == nil || userID <= 0 {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	return s.entitlementRepo.ListByUserID(ctx, userID)
}

func (s *SubscriptionEntitlementService) ListActiveUserEntitlements(ctx context.Context, userID int64, now time.Time) ([]SubscriptionEntitlement, error) {
	return s.ListActiveBindingsByUser(ctx, userID, now)
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
	return ent, nil
}
