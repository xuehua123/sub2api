package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/stretchr/testify/require"
)

type fakeSubscriptionEntitlementPlanRepo struct {
	plans map[int64]*SubscriptionEntitlementPlan
}

func (r *fakeSubscriptionEntitlementPlanRepo) GetEntitlementPlan(_ context.Context, planID int64) (*SubscriptionEntitlementPlan, error) {
	plan, ok := r.plans[planID]
	if !ok {
		return nil, ErrSubscriptionEntitlementPlanNotFound
	}
	cp := *plan
	cp.Groups = append([]SubscriptionEntitlementPlanGroup(nil), plan.Groups...)
	return &cp, nil
}

type fakeSubscriptionEntitlementRepo struct {
	mu sync.Mutex

	now time.Time

	nextID       int64
	nextEventID  int64
	entitlements map[int64]*SubscriptionEntitlement
	fulfillments map[int64]*SubscriptionEntitlementFulfillment

	createCount     int
	updateTermCount int
	eventCount      int
	beforeExtend    func()
	resetCalls      []fakeEntitlementResetCall
	windowCalls     []fakeEntitlementWindowCall
	createGroups    [][]int64
}

type conflictingSubscriptionEntitlementRepo struct {
	*fakeSubscriptionEntitlementRepo
}

func (r *conflictingSubscriptionEntitlementRepo) CompareAndSwapTerm(
	context.Context,
	int64,
	time.Time,
	time.Time,
	time.Time,
	string,
	string,
) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

type fakeEntitlementResetCall struct {
	id            int64
	resetDaily    bool
	resetWeekly   bool
	resetMonthly  bool
	dailyStart    time.Time
	periodicStart time.Time
}

type fakeEntitlementWindowCall struct {
	operation           string
	id                  int64
	expectedWindowStart *time.Time
	dailyStart          time.Time
	periodicStart       time.Time
	newWindowStart      time.Time
}

func newFakeSubscriptionEntitlementRepo(now time.Time) *fakeSubscriptionEntitlementRepo {
	return &fakeSubscriptionEntitlementRepo{
		now:          now,
		nextID:       1,
		nextEventID:  1,
		entitlements: make(map[int64]*SubscriptionEntitlement),
		fulfillments: make(map[int64]*SubscriptionEntitlementFulfillment),
	}
}

func (r *fakeSubscriptionEntitlementRepo) WithUserEntitlementMutationTx(ctx context.Context, _ int64, fn func(context.Context) error) error {
	return fn(ctx)
}

func (r *fakeSubscriptionEntitlementRepo) Create(ctx context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error {
	return r.CreateTx(ctx, ent, groupIDs)
}

func (r *fakeSubscriptionEntitlementRepo) CreateTx(_ context.Context, ent *SubscriptionEntitlement, groupIDs []int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if ent.ID == 0 {
		ent.ID = r.nextID
		r.nextID++
	}
	stored := cloneTestEntitlement(ent)
	stored.GroupGrants = testGroupGrants(groupIDs)
	stored.Groups = testGroups(groupIDs)
	r.entitlements[stored.ID] = stored
	r.createCount++
	r.createGroups = append(r.createGroups, append([]int64(nil), groupIDs...))
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) CreateWithFulfillment(_ context.Context, ent *SubscriptionEntitlement, groupIDs []int64, fulfillment *SubscriptionEntitlementFulfillment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if fulfillment != nil && r.fulfillmentSourceExistsLocked(fulfillment) {
		return ErrSubscriptionEntitlementAlreadyExists
	}
	if ent.ID == 0 {
		ent.ID = r.nextID
		r.nextID++
	}
	stored := cloneTestEntitlement(ent)
	stored.GroupGrants = testGroupGrants(groupIDs)
	stored.Groups = testGroups(groupIDs)
	r.entitlements[stored.ID] = stored
	if fulfillment != nil {
		fulfillment.EntitlementID = stored.ID
		if fulfillment.UserID == 0 {
			fulfillment.UserID = stored.UserID
		}
		if fulfillment.PlanID == nil {
			fulfillment.PlanID = cloneInt64Ptr(stored.PlanID)
		}
		r.storeFulfillmentLocked(fulfillment)
	}
	r.createCount++
	r.createGroups = append(r.createGroups, append([]int64(nil), groupIDs...))
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) GetByID(_ context.Context, id int64) (*SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	return cloneTestEntitlement(ent), nil
}

func (r *fakeSubscriptionEntitlementRepo) GetByIDForUpdate(ctx context.Context, id int64) (*SubscriptionEntitlement, error) {
	return r.GetByID(ctx, id)
}

func (r *fakeSubscriptionEntitlementRepo) GetBySourceID(_ context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ent := range r.entitlements {
		if ent.SourceType == sourceType && ent.SourceID != nil && *ent.SourceID == sourceID {
			return cloneTestEntitlement(ent), nil
		}
	}
	return nil, ErrSubscriptionEntitlementNotFound
}

func (r *fakeSubscriptionEntitlementRepo) GetBySourceExternalID(_ context.Context, sourceType, sourceExternalID string) (*SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ent := range r.entitlements {
		if ent.SourceType == sourceType && ent.SourceExternalID != nil && *ent.SourceExternalID == sourceExternalID {
			return cloneTestEntitlement(ent), nil
		}
	}
	return nil, ErrSubscriptionEntitlementNotFound
}

func (r *fakeSubscriptionEntitlementRepo) GetBySourceRedeemCodeID(_ context.Context, redeemCodeID int64) (*SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ent := range r.entitlements {
		if ent.SourceRedeemCodeID != nil && *ent.SourceRedeemCodeID == redeemCodeID {
			return cloneTestEntitlement(ent), nil
		}
	}
	return nil, ErrSubscriptionEntitlementNotFound
}

func (r *fakeSubscriptionEntitlementRepo) GetFulfillmentBySourceID(_ context.Context, sourceType string, sourceID int64) (*SubscriptionEntitlementFulfillment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, fulfillment := range r.fulfillments {
		if fulfillment.SourceType == sourceType && fulfillment.SourceID != nil && *fulfillment.SourceID == sourceID {
			return cloneTestFulfillment(fulfillment), nil
		}
	}
	return nil, ErrSubscriptionEntitlementNotFound
}

func (r *fakeSubscriptionEntitlementRepo) GetFulfillmentBySourceExternalID(_ context.Context, sourceType, sourceExternalID string) (*SubscriptionEntitlementFulfillment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, fulfillment := range r.fulfillments {
		if fulfillment.SourceType == sourceType && fulfillment.SourceExternalID != nil && *fulfillment.SourceExternalID == sourceExternalID {
			return cloneTestFulfillment(fulfillment), nil
		}
	}
	return nil, ErrSubscriptionEntitlementNotFound
}

func (r *fakeSubscriptionEntitlementRepo) GetFulfillmentBySourceRedeemCodeID(_ context.Context, redeemCodeID int64) (*SubscriptionEntitlementFulfillment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, fulfillment := range r.fulfillments {
		if fulfillment.SourceRedeemCodeID != nil && *fulfillment.SourceRedeemCodeID == redeemCodeID {
			return cloneTestFulfillment(fulfillment), nil
		}
	}
	return nil, ErrSubscriptionEntitlementNotFound
}

func (r *fakeSubscriptionEntitlementRepo) GetActiveCoveringGroup(ctx context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error) {
	return r.ListActiveCoveringGroupForUser(ctx, userID, groupID)
}

func (r *fakeSubscriptionEntitlementRepo) ListByUserID(_ context.Context, userID int64) ([]SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID == userID {
			out = append(out, *cloneTestEntitlement(ent))
		}
	}
	return out, nil
}

