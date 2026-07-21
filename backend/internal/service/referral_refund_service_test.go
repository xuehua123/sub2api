//go:build unit

package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReferralRefundService_RefundsPendingReward(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID:              10,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-1",
		PaidAmount:      100,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
			},
		},
	}

	svc := NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil)
	order, rewards, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{
		RechargeOrderID: 10,
		RefundedAmount:  100,
	})
	require.NoError(t, err)
	require.Equal(t, RechargeOrderStatusRefunded, order.Status)
	require.Len(t, rewards, 1)
	require.Equal(t, CommissionRewardStatusReversed, rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionLedgerBucketPending, commissionRepo.ledgers[0].Bucket)
	require.Equal(t, -10.0, commissionRepo.ledgers[0].Amount)
}

func TestReferralRefundService_CreatesNegativeCarryForPaidReward(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-2"] = &RechargeOrder{
		ID:              11,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-2",
		PaidAmount:      100,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              2,
				UserID:          200,
				RechargeOrderID: 11,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPaid,
			},
		},
	}

	svc := NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil)
	_, rewards, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{
		RechargeOrderID: 11,
		RefundedAmount:  50,
	})
	require.NoError(t, err)
	require.Len(t, rewards, 1)
	require.Equal(t, CommissionRewardStatusPartiallyReversed, rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionLedgerEntryNegativeCarry, commissionRepo.ledgers[0].EntryType)
	require.Equal(t, CommissionLedgerBucketAvailable, commissionRepo.ledgers[0].Bucket)
	require.Equal(t, -5.0, commissionRepo.ledgers[0].Amount)
}

func TestReferralRefundService_ReversesMixedAvailableAndFrozenBuckets(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-3"] = &RechargeOrder{
		ID:              12,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-3",
		PaidAmount:      100,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	rewardID := int64(3)
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              rewardID,
				UserID:          200,
				RechargeOrderID: 12,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPartiallyFrozen,
			},
		},
		ledgers: []CommissionLedger{
			{
				UserID:          200,
				RewardID:        int64ValuePtr(rewardID),
				RechargeOrderID: int64ValuePtr(12),
				EntryType:       CommissionLedgerEntryRewardPendingToAvailable,
				Bucket:          CommissionLedgerBucketAvailable,
				Amount:          6,
				Currency:        ReferralSettlementCurrencyCNY,
			},
			{
				UserID:          200,
				RewardID:        int64ValuePtr(rewardID),
				RechargeOrderID: int64ValuePtr(12),
				EntryType:       CommissionLedgerEntryWithdrawFreeze,
				Bucket:          CommissionLedgerBucketFrozen,
				Amount:          4,
				Currency:        ReferralSettlementCurrencyCNY,
			},
		},
	}

	svc := NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil)
	_, rewards, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{
		RechargeOrderID: 12,
		RefundedAmount:  100,
	})
	require.NoError(t, err)
	require.Len(t, rewards, 1)
	require.Equal(t, CommissionRewardStatusReversed, rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 4)
	require.Equal(t, CommissionLedgerBucketAvailable, commissionRepo.ledgers[2].Bucket)
	require.Equal(t, -6.0, commissionRepo.ledgers[2].Amount)
	require.Equal(t, CommissionLedgerBucketFrozen, commissionRepo.ledgers[3].Bucket)
	require.Equal(t, -4.0, commissionRepo.ledgers[3].Amount)
}

// lockOrderRecordingRepo records lock acquisition sequence for a single-threaded
// refund path assertion (reward FOR UPDATE before ledger FOR UPDATE).
type lockOrderRecordingRepo struct {
	commissionRepoStub
	mu     sync.Mutex
	events []string
}

func (r *lockOrderRecordingRepo) ListRewardsByRechargeOrderForUpdate(ctx context.Context, rechargeOrderID int64) ([]CommissionReward, error) {
	r.mu.Lock()
	r.events = append(r.events, "reward_for_update")
	r.mu.Unlock()
	return r.commissionRepoStub.ListRewardsByRechargeOrderForUpdate(ctx, rechargeOrderID)
}

func (r *lockOrderRecordingRepo) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	if forUpdate {
		r.mu.Lock()
		r.events = append(r.events, "ledger_for_update")
		r.mu.Unlock()
	}
	return r.commissionRepoStub.SumRewardBucketAmountForUpdate(ctx, rewardID, bucket, forUpdate)
}

