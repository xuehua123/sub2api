package service

import (
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const SubscriptionPlanExternalMappingSourceSub2PaymentPage = "sub2-payment-page"

var ErrSubscriptionPlanExternalMappingNotFound = infraerrors.NotFound("SUBSCRIPTION_PLAN_EXTERNAL_MAPPING_NOT_FOUND", "subscription plan external mapping not found")

type SubscriptionPlanExternalMapping struct {
	ID                 int64
	Source             string
	LegacyGroupID      int64
	LegacyValidityDays int
	LegacyValue        float64
	PlanID             int64
	Enabled            bool
	Priority           int
	Notes              string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          *time.Time
}
