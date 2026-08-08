//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func querySingleFloat(t *testing.T, ctx context.Context, client *dbent.Client, query string, args ...any) float64 {
	t.Helper()
	rows, err := client.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next(), "expected one row")
	var value float64
	require.NoError(t, rows.Scan(&value))
	require.NoError(t, rows.Err())
	return value
}

func querySingleInt(t *testing.T, ctx context.Context, client *dbent.Client, query string, args ...any) int {
	t.Helper()
	rows, err := client.QueryContext(ctx, query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next(), "expected one row")
	var value int
	require.NoError(t, rows.Scan(&value))
	require.NoError(t, rows.Err())
	return value
}

func TestAffiliateRepository_TransferQuotaToBalance_UsesClaimedQuotaBeforeClear(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-transfer-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      5.5,
		Concurrency:  5,
	})

	affCode := fmt.Sprintf("AFF%09d", time.Now().UnixNano()%1_000_000_000)
	_, err := client.ExecContext(txCtx, `
INSERT INTO user_affiliates (user_id, aff_code, aff_quota, aff_history_quota, created_at, updated_at)
VALUES ($1, $2, $3, $3, NOW(), NOW())`, u.ID, affCode, 12.34)
	require.NoError(t, err)

	transferred, balance, err := repo.TransferQuotaToBalance(txCtx, u.ID)
	require.NoError(t, err)
	require.InDelta(t, 12.34, transferred, 1e-9)
	require.InDelta(t, 17.84, balance, 1e-9)

	affQuota := querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", u.ID)
	require.InDelta(t, 0.0, affQuota, 1e-9)

	persistedBalance := querySingleFloat(t, txCtx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", u.ID)
	require.InDelta(t, 17.84, persistedBalance, 1e-9)

	ledgerCount := querySingleInt(t, txCtx, client,
		"SELECT COUNT(*) FROM user_affiliate_ledger WHERE user_id = $1 AND action = 'transfer'", u.ID)
	require.Equal(t, 1, ledgerCount)

	rows, err := client.QueryContext(txCtx, `
SELECT amount::double precision,
       balance_after::double precision,
       aff_quota_after::double precision,
       aff_frozen_quota_after::double precision,
       aff_history_quota_after::double precision
FROM user_affiliate_ledger
WHERE user_id = $1 AND action = 'transfer'
LIMIT 1`, u.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next(), "expected transfer ledger")
	var amount, balanceAfter, quotaAfter, frozenAfter, historyAfter float64
	require.NoError(t, rows.Scan(&amount, &balanceAfter, &quotaAfter, &frozenAfter, &historyAfter))
	require.InDelta(t, 12.34, amount, 1e-9)
	require.InDelta(t, 17.84, balanceAfter, 1e-9)
	require.InDelta(t, 0.0, quotaAfter, 1e-9)
	require.InDelta(t, 0.0, frozenAfter, 1e-9)
	require.InDelta(t, 12.34, historyAfter, 1e-9)
}

// TestAffiliateRepository_AccrueQuota_ReusesOuterTransaction guards the
// cross-layer tx propagation invariant: when AccrueQuota is called with a ctx
// that already carries a transaction (via dbent.NewTxContext), repo.withTx
// must reuse that tx rather than opening a nested one. If this invariant
// breaks, AccrueQuota would commit independently and survive a rollback of
// the outer tx, which would violate payment_fulfillment's all-or-nothing
// semantics.
func TestAffiliateRepository_AccrueQuota_ReusesOuterTransaction(t *testing.T) {
	ctx := context.Background()

	outerTx, err := integrationEntClient.Tx(ctx)
	require.NoError(t, err, "begin outer tx")
	// Defensive cleanup: if any require.* below fires before the explicit
	// Rollback, this prevents the tx from leaking until container teardown.
	// Rollback is idempotent at the driver level (extra rollback returns an
	// error we ignore).
	t.Cleanup(func() { _ = outerTx.Rollback() })
	client := outerTx.Client()
	txCtx := dbent.NewTxContext(ctx, outerTx)

	inviter := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-inviter-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  5,
	})
	invitee := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-invitee-%d@example.com", time.Now().UnixNano()+1),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Concurrency:  5,
	})

	repo := NewAffiliateRepository(client, integrationDB)
	_, err = repo.EnsureUserAffiliate(txCtx, inviter.ID)
	require.NoError(t, err)
	_, err = repo.EnsureUserAffiliate(txCtx, invitee.ID)
	require.NoError(t, err)

	bound, err := repo.BindInviter(txCtx, invitee.ID, inviter.ID)
	require.NoError(t, err)
	require.True(t, bound, "invitee must bind to inviter")

	applied, err := repo.AccrueQuota(txCtx, inviter.ID, invitee.ID, 3.5, 0, nil)
	require.NoError(t, err)
	require.True(t, applied, "AccrueQuota must report applied=true")

	// Visible inside the outer tx.
	innerQuota := querySingleFloat(t, txCtx, client,
		"SELECT aff_quota::double precision FROM user_affiliates WHERE user_id = $1", inviter.ID)
	require.InDelta(t, 3.5, innerQuota, 1e-9)

	// Roll back the outer tx; if AccrueQuota had opened its own inner tx and
	// committed it, the rows would still be visible to the global client.
	require.NoError(t, outerTx.Rollback())

	rows, err := integrationEntClient.QueryContext(ctx,
		"SELECT COUNT(*) FROM user_affiliates WHERE user_id IN ($1, $2)",
		inviter.ID, invitee.ID)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	require.True(t, rows.Next())
	var postRollbackCount int
	require.NoError(t, rows.Scan(&postRollbackCount))
	require.Equal(t, 0, postRollbackCount,
		"AccrueQuota must propagate the outer tx — found persisted rows after rollback")
}