func (r *fakeSubscriptionEntitlementRepo) ListByUserPlanID(_ context.Context, userID, planID int64) ([]SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID == userID && ent.PlanID != nil && *ent.PlanID == planID {
			out = append(out, *cloneTestEntitlement(ent))
		}
	}
	return out, nil
}

func (r *fakeSubscriptionEntitlementRepo) ListByUserPlanIDForUpdate(ctx context.Context, userID, planID int64) ([]SubscriptionEntitlement, error) {
	return r.ListByUserPlanID(ctx, userID, planID)
}

func (r *fakeSubscriptionEntitlementRepo) ListActiveByUserID(_ context.Context, userID int64) ([]SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID == userID && ent.IsActiveAt(r.now) {
			out = append(out, *cloneTestEntitlement(ent))
		}
	}
	return out, nil
}

func (r *fakeSubscriptionEntitlementRepo) ListActiveCoveringGroupForUser(_ context.Context, userID, groupID int64) ([]SubscriptionEntitlement, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SubscriptionEntitlement, 0)
	for _, ent := range r.entitlements {
		if ent.UserID == userID && ent.IsActiveAt(r.now) && entitlementCoversGroup(ent, groupID) {
			out = append(out, *cloneTestEntitlement(ent))
		}
	}
	return out, nil
}

func (r *fakeSubscriptionEntitlementRepo) UpdateTerm(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string) error {
	return r.UpdateTermAndSource(ctx, id, startsAt, expiresAt, status, notes, SubscriptionEntitlementSourceRef{})
}

func (r *fakeSubscriptionEntitlementRepo) CompareAndSwapTerm(
	_ context.Context,
	id int64,
	expectedUpdatedAt time.Time,
	startsAt time.Time,
	expiresAt time.Time,
	status string,
	notes string,
) (time.Time, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return time.Time{}, false, ErrSubscriptionEntitlementNotFound
	}
	if !ent.UpdatedAt.Equal(expectedUpdatedAt) {
		return time.Time{}, false, nil
	}
	updatedAt := r.now
	if !updatedAt.After(expectedUpdatedAt) {
		updatedAt = expectedUpdatedAt.Add(time.Microsecond)
	}
	ent.StartsAt = startsAt
	ent.ExpiresAt = expiresAt
	ent.Status = status
	ent.Notes = notes
	ent.UpdatedAt = updatedAt
	r.updateTermCount++
	return updatedAt, true, nil
}

func (r *fakeSubscriptionEntitlementRepo) UpdateTermAndSource(_ context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source SubscriptionEntitlementSourceRef) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}
	ent.StartsAt = startsAt
	ent.ExpiresAt = expiresAt
	ent.Status = status
	ent.Notes = notes
	if source.SourceType != "" {
		ent.SourceType = source.SourceType
	}
	if source.SourceID != nil {
		ent.SourceID = cloneInt64Ptr(source.SourceID)
	}
	if source.SourceExternalID != nil {
		ent.SourceExternalID = cloneStringPtr(source.SourceExternalID)
	}
	if source.SourceRedeemCodeID != nil {
		ent.SourceRedeemCodeID = cloneInt64Ptr(source.SourceRedeemCodeID)
	}
	if source.LegacySubscriptionID != nil {
		ent.LegacySubscriptionID = cloneInt64Ptr(source.LegacySubscriptionID)
	}
	if source.AssignedBy != nil {
		ent.AssignedBy = cloneInt64Ptr(source.AssignedBy)
	}
	if !source.AssignedAt.IsZero() {
		ent.AssignedAt = source.AssignedAt
	}
	ent.UpdatedAt = r.now
	r.updateTermCount++
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) ExtendWithFulfillment(_ context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source SubscriptionEntitlementSourceRef, fulfillment *SubscriptionEntitlementFulfillment, resetUsage bool, resetDailyStart, resetPeriodicStart time.Time) error {
	if r.beforeExtend != nil {
		r.beforeExtend()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}
	if fulfillment != nil && r.fulfillmentSourceExistsLocked(fulfillment) {
		return ErrSubscriptionEntitlementAlreadyExists
	}
	before := cloneTestEntitlement(ent)
	ent.StartsAt = startsAt
	ent.ExpiresAt = expiresAt
	ent.Status = status
	ent.Notes = notes
	if source.SourceType != "" {
		ent.SourceType = source.SourceType
	}
	if source.SourceID != nil {
		ent.SourceID = cloneInt64Ptr(source.SourceID)
	}
	if source.SourceExternalID != nil {
		ent.SourceExternalID = cloneStringPtr(source.SourceExternalID)
	}
	if source.SourceRedeemCodeID != nil {
		ent.SourceRedeemCodeID = cloneInt64Ptr(source.SourceRedeemCodeID)
	}
	if source.LegacySubscriptionID != nil {
		ent.LegacySubscriptionID = cloneInt64Ptr(source.LegacySubscriptionID)
	}
	if source.AssignedBy != nil {
		ent.AssignedBy = cloneInt64Ptr(source.AssignedBy)
	}
	if !source.AssignedAt.IsZero() {
		ent.AssignedAt = source.AssignedAt
	}
	if resetUsage {
		ent.DailyUsageUSD = 0
		ent.WeeklyUsageUSD = 0
		ent.MonthlyUsageUSD = 0
		ent.DailyWindowStart = cloneTimeValue(resetDailyStart)
		ent.WeeklyWindowStart = cloneTimeValue(resetPeriodicStart)
		ent.MonthlyWindowStart = cloneTimeValue(resetPeriodicStart)
	}
	if fulfillment != nil {
		fulfillment.EntitlementID = id
		if fulfillment.UserID == 0 {
			fulfillment.UserID = ent.UserID
		}
		if fulfillment.PlanID == nil {
			fulfillment.PlanID = cloneInt64Ptr(ent.PlanID)
		}
		if r.fulfillmentSourceExistsLocked(fulfillment) {
			r.entitlements[id] = before
			return ErrSubscriptionEntitlementAlreadyExists
		}
		r.storeFulfillmentLocked(fulfillment)
	}
	ent.UpdatedAt = r.now
	r.updateTermCount++
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) ActivateWindows(_ context.Context, id int64, dailyStart, periodicStart time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.windowCalls = append(r.windowCalls, fakeEntitlementWindowCall{
		operation:     "activate",
		id:            id,
		dailyStart:    dailyStart,
		periodicStart: periodicStart,
	})
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}
	if ent.DailyWindowStart != nil || ent.WeeklyWindowStart != nil || ent.MonthlyWindowStart != nil {
		return nil
	}
	ent.DailyUsageUSD = 0
	ent.WeeklyUsageUSD = 0
	ent.MonthlyUsageUSD = 0
	ent.DailyWindowStart = cloneTimeValue(dailyStart)
	ent.WeeklyWindowStart = cloneTimeValue(periodicStart)
	ent.MonthlyWindowStart = cloneTimeValue(periodicStart)
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) ResetUsage(_ context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}
	if resetDaily {
		ent.DailyUsageUSD = 0
		ent.DailyWindowStart = &dailyStart
	}
	if resetWeekly {
		ent.WeeklyUsageUSD = 0
		ent.WeeklyWindowStart = &periodicStart
	}
	if resetMonthly {
		ent.MonthlyUsageUSD = 0
		ent.MonthlyWindowStart = &periodicStart
	}
	r.resetCalls = append(r.resetCalls, fakeEntitlementResetCall{
		id:            id,
		resetDaily:    resetDaily,
		resetWeekly:   resetWeekly,
		resetMonthly:  resetMonthly,
		dailyStart:    dailyStart,
		periodicStart: periodicStart,
	})
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) ResetDailyUsage(_ context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	return r.resetWindowUsage(id, "daily", expectedWindowStart, newWindowStart)
}

