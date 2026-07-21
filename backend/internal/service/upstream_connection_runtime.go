package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// GetRuntimeOverview returns local account concurrency and per-local-group
// traffic for the connection list. It is read-only and deliberately keeps the
// account-wide concurrency separate from group traffic: slots are not tracked
// at an account-plus-group granularity.
func (s *UpstreamConnectionService) GetRuntimeOverview(ctx context.Context, accountIDs []int64) (*UpstreamConnectionRuntimeOverview, error) {
	accountIDs = normalizeUpstreamRuntimeAccountIDs(accountIDs)
	result := &UpstreamConnectionRuntimeOverview{Accounts: make([]UpstreamConnectionRuntimeAccount, 0, len(accountIDs))}
	if len(accountIDs) == 0 {
		return result, nil
	}
	if s == nil || s.accountRepo == nil {
		return nil, errors.New("upstream connection account repository is unavailable")
	}
	if s.runtimeReader == nil {
		return nil, errors.New("upstream connection runtime reader is unavailable")
	}

	accounts, err := s.accountRepo.GetByIDs(ctx, accountIDs)
	if err != nil {
		return nil, fmt.Errorf("load upstream runtime accounts: %w", err)
	}
	accountsByID := make(map[int64]*Account, len(accounts))
	loads := make([]AccountWithConcurrency, 0, len(accounts))
	for _, account := range accounts {
		if account == nil || account.ID <= 0 {
			continue
		}
		accountsByID[account.ID] = account
		loads = append(loads, AccountWithConcurrency{ID: account.ID, MaxConcurrency: account.Concurrency})
	}

	now := timezone.Now()
	if s.now != nil {
		now = s.now().In(timezone.Location())
	}
	start := timezone.StartOfDay(now)
	// Clip the 5m window to local start-of-day so early-morning rates are not
	// diluted by pre-midnight traffic that is outside the "today" snapshot.
	fiveMinuteStart := now.Add(-5 * time.Minute)
	if fiveMinuteStart.Before(start) {
		fiveMinuteStart = start
	}
	metrics, err := s.runtimeReader.GetUpstreamConnectionRuntimeGroups(ctx, accountIDs, start, now, fiveMinuteStart)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection runtime groups: %w", err)
	}

	// Runtime observability must not turn a transient Redis outage into a total
	// loss of usage visibility. Nil concurrency fields tell the client it is
	// unavailable, while a real zero remains distinguishable as 0.
	loadByAccount := map[int64]*AccountLoadInfo{}
	if s.concurrencyService != nil {
		if loadsResult, loadErr := s.concurrencyService.GetAccountsLoadBatch(ctx, loads); loadErr == nil {
			loadByAccount = loadsResult
		}
	}

	groupsByAccount := make(map[int64][]UpstreamConnectionRuntimeGroup, len(accountIDs))
	for _, metric := range metrics {
		if _, ok := accountsByID[metric.AccountID]; !ok {
			continue
		}
		group := UpstreamConnectionRuntimeGroup{
			GroupID: metric.GroupID, GroupName: metric.GroupName, Today: metric.Today,
			FiveMinuteRequests:     metric.FiveMinuteRequests,
			FiveMinuteSuccessCount: metric.FiveMinuteSuccessCount,
			FiveMinuteErrorCount:   metric.FiveMinuteErrorCount,
		}
		if metric.FiveMinuteRequests > 0 {
			rate := float64(metric.FiveMinuteSuccessCount) * 100 / float64(metric.FiveMinuteRequests)
			group.FiveMinuteSuccessRate = &rate
		}
		groupsByAccount[metric.AccountID] = append(groupsByAccount[metric.AccountID], group)
	}

	for _, accountID := range accountIDs {
		account, ok := accountsByID[accountID]
		if !ok {
			continue
		}
		groups := groupsByAccount[accountID]
		sort.Slice(groups, func(i, j int) bool {
			if groups[i].Today.AccountCost != groups[j].Today.AccountCost {
				return groups[i].Today.AccountCost > groups[j].Today.AccountCost
			}
			if groups[i].Today.Requests != groups[j].Today.Requests {
				return groups[i].Today.Requests > groups[j].Today.Requests
			}
			return groups[i].GroupName < groups[j].GroupName
		})
		item := UpstreamConnectionRuntimeAccount{AccountID: account.ID, AccountName: account.Name, Groups: groups}
		if load := loadByAccount[account.ID]; load != nil {
			concurrency, waiting := load.CurrentConcurrency, load.WaitingCount
			item.CurrentConcurrency = &concurrency
			item.WaitingCount = &waiting
		}
		result.Accounts = append(result.Accounts, item)
	}
	return result, nil
}

func normalizeUpstreamRuntimeAccountIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
