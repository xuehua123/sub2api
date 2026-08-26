package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

type SubscriptionEntitlementService struct {
	entitlementRepo        SubscriptionEntitlementRepository
	planRepo               SubscriptionEntitlementPlanRepository
	legacySubscriptionRepo UserSubscriptionRepository
	legacyAliasInvalidator func(*SubscriptionEntitlement)
	nowFunc                func() time.Time
}

func (s *SubscriptionEntitlementService) SetLegacySubscriptionRepository(repo UserSubscriptionRepository) {
	if s != nil {
		s.legacySubscriptionRepo = repo
	}
}

func (s *SubscriptionEntitlementService) SetLegacyAliasInvalidator(invalidator func(*SubscriptionEntitlement)) {
	if s != nil {
		s.legacyAliasInvalidator = invalidator
	}
}

func (s *SubscriptionEntitlementService) invalidateLinkedLegacyAlias(ent *SubscriptionEntitlement) {
	if s == nil || ent == nil || ent.LegacySubscriptionID == nil || s.legacyAliasInvalidator == nil {
		return
	}
	s.legacyAliasInvalidator(ent)
}

type AssignEntitlementFromPlanInput struct {
	UserID                int64
	PlanID                int64
	LegacySubscriptionID  *int64
	RequireEligibleGroups bool
	PlanSnapshot          *SubscriptionEntitlementPlan

	OrderID int64

	SourceType         string
	SourceID           *int64
	SourceExternalID   *string
	SourceRedeemCodeID *int64

	ValidityDaysOverride  int
	AssignedBy            int64
	AssignedAt            time.Time
	Notes                 string
	PurchasePrice         *float64
	PurchaseCurrency      string
	PurchaseCNYPerUSDRate *float64

	Now time.Time
}

func NewSubscriptionEntitlementService(
	entitlementRepo SubscriptionEntitlementRepository,
	planRepo SubscriptionEntitlementPlanRepository,
) *SubscriptionEntitlementService {
	return &SubscriptionEntitlementService{
		entitlementRepo: entitlementRepo,
		planRepo:        planRepo,
		nowFunc:         time.Now,
	}
}

func (s *SubscriptionEntitlementService) SetNowFunc(fn func() time.Time) {
	if fn != nil {
		s.nowFunc = fn
	}
}

func (s *SubscriptionEntitlementService) AssignOrExtendFromPlan(ctx context.Context, input AssignEntitlementFromPlanInput) (*SubscriptionEntitlement, bool, error) {
	return s.AssignOrExtendFromPlanTx(ctx, input)
}

func (s *SubscriptionEntitlementService) AssignOrExtendFromPlanTx(ctx context.Context, input AssignEntitlementFromPlanInput) (*SubscriptionEntitlement, bool, error) {
	if s == nil || s.entitlementRepo == nil || s.planRepo == nil {
		return nil, false, ErrSubscriptionEntitlementPlanRequired
	}
	if input.UserID <= 0 || input.PlanID <= 0 {
		return nil, false, ErrSubscriptionEntitlementPlanRequired
	}

	now := s.inputNow(input.Now)
	source := entitlementSourceFromAssignInput(input, now)
	if existing, found, err := s.findEntitlementBySource(ctx, source); err != nil || found {
		return existing, found, err
	}

	plan := input.PlanSnapshot
	if plan != nil {
		if plan.ID != input.PlanID {
			return nil, false, ErrSubscriptionEntitlementPlanInvalid
		}
	} else {
		var err error
		plan, err = s.planRepo.GetEntitlementPlan(ctx, input.PlanID)
		if err != nil {
			return nil, false, err
		}
	}
	groupIDs, err := entitlementPlanGroupIDs(plan)
	if err != nil {
		return nil, false, err
	}
	if input.RequireEligibleGroups && len(groupIDs) == 0 {
		return nil, false, ErrSubscriptionEntitlementPlanInvalid
	}
	validityDays := entitlementValidityDays(plan, input.ValidityDaysOverride)
	var result *SubscriptionEntitlement
	var reused bool
	err = s.entitlementRepo.WithUserEntitlementMutationTx(ctx, input.UserID, func(txCtx context.Context) error {
		var mutationErr error
		result, reused, mutationErr = s.assignOrExtendFromPlanLocked(txCtx, input, plan, groupIDs, validityDays, source, now)
		return mutationErr
	})
	if err != nil {
		if errors.Is(err, ErrSubscriptionEntitlementAlreadyExists) {
			if replay, replayFound, replayErr := s.findEntitlementBySource(ctx, source); replayErr == nil && replayFound {
				return replay, true, nil
			}
		}
		return nil, false, err
	}
	return result, reused, nil
}