func (r *fakeSubscriptionEntitlementRepo) ResetWeeklyUsage(_ context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	return r.resetWindowUsage(id, "weekly", expectedWindowStart, newWindowStart)
}

func (r *fakeSubscriptionEntitlementRepo) ResetMonthlyUsage(_ context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	return r.resetWindowUsage(id, "monthly", expectedWindowStart, newWindowStart)
}

func (r *fakeSubscriptionEntitlementRepo) resetWindowUsage(id int64, operation string, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.windowCalls = append(r.windowCalls, fakeEntitlementWindowCall{
		operation:           operation,
		id:                  id,
		expectedWindowStart: cloneEntitlementTestTimePtr(expectedWindowStart),
		newWindowStart:      newWindowStart,
	})
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}

	var current *time.Time
	switch operation {
	case "daily":
		current = ent.DailyWindowStart
	case "weekly":
		current = ent.WeeklyWindowStart
	case "monthly":
		current = ent.MonthlyWindowStart
	}
	if !fakeEntitlementWindowMatches(current, expectedWindowStart) {
		return nil
	}

	switch operation {
	case "daily":
		ent.DailyUsageUSD = 0
		ent.DailyWindowStart = cloneTimeValue(newWindowStart)
	case "weekly":
		ent.WeeklyUsageUSD = 0
		ent.WeeklyWindowStart = cloneTimeValue(newWindowStart)
	case "monthly":
		ent.MonthlyUsageUSD = 0
		ent.MonthlyWindowStart = cloneTimeValue(newWindowStart)
	}
	return nil
}

func fakeEntitlementWindowMatches(current, expected *time.Time) bool {
	if current == nil || expected == nil {
		return current == nil && expected == nil
	}
	return current.Equal(*expected)
}

func (r *fakeSubscriptionEntitlementRepo) ApplyEntitlementUsage(_ context.Context, id int64, costUSD float64, now time.Time) (*EntitlementUsageApplyResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if !ent.CheckDailyLimit(costUSD) || !ent.CheckWeeklyLimit(costUSD) || !ent.CheckMonthlyLimit(costUSD) {
		return nil, ErrSubscriptionEntitlementQuotaExceeded
	}
	ent.DailyUsageUSD += costUSD
	ent.WeeklyUsageUSD += costUSD
	ent.MonthlyUsageUSD += costUSD
	ent.UpdatedAt = now
	return &EntitlementUsageApplyResult{
		UpdatedAt:          now,
		DailyUsageUSD:      ent.DailyUsageUSD,
		WeeklyUsageUSD:     ent.WeeklyUsageUSD,
		MonthlyUsageUSD:    ent.MonthlyUsageUSD,
		DailyWindowStart:   cloneEntitlementTestTimePtr(ent.DailyWindowStart),
		WeeklyWindowStart:  cloneEntitlementTestTimePtr(ent.WeeklyWindowStart),
		MonthlyWindowStart: cloneEntitlementTestTimePtr(ent.MonthlyWindowStart),
	}, nil
}

func (r *fakeSubscriptionEntitlementRepo) ReplaceGroups(_ context.Context, id int64, groupIDs []int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}
	ent.GroupGrants = testGroupGrants(groupIDs)
	ent.Groups = testGroups(groupIDs)
	return nil
}

func (r *fakeSubscriptionEntitlementRepo) fulfillmentSourceExistsLocked(candidate *SubscriptionEntitlementFulfillment) bool {
	if candidate == nil {
		return false
	}
	for _, existing := range r.fulfillments {
		if candidate.SourceRedeemCodeID != nil && existing.SourceRedeemCodeID != nil && *candidate.SourceRedeemCodeID == *existing.SourceRedeemCodeID {
			return true
		}
		if candidate.SourceID != nil && existing.SourceID != nil && candidate.SourceType == existing.SourceType && *candidate.SourceID == *existing.SourceID {
			return true
		}
		if candidate.SourceExternalID != nil && existing.SourceExternalID != nil && candidate.SourceType == existing.SourceType && *candidate.SourceExternalID == *existing.SourceExternalID {
			return true
		}
	}
	return false
}

func (r *fakeSubscriptionEntitlementRepo) storeFulfillmentLocked(fulfillment *SubscriptionEntitlementFulfillment) {
	if fulfillment.ID == 0 {
		fulfillment.ID = r.nextEventID
		r.nextEventID++
	}
	r.fulfillments[fulfillment.ID] = cloneTestFulfillment(fulfillment)
	r.eventCount++
}

func TestSubscriptionEntitlementService_AssignOrExtendFromPlanCreatesEntitlementWithGroups(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	monthlyLimit := 100.0
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{101, 202}, &monthlyLimit),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetNowFunc(func() time.Time { return now })

	ent, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:  42,
		PlanID:  1,
		OrderID: 9001,
		Notes:   "paid order",
	})
	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, 1, repo.createCount)
	require.Equal(t, 1, repo.eventCount)
	require.Equal(t, []int64{101, 202}, repo.createGroups[0])
	require.Equal(t, int64(42), ent.UserID)
	require.NotNil(t, ent.PlanID)
	require.Equal(t, int64(1), *ent.PlanID)
	require.Equal(t, timezone.StartOfDay(now), *ent.DailyWindowStart)
	require.Equal(t, now, *ent.WeeklyWindowStart)
	require.Equal(t, now, *ent.MonthlyWindowStart)
	require.Equal(t, now.AddDate(0, 0, 30), ent.ExpiresAt)
	require.NotNil(t, ent.MonthlyLimitUSD)
	require.Equal(t, monthlyLimit, *ent.MonthlyLimitUSD)

	resolvedA, err := svc.ResolveForGroup(context.Background(), 42, 101, 1)
	require.NoError(t, err)
	resolvedB, err := svc.ResolveForGroup(context.Background(), 42, 202, 1)
	require.NoError(t, err)
	require.Equal(t, ent.ID, resolvedA.Entitlement.ID)
	require.Equal(t, ent.ID, resolvedB.Entitlement.ID)
}

