//go:build unit

package service

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type redeemCodeMemoryRepo struct {
	byID   map[int64]*RedeemCode
	byCode map[string]*RedeemCode

	useCalls    int
	updateCalls int
}

func newRedeemCodeMemoryRepo(codes ...*RedeemCode) *redeemCodeMemoryRepo {
	repo := &redeemCodeMemoryRepo{
		byID:   make(map[int64]*RedeemCode),
		byCode: make(map[string]*RedeemCode),
	}
	for _, code := range codes {
		if code == nil {
			continue
		}
		cp := *code
		repo.byID[cp.ID] = &cp
		repo.byCode[cp.Code] = &cp
	}
	return repo
}

func (r *redeemCodeMemoryRepo) Create(_ context.Context, code *RedeemCode) error {
	if code.ID == 0 {
		code.ID = int64(len(r.byID) + 1)
	}
	cp := *code
	r.byID[cp.ID] = &cp
	r.byCode[cp.Code] = &cp
	return nil
}

func (r *redeemCodeMemoryRepo) CreateBatch(ctx context.Context, codes []RedeemCode) error {
	for i := range codes {
		if err := r.Create(ctx, &codes[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *redeemCodeMemoryRepo) GetByID(_ context.Context, id int64) (*RedeemCode, error) {
	code := r.byID[id]
	if code == nil {
		return nil, ErrRedeemCodeNotFound
	}
	cp := *code
	return &cp, nil
}

func (r *redeemCodeMemoryRepo) GetByCode(_ context.Context, code string) (*RedeemCode, error) {
	stored := r.byCode[code]
	if stored == nil {
		return nil, ErrRedeemCodeNotFound
	}
	cp := *stored
	return &cp, nil
}

func (r *redeemCodeMemoryRepo) Update(_ context.Context, code *RedeemCode) error {
	if code == nil || r.byID[code.ID] == nil {
		return ErrRedeemCodeNotFound
	}
	r.updateCalls++
	cp := *code
	r.byID[cp.ID] = &cp
	r.byCode[cp.Code] = &cp
	return nil
}

func (r *redeemCodeMemoryRepo) BatchUpdate(context.Context, []int64, RedeemCodeBatchUpdateFields) (int64, error) {
	panic("unexpected BatchUpdate call")
}

func (r *redeemCodeMemoryRepo) Delete(_ context.Context, id int64) error {
	delete(r.byCode, r.byID[id].Code)
	delete(r.byID, id)
	return nil
}

func (r *redeemCodeMemoryRepo) Use(_ context.Context, id, userID int64) error {
	code := r.byID[id]
	if code == nil {
		return ErrRedeemCodeNotFound
	}
	if code.Status != StatusUnused {
		return ErrRedeemCodeUsed
	}
	r.useCalls++
	now := time.Now()
	code.Status = StatusUsed
	code.UsedBy = &userID
	code.UsedAt = &now
	return nil
}

func (r *redeemCodeMemoryRepo) List(context.Context, pagination.PaginationParams) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected List call")
}

func (r *redeemCodeMemoryRepo) ListWithFilters(context.Context, pagination.PaginationParams, string, string, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListWithFilters call")
}

func (r *redeemCodeMemoryRepo) ListByUser(context.Context, int64, int) ([]RedeemCode, error) {
	panic("unexpected ListByUser call")
}

func (r *redeemCodeMemoryRepo) ListByUserPaginated(context.Context, int64, pagination.PaginationParams, string) ([]RedeemCode, *pagination.PaginationResult, error) {
	panic("unexpected ListByUserPaginated call")
}

func (r *redeemCodeMemoryRepo) SumPositiveBalanceByUser(context.Context, int64) (float64, error) {
	panic("unexpected SumPositiveBalanceByUser call")
}

type redeemExternalMappingRepoStub struct {
	source       string
	groupID      int64
	validityDays int
	value        float64
	planID       int64
	err          error
	calls        int
}

func (r *redeemExternalMappingRepoStub) FindEnabled(_ context.Context, source string, legacyGroupID int64, legacyValidityDays int, legacyValue float64) (*SubscriptionPlanExternalMapping, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	if source != r.source || legacyGroupID != r.groupID || legacyValidityDays != r.validityDays || legacyValue != r.value {
		return nil, ErrSubscriptionPlanExternalMappingNotFound
	}
	return &SubscriptionPlanExternalMapping{
		ID:                 1,
		Source:             source,
		LegacyGroupID:      legacyGroupID,
		LegacyValidityDays: legacyValidityDays,
		LegacyValue:        legacyValue,
		PlanID:             r.planID,
		Enabled:            true,
	}, nil
}

func newRedeemEntitlementsSettingService(enabled, legacyMapping bool) *SettingService {
	repo := newMockSettingRepo()
	_ = repo.Set(context.Background(), SettingKeySubscriptionEntitlementsV2Enabled, strconv.FormatBool(enabled))
	_ = repo.Set(context.Background(), SettingKeySub2PaymentPageLegacyMappingEnabled, strconv.FormatBool(legacyMapping))
	return NewSettingService(repo, &config.Config{})
}

func TestDetectRedeemSourceContext_Sub2PaymentPageStrongMatch(t *testing.T) {
	groupID := int64(5)
	source := DetectRedeemSourceContext(RedeemSourceDetectionInput{
		IdempotencyKey: "s2p_order_123",
		Code:           "auto_order_123",
		Type:           RedeemTypeSubscription,
		GroupID:        &groupID,
		ValidityDays:   30,
		Value:          68,
	})

	require.True(t, source.IsSub2PaymentPageLegacy())
	require.Equal(t, SubscriptionPlanExternalMappingSourceSub2PaymentPage, source.Source)
	require.Equal(t, "order_123", source.ExternalOrderID)
	require.Equal(t, groupID, source.LegacyGroupID)
	require.Equal(t, 30, source.LegacyValidityDays)
	require.Equal(t, 68.0, source.LegacyValue)
}

func TestDetectRedeemSourceContext_RejectsWeakOrMismatchedSignals(t *testing.T) {
	groupID := int64(5)
	cases := []RedeemSourceDetectionInput{
		{IdempotencyKey: "s2p_a", Code: "auto_b", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68},
		{IdempotencyKey: "s2p_a", Code: "manual", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68},
		{IdempotencyKey: "", Code: "auto_a", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68},
		{IdempotencyKey: "s2p_a", Code: "auto_a", Type: RedeemTypeBalance, GroupID: &groupID, ValidityDays: 30, Value: 68},
		{IdempotencyKey: "s2p_a", Code: "auto_a", Type: RedeemTypeSubscription, GroupID: nil, ValidityDays: 30, Value: 68},
		{IdempotencyKey: "s2p_a", Code: "auto_a", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 0, Value: 68},
		{IdempotencyKey: "s2p_a", Code: "auto_a", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 0},
	}
	for _, tc := range cases {
		source := DetectRedeemSourceContext(tc)
		require.False(t, source.IsSub2PaymentPageLegacy(), "input=%+v", tc)
	}
}

func TestRedeemService_SubscriptionEntitlementFlagMatrix(t *testing.T) {
	ctx := context.Background()
	userID := int64(42)
	groupID := int64(5)
	planID := int64(9)

	run := func(t *testing.T, enabled bool, legacyMapping bool, code *RedeemCode, source RedeemSourceContext, mapping *redeemExternalMappingRepoStub) (*RedeemCode, *subscriptionUserSubRepoStub, *fakeSubscriptionEntitlementRepo, *redeemExternalMappingRepoStub) {
		t.Helper()
		client := newPaymentServiceEntClient(t)
		redeemRepo := newRedeemCodeMemoryRepo(code)
		subRepo := newSubscriptionUserSubRepoStub()
		entRepo := newFakeSubscriptionEntitlementRepo(time.Now())
		entSvc := NewSubscriptionEntitlementService(entRepo, &fakeSubscriptionEntitlementPlanRepo{
			plans: map[int64]*SubscriptionEntitlementPlan{
				planID: testEntitlementPlan(planID, []int64{groupID, groupID + 1}, nil),
			},
		})
		if mapping == nil {
			mapping = &redeemExternalMappingRepoStub{
				source:       SubscriptionPlanExternalMappingSourceSub2PaymentPage,
				groupID:      groupID,
				validityDays: 30,
				value:        68,
				planID:       planID,
			}
		}
		svc := NewRedeemService(
			redeemRepo,
			&userRepoStub{user: &User{ID: userID, Status: StatusActive}},
			NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: groupID, SubscriptionType: SubscriptionTypeSubscription}}, subRepo, nil, nil, nil),
			nil,
			nil,
			client,
			nil,
			nil,
		)
		svc.SetSubscriptionEntitlementDependencies(
			newRedeemEntitlementsSettingService(enabled, legacyMapping),
			entSvc,
			NewSubscriptionPlanExternalMappingService(mapping),
		)
		redeemed, err := svc.RedeemWithOptions(ctx, RedeemInput{UserID: userID, Code: code.Code, Source: source})
		require.NoError(t, err)
		return redeemed, subRepo, entRepo, mapping
	}

	t.Run("v2 disabled keeps legacy group redeem", func(t *testing.T) {
		source := DetectRedeemSourceContext(RedeemSourceDetectionInput{IdempotencyKey: "s2p_order1", Code: "auto_order1", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68})
		redeemed, subRepo, entRepo, _ := run(t, false, true, &RedeemCode{ID: 1, Code: "auto_order1", Type: RedeemTypeSubscription, Value: 68, Status: StatusUnused, GroupID: &groupID, ValidityDays: 30}, source, nil)
		require.Equal(t, 1, subRepo.createCalls)
		require.Zero(t, entRepo.createCount)
		require.Nil(t, redeemed.PlanID)
		require.Nil(t, redeemed.SubscriptionEntitlementID)
	})

	t.Run("v2 enabled mapping disabled keeps payment page legacy", func(t *testing.T) {
		source := DetectRedeemSourceContext(RedeemSourceDetectionInput{IdempotencyKey: "s2p_order2", Code: "auto_order2", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68})
		redeemed, subRepo, entRepo, mapping := run(t, true, false, &RedeemCode{ID: 2, Code: "auto_order2", Type: RedeemTypeSubscription, Value: 68, Status: StatusUnused, GroupID: &groupID, ValidityDays: 30}, source, nil)
		require.Equal(t, 1, subRepo.createCalls)
		require.Zero(t, entRepo.createCount)
		require.Zero(t, mapping.calls)
		require.Nil(t, redeemed.SubscriptionEntitlementID)
	})

	t.Run("v2 enabled mapping hit assigns entitlement", func(t *testing.T) {
		source := DetectRedeemSourceContext(RedeemSourceDetectionInput{IdempotencyKey: "s2p_order3", Code: "auto_order3", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68})
		redeemed, subRepo, entRepo, mapping := run(t, true, true, &RedeemCode{ID: 3, Code: "auto_order3", Type: RedeemTypeSubscription, Value: 68, Status: StatusUnused, GroupID: &groupID, ValidityDays: 30}, source, nil)
		require.Zero(t, subRepo.createCalls)
		require.Equal(t, 1, mapping.calls)
		require.Equal(t, 1, entRepo.createCount)
		require.Equal(t, 1, entRepo.eventCount)
		require.NotNil(t, redeemed.PlanID)
		require.Equal(t, planID, *redeemed.PlanID)
		require.NotNil(t, redeemed.SubscriptionEntitlementID)
		require.Equal(t, int64(1), *redeemed.SubscriptionEntitlementID)
		require.Len(t, entRepo.fulfillments, 1)
		for _, fulfillment := range entRepo.fulfillments {
			require.NotNil(t, fulfillment.SourceRedeemCodeID)
			require.Equal(t, int64(3), *fulfillment.SourceRedeemCodeID)
			require.NotNil(t, fulfillment.SourceExternalID)
			require.Equal(t, "order3", *fulfillment.SourceExternalID)
		}
	})

	t.Run("mapping miss amount mismatch falls back legacy", func(t *testing.T) {
		source := DetectRedeemSourceContext(RedeemSourceDetectionInput{IdempotencyKey: "s2p_order4", Code: "auto_order4", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 69})
		redeemed, subRepo, entRepo, mapping := run(t, true, true, &RedeemCode{ID: 4, Code: "auto_order4", Type: RedeemTypeSubscription, Value: 69, Status: StatusUnused, GroupID: &groupID, ValidityDays: 30}, source, nil)
		require.Equal(t, 1, subRepo.createCalls)
		require.Equal(t, 1, mapping.calls)
		require.Zero(t, entRepo.createCount)
		require.Nil(t, redeemed.SubscriptionEntitlementID)
	})

	t.Run("normal plan redeem uses v2", func(t *testing.T) {
		redeemed, subRepo, entRepo, mapping := run(t, true, false, &RedeemCode{ID: 5, Code: "plan-code", Type: RedeemTypeSubscription, Value: 68, Status: StatusUnused, PlanID: &planID}, RedeemSourceContext{}, nil)
		require.Zero(t, subRepo.createCalls)
		require.Zero(t, mapping.calls)
		require.Equal(t, 1, entRepo.createCount)
		require.NotNil(t, redeemed.SubscriptionEntitlementID)
	})

	t.Run("normal admin group redeem is not mapped", func(t *testing.T) {
		redeemed, subRepo, entRepo, mapping := run(t, true, true, &RedeemCode{ID: 6, Code: "manual-code", Type: RedeemTypeSubscription, Value: 68, Status: StatusUnused, GroupID: &groupID, ValidityDays: 30}, RedeemSourceContext{}, nil)
		require.Equal(t, 1, subRepo.createCalls)
		require.Zero(t, mapping.calls)
		require.Zero(t, entRepo.createCount)
		require.Nil(t, redeemed.SubscriptionEntitlementID)
	})
}

