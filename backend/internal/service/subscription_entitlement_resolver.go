package service

import (
	"context"
	"sort"
	"time"
)

type ResolveSubscriptionEntitlementInput struct {
	UserID        int64
	GroupID       int64
	EntitlementID *int64

	AdditionalCostUSD float64
	Now               time.Time
}

func (s *SubscriptionEntitlementService) ResolveForGroup(ctx context.Context, userID, groupID int64, additionalCostUSD float64) (*EntitlementResolution, error) {
	return s.Resolve(ctx, ResolveSubscriptionEntitlementInput{
		UserID:            userID,
		GroupID:           groupID,
		AdditionalCostUSD: additionalCostUSD,
	})
}

func (s *SubscriptionEntitlementService) Resolve(ctx context.Context, input ResolveSubscriptionEntitlementInput) (*EntitlementResolution, error) {
	if s == nil || s.entitlementRepo == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	now := s.inputNow(input.Now)
	if input.EntitlementID != nil {
		ent, err := s.entitlementRepo.GetByID(ctx, *input.EntitlementID)
		if err != nil {
			return nil, err
		}
		if input.UserID > 0 && ent.UserID != input.UserID {
			return nil, ErrGroupNotAllowed
		}
		return s.resolveCandidate(ctx, ent, input.GroupID, input.AdditionalCostUSD, now, "explicit_entitlement")
	}

	candidates, err := s.entitlementRepo.ListActiveCoveringGroupForUser(ctx, input.UserID, input.GroupID)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrGroupNotAllowed
	}
	sortEntitlementCandidatesForGroup(candidates, input.GroupID)

	var quotaExceeded bool
	for i := range candidates {
		ent := candidates[i]
		resolution, err := s.resolveCandidate(ctx, &ent, input.GroupID, input.AdditionalCostUSD, now, "default_entitlement")
		if err == nil {
			return resolution, nil
		}
		if err == ErrSubscriptionEntitlementQuotaExceeded {
			quotaExceeded = true
			continue
		}
		return nil, err
	}
	if quotaExceeded {
		return nil, ErrSubscriptionEntitlementQuotaExceeded
	}
	return nil, ErrGroupNotAllowed
}

func (s *SubscriptionEntitlementService) ValidateBindingForGroup(ctx context.Context, userID, groupID, entitlementID int64, now time.Time) (*EntitlementResolution, error) {
	if s == nil || s.entitlementRepo == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if userID <= 0 || groupID <= 0 || entitlementID <= 0 {
		return nil, ErrGroupNotAllowed
	}
	ent, err := s.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if ent.UserID != userID {
		return nil, ErrGroupNotAllowed
	}
	return s.resolveBindingCandidate(ent, groupID, s.inputNow(now), "explicit_entitlement")
}

// resolveExplicitBinding verifies an explicitly selected entitlement before an
// API key is moved to another group that the same entitlement still covers.
func (s *SubscriptionEntitlementService) resolveExplicitBinding(ctx context.Context, userID, entitlementID int64, now time.Time) (*SubscriptionEntitlement, error) {
	if s == nil || s.entitlementRepo == nil || userID <= 0 || entitlementID <= 0 {
		return nil, ErrGroupNotAllowed
	}
	ent, err := s.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	if ent.UserID != userID {
		return nil, ErrGroupNotAllowed
	}
	resolvedNow := s.inputNow(now)
	if err := validateEntitlementAvailabilityAt(ent, resolvedNow); err != nil {
		return nil, err
	}
	if err := s.CheckAndResetWindows(ctx, ent, resolvedNow); err != nil {
		return nil, err
	}
	if _, err := s.ValidateAndCheckLimits(ent, 0, resolvedNow); err != nil {
		return nil, err
	}
	return ent, nil
}

func (s *SubscriptionEntitlementService) ResolveBindingForGroup(ctx context.Context, userID, groupID int64, now time.Time) (*EntitlementResolution, error) {
	if s == nil || s.entitlementRepo == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if userID <= 0 || groupID <= 0 {
		return nil, ErrGroupNotAllowed
	}
	candidates, err := s.entitlementRepo.ListActiveCoveringGroupForUser(ctx, userID, groupID)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return nil, ErrGroupNotAllowed
	}
	sortEntitlementCandidatesForGroup(candidates, groupID)
	resolutionNow := s.inputNow(now)
	for i := range candidates {
		ent := candidates[i]
		resolution, err := s.resolveBindingCandidate(&ent, groupID, resolutionNow, "default_entitlement")
		if err == nil {
			return resolution, nil
		}
	}
	return nil, ErrGroupNotAllowed
}