func TestSubscriptionEntitlementService_AssignOrExtendFromPlanAllowsHiddenPlanGroups(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: {
			ID:           1,
			GroupID:      101,
			Name:         "Hidden Group Plan",
			ValidityDays: 30,
			ValidityUnit: validityUnitDay,
			ForSale:      true,
			Groups: []SubscriptionEntitlementPlanGroup{
				{
					GroupID: 101,
					Enabled: true,
					Group: &Group{
						ID:                  101,
						Status:              StatusDisabled,
						SubscriptionType:    SubscriptionTypeSubscription,
						SubscriptionEnabled: false,
					},
				},
			},
		},
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetNowFunc(func() time.Time { return now })

	ent, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:  42,
		PlanID:  1,
		OrderID: 9002,
		Notes:   "paid order",
	})

	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, 1, repo.createCount)
	require.Equal(t, 1, repo.eventCount)
	require.Empty(t, repo.createGroups[0])
	require.NotNil(t, ent.PrimaryGroupID)
	require.Equal(t, int64(101), *ent.PrimaryGroupID)
}

func TestSubscriptionEntitlementService_AssignOrExtendFromPlanRejectsPaymentWithoutEligibleGroups(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: {
			ID:           1,
			GroupID:      101,
			Name:         "Disabled Group Plan",
			ValidityDays: 30,
			ValidityUnit: validityUnitDay,
			ForSale:      true,
			Groups: []SubscriptionEntitlementPlanGroup{{
				GroupID: 101,
				Enabled: true,
				Group:   &Group{ID: 101, Status: StatusDisabled, SubscriptionType: SubscriptionTypeSubscription, SubscriptionEnabled: false},
			}},
		},
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetNowFunc(func() time.Time { return now })

	ent, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:                42,
		PlanID:                1,
		OrderID:               9003,
		RequireEligibleGroups: true,
	})

	require.Nil(t, ent)
	require.False(t, reused)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementPlanInvalid)
	require.Zero(t, repo.createCount)
}

func TestSubscriptionEntitlementService_AssignOrExtendFromPlanRejectsMissingGroupEntity(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: {
			ID:           1,
			GroupID:      101,
			Name:         "Deleted Group Plan",
			ValidityDays: 30,
			ValidityUnit: validityUnitDay,
			ForSale:      true,
			Groups: []SubscriptionEntitlementPlanGroup{{
				GroupID: 101,
				Enabled: true,
				Group:   nil,
			}},
		},
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetNowFunc(func() time.Time { return now })

	ent, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:                42,
		PlanID:                1,
		OrderID:               9004,
		RequireEligibleGroups: true,
	})

	require.Nil(t, ent)
	require.False(t, reused)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementPlanInvalid)
	require.Zero(t, repo.createCount)
}

func TestSubscriptionEntitlementService_AssignOrExtendFromPlanUsesOrderSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	paidMonthlyLimit := 100.0
	changedMonthlyLimit := 10.0
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{303}, &changedMonthlyLimit),
	}}
	paidPlan := testEntitlementPlan(1, []int64{101, 202}, &paidMonthlyLimit)
	paidPlan.Name = "Paid Snapshot"
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetNowFunc(func() time.Time { return now })

	ent, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:                42,
		PlanID:                1,
		OrderID:               9005,
		RequireEligibleGroups: true,
		PlanSnapshot:          paidPlan,
	})

	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, []int64{101, 202}, repo.createGroups[0])
	require.Equal(t, "Paid Snapshot", ent.Name)
	require.NotNil(t, ent.MonthlyLimitUSD)
	require.Equal(t, paidMonthlyLimit, *ent.MonthlyLimitUSD)
}

func TestEntitlementPlanSnapshotAndEconomicsUsePlanCurrency(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	plan := testEntitlementPlan(1, []int64{101}, nil)
	plan.Price = 12.5
	plan.Currency = "usd"

	snapshot := entitlementPlanSnapshot(plan)
	require.Equal(t, "USD", snapshot["currency"])

	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{1: plan}})
	planID := int64(1)
	ent := &SubscriptionEntitlement{PlanID: &planID, PlanSnapshot: map[string]any{}}
	svc.attachEntitlementEconomics(context.Background(), ent)

	require.NotNil(t, ent.PurchasePrice)
	require.Equal(t, 12.5, *ent.PurchasePrice)
	require.Equal(t, "USD", ent.PurchaseCurrency)
}

func TestEntitlementEconomicsConvertsUSDPurchaseWithOrderRate(t *testing.T) {
	quota := 1000.0
	ent := &SubscriptionEntitlement{
		MonthlyLimitUSD: &quota,
		PlanSnapshot: map[string]any{
			"purchase_price":            10.0,
			"purchase_currency":         "USD",
			"purchase_cny_per_usd_rate": 7.2,
			"monthly_limit_usd":         quota,
		},
	}
	svc := NewSubscriptionEntitlementService(nil, nil)

	svc.attachEntitlementEconomics(context.Background(), ent)

	require.Equal(t, "USD", ent.PurchaseCurrency)
	require.NotNil(t, ent.UnitCostPerUSD)
	require.InDelta(t, 0.072, *ent.UnitCostPerUSD, 1e-12)
}

func TestEntitlementEconomicsDoesNotMislabelUSDWithoutOrderRate(t *testing.T) {
	quota := 1000.0
	ent := &SubscriptionEntitlement{
		MonthlyLimitUSD: &quota,
		PlanSnapshot: map[string]any{
			"purchase_price":    10.0,
			"purchase_currency": "USD",
			"monthly_limit_usd": quota,
		},
	}
	svc := NewSubscriptionEntitlementService(nil, nil)

	svc.attachEntitlementEconomics(context.Background(), ent)

	require.Nil(t, ent.UnitCostPerUSD)
}

func TestSubscriptionEntitlementService_ShortenForRefundRejectsConcurrentTermChange(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	baseRepo := newFakeSubscriptionEntitlementRepo(now)
	baseRepo.entitlements[1] = &SubscriptionEntitlement{
		ID:        1,
		UserID:    42,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.AddDate(0, 0, -1),
		ExpiresAt: now.AddDate(0, 0, 30),
		UpdatedAt: now.Add(-time.Minute),
	}
	svc := NewSubscriptionEntitlementService(
		&conflictingSubscriptionEntitlementRepo{fakeSubscriptionEntitlementRepo: baseRepo},
		nil,
	)

	adjustment, err := svc.ShortenForRefund(context.Background(), 1, 10, now)

	require.Nil(t, adjustment)
	require.ErrorIs(t, err, ErrSubscriptionEntitlementTermConflict)
	require.Equal(t, now.AddDate(0, 0, 30), baseRepo.entitlements[1].ExpiresAt)
}

