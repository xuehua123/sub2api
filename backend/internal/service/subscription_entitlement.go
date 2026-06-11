package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SubscriptionEntitlementSourceUnknown      = "unknown"
	SubscriptionEntitlementSourcePaymentOrder = "payment_order"
	SubscriptionEntitlementSourceRedeemCode   = "redeem_code"

	SubscriptionEntitlementOverageBlock = "block"
)

var (
	ErrSubscriptionEntitlementNotFound      = infraerrors.NotFound("SUBSCRIPTION_ENTITLEMENT_NOT_FOUND", "subscription entitlement not found")
	ErrSubscriptionEntitlementExpired       = infraerrors.Forbidden("SUBSCRIPTION_ENTITLEMENT_EXPIRED", "subscription entitlement has expired")
	ErrSubscriptionEntitlementInactive      = infraerrors.Forbidden("SUBSCRIPTION_ENTITLEMENT_INACTIVE", "subscription entitlement is inactive")
	ErrSubscriptionEntitlementNilInput      = infraerrors.BadRequest("SUBSCRIPTION_ENTITLEMENT_NIL_INPUT", "subscription entitlement input cannot be nil")
	ErrSubscriptionEntitlementAlreadyExists = infraerrors.Conflict("SUBSCRIPTION_ENTITLEMENT_ALREADY_EXISTS", "subscription entitlement already exists")
	ErrSubscriptionEntitlementInvalidReset  = infraerrors.BadRequest("SUBSCRIPTION_ENTITLEMENT_INVALID_RESET", "at least one entitlement usage window must be reset")
	ErrSubscriptionEntitlementInvalidUsage  = infraerrors.BadRequest("SUBSCRIPTION_ENTITLEMENT_INVALID_USAGE", "entitlement usage cost must be non-negative")
	ErrSubscriptionEntitlementQuotaExceeded = infraerrors.TooManyRequests("SUBSCRIPTION_ENTITLEMENT_QUOTA_EXCEEDED", "subscription entitlement quota exceeded")
)

type SubscriptionEntitlement struct {
	ID     int64
	UserID int64

	PlanID               *int64
	LegacySubscriptionID *int64
	PrimaryGroupID       *int64

	Name       string
	SourceType string
	Status     string

	StartsAt  time.Time
	ExpiresAt time.Time

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	OveragePolicy string
	PlanSnapshot  map[string]any

	SourceID           *int64
	SourceExternalID   *string
	SourceRedeemCodeID *int64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time

	Groups []Group
}

func (e *SubscriptionEntitlement) IsActive() bool {
	return e != nil && e.Status == SubscriptionStatusActive && time.Now().Before(e.ExpiresAt)
}

type EntitlementResolution struct {
	Entitlement *SubscriptionEntitlement
	Group       *Group
	FromGroupID int64
	ToGroupID   int64
	Switched    bool
	Reason      string

	UseBalanceFallback bool
}

type EntitlementUsageApplyResult struct {
	UpdatedAt time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time
}
