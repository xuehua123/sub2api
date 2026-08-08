//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type SubscriptionEntitlementRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *subscriptionEntitlementRepository
}

func (s *SubscriptionEntitlementRepoSuite) SetupTest() {
	tx := testEntTx(s.T())
	s.ctx = dbent.NewTxContext(context.Background(), tx)
	s.client = tx.Client()
	s.repo = NewSubscriptionEntitlementRepository(integrationEntClient).(*subscriptionEntitlementRepository)
}

func TestSubscriptionEntitlementRepoSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionEntitlementRepoSuite))
}

func (s *SubscriptionEntitlementRepoSuite) TestCreateQueryReplaceGroupsAndApplyUsage() {
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := mustCreateUser(s.T(), s.client, &service.User{Email: uniqueTestEmail("entitlement")})
	groupA := mustCreateGroup(s.T(), s.client, &service.Group{Name: uniqueTestName("ent-group-a")})
	groupB := mustCreateGroup(s.T(), s.client, &service.Group{Name: uniqueTestName("ent-group-b")})
	plan := s.mustCreatePlan(groupA.ID)
	redeemCodeID := s.mustCreateRedeemCode(groupA.ID, plan.ID)
	sourceID := int64(990001)
	sourceExternalID := "ext-entitlement-990001"
	monthlyLimit := 2.0

	ent := &service.SubscriptionEntitlement{
		UserID:             user.ID,
		PlanID:             &plan.ID,
		PrimaryGroupID:     &groupA.ID,
		Name:               "V2 entitlement test",
		SourceType:         service.SubscriptionEntitlementSourcePaymentOrder,
		Status:             service.SubscriptionStatusActive,
		StartsAt:           now.Add(-time.Hour),
		ExpiresAt:          now.Add(24 * time.Hour),
		MonthlyLimitUSD:    &monthlyLimit,
		OveragePolicy:      service.SubscriptionEntitlementOverageBlock,
		PlanSnapshot:       map[string]any{"plan_id": plan.ID, "name": plan.Name},
		SourceID:           &sourceID,
		SourceExternalID:   &sourceExternalID,
		SourceRedeemCodeID: &redeemCodeID,
	}

	s.Require().NoError(s.repo.CreateTx(s.ctx, ent, []int64{groupA.ID, groupB.ID}))
	s.Require().NotZero(ent.ID)

	got, err := s.repo.GetByID(s.ctx, ent.ID)
	s.Require().NoError(err)
	s.Require().Equal(user.ID, got.UserID)
	s.Require().ElementsMatch([]int64{groupA.ID, groupB.ID}, entitlementGroupIDs(got))

	bySourceID, err := s.repo.GetBySourceID(s.ctx, service.SubscriptionEntitlementSourcePaymentOrder, sourceID)
	s.Require().NoError(err)
	s.Require().Equal(ent.ID, bySourceID.ID)

	byExternalID, err := s.repo.GetBySourceExternalID(s.ctx, service.SubscriptionEntitlementSourcePaymentOrder, sourceExternalID)
	s.Require().NoError(err)
	s.Require().Equal(ent.ID, byExternalID.ID)

	byRedeemCode, err := s.repo.GetBySourceRedeemCodeID(s.ctx, redeemCodeID)
	s.Require().NoError(err)
	s.Require().Equal(ent.ID, byRedeemCode.ID)

	active, err := s.repo.ListActiveByUserID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().Len(active, 1)
	s.Require().Equal(ent.ID, active[0].ID)

	coveringB, err := s.repo.GetActiveCoveringGroup(s.ctx, user.ID, groupB.ID)
	s.Require().NoError(err)
	s.Require().Len(coveringB, 1)

	s.Require().NoError(s.repo.ReplaceGroups(s.ctx, ent.ID, []int64{groupB.ID}))
	coveringA, err := s.repo.ListActiveCoveringGroupForUser(s.ctx, user.ID, groupA.ID)
	s.Require().NoError(err)
	s.Require().Empty(coveringA)
	coveringB, err = s.repo.ListActiveCoveringGroupForUser(s.ctx, user.ID, groupB.ID)
	s.Require().NoError(err)
	s.Require().Len(coveringB, 1)

	applied, err := s.repo.ApplyEntitlementUsage(s.ctx, ent.ID, 1.25, now)
	s.Require().NoError(err)
	s.Require().InEpsilon(1.25, applied.DailyUsageUSD, 0.000001)
	s.Require().InEpsilon(1.25, applied.WeeklyUsageUSD, 0.000001)
	s.Require().InEpsilon(1.25, applied.MonthlyUsageUSD, 0.000001)
	s.Require().False(applied.UpdatedAt.IsZero())

	_, err = s.repo.ApplyEntitlementUsage(s.ctx, ent.ID, 1.00, now.Add(time.Second))
	s.Require().ErrorIs(err, service.ErrSubscriptionEntitlementQuotaExceeded)

	afterLimit, err := s.repo.GetByID(s.ctx, ent.ID)
	s.Require().NoError(err)
	s.Require().InEpsilon(1.25, afterLimit.MonthlyUsageUSD, 0.000001)
}