func TestAffiliateRepository_ReverseQuotaForOrderPreservesGrossAcrossCumulativeRefunds(t *testing.T) {
	fixture := newAffiliateReversalFixture(t, 20, 24)
	ctx := context.Background()

	partial, err := fixture.repo.ReverseQuotaForOrder(ctx, fixture.orderID, 0.25)
	require.NoError(t, err)
	require.InDelta(t, 20, partial.GrossAmount, 1e-9)
	require.InDelta(t, 5, partial.DeltaAmount, 1e-9)
	require.InDelta(t, 5, partial.ReversedAmount, 1e-9)
	require.InDelta(t, 15, partial.NetAmount, 1e-9)
	require.True(t, partial.Frozen)
	require.InDelta(t, 15, partial.FrozenAfter, 1e-9)

	full, err := fixture.repo.ReverseQuotaForOrder(ctx, fixture.orderID, 1)
	require.NoError(t, err)
	require.InDelta(t, 15, full.DeltaAmount, 1e-9)
	require.InDelta(t, 20, full.ReversedAmount, 1e-9)
	require.InDelta(t, 0, full.NetAmount, 1e-9)
	require.InDelta(t, 0, full.FrozenAfter, 1e-9)

	outOfOrder, err := fixture.repo.ReverseQuotaForOrder(ctx, fixture.orderID, 0.5)
	require.NoError(t, err)
	require.InDelta(t, 0, outOfOrder.DeltaAmount, 1e-9)
	require.InDelta(t, 20, outOfOrder.ReversedAmount, 1e-9)
	require.InDelta(t, 0, outOfOrder.FrozenAfter, 1e-9, "idempotent result must report the real profile state")

	var gross, reversed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT amount::double precision, reversed_amount::double precision
FROM user_affiliate_ledger
WHERE source_order_id = $1 AND action = 'accrue'`, fixture.orderID).Scan(&gross, &reversed))
	require.InDelta(t, 20, gross, 1e-9, "refunds must never mutate the gross accrual")
	require.InDelta(t, 20, reversed, 1e-9)

	var reverseTotal float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COALESCE(-SUM(amount), 0)::double precision
FROM user_affiliate_ledger
WHERE source_order_id = $1 AND action = 'refund_reverse'`, fixture.orderID).Scan(&reverseTotal))
	require.InDelta(t, reversed, reverseTotal, 1e-9, "reversal ledger must reconcile to accrue.reversed_amount")

	var frozen, history float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT aff_frozen_quota::double precision, aff_history_quota::double precision
FROM user_affiliates WHERE user_id = $1`, fixture.inviterID).Scan(&frozen, &history))
	require.InDelta(t, 0, frozen, 1e-9)
	require.InDelta(t, 20, history, 1e-9, "lifetime affiliate history remains gross")
}

func TestAffiliateRepository_ThawAndTransferUseNetAccrualWithNegativeCarry(t *testing.T) {
	fixture := newAffiliateReversalFixture(t, 10, 24)
	ctx := context.Background()

	partial, err := fixture.repo.ReverseQuotaForOrder(ctx, fixture.orderID, 0.4)
	require.NoError(t, err)
	require.InDelta(t, 4, partial.DeltaAmount, 1e-9)

	_, err = integrationDB.ExecContext(ctx, `