func TestReferralRefundService_LocksRewardsBeforeLedgers(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-lock"] = &RechargeOrder{
		ID: 20, UserID: 100, Provider: "provider", ExternalOrderID: "order-lock",
		PaidAmount: 100, Currency: ReferralSettlementCurrencyCNY, Status: RechargeOrderStatusCredited,
	}
	repo := &lockOrderRecordingRepo{
		commissionRepoStub: commissionRepoStub{
			rewards: []CommissionReward{
				{ID: 1, UserID: 200, RechargeOrderID: 20, RewardAmount: 10, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending},
			},
			ledgers: []CommissionLedger{
				{RewardID: int64ValuePtr(1), Bucket: CommissionLedgerBucketPending, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
			},
		},
	}
	svc := NewReferralRefundService(rechargeRepo, repo, nil, nil)
	_, _, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{RechargeOrderID: 20, RefundedAmount: 100})
	require.NoError(t, err)

	require.NotEmpty(t, repo.events)
	require.Equal(t, "reward_for_update", repo.events[0], "must lock rewards before any ledger lock")
	var sawReward, sawLedger bool
	for _, e := range repo.events {
		switch e {
		case "reward_for_update":
			sawReward = true
		case "ledger_for_update":
			require.True(t, sawReward, "ledger FOR UPDATE without prior reward FOR UPDATE")
			sawLedger = true
		}
	}
	require.True(t, sawLedger, "expected ledger lock during reverse")
}

// sessionLockKey carries a synthetic transaction id for concurrent lock-order tests.
type sessionLockKey struct{}

func withLockSession(ctx context.Context, session string) context.Context {
	return context.WithValue(ctx, sessionLockKey{}, session)
}

func lockSessionID(ctx context.Context) string {
	if v, ok := ctx.Value(sessionLockKey{}).(string); ok && v != "" {
		return v
	}
	return "default"
}

// sessionLockRepo simulates PostgreSQL row locks and detects reverse lock order
// (ledger before reward) that would deadlock against settlement (reward → ledger).
type sessionLockRepo struct {
	commissionRepoStub
	mu           sync.Mutex
	cond         *sync.Cond
	rewardOwner  map[int64]string
	ledgerOwner  map[int64]string
	orderViolations atomic.Int32
	// optional barrier: after settlement takes reward lock, notify waiters before ledger.
	afterRewardHook func(session string, rewardID int64)
}

func newSessionLockRepo(base commissionRepoStub) *sessionLockRepo {
	r := &sessionLockRepo{
		commissionRepoStub: base,
		rewardOwner:        map[int64]string{},
		ledgerOwner:        map[int64]string{},
	}
	r.cond = sync.NewCond(&r.mu)
	return r
}

func (r *sessionLockRepo) acquireReward(ctx context.Context, rewardID int64) {
	sid := lockSessionID(ctx)
	r.mu.Lock()
	for {
		owner := r.rewardOwner[rewardID]
		if owner == "" || owner == sid {
			r.rewardOwner[rewardID] = sid
			r.mu.Unlock()
			if r.afterRewardHook != nil {
				r.afterRewardHook(sid, rewardID)
			}
			return
		}
		r.cond.Wait()
	}
}

func (r *sessionLockRepo) acquireLedger(ctx context.Context, rewardID int64) {
	sid := lockSessionID(ctx)
	r.mu.Lock()
	// Enforce reward → ledger order: ledger may only be locked by a session that
	// already holds the reward row lock (matches production FOR UPDATE sequence).
	if r.rewardOwner[rewardID] != sid {
		r.orderViolations.Add(1)
		r.mu.Unlock()
		return
	}
	for {
		owner := r.ledgerOwner[rewardID]
		if owner == "" || owner == sid {
			r.ledgerOwner[rewardID] = sid
			r.mu.Unlock()
			return
		}
		r.cond.Wait()
	}
}

func (r *sessionLockRepo) ReleaseSession(ctx context.Context) {
	sid := lockSessionID(ctx)
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, owner := range r.rewardOwner {
		if owner == sid {
			delete(r.rewardOwner, id)
		}
	}
	for id, owner := range r.ledgerOwner {
		if owner == sid {
			delete(r.ledgerOwner, id)
		}
	}
	r.cond.Broadcast()
}