func (s *SubscriptionEntitlementRepoSuite) TestListActiveExcludesFutureStartsAt() {
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := mustCreateUser(s.T(), s.client, &service.User{Email: uniqueTestEmail("ent-future")})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: uniqueTestName("ent-future-group")})
	ent := &service.SubscriptionEntitlement{
		UserID:         user.ID,
		PrimaryGroupID: &group.ID,
		Name:           "future entitlement",
		Status:         service.SubscriptionStatusActive,
		StartsAt:       now.Add(time.Hour),
		ExpiresAt:      now.Add(24 * time.Hour),
	}
	s.Require().NoError(s.repo.CreateTx(s.ctx, ent, []int64{group.ID}))

	active, err := s.repo.ListActiveByUserID(s.ctx, user.ID)
	s.Require().NoError(err)
	s.Require().Empty(active)

	covering, err := s.repo.ListActiveCoveringGroupForUser(s.ctx, user.ID, group.ID)
	s.Require().NoError(err)
	s.Require().Empty(covering)
}

func (s *SubscriptionEntitlementRepoSuite) TestResetUsageAndUpdateTerm() {
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := mustCreateUser(s.T(), s.client, &service.User{Email: uniqueTestEmail("ent-reset")})
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: uniqueTestName("ent-reset-group")})
	ent := &service.SubscriptionEntitlement{
		UserID:          user.ID,
		PrimaryGroupID:  &group.ID,
		Name:            "reset entitlement",
		Status:          service.SubscriptionStatusActive,
		StartsAt:        now.Add(-time.Hour),
		ExpiresAt:       now.Add(24 * time.Hour),
		DailyUsageUSD:   1.5,
		WeeklyUsageUSD:  2.5,
		MonthlyUsageUSD: 3.5,
	}
	s.Require().NoError(s.repo.CreateTx(s.ctx, ent, []int64{group.ID}))

	windowStart := now.Add(2 * time.Hour)
	dailyStart := windowStart.Add(-time.Hour)
	s.Require().NoError(s.repo.ResetUsage(s.ctx, ent.ID, true, false, true, dailyStart, windowStart))
	got, err := s.repo.GetByID(s.ctx, ent.ID)
	s.Require().NoError(err)
	s.Require().Zero(got.DailyUsageUSD)
	s.Require().InEpsilon(2.5, got.WeeklyUsageUSD, 0.000001)
	s.Require().Zero(got.MonthlyUsageUSD)
	s.Require().Equal(dailyStart, *got.DailyWindowStart)
	s.Require().Equal(windowStart, *got.MonthlyWindowStart)

	newExpiresAt := now.Add(48 * time.Hour)
	s.Require().NoError(s.repo.UpdateTerm(s.ctx, ent.ID, now, newExpiresAt, service.SubscriptionStatusActive, "extended"))
	got, err = s.repo.GetByID(s.ctx, ent.ID)
	s.Require().NoError(err)
	s.Require().Equal(newExpiresAt, got.ExpiresAt)
	s.Require().Equal("extended", got.Notes)
}