func (s *SubscriptionEntitlementService) assignOrExtendFromPlanLocked(
	ctx context.Context,
	input AssignEntitlementFromPlanInput,
	plan *SubscriptionEntitlementPlan,
	groupIDs []int64,
	validityDays int,
	source SubscriptionEntitlementSourceRef,
	now time.Time,
) (*SubscriptionEntitlement, bool, error) {
	if existing, found, err := s.findEntitlementBySource(ctx, source); err != nil || found {
		return existing, found, err
	}

	existing, found, err := s.findReusablePlanEntitlement(ctx, input.UserID, plan.ID, groupIDs, now)
	if err != nil {
		return nil, false, err
	}
	if found {
		ent, err := s.extendExistingEntitlement(ctx, existing, validityDays, input.Notes, source, now)
		return ent, true, err
	}

	ent := newEntitlementFromPlan(plan, input.UserID, entitlementPrimaryGroupID(plan, groupIDs), validityDays, input.Notes, source, now, input.LegacySubscriptionID)
	applyEntitlementPurchaseSnapshot(ent.PlanSnapshot, input.PurchasePrice, input.PurchaseCurrency, input.PurchaseCNYPerUSDRate)
	fulfillment := newEntitlementFulfillment(ent, validityDays, source)
	if err := s.entitlementRepo.CreateWithFulfillment(ctx, ent, groupIDs, fulfillment); err != nil {
		return nil, false, err
	}
	if ent.ID > 0 {
		refreshed, err := s.entitlementRepo.GetByID(ctx, ent.ID)
		if err != nil {
			return nil, false, err
		}
		if err := syncLinkedLegacySubscriptionLifecycle(ctx, s.legacySubscriptionRepo, refreshed); err != nil {
			return nil, false, err
		}
		return refreshed, false, nil
	}
	return ent, false, nil
}

func (s *SubscriptionEntitlementService) inputNow(override time.Time) time.Time {
	if !override.IsZero() {
		return override
	}
	if s != nil && s.nowFunc != nil {
		return s.nowFunc()
	}
	return time.Now()
}

func (s *SubscriptionEntitlementService) withLockedEntitlement(
	ctx context.Context,
	entitlementID int64,
	userID int64,
	fn func(context.Context, *SubscriptionEntitlement) error,
) error {
	if s == nil || s.entitlementRepo == nil || entitlementID <= 0 || fn == nil {
		return ErrSubscriptionEntitlementNotFound
	}
	if userID <= 0 {
		ent, err := s.entitlementRepo.GetByID(ctx, entitlementID)
		if err != nil {
			return err
		}
		userID = ent.UserID
	}
	return s.entitlementRepo.WithUserEntitlementMutationTx(ctx, userID, func(txCtx context.Context) error {
		ent, err := s.entitlementRepo.GetByIDForUpdate(txCtx, entitlementID)
		if err != nil {
			return err
		}
		if ent.UserID != userID {
			return ErrSubscriptionEntitlementNotFound
		}
		return fn(txCtx, ent)
	})
}

