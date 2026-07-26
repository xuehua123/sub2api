//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	entgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementgroup"
	"github.com/stretchr/testify/require"
)

type entCreateGroupRepoForAdmin struct {
	*groupRepoStubForAdmin
	client *dbent.Client
}

func (r *entCreateGroupRepoForAdmin) Create(ctx context.Context, group *Group) error {
	client := r.client
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client = tx.Client()
	}
	created, err := client.Group.Create().
		SetName(group.Name).
		SetDescription(group.Description).
		SetPlatform(group.Platform).
		SetRateMultiplier(group.RateMultiplier).
		SetIsExclusive(group.IsExclusive).
		SetStatus(group.Status).
		SetSubscriptionType(group.SubscriptionType).
		SetBalanceEnabled(group.BalanceEnabled).
		SetSubscriptionEnabled(group.SubscriptionEnabled).
		SetPlanAutoGrantEnabled(group.PlanAutoGrantEnabled).
		Save(ctx)
	if err != nil {
		return err
	}
	group.ID = created.ID
	group.CreatedAt = created.CreatedAt
	group.UpdatedAt = created.UpdatedAt
	return nil
}

func TestAdminServiceCreateGroupRollsBackWhenPlanAutoGrantSyncFails(t *testing.T) {
	ctx := context.Background()
	client := newPaymentConfigServiceTestClient(t)
	oldGroup := createPaymentConfigPlanTestGroup(t, client, "existing-auto-openai", PlatformOpenAI, 0)

	planService := &PaymentConfigService{entClient: client}
	plan, err := planService.CreatePlan(ctx, CreatePlanRequest{
		Name:         "Dynamic OpenAI",
		AccessScope:  PlanAccessScopeAllSubscriptionGroups,
		Price:        9.99,
		ValidityDays: 30,
		ValidityUnit: "day",
	})
	require.NoError(t, err)

	user, err := client.User.Create().
		SetEmail("create-group-rollback@example.test").
		SetPasswordHash("hash").
		Save(ctx)
	require.NoError(t, err)
	now := time.Now()
	entitlement, err := client.SubscriptionEntitlement.Create().
		SetUserID(user.ID).
		SetPlanID(plan.ID).
		SetPrimaryGroupID(oldGroup.ID).
		SetName(plan.Name).
		SetSourceType(SubscriptionEntitlementSourceAdminAssign).
		SetStatus(SubscriptionStatusActive).
		SetStartsAt(now.Add(-time.Hour)).
		SetExpiresAt(now.Add(30 * 24 * time.Hour)).
		SetDailyWindowStart(now.Add(-time.Hour)).
		SetWeeklyWindowStart(now.Add(-time.Hour)).
		SetMonthlyWindowStart(now.Add(-time.Hour)).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetPlanSnapshot(map[string]any{"price": plan.Price}).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionEntitlementGroup.Create().
		SetEntitlementID(entitlement.ID).
		SetGroupID(oldGroup.ID).
		SetSortOrder(0).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	_, err = client.ExecContext(ctx, `
		CREATE TRIGGER fail_entitlement_group_insert
		BEFORE INSERT ON subscription_entitlement_groups
		BEGIN
			SELECT RAISE(ABORT, 'forced plan auto-grant sync failure');
		END
	`)
	require.NoError(t, err)

	groupName := "new-auto-openai"
	groupRepo := &entCreateGroupRepoForAdmin{
		groupRepoStubForAdmin: &groupRepoStubForAdmin{},
		client:                client,
	}
	svc := &adminServiceImpl{groupRepo: groupRepo, entClient: client}
	_, err = svc.CreateGroup(ctx, &CreateGroupInput{
		Name:             groupName,
		Platform:         PlatformOpenAI,
		RateMultiplier:   1,
		SubscriptionType: SubscriptionTypeSubscription,
	})
	require.ErrorContains(t, err, "forced plan auto-grant sync failure")

	createdCount, err := client.Group.Query().Where(entgroup.NameEQ(groupName)).Count(ctx)
	require.NoError(t, err)
	require.Zero(t, createdCount, "failed scope synchronization must roll back group creation")

	bindings, err := client.SubscriptionEntitlementGroup.Query().
		Where(subscriptionentitlementgroup.EntitlementIDEQ(entitlement.ID)).
		All(ctx)
	require.NoError(t, err)
	require.Len(t, bindings, 1)
	require.Equal(t, oldGroup.ID, bindings[0].GroupID)
}
