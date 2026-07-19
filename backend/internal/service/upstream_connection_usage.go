package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
)

// GetTodayUsage returns read-only local usage for every account bound to one
// shared upstream connection. It never mutates upstream observations or billing.
func (s *UpstreamConnectionService) GetTodayUsage(ctx context.Context, connectionID int64) (*UpstreamConnectionTodayUsage, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("upstream connection repository is unavailable")
	}
	connection, err := s.repo.GetByID(ctx, connectionID)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection: %w", err)
	}

	now := timezone.Now()
	if s.now != nil {
		now = s.now().In(timezone.Location())
	}
	start := timezone.StartOfDay(now)
	result := &UpstreamConnectionTodayUsage{
		ConnectionID: connection.ID,
		Timezone:     timezone.Location().String(),
		StartAt:      start,
		EndAt:        now,
		Trend:        buildEmptyUpstreamConnectionUsageTrend(start, now),
		Accounts:     make([]UpstreamConnectionAccountUsage, 0, len(connection.Bindings)),
	}

	accountIDs := uniqueUpstreamBindingAccountIDs(connection.Bindings)
	accountNames := make(map[int64]string, len(accountIDs))
	if len(accountIDs) > 0 {
		if s.accountRepo == nil {
			return nil, errors.New("account repository is unavailable")
		}
		accounts, accountErr := s.accountRepo.GetByIDs(ctx, accountIDs)
		if accountErr != nil {
			return nil, fmt.Errorf("load bound upstream accounts: %w", accountErr)
		}
		for _, account := range accounts {
			if account != nil {
				accountNames[account.ID] = account.Name
			}
		}
	}

	accountIndexes := make(map[int64]int, len(connection.Bindings))
	for _, binding := range connection.Bindings {
		item := UpstreamConnectionAccountUsage{
			BindingID:          binding.ID,
			AccountID:          binding.AccountID,
			AccountName:        accountNames[binding.AccountID],
			RemoteTokenID:      binding.RemoteTokenID,
			RemoteTokenName:    binding.RemoteTokenName,
			RemoteGroupName:    binding.RemoteGroupName,
			ResolutionKind:     binding.ResolutionKind,
			ObservedMultiplier: cloneFloat64Ptr(binding.ObservedMultiplier),
			Status:             binding.Status,
			Trend:              buildEmptyUpstreamConnectionUsageTrend(start, now),
		}
		accountIndexes[binding.AccountID] = len(result.Accounts)
		result.Accounts = append(result.Accounts, item)
	}

	if len(accountIDs) == 0 {
		return result, nil
	}
	if s.usageReader == nil {
		return nil, errors.New("upstream connection usage reader is unavailable")
	}
	buckets, err := s.usageReader.GetUpstreamAccountUsageBuckets(ctx, accountIDs, start, now, result.Timezone)
	if err != nil {
		return nil, fmt.Errorf("get upstream connection usage: %w", err)
	}

	connectionPointIndexes := upstreamConnectionUsagePointIndexes(result.Trend, timezone.Location())
	accountPointIndexes := make(map[int64]map[string]int, len(result.Accounts))
	for index := range result.Accounts {
		accountPointIndexes[result.Accounts[index].AccountID] = upstreamConnectionUsagePointIndexes(result.Accounts[index].Trend, timezone.Location())
	}
	for _, bucket := range buckets {
		accountIndex, ok := accountIndexes[bucket.AccountID]
		if !ok {
			continue
		}
		key := upstreamConnectionUsageHourKey(bucket.Bucket, timezone.Location())
		pointIndex, ok := accountPointIndexes[bucket.AccountID][key]
		if !ok {
			continue
		}
		stats := bucket.UpstreamConnectionUsageStats
		result.Accounts[accountIndex].Trend[pointIndex].UpstreamConnectionUsageStats = stats
		addUpstreamConnectionUsageStats(&result.Accounts[accountIndex].Stats, stats)
		if aggregateIndex, exists := connectionPointIndexes[key]; exists {
			addUpstreamConnectionUsageStats(&result.Trend[aggregateIndex].UpstreamConnectionUsageStats, stats)
		}
		addUpstreamConnectionUsageStats(&result.Summary, stats)
	}

	return result, nil
}

func uniqueUpstreamBindingAccountIDs(bindings []UpstreamAccountBinding) []int64 {
	seen := make(map[int64]struct{}, len(bindings))
	ids := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		if binding.AccountID <= 0 {
			continue
		}
		if _, ok := seen[binding.AccountID]; ok {
			continue
		}
		seen[binding.AccountID] = struct{}{}
		ids = append(ids, binding.AccountID)
	}
	return ids
}

func buildEmptyUpstreamConnectionUsageTrend(start, end time.Time) []UpstreamConnectionUsagePoint {
	location := timezone.Location()
	start = start.In(location).Truncate(time.Hour)
	last := end.In(location).Truncate(time.Hour)
	points := make([]UpstreamConnectionUsagePoint, 0, int(last.Sub(start).Hours())+1)
	for bucket := start; !bucket.After(last); bucket = bucket.Add(time.Hour) {
		points = append(points, UpstreamConnectionUsagePoint{Bucket: bucket})
	}
	return points
}

func upstreamConnectionUsagePointIndexes(points []UpstreamConnectionUsagePoint, location *time.Location) map[string]int {
	indexes := make(map[string]int, len(points))
	for index := range points {
		indexes[upstreamConnectionUsageHourKey(points[index].Bucket, location)] = index
	}
	return indexes
}

func upstreamConnectionUsageHourKey(value time.Time, location *time.Location) string {
	return value.In(location).Format("2006-01-02T15")
}

func addUpstreamConnectionUsageStats(target *UpstreamConnectionUsageStats, value UpstreamConnectionUsageStats) {
	if target == nil {
		return
	}
	target.Requests += value.Requests
	target.Tokens += value.Tokens
	target.AccountCost += value.AccountCost
	target.StandardCost += value.StandardCost
	target.UserCost += value.UserCost
}