func (s *SubscriptionEntitlementService) findEntitlementBySource(ctx context.Context, source SubscriptionEntitlementSourceRef) (*SubscriptionEntitlement, bool, error) {
	if source.SourceRedeemCodeID != nil {
		ent, err := s.getEntitlementByFulfillmentRedeemCodeID(ctx, *source.SourceRedeemCodeID)
		if err == nil {
			return ent, true, nil
		}
		if !errors.Is(err, ErrSubscriptionEntitlementNotFound) {
			return nil, false, err
		}
		ent, err = s.entitlementRepo.GetBySourceRedeemCodeID(ctx, *source.SourceRedeemCodeID)
		if err == nil {
			return ent, true, nil
		}
		if !errors.Is(err, ErrSubscriptionEntitlementNotFound) {
			return nil, false, err
		}
	}
	if source.SourceExternalID != nil && strings.TrimSpace(*source.SourceExternalID) != "" {
		externalID := strings.TrimSpace(*source.SourceExternalID)
		ent, err := s.getEntitlementByFulfillmentExternalID(ctx, source.SourceType, externalID)
		if err == nil {
			return ent, true, nil
		}
		if !errors.Is(err, ErrSubscriptionEntitlementNotFound) {
			return nil, false, err
		}
		ent, err = s.entitlementRepo.GetBySourceExternalID(ctx, source.SourceType, externalID)
		if err == nil {
			return ent, true, nil
		}
		if !errors.Is(err, ErrSubscriptionEntitlementNotFound) {
			return nil, false, err
		}
	}
	if source.SourceID != nil {
		ent, err := s.getEntitlementByFulfillmentSourceID(ctx, source.SourceType, *source.SourceID)
		if err == nil {
			return ent, true, nil
		}
		if !errors.Is(err, ErrSubscriptionEntitlementNotFound) {
			return nil, false, err
		}
		ent, err = s.entitlementRepo.GetBySourceID(ctx, source.SourceType, *source.SourceID)
		if err == nil {
			return ent, true, nil
		}
		if !errors.Is(err, ErrSubscriptionEntitlementNotFound) {
			return nil, false, err
		}
	}
	return nil, false, nil
}

func (s *SubscriptionEntitlementService) getEntitlementByFulfillmentSourceID(ctx context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlement, error) {
	fulfillment, err := s.entitlementRepo.GetFulfillmentBySourceID(ctx, sourceType, sourceID)
	if err != nil {
		return nil, err
	}
	return s.entitlementRepo.GetByID(ctx, fulfillment.EntitlementID)
}

func (s *SubscriptionEntitlementService) getEntitlementByFulfillmentExternalID(ctx context.Context, sourceType, externalID string) (*SubscriptionEntitlement, error) {
	fulfillment, err := s.entitlementRepo.GetFulfillmentBySourceExternalID(ctx, sourceType, externalID)
	if err != nil {
		return nil, err
	}
	return s.entitlementRepo.GetByID(ctx, fulfillment.EntitlementID)
}

func (s *SubscriptionEntitlementService) getEntitlementByFulfillmentRedeemCodeID(ctx context.Context, redeemCodeID int64) (*SubscriptionEntitlement, error) {
	fulfillment, err := s.entitlementRepo.GetFulfillmentBySourceRedeemCodeID(ctx, redeemCodeID)
	if err != nil {
		return nil, err
	}
	return s.entitlementRepo.GetByID(ctx, fulfillment.EntitlementID)
}

func (s *SubscriptionEntitlementService) findReusablePlanEntitlement(ctx context.Context, userID, planID int64, groupIDs []int64, now time.Time) (*SubscriptionEntitlement, bool, error) {
	candidates, err := s.entitlementRepo.ListByUserPlanIDForUpdate(ctx, userID, planID)
	if err != nil {
		return nil, false, err
	}
	reusable := make([]SubscriptionEntitlement, 0, len(candidates))
	for i := range candidates {
		ent := candidates[i]
		if !sameEntitlementGroupScope(&ent, groupIDs) {
			continue
		}
		if ent.Status == SubscriptionStatusSuspended && ent.ExpiresAt.After(now) {
			continue
		}
		reusable = append(reusable, ent)
	}
	if len(reusable) == 0 {
		return nil, false, nil
	}
	sort.SliceStable(reusable, func(i, j int) bool {
		leftActive := entitlementActiveAt(&reusable[i], now)
		rightActive := entitlementActiveAt(&reusable[j], now)
		if leftActive != rightActive {
			return leftActive
		}
		if leftActive {
			if !reusable[i].ExpiresAt.Equal(reusable[j].ExpiresAt) {
				return reusable[i].ExpiresAt.Before(reusable[j].ExpiresAt)
			}
			return reusable[i].ID < reusable[j].ID
		}
		if !reusable[i].ExpiresAt.Equal(reusable[j].ExpiresAt) {
			return reusable[i].ExpiresAt.After(reusable[j].ExpiresAt)
		}
		return reusable[i].ID < reusable[j].ID
	})
	return &reusable[0], true, nil
}

