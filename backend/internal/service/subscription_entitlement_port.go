package service

import (
	"context"
	"time"
)

type SubscriptionEntitlementRepository interface {
	Create(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error
	CreateTx(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error
	GetByID(ctx context.Context, id int64) (*SubscriptionEntitlement, error)
	GetBySourceID(ctx context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlement, error)
	GetBySourceExternalID(ctx context.Context, sourceType, sourceExternalID string) (*SubscriptionEntitlement, error)
	GetBySourceRedeemCodeID(ctx context.Context, redeemCodeID int64) (*SubscriptionEntitlement, error)
	GetActiveCoveringGroup(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error)
	ListActiveCoveringGroupForUser(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error)
	UpdateTerm(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string) error
	ResetUsage(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, windowStart time.Time) error
	ApplyEntitlementUsage(ctx context.Context, id int64, costUSD float64, now time.Time) (*EntitlementUsageApplyResult, error)
	ReplaceGroups(ctx context.Context, id int64, groupIDs []int64) error
}
