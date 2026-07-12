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
	previousStartsAt := time.Now().Add(-2 * 24 * time.Hour)
	previousWindow := previousStartsAt
	previousExpiresAt := time.Now().Add(75 * 24 * time.Hour)
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
	mock.ExpectQuery(`(?s)SELECT monthly_usage_usd, monthly_window_start, starts_at, expires_at, status, updated_at\s+FROM user_subscriptions\s+WHERE id = \$1 AND user_id = \$2 AND deleted_at IS NULL\s+FOR UPDATE`).
		WithArgs(subscriptionID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"monthly_usage_usd",
			"monthly_window_start",
			"starts_at",
			"expires_at",
			"status",
			"updated_at",
		}).AddRow(previousUsage, previousWindow, previousStartsAt, previousExpiresAt, SubscriptionStatusActive, previousUpdatedAt))
	mock.ExpectExec(`(?s)UPDATE user_subscriptions\s+SET monthly_usage_usd = 0,\s+monthly_window_start = \$1,\s+expires_at = \$2,\s+updated_at = \$3\s+WHERE id = \$4 AND user_id = \$5 AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), subscriptionID, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO subscription_cycle_reset_logs \(\s+user_id, subscription_id, group_id, previous_expires_at, new_expires_at,\s+previous_monthly_usage_usd, previous_monthly_window_start, new_monthly_window_start,\s+deducted_days, deducted_seconds, created_at\s+\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, NOW\(\)\)`).
		WithArgs(userID, subscriptionID, groupID, previousExpiresAt, sqlmock.AnyArg(), previousUsage, previousWindow, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	before := time.Now().Truncate(time.Second)
	result, err := svc.AdvanceMonthlyCycle(ctx, userID, subscriptionID)
	after := time.Now().Truncate(time.Second)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, previousUsage, result.PreviousMonthlyUsage)
	require.Greater(t, result.DeductedSeconds, int64(0))
	require.False(t, result.NewMonthlyWindowStart.Before(before.Add(-time.Second)))
	require.False(t, result.NewMonthlyWindowStart.After(after.Add(time.Second)))
	require.Zero(t, result.NewMonthlyWindowStart.Nanosecond())
	require.NotNil(t, result.Subscription)
	require.Zero(t, result.Subscription.MonthlyUsageUSD)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdvanceMonthlyCycleRespectsManualResetAnchor(t *testing.T) {
	ctx := context.Background()
	userID := int64(8)
	subscriptionID := int64(101)
	groupID := int64(21)
	limit := 10.0
	previousUsage := 9.5
	manualWindowStart := time.Now().Truncate(time.Second).Add(-10 * 24 * time.Hour)
	previousStartsAt := manualWindowStart.Add(-10 * 24 * time.Hour)
	previousExpiresAt := manualWindowStart.Add(90 * 24 * time.Hour)
	previousUpdatedAt := manualWindowStart.Add(-time.Hour)
	group := &Group{
		ID:              groupID,
		MonthlyLimitUSD: &limit,
	}
	repo := &advanceMonthlyCycleUserSubRepoStub{
		sub: &UserSubscription{
			ID:                 subscriptionID,
			UserID:             userID,
			GroupID:            groupID,
			Group:              group,
			Status:             SubscriptionStatusActive,
			MonthlyUsageUSD:    previousUsage,
			StartsAt:           previousStartsAt,
			MonthlyWindowStart: &manualWindowStart,
			ExpiresAt:          previousExpiresAt,
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
	mock.ExpectQuery(`(?s)SELECT monthly_usage_usd, monthly_window_start, starts_at, expires_at, status, updated_at\s+FROM user_subscriptions\s+WHERE id = \$1 AND user_id = \$2 AND deleted_at IS NULL\s+FOR UPDATE`).
		WithArgs(subscriptionID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"monthly_usage_usd",
			"monthly_window_start",
			"starts_at",
			"expires_at",
			"status",
			"updated_at",
		}).AddRow(previousUsage, manualWindowStart, previousStartsAt, previousExpiresAt, SubscriptionStatusActive, previousUpdatedAt))
	mock.ExpectExec(`(?s)UPDATE user_subscriptions\s+SET monthly_usage_usd = 0,\s+monthly_window_start = \$1,\s+expires_at = \$2,\s+updated_at = \$3\s+WHERE id = \$4 AND user_id = \$5 AND deleted_at IS NULL`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), subscriptionID, userID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO subscription_cycle_reset_logs \(\s+user_id, subscription_id, group_id, previous_expires_at, new_expires_at,\s+previous_monthly_usage_usd, previous_monthly_window_start, new_monthly_window_start,\s+deducted_days, deducted_seconds, created_at\s+\) VALUES \(\$1, \$2, \$3, \$4, \$5, \$6, \$7, \$8, \$9, \$10, NOW\(\)\)`).
		WithArgs(userID, subscriptionID, groupID, previousExpiresAt, sqlmock.AnyArg(), previousUsage, manualWindowStart, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	before := time.Now().Truncate(time.Second)
	result, err := svc.AdvanceMonthlyCycle(ctx, userID, subscriptionID)
	after := time.Now().Truncate(time.Second)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.NewMonthlyWindowStart.Before(before.Add(-time.Second)))
	require.False(t, result.NewMonthlyWindowStart.After(after.Add(time.Second)))
	require.Zero(t, result.NewMonthlyWindowStart.Nanosecond())
	expectedResetAt := advanceWindowStart(manualWindowStart, monthlyCycleDuration, before).Add(monthlyCycleDuration)
	require.True(t, expectedResetAt.Equal(result.NewMonthlyWindowStart.Add(time.Duration(result.DeductedSeconds)*time.Second)))
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
	previousStartsAt := time.Now().Add(-2 * 24 * time.Hour)
	previousWindow := time.Now().Add(12 * time.Hour)
	previousExpiresAt := time.Now().Add(75 * 24 * time.Hour)
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
	mock.ExpectQuery(`(?s)SELECT monthly_usage_usd, monthly_window_start, starts_at, expires_at, status, updated_at\s+FROM user_subscriptions\s+WHERE id = \$1 AND user_id = \$2 AND deleted_at IS NULL\s+FOR UPDATE`).
		WithArgs(subscriptionID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"monthly_usage_usd",
			"monthly_window_start",
			"starts_at",
			"expires_at",
			"status",
			"updated_at",
		}).AddRow(0.0, previousWindow, previousStartsAt, previousExpiresAt, SubscriptionStatusActive, previousUpdatedAt))
	mock.ExpectRollback()

	result, err := svc.AdvanceMonthlyCycle(ctx, userID, subscriptionID)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMonthlyCycleNotExhausted)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestAdvanceMonthlyCycleRejectsSingleMonthlyCardWithoutNextCycle(t *testing.T) {
	ctx := context.Background()
	userID := int64(7)
	subscriptionID := int64(100)
	groupID := int64(20)
	limit := 10.0
	now := time.Now()
	previousStartsAt := now.Add(-2 * 24 * time.Hour)
	previousWindow := previousStartsAt
	previousExpiresAt := previousStartsAt.Add(30 * 24 * time.Hour)
	previousUpdatedAt := now.Add(-time.Hour)
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
			MonthlyUsageUSD: limit,
			StartsAt:        previousStartsAt,
			ExpiresAt:       previousExpiresAt,
		},
	}
	client, mock := newAdvanceMonthlyCycleMockClient(t)
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, client, nil)

	mock.ExpectBegin()
	mock.ExpectQuery(`(?s)SELECT monthly_usage_usd, monthly_window_start, starts_at, expires_at, status, updated_at\s+FROM user_subscriptions\s+WHERE id = \$1 AND user_id = \$2 AND deleted_at IS NULL\s+FOR UPDATE`).
		WithArgs(subscriptionID, userID).
		WillReturnRows(sqlmock.NewRows([]string{
			"monthly_usage_usd",
			"monthly_window_start",
			"starts_at",
			"expires_at",
			"status",
			"updated_at",
		}).AddRow(limit, previousWindow, previousStartsAt, previousExpiresAt, SubscriptionStatusActive, previousUpdatedAt))
	mock.ExpectRollback()

	result, err := svc.AdvanceMonthlyCycle(ctx, userID, subscriptionID)

	require.Nil(t, result)
	require.ErrorIs(t, err, ErrMonthlyCycleNoFutureTime)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCanAdvanceMonthlyCycleByValidityRequiresFullNextCycle(t *testing.T) {
	startsAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	resetAt := startsAt.Add(30 * 24 * time.Hour)

	require.False(t, canAdvanceMonthlyCycleByValidity(startsAt, startsAt.Add(30*24*time.Hour), resetAt))
	require.False(t, canAdvanceMonthlyCycleByValidity(startsAt, resetAt.Add(29*24*time.Hour), resetAt))
	require.True(t, canAdvanceMonthlyCycleByValidity(startsAt, resetAt.Add(30*24*time.Hour-time.Second), resetAt))
	require.True(t, canAdvanceMonthlyCycleByValidity(startsAt, resetAt.Add(30*24*time.Hour), resetAt))
}

func TestCeilDurationSeconds(t *testing.T) {
	require.Equal(t, int64(0), ceilDurationSeconds(0))
	require.Equal(t, int64(1), ceilDurationSeconds(time.Nanosecond))
	require.Equal(t, int64(1), ceilDurationSeconds(time.Second))
	require.Equal(t, int64(2), ceilDurationSeconds(time.Second+time.Nanosecond))
}