UPDATE user_affiliate_ledger
SET frozen_until = NOW() - INTERVAL '1 minute'
WHERE source_order_id = $1 AND action = 'accrue'`, fixture.orderID)
	require.NoError(t, err)

	thawed, err := fixture.repo.ThawFrozenQuota(ctx, fixture.inviterID)
	require.NoError(t, err)
	require.InDelta(t, 6, thawed, 1e-9, "thaw must release gross minus reversed_amount")

	transferred, balanceAfterTransfer, err := fixture.repo.TransferQuotaToBalance(ctx, fixture.inviterID)
	require.NoError(t, err)
	require.InDelta(t, 6, transferred, 1e-9)
	require.InDelta(t, 11, balanceAfterTransfer, 1e-9)

	full, err := fixture.repo.ReverseQuotaForOrder(ctx, fixture.orderID, 1)
	require.NoError(t, err)
	require.InDelta(t, 6, full.DeltaAmount, 1e-9)
	require.InDelta(t, -6, full.AvailableAfter, 1e-9, "transferred rebate becomes affiliate negative carry")

	var balance, totalRecharged float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT balance::double precision, total_recharged::double precision
FROM users WHERE id = $1`, fixture.inviterID).Scan(&balance, &totalRecharged))
	require.InDelta(t, 11, balance, 1e-9, "affiliate refund must not deduct general balance")
	require.InDelta(t, 11, totalRecharged, 1e-9, "affiliate refund must not rewrite recharge history")

	var available, frozen, history float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT aff_quota::double precision, aff_frozen_quota::double precision, aff_history_quota::double precision
FROM user_affiliates WHERE user_id = $1`, fixture.inviterID).Scan(&available, &frozen, &history))
	require.InDelta(t, -6, available, 1e-9)
	require.InDelta(t, 0, frozen, 1e-9)
	require.InDelta(t, 10, history, 1e-9)
}

func TestAffiliateRepository_AccrueQuotaCappedIsAtomic(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := testEntClient(t)
	repo := NewAffiliateRepository(client, integrationDB)
	inviter := mustCreateUser(t, client, &service.User{Email: uniqueTestEmail("affiliate-cap-inviter")})
	invitee := mustCreateUser(t, client, &service.User{Email: uniqueTestEmail("affiliate-cap-invitee")})
	t.Cleanup(func() { cleanupAffiliateTestUsers(inviter.ID, invitee.ID) })
	_, err := repo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan float64, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			applied, accrueErr := repo.AccrueQuotaCapped(ctx, inviter.ID, invitee.ID, 8, 10, 0, nil)
			results <- applied
			errs <- accrueErr
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for accrueErr := range errs {
		require.NoError(t, accrueErr)
	}
	totalApplied := 0.0
	for applied := range results {
		totalApplied += applied
	}
	require.InDelta(t, 10, totalApplied, 1e-9)

	var gross, available, history float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT COALESCE(SUM(l.amount), 0)::double precision,
       ua.aff_quota::double precision,
       ua.aff_history_quota::double precision
FROM user_affiliates ua
LEFT JOIN user_affiliate_ledger l
  ON l.user_id = ua.user_id AND l.source_user_id = $2 AND l.action = 'accrue'
WHERE ua.user_id = $1
GROUP BY ua.aff_quota, ua.aff_history_quota`, inviter.ID, invitee.ID).Scan(&gross, &available, &history))
	require.InDelta(t, 10, gross, 1e-9)
	require.InDelta(t, 10, available, 1e-9)
	require.InDelta(t, 10, history, 1e-9)
}