func (s *SubscriptionEntitlementRepoSuite) mustCreatePlan(groupID int64) *dbent.SubscriptionPlan {
	s.T().Helper()

	plan, err := s.client.SubscriptionPlan.Create().
		SetGroupID(groupID).
		SetName(uniqueTestName("ent-plan")).
		SetPrice(29.90).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(s.ctx)
	s.Require().NoError(err)
	return plan
}

func (s *SubscriptionEntitlementRepoSuite) mustCreateRedeemCode(groupID, planID int64) int64 {
	s.T().Helper()

	code, err := s.client.RedeemCode.Create().
		SetCode(uniqueTestName("ent-code")).
		SetType(service.RedeemTypeSubscription).
		SetValue(29.90).
		SetStatus(service.StatusUnused).
		SetGroupID(groupID).
		SetPlanID(planID).
		SetValidityDays(30).
		Save(s.ctx)
	s.Require().NoError(err)
	return code.ID
}

func entitlementGroupIDs(ent *service.SubscriptionEntitlement) []int64 {
	ids := make([]int64, 0, len(ent.Groups))
	for _, group := range ent.Groups {
		ids = append(ids, group.ID)
	}
	return ids
}

func uniqueTestEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@example.com", prefix, time.Now().UnixNano())
}

func uniqueTestName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func TestSubscriptionEntitlementApplyUsageConcurrentLimitCheckIsAtomic(t *testing.T) {
	ctx := context.Background()
	repo := NewSubscriptionEntitlementRepository(integrationEntClient).(*subscriptionEntitlementRepository)
	now := time.Now().UTC().Truncate(time.Microsecond)
	user := mustCreateUser(t, integrationEntClient, &service.User{Email: uniqueTestEmail("ent-concurrent")})
	group := mustCreateGroup(t, integrationEntClient, &service.Group{Name: uniqueTestName("ent-concurrent-group")})
	monthlyLimit := 5.0
	ent := &service.SubscriptionEntitlement{
		UserID:          user.ID,
		PrimaryGroupID:  &group.ID,
		Name:            "concurrent entitlement",
		Status:          service.SubscriptionStatusActive,
		StartsAt:        now.Add(-time.Hour),
		ExpiresAt:       now.Add(24 * time.Hour),
		MonthlyLimitUSD: &monthlyLimit,
	}
	requireNoError := func(err error) {
		if err != nil {
			t.Helper()
			t.Fatal(err)
		}
	}
	requireNoError(repo.Create(ctx, ent, []int64{group.ID}))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM subscription_entitlements WHERE id = $1", ent.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id = $1", group.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", user.ID)
	})

	var successes int64
	var quotaFailures int64
	var unexpectedErrors int64
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.ApplyEntitlementUsage(context.Background(), ent.ID, 1, now)
			switch {
			case err == nil:
				atomic.AddInt64(&successes, 1)
			case err == service.ErrSubscriptionEntitlementQuotaExceeded:
				atomic.AddInt64(&quotaFailures, 1)
			default:
				atomic.AddInt64(&unexpectedErrors, 1)
			}
		}()
	}
	wg.Wait()

	if unexpectedErrors != 0 {
		t.Fatalf("ApplyEntitlementUsage returned %d unexpected errors", unexpectedErrors)
	}
	if successes != 5 {
		t.Fatalf("successes = %d, want 5", successes)
	}
	if quotaFailures != 5 {
		t.Fatalf("quota failures = %d, want 5", quotaFailures)
	}
	got, err := repo.GetByID(context.Background(), ent.ID)
	requireNoError(err)
	if got.MonthlyUsageUSD < 4.999999 || got.MonthlyUsageUSD > 5.000001 {
		t.Fatalf("monthly usage = %f, want 5", got.MonthlyUsageUSD)
	}
}
