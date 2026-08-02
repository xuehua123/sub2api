package service

import (
	"context"
	"time"
)

// Keep the upstream test helper signature while making the fork's actual
// architecture explicit: profit control consumes the final account rate, which
// may already have been synchronized by the centralized connection service.
const UpstreamBillingProbeStatusOK = "ok"

func upstreamCostTestAccount(id int64, _ string, rate float64, _ time.Time, _ time.Duration) *Account {
	rateCopy := rate
	return &Account{
		ID:             id,
		Platform:       PlatformOpenAI,
		Type:           AccountTypeAPIKey,
		RateMultiplier: &rateCopy,
		Extra:          map[string]any{},
	}
}

func upstreamCostTestOAuthAccount(id int64) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{}}
}

type upstreamCostTrackingConcurrencyCache struct {
	ConcurrencyCache
	loadMap       map[int64]*AccountLoadInfo
	acquireLimits map[int64][]int
	releases      map[int64]int
	rejectAcquire bool
}

func (c *upstreamCostTrackingConcurrencyCache) AcquireAccountSlot(_ context.Context, accountID int64, maxConcurrency int, _ string) (bool, error) {
	if c.acquireLimits == nil {
		c.acquireLimits = make(map[int64][]int)
	}
	c.acquireLimits[accountID] = append(c.acquireLimits[accountID], maxConcurrency)
	return !c.rejectAcquire, nil
}

func (c *upstreamCostTrackingConcurrencyCache) ReleaseAccountSlot(_ context.Context, accountID int64, _ string) error {
	if c.releases == nil {
		c.releases = make(map[int64]int)
	}
	c.releases[accountID]++
	return nil
}

func (c *upstreamCostTrackingConcurrencyCache) GetAccountsLoadBatch(_ context.Context, accounts []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	out := make(map[int64]*AccountLoadInfo, len(accounts))
	for _, account := range accounts {
		if load := c.loadMap[account.ID]; load != nil {
			copied := *load
			out[account.ID] = &copied
		}
	}
	return out, nil
}

func (c *upstreamCostTrackingConcurrencyCache) releaseCount(accountID int64) int {
	return c.releases[accountID]
}

func (c *upstreamCostTrackingConcurrencyCache) totalAcquires() int {
	total := 0
	for _, limits := range c.acquireLimits {
		total += len(limits)
	}
	return total
}