func TestAffiliateRepository_ReversalAndTransferUseCompatibleLockOrder(t *testing.T) {
	fixture := newAffiliateReversalFixture(t, 10, 24)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := integrationDB.ExecContext(ctx, `
UPDATE user_affiliate_ledger
SET frozen_until = NOW() - INTERVAL '1 minute'
WHERE source_order_id = $1 AND action = 'accrue'`, fixture.orderID)
	require.NoError(t, err)

	start := make(chan struct{})
	reversalDone := make(chan error, 1)
	transferDone := make(chan error, 1)
	go func() {
		<-start
		_, reverseErr := fixture.repo.ReverseQuotaForOrder(ctx, fixture.orderID, 1)
		reversalDone <- reverseErr
	}()
	go func() {
		<-start
		_, _, transferErr := fixture.repo.TransferQuotaToBalance(ctx, fixture.inviterID)
		if transferErr == service.ErrAffiliateQuotaEmpty {
			transferErr = nil
		}
		transferDone <- transferErr
	}()
	close(start)
	require.NoError(t, <-reversalDone)
	require.NoError(t, <-transferDone)

	var balance, available, gross, reversed float64
	require.NoError(t, integrationDB.QueryRowContext(ctx, `
SELECT u.balance::double precision,
       ua.aff_quota::double precision,
       l.amount::double precision,
       l.reversed_amount::double precision
FROM users u
JOIN user_affiliates ua ON ua.user_id = u.id
JOIN user_affiliate_ledger l ON l.user_id = u.id
WHERE u.id = $1 AND l.source_order_id = $2 AND l.action = 'accrue'`, fixture.inviterID, fixture.orderID).
		Scan(&balance, &available, &gross, &reversed))
	require.InDelta(t, 10, gross, 1e-9)
	require.InDelta(t, 10, reversed, 1e-9)
	require.InDelta(t, 0, (balance-5)+available, 1e-9,
		"whether transfer or reversal wins, transferred value must be offset by affiliate negative carry")
}

type affiliateReversalFixture struct {
	repo      service.AffiliateRepository
	inviterID int64
	inviteeID int64
	orderID   int64
}

func newAffiliateReversalFixture(t *testing.T, amount float64, freezeHours int) affiliateReversalFixture {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	inviter := mustCreateUser(t, client, &service.User{
		Email:   uniqueTestEmail("affiliate-reversal-inviter"),
		Balance: 5,
	})
	invitee := mustCreateUser(t, client, &service.User{Email: uniqueTestEmail("affiliate-reversal-invitee")})
	_, err := client.User.UpdateOneID(inviter.ID).SetTotalRecharged(5).Save(ctx)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Microsecond)
	order, err := client.PaymentOrder.Create().
		SetUserID(invitee.ID).
		SetUserEmail(invitee.Email).
		SetUserName("affiliate-reversal-invitee").
		SetAmount(100).
		SetPayAmount(100).
		SetFeeRate(0).
		SetRechargeCode(uniqueTestName("affiliate-reversal-code")).
		SetOutTradeNo(uniqueTestName("affiliate-reversal-order")).
		SetPaymentType(payment.TypeStripe).
		SetPaymentTradeNo(uniqueTestName("affiliate-reversal-trade")).
		SetOrderType(payment.OrderTypeBalance).
		SetStatus(service.OrderStatusCompleted).
		SetPaidAt(now).
		SetCompletedAt(now).
		SetExpiresAt(now.Add(time.Hour)).
		SetClientIP("127.0.0.1").
		SetSrcHost("integration.test").
		Save(ctx)
	require.NoError(t, err)

	repo := NewAffiliateRepository(client, integrationDB)
	_, err = repo.EnsureUserAffiliate(ctx, inviter.ID)
	require.NoError(t, err)
	sourceOrderID := order.ID
	applied, err := repo.AccrueQuotaCapped(ctx, inviter.ID, invitee.ID, amount, 0, freezeHours, &sourceOrderID)
	require.NoError(t, err)
	require.InDelta(t, amount, applied, 1e-9)

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM user_affiliate_ledger WHERE user_id = $1 OR source_user_id = $2", inviter.ID, invitee.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM user_affiliates WHERE user_id IN ($1, $2)", inviter.ID, invitee.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM payment_audit_logs WHERE order_id = $1", fmt.Sprintf("%d", order.ID))
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM payment_orders WHERE id = $1", order.ID)
		_, _ = integrationDB.ExecContext(cleanupCtx, "DELETE FROM users WHERE id IN ($1, $2)", inviter.ID, invitee.ID)
	})
	return affiliateReversalFixture{
		repo:      repo,
		inviterID: inviter.ID,
		inviteeID: invitee.ID,
		orderID:   order.ID,
	}
}

func cleanupAffiliateTestUsers(userIDs ...int64) {
	if len(userIDs) != 2 {
		return
	}
	ctx := context.Background()
	_, _ = integrationDB.ExecContext(ctx, "DELETE FROM user_affiliate_ledger WHERE user_id IN ($1, $2) OR source_user_id IN ($1, $2)", userIDs[0], userIDs[1])
	_, _ = integrationDB.ExecContext(ctx, "DELETE FROM user_affiliates WHERE user_id IN ($1, $2)", userIDs[0], userIDs[1])
	_, _ = integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id IN ($1, $2)", userIDs[0], userIDs[1])
}