func (s *SubscriptionEntitlementService) extendExistingEntitlement(
	ctx context.Context,
	existing *SubscriptionEntitlement,
	validityDays int,
	notes string,
	source SubscriptionEntitlementSourceRef,
	now time.Time,
) (*SubscriptionEntitlement, error) {
	if existing == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	startsAt := existing.StartsAt
	base := existing.ExpiresAt
	expired := !existing.ExpiresAt.After(now) || existing.Status == SubscriptionStatusExpired
	if expired {
		startsAt = now
		base = now
	}
	expiresAt := addEntitlementValidityDays(base, validityDays)
	mergedNotes := appendSubscriptionNotes(existing.Notes, notes)
	fulfillment := newEntitlementFulfillment(&SubscriptionEntitlement{
		ID:        existing.ID,
		UserID:    existing.UserID,
		PlanID:    cloneInt64Ptr(existing.PlanID),
		StartsAt:  startsAt,
		ExpiresAt: expiresAt,
	}, validityDays, source)
	dailyStart := timezone.StartOfDay(startsAt)
	if err := s.entitlementRepo.ExtendWithFulfillment(ctx, existing.ID, startsAt, expiresAt, SubscriptionStatusActive, mergedNotes, source, fulfillment, expired, dailyStart, startsAt); err != nil {
		return nil, err
	}
	refreshed, err := s.entitlementRepo.GetByID(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	if err := syncLinkedLegacySubscriptionLifecycle(ctx, s.legacySubscriptionRepo, refreshed); err != nil {
		return nil, err
	}
	return refreshed, nil
}

func entitlementSourceFromAssignInput(input AssignEntitlementFromPlanInput, now time.Time) SubscriptionEntitlementSourceRef {
	sourceType := strings.TrimSpace(input.SourceType)
	sourceID := cloneInt64Ptr(input.SourceID)
	if sourceID == nil && input.OrderID > 0 {
		orderID := input.OrderID
		sourceID = &orderID
	}
	if sourceType == "" {
		switch {
		case input.SourceRedeemCodeID != nil:
			sourceType = SubscriptionEntitlementSourceRedeemCode
		case sourceID != nil:
			sourceType = SubscriptionEntitlementSourcePaymentOrder
		default:
			sourceType = SubscriptionEntitlementSourceUnknown
		}
	}

	var externalID *string
	if input.SourceExternalID != nil {
		trimmed := strings.TrimSpace(*input.SourceExternalID)
		if trimmed != "" {
			externalID = &trimmed
		}
	}
	var assignedBy *int64
	if input.AssignedBy > 0 {
		assignedBy = &input.AssignedBy
	}
	assignedAt := input.AssignedAt
	if assignedAt.IsZero() {
		assignedAt = now
	}
	return SubscriptionEntitlementSourceRef{
		SourceType:           sourceType,
		SourceID:             sourceID,
		SourceExternalID:     externalID,
		SourceRedeemCodeID:   cloneInt64Ptr(input.SourceRedeemCodeID),
		LegacySubscriptionID: cloneInt64Ptr(input.LegacySubscriptionID),
		AssignedBy:           assignedBy,
		AssignedAt:           assignedAt,
	}
}

func newEntitlementFromPlan(plan *SubscriptionEntitlementPlan, userID int64, primaryGroupID *int64, validityDays int, notes string, source SubscriptionEntitlementSourceRef, now time.Time, legacySubscriptionID *int64) *SubscriptionEntitlement {
	planID := plan.ID
	dailyWindowStart := timezone.StartOfDay(now)
	periodicWindowStart := now
	name := plan.Name
	if strings.TrimSpace(name) == "" {
		name = plan.ProductName
	}
	ent := &SubscriptionEntitlement{
		UserID:               userID,
		PlanID:               &planID,
		LegacySubscriptionID: cloneInt64Ptr(legacySubscriptionID),
		PrimaryGroupID:       cloneInt64Ptr(primaryGroupID),
		Name:                 name,
		SourceType:           source.SourceType,
		Status:               SubscriptionStatusActive,
		StartsAt:             now,
		ExpiresAt:            addEntitlementValidityDays(now, validityDays),
		DailyWindowStart:     &dailyWindowStart,
		WeeklyWindowStart:    &periodicWindowStart,
		MonthlyWindowStart:   &periodicWindowStart,
		DailyLimitUSD:        cloneFloat64Ptr(plan.DailyLimitUSD),
		WeeklyLimitUSD:       cloneFloat64Ptr(plan.WeeklyLimitUSD),
		MonthlyLimitUSD:      cloneFloat64Ptr(plan.MonthlyLimitUSD),
		OveragePolicy:        normalizeEntitlementOveragePolicy(plan.OveragePolicy),
		PlanSnapshot:         entitlementPlanSnapshot(plan),
		SourceID:             cloneInt64Ptr(source.SourceID),
		SourceExternalID:     cloneStringPtr(source.SourceExternalID),
		SourceRedeemCodeID:   cloneInt64Ptr(source.SourceRedeemCodeID),
		AssignedBy:           cloneInt64Ptr(source.AssignedBy),
		AssignedAt:           source.AssignedAt,
		Notes:                notes,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	return ent
}

func newEntitlementFulfillment(ent *SubscriptionEntitlement, validityDays int, source SubscriptionEntitlementSourceRef) *SubscriptionEntitlementFulfillment {
	if ent == nil {
		return nil
	}
	return &SubscriptionEntitlementFulfillment{
		EntitlementID:      ent.ID,
		UserID:             ent.UserID,
		PlanID:             cloneInt64Ptr(ent.PlanID),
		SourceType:         source.SourceType,
		SourceID:           cloneInt64Ptr(source.SourceID),
		SourceExternalID:   cloneStringPtr(source.SourceExternalID),
		SourceRedeemCodeID: cloneInt64Ptr(source.SourceRedeemCodeID),
		ValidityDays:       validityDays,
		StartsAt:           ent.StartsAt,
		ExpiresAt:          ent.ExpiresAt,
		AssignedBy:         cloneInt64Ptr(source.AssignedBy),
		AssignedAt:         source.AssignedAt,
		Notes:              ent.Notes,
	}
}

func entitlementValidityDays(plan *SubscriptionEntitlementPlan, overrideDays int) int {
	days := overrideDays
	if days <= 0 && plan != nil {
		days = psComputeValidityDays(plan.ValidityDays, plan.ValidityUnit)
	}
	if days <= 0 {
		days = 30
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	return days
}

func addEntitlementValidityDays(start time.Time, days int) time.Time {
	if days <= 0 {
		days = 30
	}
	if days > MaxValidityDays {
		days = MaxValidityDays
	}
	expiresAt := start.AddDate(0, 0, days)
	if expiresAt.After(MaxExpiresAt) {
		return MaxExpiresAt
	}
	return expiresAt
}

func entitlementPlanGroupIDs(plan *SubscriptionEntitlementPlan) ([]int64, error) {
	if plan == nil {
		return nil, ErrSubscriptionEntitlementPlanRequired
	}
	hasConfiguredGroups := len(plan.Groups) > 0
	seen := make(map[int64]struct{}, len(plan.Groups)+1)
	groupIDs := make([]int64, 0, len(plan.Groups)+1)
	for _, grant := range plan.Groups {
		if !grant.Enabled || grant.GroupID <= 0 {
			continue
		}
		if grant.Group == nil || !grant.Group.IsActive() || !grant.Group.SupportsSubscriptionAccess() {
			continue
		}
		if _, ok := seen[grant.GroupID]; ok {
			continue
		}
		seen[grant.GroupID] = struct{}{}
		groupIDs = append(groupIDs, grant.GroupID)
	}
	if len(groupIDs) == 0 && !hasConfiguredGroups && plan.GroupID > 0 {
		groupIDs = append(groupIDs, plan.GroupID)
	}
	return groupIDs, nil
}

func entitlementPrimaryGroupID(plan *SubscriptionEntitlementPlan, groupIDs []int64) *int64 {
	if len(groupIDs) > 0 && groupIDs[0] > 0 {
		id := groupIDs[0]
		return &id
	}
	if plan != nil && plan.GroupID > 0 {
		id := plan.GroupID
		return &id
	}
	return nil
}

func sameEntitlementGroupScope(ent *SubscriptionEntitlement, groupIDs []int64) bool {
	return int64SlicesEqual(entitlementGroupIDs(ent), normalizedInt64Set(groupIDs))
}

func entitlementGroupIDs(ent *SubscriptionEntitlement) []int64 {
	if ent == nil {
		return nil
	}
	ids := make([]int64, 0, len(ent.GroupGrants)+len(ent.Groups))
	for _, grant := range ent.GroupGrants {
		if grant.Enabled && grant.GroupID > 0 {
			ids = append(ids, grant.GroupID)
		}
	}
	if len(ids) == 0 && !entitlementHasConfiguredGroupGrants(ent) {
		for _, group := range ent.Groups {
			if group.ID > 0 {
				ids = append(ids, group.ID)
			}
		}
	}
	return normalizedInt64Set(ids)
}

func normalizedInt64Set(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func int64SlicesEqual(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func normalizeEntitlementOveragePolicy(policy string) string {
	policy = strings.TrimSpace(policy)
	if policy == "" {
		return SubscriptionEntitlementOverageBlock
	}
	return policy
}

func entitlementPlanSnapshot(plan *SubscriptionEntitlementPlan) map[string]any {
	if plan == nil {
		return map[string]any{}
	}
	groupIDs := make([]int64, 0, len(plan.Groups))
	for _, grant := range plan.Groups {
		if grant.Enabled && grant.GroupID > 0 {
			groupIDs = append(groupIDs, grant.GroupID)
		}
	}
	currency := strings.ToUpper(strings.TrimSpace(plan.Currency))
	if currency == "" {
		currency = "CNY"
	}
	return map[string]any{
		"plan_id":           plan.ID,
		"name":              plan.Name,
		"price":             plan.Price,
		"currency":          currency,
		"validity_days":     plan.ValidityDays,
		"validity_unit":     normalizeValidityUnit(plan.ValidityUnit),
		"access_scope":      plan.AccessScope,
		"group_ids":         groupIDs,
		"daily_limit_usd":   floatPtrSnapshot(plan.DailyLimitUSD),
		"weekly_limit_usd":  floatPtrSnapshot(plan.WeeklyLimitUSD),
		"monthly_limit_usd": floatPtrSnapshot(plan.MonthlyLimitUSD),
		"overage_policy":    normalizeEntitlementOveragePolicy(plan.OveragePolicy),
	}
}

func applyEntitlementPurchaseSnapshot(snapshot map[string]any, purchasePrice *float64, purchaseCurrency string, cnyPerUSDRate *float64) {
	if snapshot == nil || purchasePrice == nil || *purchasePrice <= 0 {
		return
	}
	snapshot["purchase_price"] = *purchasePrice
	if currency := strings.ToUpper(strings.TrimSpace(purchaseCurrency)); currency != "" {
		snapshot["purchase_currency"] = currency
		if currency == "USD" && cnyPerUSDRate != nil && *cnyPerUSDRate > 0 {
			snapshot["purchase_cny_per_usd_rate"] = *cnyPerUSDRate
		}
	}
}

func entitlementActiveAt(ent *SubscriptionEntitlement, now time.Time) bool {
	return ent != nil && ent.Status == SubscriptionStatusActive && !now.Before(ent.StartsAt) && ent.ExpiresAt.After(now)
}

func cloneFloat64Ptr(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func floatPtrSnapshot(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}
