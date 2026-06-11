package service

import (
	"context"
	"strings"
)

type SubscriptionPlanExternalMappingService struct {
	repo SubscriptionPlanExternalMappingRepository
}

func NewSubscriptionPlanExternalMappingService(repo SubscriptionPlanExternalMappingRepository) *SubscriptionPlanExternalMappingService {
	return &SubscriptionPlanExternalMappingService{repo: repo}
}

func (s *SubscriptionPlanExternalMappingService) FindEnabled(ctx context.Context, source string, legacyGroupID int64, legacyValidityDays int, legacyValue float64) (*SubscriptionPlanExternalMapping, error) {
	if s == nil || s.repo == nil {
		return nil, ErrSubscriptionPlanExternalMappingNotFound
	}
	source = strings.TrimSpace(source)
	if source == "" || legacyGroupID <= 0 || legacyValidityDays <= 0 || legacyValue <= 0 {
		return nil, ErrSubscriptionPlanExternalMappingNotFound
	}
	return s.repo.FindEnabled(ctx, source, legacyGroupID, legacyValidityDays, legacyValue)
}

func (s *SubscriptionPlanExternalMappingService) FindSub2PaymentPageLegacyMapping(ctx context.Context, legacyGroupID int64, legacyValidityDays int, legacyValue float64) (*SubscriptionPlanExternalMapping, error) {
	return s.FindEnabled(ctx, SubscriptionPlanExternalMappingSourceSub2PaymentPage, legacyGroupID, legacyValidityDays, legacyValue)
}