func TestSubscriptionEntitlementService_RestoreRefundSnapshotRejectsConcurrentRenewal(t *testing.T) {
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	originalExpiry := now.AddDate(0, 0, 30)
	repo.entitlements[1] = &SubscriptionEntitlement{
		ID:        1,
		UserID:    42,
		Status:    SubscriptionStatusActive,
		StartsAt:  now.AddDate(0, 0, -1),
		ExpiresAt: originalExpiry,
		UpdatedAt: now.Add(-time.Minute),
	}
	svc := NewSubscriptionEntitlementService(repo, nil)

	adjustment, err := svc.ShortenForRefund(context.Background(), 1, 10, now)
	require.NoError(t, err)
	require.NotNil(t, adjustment)

	renewedExpiry := originalExpiry.AddDate(0, 0, 30)
	repo.mu.Lock()
	repo.entitlements[1].ExpiresAt = renewedExpiry
	repo.entitlements[1].UpdatedAt = adjustment.UpdatedAt.Add(time.Second)
	repo.mu.Unlock()

	err = svc.RestoreRefundSnapshot(context.Background(), adjustment.Snapshot, adjustment.UpdatedAt)

	require.ErrorIs(t, err, ErrSubscriptionEntitlementTermConflict)
	current, getErr := repo.GetByID(context.Background(), 1)
	require.NoError(t, getErr)
	require.Equal(t, renewedExpiry, current.ExpiresAt)
}

func TestSubscriptionEntitlementService_RenewalRestoresLinkedLegacyLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 30, 0, 0, timezone.Location())
	planID := int64(1)
	legacySubscriptionID := int64(912)
	groupID := int64(101)
	deletedAt := now.Add(-time.Hour)
	oldWindow := now.AddDate(0, 0, -30)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[10] = &SubscriptionEntitlement{
		ID:                   10,
		UserID:               42,
		PlanID:               &planID,
		LegacySubscriptionID: &legacySubscriptionID,
		Name:                 "Pro",
		Status:               SubscriptionStatusExpired,
		StartsAt:             now.AddDate(0, 0, -60),
		ExpiresAt:            now.AddDate(0, 0, -1),
		DailyWindowStart:     &oldWindow,
		WeeklyWindowStart:    &oldWindow,
		MonthlyWindowStart:   &oldWindow,
		DailyUsageUSD:        1,
		WeeklyUsageUSD:       2,
		MonthlyUsageUSD:      3,
		GroupGrants:          testGroupGrants([]int64{groupID}),
		Groups:               testGroups([]int64{groupID}),
	}
	legacyRepo := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    42,
		GroupID:   groupID,
		StartsAt:  now.AddDate(0, 0, -90),
		ExpiresAt: now.AddDate(0, 0, -30),
		Status:    SubscriptionStatusRevoked,
		DeletedAt: &deletedAt,
	}}
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		planID: testEntitlementPlan(planID, []int64{groupID}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetLegacySubscriptionRepository(legacyRepo)

	renewed, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:  42,
		PlanID:  planID,
		OrderID: 1234,
		Now:     now,
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, legacySubscriptionID, legacyRepo.restoredID)
	require.Nil(t, legacyRepo.sub.DeletedAt)
	require.Equal(t, renewed.StartsAt, legacyRepo.sub.StartsAt)
	require.Equal(t, renewed.ExpiresAt, legacyRepo.sub.ExpiresAt)
	require.Equal(t, renewed.Status, legacyRepo.sub.Status)
	require.Equal(t, renewed.DailyWindowStart, legacyRepo.sub.DailyWindowStart)
	require.Equal(t, renewed.WeeklyWindowStart, legacyRepo.sub.WeeklyWindowStart)
	require.Equal(t, renewed.MonthlyWindowStart, legacyRepo.sub.MonthlyWindowStart)
	require.Equal(t, renewed.DailyUsageUSD, legacyRepo.sub.DailyUsageUSD)
	require.Equal(t, renewed.WeeklyUsageUSD, legacyRepo.sub.WeeklyUsageUSD)
	require.Equal(t, renewed.MonthlyUsageUSD, legacyRepo.sub.MonthlyUsageUSD)
}

func TestSubscriptionEntitlementService_PurchaseSyncsLinkedLegacyLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 45, 0, 0, timezone.Location())
	planID := int64(1)
	legacySubscriptionID := int64(914)
	groupID := int64(101)
	legacyRepo := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    42,
		GroupID:   groupID,
		StartsAt:  now.AddDate(0, 0, -10),
		ExpiresAt: now.AddDate(0, 0, 1),
		Status:    SubscriptionStatusExpired,
	}}
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		planID: testEntitlementPlan(planID, []int64{groupID}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	svc.SetLegacySubscriptionRepository(legacyRepo)
	externalID := "admin-purchase-914"

	purchased, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:               42,
		PlanID:               planID,
		LegacySubscriptionID: &legacySubscriptionID,
		SourceType:           SubscriptionEntitlementSourceAdminAssign,
		SourceExternalID:     &externalID,
		Now:                  now,
	})

	require.NoError(t, err)
	require.False(t, reused)
	require.Equal(t, purchased.StartsAt, legacyRepo.sub.StartsAt)
	require.Equal(t, purchased.ExpiresAt, legacyRepo.sub.ExpiresAt)
	require.Equal(t, purchased.Status, legacyRepo.sub.Status)
	require.Equal(t, purchased.DailyWindowStart, legacyRepo.sub.DailyWindowStart)
	require.Equal(t, purchased.WeeklyWindowStart, legacyRepo.sub.WeeklyWindowStart)
	require.Equal(t, purchased.MonthlyWindowStart, legacyRepo.sub.MonthlyWindowStart)
	require.Equal(t, purchased.DailyUsageUSD, legacyRepo.sub.DailyUsageUSD)
	require.Equal(t, purchased.WeeklyUsageUSD, legacyRepo.sub.WeeklyUsageUSD)
	require.Equal(t, purchased.MonthlyUsageUSD, legacyRepo.sub.MonthlyUsageUSD)
}

