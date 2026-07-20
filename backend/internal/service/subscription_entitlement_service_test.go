package service

import (
	"context"
	"sync"
	"testing"
	"time"

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
	createGroups    [][]int64
}

type fakeEntitlementResetCall struct {
	id           int64
	resetDaily   bool
	resetWeekly  bool
	resetMonthly bool
	windowStart  time.Time
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

func (r *fakeSubscriptionEntitlementRepo) ExtendWithFulfillment(_ context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source SubscriptionEntitlementSourceRef, fulfillment *SubscriptionEntitlementFulfillment, resetUsage bool, resetWindowStart time.Time) error {
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
		ent.DailyWindowStart = cloneTimeValue(resetWindowStart)
		ent.WeeklyWindowStart = cloneTimeValue(resetWindowStart)
		ent.MonthlyWindowStart = cloneTimeValue(resetWindowStart)
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

func (r *fakeSubscriptionEntitlementRepo) ResetUsage(_ context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, windowStart time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	ent, ok := r.entitlements[id]
	if !ok {
		return ErrSubscriptionEntitlementNotFound
	}
	if resetDaily {
		ent.DailyUsageUSD = 0
		ent.DailyWindowStart = &windowStart
	}
	if resetWeekly {
		ent.WeeklyUsageUSD = 0
		ent.WeeklyWindowStart = &windowStart
	}
	if resetMonthly {
		ent.MonthlyUsageUSD = 0
		ent.MonthlyWindowStart = &windowStart
	}
	r.resetCalls = append(r.resetCalls, fakeEntitlementResetCall{id: id, resetDaily: resetDaily, resetWeekly: resetWeekly, resetMonthly: resetMonthly, windowStart: windowStart})
	return nil
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
		DailyWindowStart:   cloneTimePtr(ent.DailyWindowStart),
		WeeklyWindowStart:  cloneTimePtr(ent.WeeklyWindowStart),
		MonthlyWindowStart: cloneTimePtr(ent.MonthlyWindowStart),
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
	require.Len(t, repo.resetCalls, 3)
	require.NotEqual(t, dailyStart, *updated.DailyWindowStart)
	require.NotEqual(t, weeklyStart, *updated.WeeklyWindowStart)
	require.NotEqual(t, monthlyStart, *updated.MonthlyWindowStart)
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
	cp.DailyWindowStart = cloneTimePtr(ent.DailyWindowStart)
	cp.WeeklyWindowStart = cloneTimePtr(ent.WeeklyWindowStart)
	cp.MonthlyWindowStart = cloneTimePtr(ent.MonthlyWindowStart)
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

func cloneTimePtr(v *time.Time) *time.Time {
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
