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

// countingSettlementRepo observes how many times the settle job body listed ready rewards.
type countingSettlementRepo struct {
	commissionRepoStub
	listCalls atomic.Int32
}

func (s *countingSettlementRepo) ListPendingRewardsReady(ctx context.Context, readyAt time.Time, afterAvailableAt *time.Time, afterID int64, limit int) ([]CommissionReward, error) {
	s.listCalls.Add(1)
	return s.commissionRepoStub.ListPendingRewardsReady(ctx, readyAt, afterAvailableAt, afterID, limit)
}

// blockingSettlementRepo blocks inside ListPendingRewardsReady so tests can hold
// the leader lock across a simulated in-flight settle (and observe ctx cancel).
type blockingSettlementRepo struct {
	commissionRepoStub
	listCalls atomic.Int32

	// entered is signaled once List is entered (buffered so non-blocking send is safe).
	entered chan struct{}
	// release unblocks a held List call when closed/signaled (optional).
	release chan struct{}
	// blockUntilCtxCancel waits on ctx.Done() instead of release.
	blockUntilCtxCancel bool
}

func (s *blockingSettlementRepo) ListPendingRewardsReady(ctx context.Context, readyAt time.Time, afterAvailableAt *time.Time, afterID int64, limit int) ([]CommissionReward, error) {
	s.listCalls.Add(1)
	if s.entered != nil {
		select {
		case s.entered <- struct{}{}:
		default:
		}
	}
	if s.blockUntilCtxCancel {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return s.commissionRepoStub.ListPendingRewardsReady(ctx, readyAt, afterAvailableAt, afterID, limit)
}

func newRunnerTestSettlement(repo CommissionRepository) *ReferralSettlementService {
	return NewReferralSettlementService(repo, newRechargeOrderRepoStub(), nil, nil)
}

func TestReferralSettlementRunner_SkipsWhenLeaderLockHeld(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	acquired, err := cache.TryAcquireLeaderLock(context.Background(), referralSettlementRunnerLeaderLockKey, "peer", time.Minute)
	require.NoError(t, err)
	require.True(t, acquired)

	repo := &countingSettlementRepo{}
	runner := newReferralSettlementRunner(newRunnerTestSettlement(repo), cache, nil)
	runner.runOnce()
	require.Zero(t, repo.listCalls.Load(), "non-leader must not scan ready rewards")
}

func TestReferralSettlementRunner_LeaderSettlesOncePerCycle(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	repo := &countingSettlementRepo{}
	runner := newReferralSettlementRunner(newRunnerTestSettlement(repo), cache, nil)

	runner.runOnce()
	runner.runOnce()
	require.Equal(t, int32(2), repo.listCalls.Load(), "each cycle should settle once after releasing the lock")
}

// Concurrent multi-instance: A holds the leader lock mid-settle while B races runOnce.
// Only A may enter ListPendingRewardsReady.
func TestReferralSettlementRunner_ConcurrentPeersOnlyLeaderSettles(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	repoA := &blockingSettlementRepo{
		entered: make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	repoB := &countingSettlementRepo{}
	runnerA := newReferralSettlementRunner(newRunnerTestSettlement(repoA), cache, nil)
	runnerB := newReferralSettlementRunner(newRunnerTestSettlement(repoB), cache, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runnerA.runOnce()
	}()

	select {
	case <-repoA.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("leader never entered settle body")
	}

	// B contends while A still holds the leader lock inside settle.
	runnerB.runOnce()
	require.Zero(t, repoB.listCalls.Load(), "peer must not settle while leader lock is held")
	require.NotEmpty(t, cache.heldBy(referralSettlementRunnerLeaderLockKey), "leader lock must still be held mid-settle")

	close(repoA.release)
	wg.Wait()
	require.Equal(t, int32(1), repoA.listCalls.Load())
	require.Empty(t, cache.heldBy(referralSettlementRunnerLeaderLockKey), "lock released after settle")

	// After release, B can become leader.
	runnerB.runOnce()
	require.Equal(t, int32(1), repoB.listCalls.Load())
}

func TestReferralSettlementRunner_StopExitsPromptly(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	repo := &countingSettlementRepo{}
	runner := newReferralSettlementRunner(newRunnerTestSettlement(repo), cache, nil)
	runner.interval = 50 * time.Millisecond

	runner.Start()
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		runner.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() did not return within 2s")
	}

	callsAfterStop := repo.listCalls.Load()
	time.Sleep(120 * time.Millisecond)
	require.Equal(t, callsAfterStop, repo.listCalls.Load(), "no further settle after Stop")
}

// Job timeout must cancel an in-flight List (simulating a stuck DB settle) and
// release the leader lock so the next cycle can run.
func TestReferralSettlementRunner_JobTimeoutCancelsInFlightSettleAndReleasesLock(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	repo := &blockingSettlementRepo{
		entered:             make(chan struct{}, 1),
		blockUntilCtxCancel: true,
	}
	runner := newReferralSettlementRunner(newRunnerTestSettlement(repo), cache, nil)
	runner.jobTimeout = 40 * time.Millisecond

	start := time.Now()
	runner.runOnce()
	elapsed := time.Since(start)

	require.GreaterOrEqual(t, elapsed, 40*time.Millisecond, "should wait for job timeout")
	require.Less(t, elapsed, 2*time.Second, "must not hang after ctx cancel")
	require.Equal(t, int32(1), repo.listCalls.Load(), "settle body entered once")
	require.Empty(t, cache.heldBy(referralSettlementRunnerLeaderLockKey), "leader lock released after timed-out settle")

	// Next cycle (same or peer instance) can acquire and run again.
	repo2 := &countingSettlementRepo{}
	runner2 := newReferralSettlementRunner(newRunnerTestSettlement(repo2), cache, nil)
	runner2.runOnce()
	require.Equal(t, int32(1), repo2.listCalls.Load(), "next cycle recovers after timeout")
}

func TestReferralSettlementRunner_StartIsIdempotentAndStopIsIdempotent(t *testing.T) {
	cache := &fakeLeaderLockCache{}
	repo := &countingSettlementRepo{}
	runner := newReferralSettlementRunner(newRunnerTestSettlement(repo), cache, nil)
	runner.interval = time.Hour // avoid extra ticks during the test

	runner.Start()
	runner.Start() // second start is a no-op

	require.Eventually(t, func() bool {
		return repo.listCalls.Load() >= 1
	}, 2*time.Second, 10*time.Millisecond, "initial runOnce should have executed")

	runner.Stop()
	runner.Stop() // second stop is a no-op
}