func TestAffiliateRepository_TransferQuotaToBalance_EmptyQuota(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-empty-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
		Balance:      3.21,
		Concurrency:  5,
	})

	affCode := fmt.Sprintf("AFF%09d", time.Now().UnixNano()%1_000_000_000)
	_, err := client.ExecContext(txCtx, `
INSERT INTO user_affiliates (user_id, aff_code, aff_quota, aff_history_quota, created_at, updated_at)
VALUES ($1, $2, 0, 0, NOW(), NOW())`, u.ID, affCode)
	require.NoError(t, err)

	transferred, balance, err := repo.TransferQuotaToBalance(txCtx, u.ID)
	require.ErrorIs(t, err, service.ErrAffiliateQuotaEmpty)
	require.InDelta(t, 0.0, transferred, 1e-9)
	require.InDelta(t, 0.0, balance, 1e-9)

	persistedBalance := querySingleFloat(t, txCtx, client,
		"SELECT balance::double precision FROM users WHERE id = $1", u.ID)
	require.InDelta(t, 3.21, persistedBalance, 1e-9)
}

// TestAffiliateRepository_AdminCustomCode covers the success path of admin
// invite-code rewrite + reset within a shared test transaction:
// - UpdateUserAffCode replaces aff_code, sets aff_code_custom=true, lookup works
// - the old code can no longer be found
// - ResetUserAffCode reverts aff_code_custom and assigns a new system-format code
//
// The conflict path (duplicate code → ErrAffiliateCodeTaken) lives in its own
// test because a unique-violation aborts the surrounding Postgres tx, which
// would poison subsequent assertions in the same transaction.
func TestAffiliateRepository_AdminCustomCode(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-custom-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})

	original, err := repo.EnsureUserAffiliate(txCtx, u.ID)
	require.NoError(t, err)
	require.False(t, original.AffCodeCustom, "system-generated codes start as non-custom")
	originalCode := original.AffCode

	// Rewrite to a custom code
	customCode := fmt.Sprintf("VIP%09d", time.Now().UnixNano()%1_000_000_000)
	require.NoError(t, repo.UpdateUserAffCode(txCtx, u.ID, customCode))

	updated, err := repo.EnsureUserAffiliate(txCtx, u.ID)
	require.NoError(t, err)
	require.Equal(t, customCode, updated.AffCode)
	require.True(t, updated.AffCodeCustom)

	// Lookup by new custom code finds the user
	byCode, err := repo.GetAffiliateByCode(txCtx, customCode)
	require.NoError(t, err)
	require.Equal(t, u.ID, byCode.UserID)

	// Old system code should no longer match
	_, err = repo.GetAffiliateByCode(txCtx, originalCode)
	require.ErrorIs(t, err, service.ErrAffiliateProfileNotFound)

	// Reset back to a fresh system code, clears custom flag
	newSysCode, err := repo.ResetUserAffCode(txCtx, u.ID)
	require.NoError(t, err)
	require.NotEqual(t, customCode, newSysCode)

	reset, err := repo.EnsureUserAffiliate(txCtx, u.ID)
	require.NoError(t, err)
	require.Equal(t, newSysCode, reset.AffCode)
	require.False(t, reset.AffCodeCustom)

	// The old custom code is now free again
	_, err = repo.GetAffiliateByCode(txCtx, customCode)
	require.ErrorIs(t, err, service.ErrAffiliateProfileNotFound)
}

// TestAffiliateRepository_AdminCustomCode_Conflict isolates the unique-violation
// path. PostgreSQL aborts the enclosing tx when a unique constraint fires, so
// this test must be the only assertion and run in its own tx — production
// callers each have their own outer tx, so this matches real behavior.
func TestAffiliateRepository_AdminCustomCode_Conflict(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	taker := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-conflict-taker-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})
	requester := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-conflict-req-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})

	takenCode := fmt.Sprintf("HOT%09d", time.Now().UnixNano()%1_000_000_000)
	require.NoError(t, repo.UpdateUserAffCode(txCtx, taker.ID, takenCode))

	// Now requester tries to grab the same code → conflict.
	err := repo.UpdateUserAffCode(txCtx, requester.ID, takenCode)
	require.ErrorIs(t, err, service.ErrAffiliateCodeTaken)
}

