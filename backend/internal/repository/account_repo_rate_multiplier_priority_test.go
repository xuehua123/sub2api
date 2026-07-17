package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUpdateRateMultiplierPrioritiesWritesSortedActiveSchedulableAccountsAndGroupBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)WITH priority_updates.*UPDATE account_groups.*UPDATE accounts`).
		WithArgs(int64(3), 1, int64(9), 20).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: []int64{3, 9}}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	updated, err := repo.UpdateRateMultiplierPriorities(context.Background(), map[int64]int{9: 20, 3: 1})

	require.NoError(t, err)
	require.EqualValues(t, 2, updated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateRateMultipliersWritesSortedActiveSchedulableAccounts(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(regexp.QuoteMeta("UPDATE accounts SET rate_multiplier = CASE id WHEN $1 THEN $2 WHEN $3 THEN $4 ELSE rate_multiplier END, updated_at = NOW() WHERE id = ANY($5) AND deleted_at IS NULL AND status = 'active' AND schedulable = TRUE AND rate_multiplier IS DISTINCT FROM CASE id WHEN $1 THEN $2 WHEN $3 THEN $4 ELSE rate_multiplier END")).
		WithArgs(int64(3), 0.5, int64(9), 1.25, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO scheduler_outbox (event_type, account_id, group_id, payload)")).
		WithArgs(service.SchedulerOutboxEventAccountBulkChanged, nil, nil, accountIDsPayloadMatcher{want: []int64{3, 9}}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	updated, err := repo.UpdateRateMultipliers(context.Background(), map[int64]float64{9: 1.25, 3: 0.5})

	require.NoError(t, err)
	require.EqualValues(t, 2, updated)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUpstreamManagementAuthsIfUnchangedUpdatesCredentialFamilyAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)WITH requested AS.*matched_count.*UPDATE accounts AS a.*INSERT INTO scheduler_outbox`).
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), service.UpstreamManagementAuthCredentialKey, 2, service.SchedulerOutboxEventAccountChanged).
		WillReturnResult(sqlmock.NewResult(1, 2))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	updated, err := repo.UpdateUpstreamManagementAuthsIfUnchanged(context.Background(), []service.UpstreamManagementAuthUpdate{
		{AccountID: 9, ExpectedCiphertext: "expected-nine", NextCiphertext: "next-shared"},
		{AccountID: 3, ExpectedCiphertext: "expected-three", NextCiphertext: "next-shared"},
	})

	require.NoError(t, err)
	require.True(t, updated)
	require.NoError(t, mock.ExpectationsWereMet())
}
