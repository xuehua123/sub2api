package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	SubscriptionEntitlementSourceUnknown      = "unknown"
	SubscriptionEntitlementSourcePaymentOrder = "payment_order"
	SubscriptionEntitlementSourceRedeemCode   = "redeem_code"
	SubscriptionEntitlementSourceAdminAssign  = "admin_subscription_assign"

	SubscriptionEntitlementOverageBlock           = "block"
	SubscriptionEntitlementOverageBalanceFallback = "balance_fallback"
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
	ErrSubscriptionEntitlementPlanRequired  = infraerrors.BadRequest("SUBSCRIPTION_ENTITLEMENT_PLAN_REQUIRED", "subscription plan is required")
	ErrSubscriptionEntitlementPlanInvalid   = infraerrors.BadRequest("SUBSCRIPTION_ENTITLEMENT_PLAN_INVALID", "subscription plan cannot grant an entitlement")
	ErrSubscriptionEntitlementPlanNotFound  = infraerrors.NotFound("SUBSCRIPTION_ENTITLEMENT_PLAN_NOT_FOUND", "subscription plan not found")
	ErrSubscriptionEntitlementTermConflict  = infraerrors.Conflict("SUBSCRIPTION_ENTITLEMENT_TERM_CONFLICT", "subscription entitlement term changed concurrently")
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

	PurchasePrice    *float64
	PurchaseCurrency string
	QuotaUSD         *float64
	QuotaPeriod      string
	UnitCostPerUSD   *float64

	SourceID           *int64
	SourceExternalID   *string
	SourceRedeemCodeID *int64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time

	Groups      []Group
	GroupGrants []SubscriptionEntitlementGroupGrant
}

type SubscriptionEntitlementGroupGrant struct {
	GroupID   int64
	SortOrder int
	Enabled   bool
	Group     *Group
}

type SubscriptionEntitlementSourceRef struct {
	SourceType           string
	SourceID             *int64
	SourceExternalID     *string
	SourceRedeemCodeID   *int64
	LegacySubscriptionID *int64
	AssignedBy           *int64
	AssignedAt           time.Time
}

type SubscriptionEntitlementFulfillment struct {
	ID            int64
	EntitlementID int64
	UserID        int64
	PlanID        *int64

	SourceType         string
	SourceID           *int64
	SourceExternalID   *string
	SourceRedeemCodeID *int64

	ValidityDays int
	StartsAt     time.Time
	ExpiresAt    time.Time

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time
}

type SubscriptionEntitlementPlan struct {
	ID           int64
	GroupID      int64
	Name         string
	Description  string
	Price        float64
	Currency     string
	ValidityDays int
	ValidityUnit string
	AccessScope  string

	AllowedPlatforms []string

	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64
	OveragePolicy   string

	Features    string
	ProductName string
	ForSale     bool
	SortOrder   int

	Groups []SubscriptionEntitlementPlanGroup
}

type SubscriptionEntitlementPlanGroup struct {
	GroupID   int64
	SortOrder int
	Enabled   bool
	Group     *Group
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

type AdvanceEntitlementMonthlyCycleResult struct {
	Entitlement           *SubscriptionEntitlement `json:"entitlement"`
	PreviousExpiresAt     time.Time                `json:"previous_expires_at"`
	NewExpiresAt          time.Time                `json:"new_expires_at"`
	DeductedDays          int                      `json:"deducted_days"`
	DeductedSeconds       int64                    `json:"deducted_seconds"`
	PreviousMonthlyUsage  float64                  `json:"previous_monthly_usage_usd"`
	NewMonthlyWindowStart time.Time                `json:"new_monthly_window_start"`
}

type SubscriptionEntitlementMonthlyCycleSnapshot struct {
	ID     int64
	UserID int64
	PlanID *int64

	Status    string
	StartsAt  time.Time
	ExpiresAt time.Time

	MonthlyLimitUSD    *float64
	MonthlyUsageUSD    float64
	MonthlyWindowStart *time.Time
}

type SubscriptionEntitlementMonthlyCycleUpdate struct {
	EntitlementID         int64
	UserID                int64
	NewExpiresAt          time.Time
	NewMonthlyWindowStart time.Time
	NewMonthlyUsageUSD    float64
	UpdatedAt             time.Time
}

type SubscriptionEntitlementCycleResetLog struct {
	UserID        int64
	EntitlementID int64
	PlanID        *int64

	PreviousExpiresAt          time.Time
	NewExpiresAt               time.Time
	PreviousMonthlyUsageUSD    float64
	PreviousMonthlyWindowStart *time.Time
	NewMonthlyWindowStart      time.Time
	DeductedDays               int
	DeductedSeconds            int64
	ResetMonthlyUsage          bool
	Mode                       MonthlyCycleAdjustmentMode
	Reason                     string
	AdminID                    *int64
}
