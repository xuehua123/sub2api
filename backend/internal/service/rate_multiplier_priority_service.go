package service

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"
)

const (
	rateMultiplierPriorityPollInterval = time.Minute
	rateMultiplierPriorityBase         = 1
	rateMultiplierPriorityEpsilon      = 1e-9
	rateMultiplierPriorityTimeout      = 20 * time.Second
)

// RateMultiplierPriorityRepository is deliberately narrower than AccountRepository.
// The reconciler only reads accounts explicitly enabled for scheduling and keeps both
// account priority and each bound local-group priority on the same multiplier band.
type RateMultiplierPriorityRepository interface {
	ListActiveSchedulableForRateMultiplierPriority(ctx context.Context) ([]Account, error)
	UpdateRateMultiplierPriorities(ctx context.Context, priorities map[int64]int) (int64, error)
}

// RateMultiplierPrioritySettingsReader keeps the scheduler independent from the
// concrete settings implementation while making automatic rewrites opt-in.
type RateMultiplierPrioritySettingsReader interface {
	GetRateMultiplierPrioritySettings(ctx context.Context) (*RateMultiplierPrioritySettings, error)
}

// RateMultiplierPriorityService keeps scheduler priority aligned with the existing
// account cost multiplier. Lower multipliers are preferred by assigning a lower
// numeric priority. Equal multipliers always share the same priority band.
type RateMultiplierPriorityService struct {
	repo     RateMultiplierPriorityRepository
	settings RateMultiplierPrioritySettingsReader
	interval time.Duration

	stopCh    chan struct{}
	runCtx    context.Context
	runCancel context.CancelFunc
	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup

	periodicMu          sync.Mutex
	lastPeriodicAttempt time.Time
}

func NewRateMultiplierPriorityService(repo RateMultiplierPriorityRepository, settings RateMultiplierPrioritySettingsReader, interval time.Duration) *RateMultiplierPriorityService {
	if interval <= 0 {
		interval = rateMultiplierPriorityPollInterval
	}
	runCtx, runCancel := context.WithCancel(context.Background())
	return &RateMultiplierPriorityService{
		repo:                repo,
		settings:            settings,
		interval:            interval,
		stopCh:              make(chan struct{}),
		runCtx:              runCtx,
		runCancel:           runCancel,
		lastPeriodicAttempt: time.Now(),
	}
}

// Start periodically reconciles priorities only after the administrator enables
// automatic rate-based sorting. It intentionally does not rewrite priorities at
// process startup.
func (s *RateMultiplierPriorityService) Start() {
	if s == nil || s.repo == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(s.interval)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					s.reconcileAndLog()
				case <-s.stopCh:
					return
				}
			}
		}()
	})
}

func (s *RateMultiplierPriorityService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.runCancel != nil {
			s.runCancel()
		}
		close(s.stopCh)
		s.wg.Wait()
	})
}

func (s *RateMultiplierPriorityService) reconcileAndLog() {
	ctx := s.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, rateMultiplierPriorityTimeout)
	defer cancel()
	updated, err := s.reconcilePeriodically(ctx, time.Now())
	if err != nil {
		slog.Error("rate_multiplier_priority_reconcile_failed", "error", err)
		return
	}
	if updated > 0 {
		slog.Info("rate_multiplier_priority_reconciled", "updated_accounts", updated)
	}
}

// ReconcileIfEnabled leaves priorities untouched unless the persisted global
// setting is enabled. It is the entry point for every automatic trigger.
func (s *RateMultiplierPriorityService) ReconcileIfEnabled(ctx context.Context) (int64, error) {
	if s == nil || s.settings == nil {
		return 0, nil
	}
	settings, err := s.settings.GetRateMultiplierPrioritySettings(ctx)
	if err != nil {
		return 0, err
	}
	if settings == nil || !settings.Enabled {
		return 0, nil
	}
	return s.reconcile(ctx, settings)
}

// Reconcile assigns every distinct multiplier its own priority band. The lowest
// multiplier receives priority 1; each next distinct multiplier advances by the
// default step. Periodic reconciliation uses the persisted settings instead.
// Accounts with equal multipliers always share exactly the same priority. Inactive
// or non-schedulable accounts are never selected or modified.
func (s *RateMultiplierPriorityService) Reconcile(ctx context.Context) (int64, error) {
	return s.reconcile(ctx, DefaultRateMultiplierPrioritySettings())
}

func (s *RateMultiplierPriorityService) reconcilePeriodically(ctx context.Context, now time.Time) (int64, error) {
	if s == nil || s.settings == nil {
		return 0, nil
	}
	settings, err := s.settings.GetRateMultiplierPrioritySettings(ctx)
	if err != nil {
		return 0, err
	}
	if settings == nil || !settings.Enabled {
		s.markPeriodicAttempt(now)
		return 0, nil
	}
	settings, err = normalizeRateMultiplierPrioritySettings(settings)
	if err != nil {
		return 0, err
	}
	if !s.periodicDue(now, settings.IntervalMinutes) {
		return 0, nil
	}

	updated, err := s.reconcile(ctx, settings)
	if err == nil {
		s.markPeriodicAttempt(now)
	}
	return updated, err
}

func (s *RateMultiplierPriorityService) periodicDue(now time.Time, intervalMinutes int) bool {
	interval := time.Duration(intervalMinutes) * time.Minute
	s.periodicMu.Lock()
	defer s.periodicMu.Unlock()
	return s.lastPeriodicAttempt.IsZero() || now.Sub(s.lastPeriodicAttempt) >= interval
}

func (s *RateMultiplierPriorityService) markPeriodicAttempt(now time.Time) {
	s.periodicMu.Lock()
	defer s.periodicMu.Unlock()
	s.lastPeriodicAttempt = now
}

func (s *RateMultiplierPriorityService) reconcile(ctx context.Context, settings *RateMultiplierPrioritySettings) (int64, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	settings, err := normalizeRateMultiplierPrioritySettings(settings)
	if err != nil {
		return 0, err
	}
	accounts, err := s.repo.ListActiveSchedulableForRateMultiplierPriority(ctx)
	if err != nil {
		return 0, err
	}

	eligible := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Status == StatusActive && account.Schedulable {
			eligible = append(eligible, account)
		}
	}
	sort.Slice(eligible, func(i, j int) bool {
		left, right := eligible[i].BillingRateMultiplier(), eligible[j].BillingRateMultiplier()
		if math.Abs(left-right) <= rateMultiplierPriorityEpsilon {
			return eligible[i].ID < eligible[j].ID
		}
		return left < right
	})

	priorities := make(map[int64]int, len(eligible))
	band := -1
	lastMultiplier := 0.0
	for index, account := range eligible {
		multiplier := account.BillingRateMultiplier()
		if index == 0 || math.Abs(multiplier-lastMultiplier) > rateMultiplierPriorityEpsilon {
			band++
			lastMultiplier = multiplier
		}

		priority := rateMultiplierPriorityBase + band*settings.PriorityStep
		if account.Priority != priority || hasMismatchedBoundGroupPriority(account.AccountGroups, priority) {
			priorities[account.ID] = priority
		}
	}
	if len(priorities) == 0 {
		return 0, nil
	}
	return s.repo.UpdateRateMultiplierPriorities(ctx, priorities)
}

func hasMismatchedBoundGroupPriority(groups []AccountGroup, priority int) bool {
	for _, group := range groups {
		if group.Priority != priority {
			return true
		}
	}
	return false
}
