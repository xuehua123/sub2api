//go:build unit

package repository

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetUpstreamAccountUsageBucketsAggregatesAccountsInOneQuery(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, time.July, 19, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	end := start.Add(12 * time.Hour)
	bucket := start.Add(9 * time.Hour)

	mock.ExpectQuery(regexp.QuoteMeta("WHERE account_id = ANY($1)")).
		WithArgs(sqlmock.AnyArg(), start, end, "Asia/Shanghai").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "bucket_start", "requests", "tokens", "account_cost", "standard_cost", "user_cost",
		}).AddRow(int64(11), bucket, int64(4), int64(500), 2.5, 2.0, 3.0))

	rows, err := repo.GetUpstreamAccountUsageBuckets(
		context.Background(), []int64{11, 11, 0, -2}, start, end, "Asia/Shanghai",
	)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(11), rows[0].AccountID)
	require.Equal(t, bucket, rows[0].Bucket)
	require.Equal(t, int64(4), rows[0].Requests)
	require.Equal(t, int64(500), rows[0].Tokens)
	require.InDelta(t, 2.5, rows[0].AccountCost, 0.000001)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUpstreamAccountUsageBucketsSkipsQueryWithoutAccounts(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}

	rows, err := repo.GetUpstreamAccountUsageBuckets(
		context.Background(), []int64{0, -1}, time.Now(), time.Now(), "Asia/Shanghai",
	)

	require.NoError(t, err)
	require.Empty(t, rows)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestGetUpstreamConnectionRuntimeGroupsCombinesUsageAndErrors(t *testing.T) {
	db, mock := newSQLMock(t)
	repo := &usageLogRepository{sql: db}
	start := time.Date(2026, time.July, 21, 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	end := start.Add(10 * time.Hour)
	fiveMinutesAgo := end.Add(-5 * time.Minute)

	mock.ExpectQuery(regexp.QuoteMeta("FROM ops_error_logs o")).
		WithArgs(sqlmock.AnyArg(), start, end, fiveMinutesAgo).
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "group_id", "group_name", "today_requests", "today_tokens", "today_account_cost", "today_standard_cost", "today_user_cost",
			"five_minute_requests", "five_minute_success_count", "five_minute_error_count",
		}).AddRow(int64(11), int64(7), "VIP", int64(9), int64(800), 2.5, 2.0, 3.0, int64(4), int64(3), int64(1)))

	rows, err := repo.GetUpstreamConnectionRuntimeGroups(context.Background(), []int64{11, 11, 0}, start, end, fiveMinutesAgo)

	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, int64(11), rows[0].AccountID)
	require.Equal(t, int64(7), rows[0].GroupID)
	require.Equal(t, "VIP", rows[0].GroupName)
	require.Equal(t, int64(9), rows[0].Today.Requests)
	require.Equal(t, int64(800), rows[0].Today.Tokens)
	require.InDelta(t, 2.5, rows[0].Today.AccountCost, 0.000001)
	require.Equal(t, int64(4), rows[0].FiveMinuteRequests)
	require.Equal(t, int64(3), rows[0].FiveMinuteSuccessCount)
	require.Equal(t, int64(1), rows[0].FiveMinuteErrorCount)
	require.NoError(t, mock.ExpectationsWereMet())
}