func TestSubscriptionEntitlementService_RefundAdjustmentSyncsAndRestoresLinkedLegacyLifecycle(t *testing.T) {
	now := time.Date(2026, 8, 8, 11, 0, 0, 0, timezone.Location())
	legacySubscriptionID := int64(913)
	dailyWindow := timezone.StartOfDay(now)
	weeklyWindow := now.AddDate(0, 0, -3)
	monthlyWindow := now.AddDate(0, 0, -20)
	originalExpiry := now.AddDate(0, 0, 30)
	repo := newFakeSubscriptionEntitlementRepo(now)
	repo.entitlements[11] = &SubscriptionEntitlement{
		ID:                   11,
		UserID:               42,
		LegacySubscriptionID: &legacySubscriptionID,
		Status:               SubscriptionStatusActive,
		StartsAt:             now.AddDate(0, 0, -1),
		ExpiresAt:            originalExpiry,
		UpdatedAt:            now.Add(-time.Minute),
		DailyWindowStart:     &dailyWindow,
		WeeklyWindowStart:    &weeklyWindow,
		MonthlyWindowStart:   &monthlyWindow,
		DailyUsageUSD:        1.25,
		WeeklyUsageUSD:       2.5,
		MonthlyUsageUSD:      3.75,
	}
	legacyRepo := &linkedEntitlementUserSubRepoStub{sub: &UserSubscription{
		ID:        legacySubscriptionID,
		UserID:    42,
		GroupID:   101,
		StartsAt:  now.AddDate(0, 0, -30),
		ExpiresAt: now.AddDate(0, 0, 1),
		Status:    SubscriptionStatusExpired,
	}}
	svc := NewSubscriptionEntitlementService(repo, nil)
	svc.SetLegacySubscriptionRepository(legacyRepo)

	adjustment, err := svc.ShortenForRefund(context.Background(), 11, 10, now)

	require.NoError(t, err)
	require.NotNil(t, adjustment)
	require.Equal(t, originalExpiry.AddDate(0, 0, -10), legacyRepo.sub.ExpiresAt)
	require.Equal(t, SubscriptionStatusActive, legacyRepo.sub.Status)
	require.Equal(t, dailyWindow, *legacyRepo.sub.DailyWindowStart)
	require.Equal(t, weeklyWindow, *legacyRepo.sub.WeeklyWindowStart)
	require.Equal(t, monthlyWindow, *legacyRepo.sub.MonthlyWindowStart)
	require.Equal(t, 1.25, legacyRepo.sub.DailyUsageUSD)
	require.Equal(t, 2.5, legacyRepo.sub.WeeklyUsageUSD)
	require.Equal(t, 3.75, legacyRepo.sub.MonthlyUsageUSD)

	deletedAt := now.Add(time.Minute)
	legacyRepo.sub.DeletedAt = &deletedAt
	legacyRepo.sub.Status = SubscriptionStatusRevoked
	require.NoError(t, svc.RestoreRefundSnapshot(context.Background(), adjustment.Snapshot, adjustment.UpdatedAt))

	require.Equal(t, legacySubscriptionID, legacyRepo.restoredID)
	require.Nil(t, legacyRepo.sub.DeletedAt)
	require.Equal(t, adjustment.Snapshot.StartsAt, legacyRepo.sub.StartsAt)
	require.Equal(t, adjustment.Snapshot.ExpiresAt, legacyRepo.sub.ExpiresAt)
	require.Equal(t, adjustment.Snapshot.Status, legacyRepo.sub.Status)
	require.Equal(t, adjustment.Snapshot.DailyWindowStart, legacyRepo.sub.DailyWindowStart)
	require.Equal(t, adjustment.Snapshot.WeeklyWindowStart, legacyRepo.sub.WeeklyWindowStart)
	require.Equal(t, adjustment.Snapshot.MonthlyWindowStart, legacyRepo.sub.MonthlyWindowStart)
	require.Equal(t, adjustment.Snapshot.DailyUsageUSD, legacyRepo.sub.DailyUsageUSD)
	require.Equal(t, adjustment.Snapshot.WeeklyUsageUSD, legacyRepo.sub.WeeklyUsageUSD)
	require.Equal(t, adjustment.Snapshot.MonthlyUsageUSD, legacyRepo.sub.MonthlyUsageUSD)
}

func TestSubscriptionEntitlementService_SourceRedeemCodeReplayDoesNotExtendTwice(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{101}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	redeemCodeID := int64(88)

	first, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:             42,
		PlanID:             1,
		SourceRedeemCodeID: &redeemCodeID,
		Now:                now,
	})
	require.NoError(t, err)
	require.False(t, reused)

	second, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:             42,
		PlanID:             1,
		SourceRedeemCodeID: &redeemCodeID,
		Now:                now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 1, repo.createCount)
	require.Equal(t, 1, repo.eventCount)
	require.Equal(t, 0, repo.updateTermCount)
	require.Equal(t, first.ExpiresAt, second.ExpiresAt)
}

func TestSubscriptionEntitlementService_SourceExternalIDReplayDoesNotExtendTwice(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{101}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	externalID := "out_trade_no_1"

	first, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:           42,
		PlanID:           1,
		SourceType:       SubscriptionPlanExternalMappingSourceSub2PaymentPage,
		SourceExternalID: &externalID,
		Now:              now,
	})
	require.NoError(t, err)
	require.False(t, reused)

	second, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:           42,
		PlanID:           1,
		SourceType:       SubscriptionPlanExternalMappingSourceSub2PaymentPage,
		SourceExternalID: &externalID,
		Now:              now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, 1, repo.createCount)
	require.Equal(t, 1, repo.eventCount)
	require.Equal(t, 0, repo.updateTermCount)
	require.Equal(t, first.ExpiresAt, second.ExpiresAt)
}

func TestSubscriptionEntitlementService_SourceReplaySurvivesDeletedPlan(t *testing.T) {
	now := time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{101}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	orderID := int64(9001)

	first, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:   42,
		PlanID:   1,
		SourceID: &orderID,
		Now:      now,
	})
	require.NoError(t, err)
	require.False(t, reused)

	delete(planRepo.plans, 1)
	replayed, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:   42,
		PlanID:   1,
		SourceID: &orderID,
		Now:      now.Add(time.Hour),
	})

	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, first.ID, replayed.ID)
	require.Equal(t, first.ExpiresAt, replayed.ExpiresAt)
	require.Equal(t, 1, repo.createCount)
	require.Equal(t, 1, repo.eventCount)
}

func TestSubscriptionEntitlementService_ExtendWritesSourceIDForReplay(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{101}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	planID := int64(1)
	repo.entitlements[10] = &SubscriptionEntitlement{
		ID:                 10,
		UserID:             42,
		PlanID:             &planID,
		Name:               "Pro",
		Status:             SubscriptionStatusActive,
		StartsAt:           now.Add(-24 * time.Hour),
		ExpiresAt:          now.AddDate(0, 0, 10),
		DailyWindowStart:   cloneTimeValue(now),
		WeeklyWindowStart:  cloneTimeValue(now),
		MonthlyWindowStart: cloneTimeValue(now),
		GroupGrants:        testGroupGrants([]int64{101}),
		Groups:             testGroups([]int64{101}),
	}

	extended, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:  42,
		PlanID:  1,
		OrderID: 1234,
		Now:     now,
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, int64(10), extended.ID)
	require.Equal(t, now.AddDate(0, 0, 40), extended.ExpiresAt)
	require.Equal(t, 1, repo.updateTermCount)

	replayed, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:  42,
		PlanID:  1,
		OrderID: 1234,
		Now:     now.Add(time.Minute),
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, extended.ID, replayed.ID)
	require.Equal(t, extended.ExpiresAt, replayed.ExpiresAt)
	require.Equal(t, 1, repo.updateTermCount)
	require.Equal(t, 1, repo.eventCount)
}

func TestSubscriptionEntitlementService_OldSourceReplayAfterLaterRenewalDoesNotExtendAgain(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	planRepo := &fakeSubscriptionEntitlementPlanRepo{plans: map[int64]*SubscriptionEntitlementPlan{
		1: testEntitlementPlan(1, []int64{101}, nil),
	}}
	svc := NewSubscriptionEntitlementService(repo, planRepo)
	firstOrderID := int64(100)
	first, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:   42,
		PlanID:   1,
		SourceID: &firstOrderID,
		Now:      now,
	})
	require.NoError(t, err)
	require.False(t, reused)

	secondOrderID := int64(200)
	second, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:   42,
		PlanID:   1,
		SourceID: &secondOrderID,
		Now:      now.Add(24 * time.Hour),
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, now.AddDate(0, 0, 60), second.ExpiresAt)
	require.Equal(t, 2, repo.eventCount)

	replayed, reused, err := svc.AssignOrExtendFromPlan(context.Background(), AssignEntitlementFromPlanInput{
		UserID:   42,
		PlanID:   1,
		SourceID: &firstOrderID,
		Now:      now.Add(48 * time.Hour),
	})
	require.NoError(t, err)
	require.True(t, reused)
	require.Equal(t, first.ID, replayed.ID)
	require.Equal(t, second.ExpiresAt, replayed.ExpiresAt)
	require.Equal(t, 1, repo.updateTermCount)
	require.Equal(t, 2, repo.eventCount)
}

