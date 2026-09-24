package service

import (
	"context"
	"errors"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type APIKeyEntitlementAuthResult struct {
	Entitlement *SubscriptionEntitlement
	Group       *Group

	Switched    bool
	FromGroupID int64
	ToGroupID   int64
	Reason      string

	UseBalanceFallback bool
	LegacySubscription *UserSubscription
}

func (s *APIKeyService) IsSubscriptionEntitlementsV2Enabled(ctx context.Context) bool {
	return s.subscriptionEntitlementsRuntime(ctx).Enabled
}

func (s *APIKeyService) ResolveEntitlementForAPIKeyAuth(ctx context.Context, apiKey *APIKey, req SubscriptionSwitchRequest, currentGroupUnavailable bool, autoAdvance ...bool) (*APIKeyEntitlementAuthResult, error) {
	if s == nil || !s.subscriptionEntitlementsRuntime(ctx).Enabled {
		return nil, nil
	}
	if apiKey == nil || apiKey.User == nil || apiKey.Group == nil {
		return nil, ErrGroupNotAllowed
	}
	if s.subscriptionEntitlementSvc == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}

	fromGroup := apiKey.Group
	fromGroupID := fromGroup.ID
	if !apiKey.User.AllowsGroupByPolicy(fromGroup.ID, fromGroup.IsExclusive) {
		return nil, ErrGroupNotAllowed
	}
	if !fromGroup.SupportsSubscriptionAccess() {
		currentGroupUnavailable = true
	}
	currentErr := entitlementCurrentGroupError(fromGroup, req, currentGroupUnavailable)
	allowAuto := len(autoAdvance) > 0 && autoAdvance[0]
	resolution, err := s.resolveEntitlementAuthBinding(ctx, apiKey.User.ID, fromGroupID, apiKey.SubscriptionEntitlementID, allowAuto)
	if err != nil {
		if !apiKey.AutoSwitchGroupEnabled || apiKey.SubscriptionEntitlementID == nil || !errors.Is(err, ErrGroupNotAllowed) {
			return nil, err
		}
		entitlement, bindingErr := s.subscriptionEntitlementSvc.resolveExplicitBinding(ctx, apiKey.User.ID, *apiKey.SubscriptionEntitlementID, time.Time{}, allowAuto)
		if bindingErr != nil {
			return nil, bindingErr
		}
		if entitlement.UserID != apiKey.User.ID {
			return nil, ErrGroupNotAllowed
		}
		switchGroup := selectEntitlementSwitchGroup(entitlement, fromGroup, fromGroupID, req, apiKey.User)
		if switchGroup == nil {
			return nil, err
		}
		return s.finishEntitlementAuth(ctx, entitlement, switchGroup, true, fromGroupID, entitlementSwitchReason(err), apiKey.User.ID, allowAuto)
	}
	if currentErr == nil {
		return s.finishEntitlementAuth(ctx, resolution.Entitlement, resolution.Group, false, fromGroupID, "", apiKey.User.ID, allowAuto)
	}
	if !apiKey.AutoSwitchGroupEnabled {
		return nil, currentErr
	}

	switchGroup := selectEntitlementSwitchGroup(resolution.Entitlement, fromGroup, fromGroupID, req, apiKey.User)
	if switchGroup == nil {
		return nil, currentErr
	}
	return s.finishEntitlementAuth(ctx, resolution.Entitlement, switchGroup, true, fromGroupID, entitlementSwitchReason(currentErr), apiKey.User.ID, allowAuto)
}

