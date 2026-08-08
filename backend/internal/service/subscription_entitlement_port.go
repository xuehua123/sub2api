package service

import (
	"context"
	"time"
)

type SubscriptionEntitlementRepository interface {
	WithUserEntitlementMutationTx(ctx context.Context, userID int64, fn func(context.Context) error) error
	Create(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error
	CreateTx(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error
	CreateWithFulfillment(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64, fulfillment *SubscriptionEntitlementFulfillment) error
	GetByID(ctx context.Context, id int64) (*SubscriptionEntitlement, error)
	GetByIDForUpdate(ctx context.Context, id int64) (*SubscriptionEntitlement, error)
	GetBySourceID(ctx context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlement, error)
	GetBySourceExternalID(ctx context.Context, sourceType, sourceExternalID string) (*SubscriptionEntitlement, error)
	GetBySourceRedeemCodeID(ctx context.Context, redeemCodeID int64) (*SubscriptionEntitlement, error)
	GetFulfillmentBySourceID(ctx context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlementFulfillment, error)
	GetFulfillmentBySourceExternalID(ctx context.Context, sourceType, sourceExternalID string) (*SubscriptionEntitlementFulfillment, error)
	GetFulfillmentBySourceRedeemCodeID(ctx context.Context, redeemCodeID int64) (*SubscriptionEntitlementFulfillment, error)
	GetActiveCoveringGroup(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error)
	ListByUserID(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error)
	ListByUserPlanID(ctx context.Context, userID, planID int64) ([]SubscriptionEntitlement, error)
	ListByUserPlanIDForUpdate(ctx context.Context, userID, planID int64) ([]SubscriptionEntitlement, error)
	ListActiveByUserID(ctx context.Context, userID int64) ([]SubscriptionEntitlement, error)
	ListActiveCoveringGroupForUser(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error)
	UpdateTerm(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string) error
	UpdateTermAndSource(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source SubscriptionEntitlementSourceRef) error
	ExtendWithFulfillment(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source SubscriptionEntitlementSourceRef, fulfillment *SubscriptionEntitlementFulfillment, resetUsage bool, resetDailyStart, resetPeriodicStart time.Time) error
	ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error
	ResetUsage(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error
	ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error
	ApplyEntitlementUsage(ctx context.Context, id int64, costUSD float64, now time.Time) (*EntitlementUsageApplyResult, error)
	ReplaceGroups(ctx context.Context, id int64, groupIDs []int64) error
}

type SubscriptionEntitlementPlanRepository interface {
	GetEntitlementPlan(ctx context.Context, planID int64) (*SubscriptionEntitlementPlan, error)
}