func TestSubscriptionEntitlementService_CheckAndResetWindowsResetsExpiredDailyWeeklyMonthly(t *testing.T) {
	now := time.Date(2026, 6, 11, 10, 0, 0, 0, time.UTC)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	startsAt := now.Add(-40 * 24 * time.Hour)
	dailyStart := now.Add(-25 * time.Hour)
	weeklyStart := now.Add(-8 * 24 * time.Hour)
	monthlyStart := now.Add(-31 * 24 * time.Hour)
	repo.entitlements[7] = &SubscriptionEntitlement{
		ID:                 7,
		UserID:             42,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          now.Add(24 * time.Hour),
		DailyWindowStart:   &dailyStart,
		WeeklyWindowStart:  &weeklyStart,
		MonthlyWindowStart: &monthlyStart,
		DailyUsageUSD:      1,
		WeeklyUsageUSD:     2,
		MonthlyUsageUSD:    3,
	}
	ent, err := repo.GetByID(context.Background(), 7)
	require.NoError(t, err)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), ent, now))

	updated, err := repo.GetByID(context.Background(), 7)
	require.NoError(t, err)
	require.Zero(t, updated.DailyUsageUSD)
	require.Zero(t, updated.WeeklyUsageUSD)
	require.Zero(t, updated.MonthlyUsageUSD)
	require.Len(t, repo.windowCalls, 3)
	require.Equal(t, "daily", repo.windowCalls[0].operation)
	require.Equal(t, dailyStart, *repo.windowCalls[0].expectedWindowStart)
	require.Equal(t, timezone.StartOfDay(now), repo.windowCalls[0].newWindowStart)
	require.Equal(t, "weekly", repo.windowCalls[1].operation)
	require.Equal(t, weeklyStart, *repo.windowCalls[1].expectedWindowStart)
	require.Equal(t, "monthly", repo.windowCalls[2].operation)
	require.Equal(t, monthlyStart, *repo.windowCalls[2].expectedWindowStart)
	require.NotEqual(t, dailyStart, *updated.DailyWindowStart)
	require.NotEqual(t, weeklyStart, *updated.WeeklyWindowStart)
	require.NotEqual(t, monthlyStart, *updated.MonthlyWindowStart)
}

func TestSubscriptionEntitlementService_DailyWindowUsesCalendarMidnight(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(9*time.Hour + 30*time.Minute)
	dailyStart := base.Add(16*time.Hour + 45*time.Minute)
	weeklyStart := startsAt
	monthlyStart := startsAt
	repo := newFakeSubscriptionEntitlementRepo(base)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	repo.entitlements[8] = &SubscriptionEntitlement{
		ID:                 8,
		UserID:             42,
		Status:             SubscriptionStatusActive,
		StartsAt:           startsAt,
		ExpiresAt:          startsAt.AddDate(0, 0, 10),
		DailyWindowStart:   &dailyStart,
		WeeklyWindowStart:  &weeklyStart,
		MonthlyWindowStart: &monthlyStart,
		DailyUsageUSD:      7,
	}
	ent, err := repo.GetByID(context.Background(), 8)
	require.NoError(t, err)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), ent, base.Add(23*time.Hour+59*time.Minute)))
	require.Empty(t, repo.windowCalls, "同一日历日内不应按滚动 24 小时重置")

	nextDay := base.AddDate(0, 0, 1).Add(time.Minute)
	require.NoError(t, svc.CheckAndResetWindows(context.Background(), ent, nextDay))
	require.Len(t, repo.windowCalls, 1)
	require.Equal(t, "daily", repo.windowCalls[0].operation)
	require.Equal(t, dailyStart, *repo.windowCalls[0].expectedWindowStart)
	require.Equal(t, timezone.StartOfDay(nextDay), repo.windowCalls[0].newWindowStart)
	require.Equal(t, timezone.StartOfDay(nextDay), *ent.DailyWindowStart)
	require.Zero(t, ent.DailyUsageUSD)
}

func TestSubscriptionEntitlementService_StaleDailyResetReloadsCurrentUsage(t *testing.T) {
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, timezone.Location())
	staleDailyStart := timezone.StartOfDay(now.AddDate(0, 0, -1))
	periodicStart := now.Add(-time.Hour)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	repo.entitlements[12] = &SubscriptionEntitlement{
		ID:                 12,
		UserID:             42,
		Status:             SubscriptionStatusActive,
		StartsAt:           now.AddDate(0, 0, -5),
		ExpiresAt:          now.AddDate(0, 0, 5),
		DailyWindowStart:   &staleDailyStart,
		WeeklyWindowStart:  &periodicStart,
		MonthlyWindowStart: &periodicStart,
		DailyUsageUSD:      8,
	}
	firstSnapshot, err := repo.GetByID(context.Background(), 12)
	require.NoError(t, err)
	staleSnapshot, err := repo.GetByID(context.Background(), 12)
	require.NoError(t, err)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), firstSnapshot, now))
	_, err = repo.ApplyEntitlementUsage(context.Background(), 12, 4.5, now)
	require.NoError(t, err)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), staleSnapshot, now))
	require.Equal(t, 4.5, staleSnapshot.DailyUsageUSD)
	require.Equal(t, timezone.StartOfDay(now), *staleSnapshot.DailyWindowStart)
	require.Len(t, repo.windowCalls, 2)
	require.Equal(t, staleDailyStart, *repo.windowCalls[1].expectedWindowStart)
}

func TestSubscriptionEntitlementService_OneDayCardKeepsOneTimeDailyQuota(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(17 * time.Hour)
	dailyStart := base
	repo := newFakeSubscriptionEntitlementRepo(base)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	repo.entitlements[9] = &SubscriptionEntitlement{
		ID:               9,
		UserID:           42,
		Status:           SubscriptionStatusActive,
		StartsAt:         startsAt,
		ExpiresAt:        startsAt.AddDate(0, 0, 1),
		DailyWindowStart: &dailyStart,
		DailyUsageUSD:    7,
	}
	ent, err := repo.GetByID(context.Background(), 9)
	require.NoError(t, err)

	require.NoError(t, svc.CheckAndResetWindows(context.Background(), ent, base.AddDate(0, 0, 1).Add(time.Hour)))
	require.Empty(t, repo.windowCalls)
	require.Equal(t, 7.0, ent.DailyUsageUSD)
}

