package service

import (
	"context"
	"database/sql"
	"sync"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/google/uuid"
)

const (
	referralSettlementRunnerInterval      = 5 * time.Minute
	referralSettlementRunnerLeaderLockKey = "referral:settlement:runner:v1"
	referralSettlementRunnerLeaderLockTTL = 4 * time.Minute
	referralSettlementRunnerJobTimeout    = 2 * time.Minute
)

// referralSettlementRunner periodically settles due pending commission rewards
// so admin overview does not need to run a global settle on every page open.
type referralSettlementRunner struct {
	settlement *ReferralSettlementService
	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
	now        func() time.Time
	// interval overrides the production ticker for tests; zero uses default.
	interval time.Duration
	// jobTimeout overrides the per-cycle context timeout; zero uses default.
	jobTimeout time.Duration

	mu           sync.Mutex
	started      bool
	stopped      bool
	parentCtx    context.Context
	parentCancel context.CancelFunc
	wg           sync.WaitGroup
}

// ProvideReferralSettlementService constructs the settlement service and starts
// a background settle loop (leader-locked) so overdue pending rewards keep
// advancing without admin/user page traffic.
func ProvideReferralSettlementService(
	commissionRepo CommissionRepository,
	rechargeRepo RechargeOrderRepository,
	entClient *dbent.Client,
	settingService *SettingService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *ReferralSettlementService {
	settlement := NewReferralSettlementService(commissionRepo, rechargeRepo, entClient, settingService)
	runner := newReferralSettlementRunner(settlement, lockCache, db)
	settlement.backgroundRunner = runner
	runner.Start()
	return settlement
}

// StopBackgroundRunner stops the periodic settlement loop if started via Provide.
func (s *ReferralSettlementService) StopBackgroundRunner() {
	if s == nil || s.backgroundRunner == nil {
		return
	}
	s.backgroundRunner.Stop()
}

func newReferralSettlementRunner(
	settlement *ReferralSettlementService,
	lockCache LeaderLockCache,
	db *sql.DB,
) *referralSettlementRunner {
	ctx, cancel := context.WithCancel(context.Background())
	return &referralSettlementRunner{
		settlement:   settlement,
		lockCache:    lockCache,
		db:           db,
		instanceID:   uuid.NewString(),
		now:          time.Now,
		parentCtx:    ctx,
		parentCancel: cancel,
	}
}

func (r *referralSettlementRunner) Start() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.started || r.stopped {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.wg.Add(1)
	r.mu.Unlock()
	go r.runLoop()
}

func (r *referralSettlementRunner) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.stopped {
		r.mu.Unlock()
		return
	}
	r.stopped = true
	r.parentCancel()
	r.mu.Unlock()
	r.wg.Wait()
}

func (r *referralSettlementRunner) runLoop() {
	defer r.wg.Done()
	r.runOnce()
	interval := r.interval
	if interval <= 0 {
		interval = referralSettlementRunnerInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.parentCtx.Done():
			return
		case <-ticker.C:
			r.runOnce()
		}
	}
}

func (r *referralSettlementRunner) runOnce() {
	if r == nil || r.settlement == nil {
		return
	}
	timeout := r.jobTimeout
	if timeout <= 0 {
		timeout = referralSettlementRunnerJobTimeout
	}
	ctx, cancel := context.WithTimeout(r.parentCtx, timeout)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(
		ctx, r.lockCache, r.db,
		referralSettlementRunnerLeaderLockKey, r.instanceID, referralSettlementRunnerLeaderLockTTL,
	)
	if !acquired {
		return
	}
	defer release()

	if _, err := r.settlement.SettlePendingRewards(ctx, r.now()); err != nil {
		logger.LegacyPrintf("service.referral_settlement_runner", "settle_failed: err=%v", err)
	}
}