func TestRedeemService_MappingErrorPropagates(t *testing.T) {
	ctx := context.Background()
	userID := int64(42)
	groupID := int64(5)
	source := DetectRedeemSourceContext(RedeemSourceDetectionInput{IdempotencyKey: "s2p_order_err", Code: "auto_order_err", Type: RedeemTypeSubscription, GroupID: &groupID, ValidityDays: 30, Value: 68})
	mappingErr := errors.New("mapping db down")
	client := newPaymentServiceEntClient(t)
	redeemRepo := newRedeemCodeMemoryRepo(&RedeemCode{ID: 1, Code: "auto_order_err", Type: RedeemTypeSubscription, Value: 68, Status: StatusUnused, GroupID: &groupID, ValidityDays: 30})
	subRepo := newSubscriptionUserSubRepoStub()
	svc := NewRedeemService(
		redeemRepo,
		&userRepoStub{user: &User{ID: userID, Status: StatusActive}},
		NewSubscriptionService(&subscriptionGroupRepoStub{group: &Group{ID: groupID, SubscriptionType: SubscriptionTypeSubscription}}, subRepo, nil, nil, nil),
		nil,
		nil,
		client,
		nil,
		nil,
	)
	svc.SetSubscriptionEntitlementDependencies(
		newRedeemEntitlementsSettingService(true, true),
		NewSubscriptionEntitlementService(newFakeSubscriptionEntitlementRepo(time.Now()), &fakeSubscriptionEntitlementPlanRepo{}),
		NewSubscriptionPlanExternalMappingService(&redeemExternalMappingRepoStub{err: mappingErr}),
	)

	_, err := svc.RedeemWithOptions(ctx, RedeemInput{UserID: userID, Code: "auto_order_err", Source: source})
	require.ErrorIs(t, err, mappingErr)
	require.Zero(t, subRepo.createCalls)
}