func (s *SubscriptionEntitlementService) ListActiveBindingsByUser(ctx context.Context, userID int64, now time.Time) ([]SubscriptionEntitlement, error) {
	if s == nil || s.entitlementRepo == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if userID <= 0 {
		return nil, ErrGroupNotAllowed
	}
	candidates, err := s.entitlementRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	bindingNow := s.inputNow(now)
	out := make([]SubscriptionEntitlement, 0, len(candidates))
	for i := range candidates {
		ent := candidates[i]
		if validateEntitlementAvailabilityAt(&ent, bindingNow) == nil {
			s.attachEntitlementEconomics(ctx, &ent)
			out = append(out, ent)
		}
	}
	return out, nil
}

func (s *SubscriptionEntitlementService) resolveBindingCandidate(ent *SubscriptionEntitlement, groupID int64, now time.Time, reason string) (*EntitlementResolution, error) {
	if ent == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if ent.UserID <= 0 || !entitlementCoversGroup(ent, groupID) {
		return nil, ErrGroupNotAllowed
	}
	if err := validateEntitlementAvailabilityAt(ent, now); err != nil {
		return nil, err
	}
	return &EntitlementResolution{
		Entitlement: ent,
		Group:       entitlementResolutionGroup(ent, groupID),
		FromGroupID: groupID,
		ToGroupID:   groupID,
		Reason:      reason,
	}, nil
}

func (s *SubscriptionEntitlementService) resolveCandidate(ctx context.Context, ent *SubscriptionEntitlement, groupID int64, additionalCostUSD float64, now time.Time, reason string) (*EntitlementResolution, error) {
	if ent == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if !entitlementCoversGroup(ent, groupID) {
		return nil, ErrGroupNotAllowed
	}
	if err := validateEntitlementAvailabilityAt(ent, now); err != nil {
		return nil, err
	}
	if err := s.CheckAndResetWindows(ctx, ent, now); err != nil {
		return nil, err
	}
	if _, err := s.ValidateAndCheckLimits(ent, additionalCostUSD, now); err != nil {
		return nil, err
	}
	return &EntitlementResolution{
		Entitlement: ent,
		Group:       entitlementResolutionGroup(ent, groupID),
		FromGroupID: groupID,
		ToGroupID:   groupID,
		Reason:      reason,
	}, nil
}

func validateEntitlementAvailabilityAt(ent *SubscriptionEntitlement, now time.Time) error {
	if ent == nil {
		return ErrSubscriptionEntitlementNotFound
	}
	if ent.Status != SubscriptionStatusActive {
		return ErrSubscriptionEntitlementInactive
	}
	if now.Before(ent.StartsAt) || !now.Before(ent.ExpiresAt) {
		return ErrSubscriptionEntitlementExpired
	}
	return nil
}

func sortEntitlementCandidatesForGroup(candidates []SubscriptionEntitlement, groupID int64) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if !candidates[i].ExpiresAt.Equal(candidates[j].ExpiresAt) {
			return candidates[i].ExpiresAt.Before(candidates[j].ExpiresAt)
		}
		leftOrder := entitlementGroupSortOrder(&candidates[i], groupID)
		rightOrder := entitlementGroupSortOrder(&candidates[j], groupID)
		if leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		return candidates[i].ID < candidates[j].ID
	})
}

func entitlementCoversGroup(ent *SubscriptionEntitlement, groupID int64) bool {
	if ent == nil || groupID <= 0 {
		return false
	}
	for _, grant := range ent.GroupGrants {
		if grant.Enabled && grant.GroupID == groupID {
			return true
		}
	}
	if len(ent.GroupGrants) > 0 {
		return false
	}
	for _, group := range ent.Groups {
		if group.ID == groupID {
			return true
		}
	}
	return false
}

func entitlementGroupSortOrder(ent *SubscriptionEntitlement, groupID int64) int {
	if ent == nil {
		return int(^uint(0) >> 1)
	}
	for _, grant := range ent.GroupGrants {
		if grant.Enabled && grant.GroupID == groupID {
			return grant.SortOrder
		}
	}
	return int(^uint(0) >> 1)
}

func entitlementResolutionGroup(ent *SubscriptionEntitlement, groupID int64) *Group {
	if ent == nil {
		return &Group{ID: groupID}
	}
	for _, grant := range ent.GroupGrants {
		if grant.Enabled && grant.GroupID == groupID && grant.Group != nil {
			return grant.Group
		}
	}
	if len(ent.GroupGrants) > 0 {
		return &Group{ID: groupID}
	}
	for i := range ent.Groups {
		if ent.Groups[i].ID == groupID {
			return &ent.Groups[i]
		}
	}
	return &Group{ID: groupID}
}