// Authentication may defer an exhausted monthly quota, but never consumes a cycle.
func (s *APIKeyService) finishEntitlementAuth(ctx context.Context, ent *SubscriptionEntitlement, group *Group, switched bool, fromGroupID int64, reason string, userID int64, allowAuto bool) (*APIKeyEntitlementAuthResult, error) {
	now := s.subscriptionEntitlementSvc.inputNow(time.Time{})
	if err := validateEntitlementAvailabilityAt(ent, now); err != nil {
		return nil, err
	}
	if err := s.subscriptionEntitlementSvc.CheckAndResetWindows(ctx, ent, now); err != nil {
		return nil, err
	}
	if allowAuto && ent.AutoAdvanceMonthly && ent.MonthlyLimitUSD != nil && *ent.MonthlyLimitUSD > 0 && ent.MonthlyUsageUSD >= *ent.MonthlyLimitUSD && ent.CheckDailyLimit(0) && ent.CheckWeeklyLimit(0) && PreviewEntitlementMonthlyCycle(ent, now).CanAdvance {
		result := s.entitlementAuthResult(ctx, ent, group, switched, fromGroupID, group.ID, reason, userID)
		result.LegacySubscription = nil
		return result, nil
	}
	if _, err := s.subscriptionEntitlementSvc.ValidateAndCheckLimits(ent, 0, now); err != nil {
		return nil, err
	}
	return s.entitlementAuthResult(ctx, ent, group, switched, fromGroupID, group.ID, reason, userID), nil
}

// AdmitEntitlementGeneration runs after local generation checks, using current settings.
func (s *APIKeyService) AdmitEntitlementGeneration(ctx context.Context, apiKey *APIKey, entitlementID int64, expectedCost ...float64) (result *APIKeyEntitlementAuthResult, admissionErr error) {
	defer func() {
		if admissionErr != nil && infraerrors.Code(admissionErr) >= 500 {
			admissionErr = ErrBillingServiceUnavailable.WithCause(admissionErr)
		}
	}()
	if s == nil || s.subscriptionEntitlementSvc == nil || apiKey == nil || apiKey.User == nil || apiKey.Group == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	now := s.subscriptionEntitlementSvc.inputNow(time.Time{})
	resolution, err := s.subscriptionEntitlementSvc.ValidateBindingForGroup(ctx, apiKey.User.ID, apiKey.Group.ID, entitlementID, now)
	if err != nil {
		return nil, err
	}
	ent := resolution.Entitlement
	if err := s.subscriptionEntitlementSvc.CheckAndResetWindows(ctx, ent, now); err != nil {
		return nil, err
	}
	ent, err = s.subscriptionEntitlementSvc.TryAutoAdvanceMonthlyCycle(ctx, ent, expectedCost...)
	if err != nil {
		return nil, err
	}
	if _, err := s.subscriptionEntitlementSvc.ValidateAndCheckLimits(ent, 0, s.subscriptionEntitlementSvc.inputNow(time.Time{})); err != nil {
		return nil, err
	}
	return s.entitlementAuthResult(ctx, ent, apiKey.Group, false, apiKey.Group.ID, apiKey.Group.ID, "", apiKey.User.ID), nil
}

func (s *APIKeyService) CompareAndSwapGroupIDWithEntitlement(ctx context.Context, apiKey *APIKey, oldGroupID, newGroupID int64, resolvedEntitlementID int64) (bool, error) {
	if apiKey == nil {
		return false, ErrAPIKeyNotFound
	}
	if oldGroupID <= 0 || newGroupID <= 0 || oldGroupID == newGroupID || resolvedEntitlementID <= 0 {
		return false, nil
	}
	newEntitlementID := resolvedEntitlementID
	swapped, err := s.apiKeyRepo.CompareAndSwapGroupIDWithEntitlement(ctx, apiKey.ID, oldGroupID, newGroupID, apiKey.SubscriptionEntitlementID, &newEntitlementID)
	if err != nil {
		return false, err
	}
	if swapped {
		apiKey.GroupID = &newGroupID
		apiKey.SubscriptionEntitlementID = &newEntitlementID
		s.InvalidateAuthCacheByKey(ctx, apiKey.Key)
	}
	return swapped, nil
}