func TestSubscriptionEntitlementService_DelayedActivationKeepsPaidTermPeriodicAnchor(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	startsAt := base.Add(9 * time.Hour)
	activatedAt := base.AddDate(0, 0, 2).Add(18 * time.Hour)
	repo := newFakeSubscriptionEntitlementRepo(base)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	repo.entitlements[10] = &SubscriptionEntitlement{
		ID:        10,
		UserID:    42,
		Status:    SubscriptionStatusActive,
		StartsAt:  startsAt,
		ExpiresAt: startsAt.AddDate(0, 0, 45),
	}
	ent, err := repo.GetByID(context.Background(), 10)
	require.NoError(t, err)

	require.NoError(t, svc.CheckAndActivateWindow(context.Background(), ent, activatedAt))
	require.Len(t, repo.windowCalls, 1)
	require.Equal(t, "activate", repo.windowCalls[0].operation)
	require.Equal(t, timezone.StartOfDay(activatedAt), repo.windowCalls[0].dailyStart)
	require.Equal(t, startsAt, repo.windowCalls[0].periodicStart)
	require.Equal(t, startsAt, *ent.WeeklyWindowStart)
	require.Equal(t, startsAt, *ent.MonthlyWindowStart)
}

func TestSubscriptionEntitlementService_ExpiredExtensionSeparatesDailyAndPeriodicAnchors(t *testing.T) {
	base := time.Date(2026, 6, 11, 0, 0, 0, 0, timezone.Location())
	now := base.Add(15*time.Hour + 30*time.Minute)
	planID := int64(1)
	repo := newFakeSubscriptionEntitlementRepo(now)
	svc := NewSubscriptionEntitlementService(repo, &fakeSubscriptionEntitlementPlanRepo{})
	existing := &SubscriptionEntitlement{
		ID:              11,
		UserID:          42,
		PlanID:          &planID,
		Status:          SubscriptionStatusExpired,
		StartsAt:        now.AddDate(0, 0, -10),
		ExpiresAt:       now.AddDate(0, 0, -1),
		DailyUsageUSD:   1,
		WeeklyUsageUSD:  2,
		MonthlyUsageUSD: 3,
	}
	repo.entitlements[existing.ID] = cloneTestEntitlement(existing)

	extended, err := svc.extendExistingEntitlement(context.Background(), existing, 30, "renewed", SubscriptionEntitlementSourceRef{}, now)

	require.NoError(t, err)
	require.Equal(t, timezone.StartOfDay(now), *extended.DailyWindowStart)
	require.Equal(t, now, *extended.WeeklyWindowStart)
	require.Equal(t, now, *extended.MonthlyWindowStart)
	require.Zero(t, extended.DailyUsageUSD)
	require.Zero(t, extended.WeeklyUsageUSD)
	require.Zero(t, extended.MonthlyUsageUSD)
}

func testEntitlementPlan(id int64, groupIDs []int64, monthlyLimit *float64) *SubscriptionEntitlementPlan {
	groups := make([]SubscriptionEntitlementPlanGroup, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		groups = append(groups, SubscriptionEntitlementPlanGroup{
			GroupID:   groupID,
			SortOrder: i,
			Enabled:   true,
			Group: &Group{
				ID:                  groupID,
				Status:              StatusActive,
				SubscriptionType:    SubscriptionTypeSubscription,
				SubscriptionEnabled: true,
			},
		})
	}
	return &SubscriptionEntitlementPlan{
		ID:              id,
		GroupID:         groupIDs[0],
		Name:            "Pro",
		ValidityDays:    30,
		ValidityUnit:    validityUnitDay,
		MonthlyLimitUSD: monthlyLimit,
		OveragePolicy:   SubscriptionEntitlementOverageBlock,
		ForSale:         true,
		Groups:          groups,
	}
}

func testGroupGrants(groupIDs []int64) []SubscriptionEntitlementGroupGrant {
	grants := make([]SubscriptionEntitlementGroupGrant, 0, len(groupIDs))
	for i, groupID := range groupIDs {
		grants = append(grants, SubscriptionEntitlementGroupGrant{
			GroupID:   groupID,
			SortOrder: i,
			Enabled:   true,
			Group: &Group{
				ID:                  groupID,
				Status:              StatusActive,
				SubscriptionType:    SubscriptionTypeSubscription,
				SubscriptionEnabled: true,
			},
		})
	}
	return grants
}

func testGroups(groupIDs []int64) []Group {
	groups := make([]Group, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		groups = append(groups, Group{
			ID:                  groupID,
			Status:              StatusActive,
			SubscriptionType:    SubscriptionTypeSubscription,
			SubscriptionEnabled: true,
		})
	}
	return groups
}

func cloneTestEntitlement(ent *SubscriptionEntitlement) *SubscriptionEntitlement {
	if ent == nil {
		return nil
	}
	cp := *ent
	cp.PlanID = cloneInt64Ptr(ent.PlanID)
	cp.LegacySubscriptionID = cloneInt64Ptr(ent.LegacySubscriptionID)
	cp.PrimaryGroupID = cloneInt64Ptr(ent.PrimaryGroupID)
	cp.DailyWindowStart = cloneEntitlementTestTimePtr(ent.DailyWindowStart)
	cp.WeeklyWindowStart = cloneEntitlementTestTimePtr(ent.WeeklyWindowStart)
	cp.MonthlyWindowStart = cloneEntitlementTestTimePtr(ent.MonthlyWindowStart)
	cp.DailyLimitUSD = cloneFloat64Ptr(ent.DailyLimitUSD)
	cp.WeeklyLimitUSD = cloneFloat64Ptr(ent.WeeklyLimitUSD)
	cp.MonthlyLimitUSD = cloneFloat64Ptr(ent.MonthlyLimitUSD)
	cp.SourceID = cloneInt64Ptr(ent.SourceID)
	cp.SourceExternalID = cloneStringPtr(ent.SourceExternalID)
	cp.SourceRedeemCodeID = cloneInt64Ptr(ent.SourceRedeemCodeID)
	cp.AssignedBy = cloneInt64Ptr(ent.AssignedBy)
	cp.Groups = append([]Group(nil), ent.Groups...)
	cp.GroupGrants = append([]SubscriptionEntitlementGroupGrant(nil), ent.GroupGrants...)
	if ent.PlanSnapshot != nil {
		cp.PlanSnapshot = make(map[string]any, len(ent.PlanSnapshot))
		for k, v := range ent.PlanSnapshot {
			cp.PlanSnapshot[k] = v
		}
	}
	return &cp
}

func cloneTestFulfillment(fulfillment *SubscriptionEntitlementFulfillment) *SubscriptionEntitlementFulfillment {
	if fulfillment == nil {
		return nil
	}
	cp := *fulfillment
	cp.PlanID = cloneInt64Ptr(fulfillment.PlanID)
	cp.SourceID = cloneInt64Ptr(fulfillment.SourceID)
	cp.SourceExternalID = cloneStringPtr(fulfillment.SourceExternalID)
	cp.SourceRedeemCodeID = cloneInt64Ptr(fulfillment.SourceRedeemCodeID)
	cp.AssignedBy = cloneInt64Ptr(fulfillment.AssignedBy)
	return &cp
}

func cloneEntitlementTestTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneTimeValue(v time.Time) *time.Time {
	out := v
	return &out
}
