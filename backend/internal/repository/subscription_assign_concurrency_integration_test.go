//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementfulfillment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

type enabledSubscriptionEntitlementsRuntime struct{}

func (enabledSubscriptionEntitlementsRuntime) GetSubscriptionEntitlementsRuntime(context.Context) service.SubscriptionEntitlementsRuntime {
	return service.SubscriptionEntitlementsRuntime{Enabled: true}
}

type concurrentAssignUserSubscriptionRepo struct {
	service.UserSubscriptionRepository
	start sync.WaitGroup
}

func newConcurrentAssignUserSubscriptionRepo(repo service.UserSubscriptionRepository) *concurrentAssignUserSubscriptionRepo {
	out := &concurrentAssignUserSubscriptionRepo{UserSubscriptionRepository: repo}
	out.start.Add(2)
	return out
}

func (r *concurrentAssignUserSubscriptionRepo) waitForBothCalls() {
	r.start.Done()
	r.start.Wait()
}

func (r *concurrentAssignUserSubscriptionRepo) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*service.UserSubscription, error) {
	sub, err := r.UserSubscriptionRepository.GetByUserIDAndGroupID(ctx, userID, groupID)
	if err == nil {
		r.waitForBothCalls()
	}
	return sub, err
}

func TestAssignSubscriptionConcurrentExpiredPlanRenewsEntitlementOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: uniqueTestEmail("concurrent-sub-renew")})
	group := mustCreateGroup(t, client, &service.Group{
		Name:             uniqueTestName("concurrent-sub-renew-group"),
		Platform:         service.PlatformOpenAI,
		SubscriptionType: service.SubscriptionTypeSubscription,
	})
	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName(uniqueTestName("concurrent-sub-renew-plan")).
		SetPrice(29.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)

	groupRepo := NewGroupRepository(client, integrationDB)
	baseSubRepo := NewUserSubscriptionRepository(client)
	entitlementRepo := NewSubscriptionEntitlementRepository(client)
	planRepo := NewSubscriptionEntitlementPlanRepository(client)
	entitlementSvc := service.NewSubscriptionEntitlementService(entitlementRepo, planRepo)
	newSubscriptionSvc := func(subRepo service.UserSubscriptionRepository) *service.SubscriptionService {
		svc := service.NewSubscriptionService(groupRepo, subRepo, nil, client, nil)
		svc.SetSubscriptionEntitlementAliasDependencies(enabledSubscriptionEntitlementsRuntime{}, entitlementSvc)
		return svc
	}
	input := &service.AssignSubscriptionInput{
		UserID:       user.ID,
		GroupID:      group.ID,
		PlanID:       plan.ID,
		ValidityDays: 30,
		AssignedBy:   user.ID,
		Notes:        "concurrent renewal",
	}

	initial, err := newSubscriptionSvc(baseSubRepo).AssignSubscription(ctx, input)
	require.NoError(t, err)
	entitlement, err := client.SubscriptionEntitlement.Query().
		Where(subscriptionentitlement.LegacySubscriptionIDEQ(initial.ID)).
		Only(ctx)
	require.NoError(t, err)

	expiredStartsAt := time.Now().UTC().AddDate(0, 0, -60)
	expiredAt := expiredStartsAt.AddDate(0, 0, 30)
	_, err = client.UserSubscription.UpdateOneID(initial.ID).
		SetStartsAt(expiredStartsAt).
		SetExpiresAt(expiredAt).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEntitlement.UpdateOneID(entitlement.ID).
		SetStartsAt(expiredStartsAt).
		SetExpiresAt(expiredAt).
		SetStatus(service.SubscriptionStatusExpired).
		Save(ctx)
	require.NoError(t, err)

	concurrentRepo := newConcurrentAssignUserSubscriptionRepo(baseSubRepo)
	svc := newSubscriptionSvc(concurrentRepo)
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			_, assignErr := svc.AssignSubscription(ctx, input)
			errCh <- assignErr
		}()
	}
	for range 2 {
		require.NoError(t, <-errCh)
	}

	fulfillmentCount, err := client.SubscriptionEntitlementFulfillment.Query().
		Where(subscriptionentitlementfulfillment.EntitlementIDEQ(entitlement.ID)).
		Count(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, fulfillmentCount, "initial grant plus one renewed term must be recorded")

	renewedSubscription, err := client.UserSubscription.Get(ctx, initial.ID)
	require.NoError(t, err)
	renewedEntitlement, err := client.SubscriptionEntitlement.Get(ctx, entitlement.ID)
	require.NoError(t, err)
	require.WithinDuration(t, renewedSubscription.ExpiresAt, renewedEntitlement.ExpiresAt, time.Second,
		"concurrent idempotent assignment must keep the legacy subscription and entitlement term aligned")
}