func (s *APIKeyService) resolveEntitlementAuthBinding(ctx context.Context, userID, groupID int64, explicitEntitlementID *int64, deferQuota ...bool) (*EntitlementResolution, error) {
	now := s.subscriptionEntitlementSvc.inputNow(time.Time{})
	var (
		resolution *EntitlementResolution
		err        error
	)
	if explicitEntitlementID != nil {
		resolution, err = s.subscriptionEntitlementSvc.ValidateBindingForGroup(ctx, userID, groupID, *explicitEntitlementID, now)
	} else {
		resolution, err = s.subscriptionEntitlementSvc.ResolveBindingForGroup(ctx, userID, groupID, now)
	}
	if err != nil {
		return nil, err
	}
	if resolution == nil || resolution.Entitlement == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if len(deferQuota) > 0 && deferQuota[0] {
		return resolution, nil
	}
	if err := s.subscriptionEntitlementSvc.CheckAndResetWindows(ctx, resolution.Entitlement, now); err != nil {
		return nil, err
	}
	if _, err := s.subscriptionEntitlementSvc.ValidateAndCheckLimits(resolution.Entitlement, 0, now); err != nil {
		return nil, err
	}
	return resolution, nil
}

func (s *APIKeyService) entitlementAuthResult(ctx context.Context, ent *SubscriptionEntitlement, group *Group, switched bool, fromGroupID, toGroupID int64, reason string, userID int64) *APIKeyEntitlementAuthResult {
	if ent == nil {
		return nil
	}
	return &APIKeyEntitlementAuthResult{
		Entitlement:        ent,
		Group:              group,
		Switched:           switched,
		FromGroupID:        fromGroupID,
		ToGroupID:          toGroupID,
		Reason:             reason,
		UseBalanceFallback: ent.OveragePolicy == SubscriptionEntitlementOverageBalanceFallback,
		LegacySubscription: s.compatibleLegacySubscription(ctx, ent, group, userID),
	}
}

func (s *APIKeyService) compatibleLegacySubscription(ctx context.Context, ent *SubscriptionEntitlement, group *Group, userID int64) *UserSubscription {
	if s == nil || s.userSubRepo == nil || ent == nil || ent.LegacySubscriptionID == nil || *ent.LegacySubscriptionID <= 0 || group == nil {
		return nil
	}
	sub, err := s.userSubRepo.GetByID(ctx, *ent.LegacySubscriptionID)
	if err != nil || sub == nil {
		return nil
	}
	if sub.UserID != userID || sub.GroupID != group.ID || !sub.IsActive() {
		return nil
	}
	return sub
}

func entitlementCurrentGroupError(group *Group, req SubscriptionSwitchRequest, currentGroupUnavailable bool) error {
	if currentGroupUnavailable {
		return ErrGroupNotAllowed
	}
	return subscriptionSwitchRequestEligibilityError(group, req)
}

func selectEntitlementSwitchGroup(ent *SubscriptionEntitlement, currentGroup *Group, currentGroupID int64, req SubscriptionSwitchRequest, user *User) *Group {
	if ent == nil || currentGroup == nil {
		return nil
	}
	for _, grant := range ent.GroupGrants {
		group := entitlementGrantGroup(ent, grant)
		if group == nil || group.ID == currentGroupID || !group.SupportsSubscriptionAccess() || !group.IsActive() {
			continue
		}
		if user != nil && !user.AllowsGroupByPolicy(group.ID, group.IsExclusive) {
			continue
		}
		if err := subscriptionSwitchRequestEligibilityError(group, req); err != nil {
			continue
		}
		if !subscriptionSwitchGroupsCompatible(currentGroup, group, req) {
			continue
		}
		return group
	}
	return nil
}

func entitlementGrantGroup(ent *SubscriptionEntitlement, grant SubscriptionEntitlementGroupGrant) *Group {
	if !grant.Enabled || grant.GroupID <= 0 {
		return nil
	}
	if grant.Group != nil {
		return grant.Group
	}
	for i := range ent.Groups {
		if ent.Groups[i].ID == grant.GroupID {
			return &ent.Groups[i]
		}
	}
	return nil
}

func entitlementSwitchReason(err error) string {
	switch {
	case errors.Is(err, ErrSubscriptionEndpointUnsupported):
		return "endpoint_unsupported"
	case errors.Is(err, ErrGroupNotAllowed):
		return "group_unavailable"
	default:
		return "entitlement_group_unavailable"
	}
}