func (r *sessionLockRepo) ListRewardsByRechargeOrderForUpdate(ctx context.Context, rechargeOrderID int64) ([]CommissionReward, error) {
	rewards, err := r.commissionRepoStub.ListRewardsByRechargeOrder(ctx, rechargeOrderID)
	if err != nil {
		return nil, err
	}
	// id ascending (matches production listRewardsByRechargeOrder)
	for i := 0; i < len(rewards); i++ {
		for j := i + 1; j < len(rewards); j++ {
			if rewards[j].ID < rewards[i].ID {
				rewards[i], rewards[j] = rewards[j], rewards[i]
			}
		}
	}
	for _, reward := range rewards {
		r.acquireReward(ctx, reward.ID)
	}
	return rewards, nil
}

func (r *sessionLockRepo) ListPendingRewardsReady(ctx context.Context, readyAt time.Time, afterAvailableAt *time.Time, afterID int64, limit int) ([]CommissionReward, error) {
	rewards, err := r.commissionRepoStub.ListPendingRewardsReady(ctx, readyAt, afterAvailableAt, afterID, limit)
	if err != nil {
		return nil, err
	}
	for _, reward := range rewards {
		r.acquireReward(ctx, reward.ID)
	}
	return rewards, nil
}

func (r *sessionLockRepo) SumRewardBucketAmountForUpdate(ctx context.Context, rewardID int64, bucket string, forUpdate bool) (float64, error) {
	if forUpdate {
		r.acquireLedger(ctx, rewardID)
	}
	return r.commissionRepoStub.SumRewardBucketAmountForUpdate(ctx, rewardID, bucket, forUpdate)
}

// Concurrent settlement + refund on the same reward must serialize on the reward
// row lock (reward → ledger). Neither path may lock ledgers first.
func TestReferralRefundAndSettlement_ConcurrentSameReward_UsesRewardThenLedgerOrder(t *testing.T) {
	now := time.Now()
	base := commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID: 1, UserID: 200, RechargeOrderID: 30, RewardAmount: 10,
				Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending,
				AvailableAt: timeValuePtr(now.Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{RewardID: int64ValuePtr(1), Bucket: CommissionLedgerBucketPending, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
		},
	}
	repo := newSessionLockRepo(base)
	// Let settlement hold the reward lock until refund has started waiting, then continue.
	var refundWaiting sync.WaitGroup
	refundWaiting.Add(1)
	settlementEnteredReward := make(chan struct{})
	var once sync.Once
	repo.afterRewardHook = func(session string, _ int64) {
		if session == "settlement" {
			once.Do(func() { close(settlementEnteredReward) })
			// Wait until refund is contending (or short timeout) so both run overlapped.
			done := make(chan struct{})
			go func() {
				refundWaiting.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(50 * time.Millisecond):
			}
		}
	}

	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["p::30"] = &RechargeOrder{
		ID: 30, UserID: 100, Provider: "p", ExternalOrderID: "30",
		PaidAmount: 100, Currency: ReferralSettlementCurrencyCNY, Status: RechargeOrderStatusCredited,
	}

	settlement := NewReferralSettlementService(repo, rechargeRepo, nil, nil)
	refund := NewReferralRefundService(rechargeRepo, repo, nil, nil)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := withLockSession(context.Background(), "settlement")
		defer repo.ReleaseSession(ctx)
		_, err := settlement.SettlePendingRewards(ctx, now)
		errCh <- err
	}()

	// Start refund after settlement has acquired the reward lock.
	select {
	case <-settlementEnteredReward:
	case <-time.After(2 * time.Second):
		t.Fatal("settlement never acquired reward lock")
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx := withLockSession(context.Background(), "refund")
		defer repo.ReleaseSession(ctx)
		refundWaiting.Done()
		_, _, err := refund.ApplyRefund(ctx, &RechargeRefundInput{RechargeOrderID: 30, RefundedAmount: 100})
		errCh <- err
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("settlement+refund deadlocked (likely reverse lock order)")
	}

	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
	require.Zero(t, repo.orderViolations.Load(), "ledger locked without holding reward (order inversion)")
}
