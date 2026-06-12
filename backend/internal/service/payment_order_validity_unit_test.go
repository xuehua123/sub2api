package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/enttest"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "modernc.org/sqlite"
)

func TestValidateSubOrder_RejectsPlanWithInvalidValidityUnit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client := newPaymentOrderValidityUnitTestClient(t)

	group, err := client.Group.Create().
		SetName("invalid-validity-group").
		SetStatus(payment.EntityStatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName("Broken Public Plan").
		SetDescription("broken validity unit").
		SetPrice(9.9).
		SetValidityDays(1).
		SetValidityUnit("wek").
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{
		configService: &PaymentConfigService{entClient: client},
		groupRepo: &subscriptionGroupRepoStub{
			group: &Group{
				ID:               group.ID,
				Status:           payment.EntityStatusActive,
				SubscriptionType: SubscriptionTypeSubscription,
			},
		},
	}

	result, access, err := svc.validateSubOrder(ctx, CreateOrderRequest{PlanID: plan.ID})

	require.Nil(t, result)
	require.Nil(t, access)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PLAN_NOT_AVAILABLE")
}

func TestCreateSubscriptionOrderUsesEffectivePlanGroupAsLegacyAnchor(t *testing.T) {
	ctx := context.Background()
	client := newPaymentOrderValidityUnitTestClient(t)

	legacyGroup, err := client.Group.Create().
		SetName("legacy-plan-anchor").
		SetStatus(payment.EntityStatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetSortOrder(30).
		Save(ctx)
	require.NoError(t, err)
	firstGroup, err := client.Group.Create().
		SetName("effective-plan-anchor-a").
		SetStatus(payment.EntityStatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetSortOrder(10).
		Save(ctx)
	require.NoError(t, err)
	secondGroup, err := client.Group.Create().
		SetName("effective-plan-anchor-b").
		SetStatus(payment.EntityStatusActive).
		SetSubscriptionType(SubscriptionTypeSubscription).
		SetSortOrder(20).
		Save(ctx)
	require.NoError(t, err)

	plan, err := client.SubscriptionPlan.Create().
		SetGroupID(legacyGroup.ID).
		SetName("Effective Plan").
		SetDescription("uses v2 groups").
		SetPrice(19.9).
		SetValidityDays(30).
		SetValidityUnit("day").
		SetAccessScope(PlanAccessScopeExplicit).
		SetOveragePolicy(SubscriptionEntitlementOverageBlock).
		SetForSale(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionPlanGroup.Create().
		SetPlanID(plan.ID).
		SetGroupID(firstGroup.ID).
		SetSortOrder(0).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)
	_, err = client.SubscriptionPlanGroup.Create().
		SetPlanID(plan.ID).
		SetGroupID(secondGroup.ID).
		SetSortOrder(1).
		SetEnabled(true).
		Save(ctx)
	require.NoError(t, err)

	svc := &PaymentService{entClient: client, configService: &PaymentConfigService{entClient: client}}
	validPlan, access, err := svc.validateSubOrder(ctx, CreateOrderRequest{PlanID: plan.ID})
	require.NoError(t, err)
	require.Equal(t, plan.ID, validPlan.ID)
	require.Equal(t, firstGroup.ID, access.PrimaryGroupID)
	require.Equal(t, []int64{firstGroup.ID, secondGroup.ID}, access.GroupIDs)

	user, err := client.User.Create().
		SetEmail("effective-plan-anchor@example.com").
		SetPasswordHash("hash").
		SetUsername("effective-plan-anchor").
		Save(ctx)
	require.NoError(t, err)
	order, err := svc.createOrderInTx(
		ctx,
		CreateOrderRequest{
			UserID:      user.ID,
			OrderType:   payment.OrderTypeSubscription,
			PaymentType: payment.TypeAlipay,
			PlanID:      plan.ID,
			ClientIP:    "127.0.0.1",
			SrcHost:     "example.com",
		},
		&User{ID: user.ID, Email: user.Email, Username: user.Username},
		validPlan,
		access,
		&PaymentConfig{OrderTimeoutMin: 15},
		plan.Price,
		plan.Price,
		0,
		plan.Price,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, order.SubscriptionGroupID)
	require.Equal(t, firstGroup.ID, *order.SubscriptionGroupID)
	require.NotEqual(t, legacyGroup.ID, *order.SubscriptionGroupID)
}

func newPaymentOrderValidityUnitTestClient(t *testing.T) *dbent.Client {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := sql.Open("sqlite", dsn)
	require.NoError(t, err)
	_, err = db.Exec("PRAGMA foreign_keys = ON")
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.SQLite, db)
	client := enttest.NewClient(t, enttest.WithOptions(dbent.Driver(drv)))
	t.Cleanup(func() {
		require.NoError(t, client.Close())
		require.NoError(t, db.Close())
	})
	return client
}
