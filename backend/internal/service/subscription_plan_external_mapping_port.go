package service

import "context"

type SubscriptionPlanExternalMappingRepository interface {
	FindEnabled(ctx context.Context, source string, legacyGroupID int64, legacyValidityDays int, legacyValue float64) (*SubscriptionPlanExternalMapping, error)
}
