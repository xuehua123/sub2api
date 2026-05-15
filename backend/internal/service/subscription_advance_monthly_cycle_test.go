package service

import (
	"context"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	sqlmock "github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/stretchr/testify/require"
)

type advanceMonthlyCycleUserSubRepoStub struct {
	userSubRepoNoop

	sub          *UserSubscription
	refreshedSub *UserSubscription
	getCalls     int
}

func (r *advanceMonthlyCycleUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	if r.sub == nil || r.sub.ID != id {
		return nil, ErrSubscriptionNotFound
	}
	r.getCalls++
	if r.getCalls > 1 && r.refreshedSub != nil {
		cp := *r.refreshedSub
		return &cp, nil
	}
	cp := *r.sub
	return &cp, nil
}

func newAdvanceMonthlyCycleMockClient(t *testing.T) (*dbent.Client, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	drv := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(drv))

	return client, mock
}

func TestAdvanceMonthlyCycleRunsLockedUpdateAndLogInTransaction(t *testing.T) {
	ctx := context.Background()
	userID := int64(7)
	subscriptionID := int64(100)
	groupID := int64(20)
	limit := 10.0
	previousUsage := 9.0
	previousWindow := time.Now().Add(12 * time.Hour)
	previousExpiresAt := time.Now().Add(45 * 24 * time.Hour)
	previousUpdatedAt := time.Now().Add(-time.Hour)
	group := &Group{
		ID:              groupID,
		MonthlyLimitUSD: &limit,
	}
	repo := &advanceMonthlyCycleUserSubRepoStub{
		sub: &UserSubscription{
			ID:              subscriptionID,
			UserID:          userID,
			GroupID:         groupID,
			Group:           group,
			Status:          SubscriptionStatusActive,
			MonthlyUsageUSD: previousUsage,
			ExpiresAt:       previousExpiresAt,
		},
		refreshedSub: &UserSubscription{
			ID:              subscriptionID,
			UserID:          userID,
			GroupID:         groupID,
			Group:           group,
			Status:          SubscriptionStatusActive,
			MonthlyUsageUSD: 0,
			ExpiresAt:       previousExpiresAt.AddDate(0, 0, -1),
		},
	}
	client, mock := newAdvanceMonthlyCycleMockClient(t)
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, client, nil)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT monthly_usage_usd, monthly_window_start, expires_at, status, updated_at\s+FROM user_subscriptions\s+WHERE id = \$1 AND user_id = \$2 AND deleted_at IS NULL\s+FOR UPDATE`).
		WithArgs(subscriptionID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"monthly_usage_usd",
			"monthly_window_start",
			"expires_at",
			"status",
			"updated_at",
		}).AddRow(previousUsage, previousWindow, previousExpiresAt, SubscriptionStatusActive, previousUpdatedAt))
	mock.ExpectExec(`(?s)UPDATE user_subscriptions\s+SET monthly_usage_usd = 0,\s+monthly_window_start = \$1,\s+expires_at = \$2,\s+updated_at = \$3\s+WHERE id = \$4 AND user_id = \$5 AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), subscriptionID, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO subscription_cycle_reset_logs \(\s+user_id, subscription_id, group_id, previous_expires_at, new_expires_at,\s+previous_monthly_usage_usd, previous_monthly_window_start, new_monthly_window_start, deducted_days, created_at\s+\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, NOW\(\)\)`).
		WithArgs(userID, subscriptionID, groupID, previousExpiresAt, sqlmock.AnyArg(), previousUsage, previousWindow, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := svc.AdvanceMonthlyCycle(ctx, userID, subscriptionID)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, previousUsage, result.PreviousMonthlyUsage)
	require.NotNil(t, result.Subscription)
	require.Zero(t, result.Subscription.MonthlyUsageUSD)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCanAdvanceMonthlyCycleByUsageAllowsLastTenPercent(t *testing.T) {
	require.True(t, canAdvanceMonthlyCycleByUsage(9.0, 10.0))
	require.True(t, canAdvanceMonthlyCycleByUsage(10.0, 10.0))
	require.False(t, canAdvanceMonthlyCycleByUsage(8.99, 10.0))
	require.False(t, canAdvanceMonthlyCycleByUsage(0, 10.0))
	require.False(t, canAdvanceMonthlyCycleByUsage(10.0, 0))
}

func TestAdvanceMonthlyCycleRollsBackWhenLockedRowIsAlreadyReset(t *testing.T) {
	ctx := context.Background()
	userID := int64(7)
	subscriptionID := int64(100)
	groupID := int64(20)
	limit := 10.0
	previousWindow := time.Now().Add(12 * time.Hour)
	previousExpiresAt := time.Now().Add(45 * 24 * time.Hour)
	previousUpdatedAt := time.Now().Add(-time.Hour)
	group := &Group{
		ID:              groupID,
		MonthlyLimitUSD: &limit,
	}
	repo := &advanceMonthlyCycleUserSubRepoStub{
		sub: &UserSubscription{
			ID:              subscriptionID,
			UserID:          userID,
			GroupID:         groupID,
			Group:           group,
			Status:          SubscriptionStatusActive,
			MonthlyUsageUSD: limit + 1,
			ExpiresAt:       previousExpiresAt,
		},
	}
	client, mock := newAdvanceMonthlyCycleMockClient(t)
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, client, nil)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT monthly_usage_usd, monthly_window_start, expires_at, status, updated_at\s+FROM user_subscriptions\s+WHERE id = \$1 AND user_id = \$2 AND deleted_at IS NULL\s+FOR UPDATE`).
		WithArgs(subscriptionID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"monthly_usage_usd",
			"monthly_window_start",
			"expires_at",
			"status",
			"updated_at",
		}).AddRow(0.0, previousWindow, previousExpiresAt, SubscriptionStatusActive, previousUpdatedAt))
	mock.ExpectRollback()

	result, err := svc.AdvanceMonthlyCycle(ctx, userID, subscriptionID)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMonthlyCycleNotExhausted)
	require.NoError(t, mock.ExpectationsWereMet())
}
