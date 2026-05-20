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

	result, err := svc.validateSubOrder(ctx, CreateOrderRequest{PlanID: plan.ID})

	require.Nil(t, result)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PLAN_NOT_AVAILABLE")
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