// TestAffiliateRepository_AdminRebateRate covers per-user exclusive rate
// set/clear and the Batch variant including NULL semantics.
func TestAffiliateRepository_AdminRebateRate(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	u1 := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-rate-%d-a@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})
	u2 := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-rate-%d-b@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser,
		Status:       service.StatusActive,
	})

	// Set exclusive rate for u1
	rate := 42.5
	require.NoError(t, repo.SetUserRebateRate(txCtx, u1.ID, &rate))

	got, err := repo.EnsureUserAffiliate(txCtx, u1.ID)
	require.NoError(t, err)
	require.NotNil(t, got.AffRebateRatePercent)
	require.InDelta(t, 42.5, *got.AffRebateRatePercent, 1e-9)

	// Clear exclusive rate
	require.NoError(t, repo.SetUserRebateRate(txCtx, u1.ID, nil))
	cleared, err := repo.EnsureUserAffiliate(txCtx, u1.ID)
	require.NoError(t, err)
	require.Nil(t, cleared.AffRebateRatePercent)

	// Batch set both users
	batchRate := 15.0
	require.NoError(t, repo.BatchSetUserRebateRate(txCtx, []int64{u1.ID, u2.ID}, &batchRate))

	for _, uid := range []int64{u1.ID, u2.ID} {
		v, err := repo.EnsureUserAffiliate(txCtx, uid)
		require.NoError(t, err)
		require.NotNil(t, v.AffRebateRatePercent)
		require.InDelta(t, 15.0, *v.AffRebateRatePercent, 1e-9)
	}

	// Batch clear
	require.NoError(t, repo.BatchSetUserRebateRate(txCtx, []int64{u1.ID, u2.ID}, nil))
	for _, uid := range []int64{u1.ID, u2.ID} {
		v, err := repo.EnsureUserAffiliate(txCtx, uid)
		require.NoError(t, err)
		require.Nil(t, v.AffRebateRatePercent)
	}
}

// TestAffiliateRepository_ListUsersWithCustomSettings verifies the admin list
// only includes users with at least one override applied.
func TestAffiliateRepository_ListUsersWithCustomSettings(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	txCtx := dbent.NewTxContext(ctx, tx)
	client := tx.Client()

	repo := NewAffiliateRepository(client, integrationDB)

	// User without any custom config — should NOT appear in the list.
	plainEmail := fmt.Sprintf("affiliate-plain-%d@example.com", time.Now().UnixNano())
	uPlain := mustCreateUser(t, client, &service.User{
		Email: plainEmail, PasswordHash: "hash",
		Role: service.RoleUser, Status: service.StatusActive,
	})
	_, err := repo.EnsureUserAffiliate(txCtx, uPlain.ID)
	require.NoError(t, err)

	// User with a custom code — should appear.
	uCode := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-codeonly-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})
	require.NoError(t, repo.UpdateUserAffCode(txCtx, uCode.ID, fmt.Sprintf("VIP%09d", time.Now().UnixNano()%1_000_000_000)))

	// User with only an exclusive rate — should appear.
	uRate := mustCreateUser(t, client, &service.User{
		Email:        fmt.Sprintf("affiliate-rateonly-%d@example.com", time.Now().UnixNano()),
		PasswordHash: "hash",
		Role:         service.RoleUser, Status: service.StatusActive,
	})
	r := 33.3
	require.NoError(t, repo.SetUserRebateRate(txCtx, uRate.ID, &r))

	entries, total, err := repo.ListUsersWithCustomSettings(txCtx, service.AffiliateAdminFilter{
		Page: 1, PageSize: 100,
	})
	require.NoError(t, err)

	// Build a quick lookup to assert per-user attributes (other tests may have
	// inserted custom rows in the same DB; we only care about our 3).
	byUserID := make(map[int64]service.AffiliateAdminEntry, len(entries))
	for _, e := range entries {
		byUserID[e.UserID] = e
	}

	require.NotContains(t, byUserID, uPlain.ID, "users without overrides must not appear")

	codeEntry, ok := byUserID[uCode.ID]
	require.True(t, ok, "custom-code user missing from list")
	require.True(t, codeEntry.AffCodeCustom)
	require.Nil(t, codeEntry.AffRebateRatePercent)

	rateEntry, ok := byUserID[uRate.ID]
	require.True(t, ok, "custom-rate user missing from list")
	require.False(t, rateEntry.AffCodeCustom)
	require.NotNil(t, rateEntry.AffRebateRatePercent)
	require.InDelta(t, 33.3, *rateEntry.AffRebateRatePercent, 1e-9)

	require.GreaterOrEqual(t, total, int64(2), "total must include at least our 2 custom rows")
}
