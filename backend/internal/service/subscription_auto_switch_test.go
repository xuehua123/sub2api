package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type autoSwitchUserSubRepoStub struct {
	userSubRepoNoop

	activeByGroup     map[int64]UserSubscription
	list              []UserSubscription
	resetMonthlyCalls int
	resetMonthlyID    int64
}

func (r *autoSwitchUserSubRepoStub) GetActiveByUserIDAndGroupID(_ context.Context, userID, groupID int64) (*UserSubscription, error) {
	sub, ok := r.activeByGroup[groupID]
	if !ok || sub.UserID != userID {
		return nil, ErrSubscriptionNotFound
	}
	cp := sub
	return &cp, nil
}

func (r *autoSwitchUserSubRepoStub) ListActiveByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	out := make([]UserSubscription, 0, len(r.list))
	for _, sub := range r.list {
		if sub.UserID == userID {
			out = append(out, sub)
		}
	}
	return out, nil
}

func (r *autoSwitchUserSubRepoStub) ListByGroupID(context.Context, int64, pagination.PaginationParams) ([]UserSubscription, *pagination.PaginationResult, error) {
	panic("unexpected ListByGroupID call")
}

func (r *autoSwitchUserSubRepoStub) ResetMonthlyUsage(_ context.Context, id int64, windowStart time.Time) error {
	r.resetMonthlyCalls++
	r.resetMonthlyID = id
	for groupID, sub := range r.activeByGroup {
		if sub.ID == id {
			sub.MonthlyUsageUSD = 0
			sub.MonthlyWindowStart = &windowStart
			r.activeByGroup[groupID] = sub
		}
	}
	for i := range r.list {
		if r.list[i].ID == id {
			r.list[i].MonthlyUsageUSD = 0
			r.list[i].MonthlyWindowStart = &windowStart
		}
	}
	return nil
}

type autoSwitchBillingCacheStub struct {
	subscriptions map[int64]SubscriptionCacheData
}

func (s *autoSwitchBillingCacheStub) GetUserBalance(context.Context, int64) (float64, error) {
	return 0, ErrSubscriptionNotFound
}

func (s *autoSwitchBillingCacheStub) SetUserBalance(context.Context, int64, float64) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) DeductUserBalance(context.Context, int64, float64) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) InvalidateUserBalance(context.Context, int64) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) GetSubscriptionCache(_ context.Context, _, groupID int64) (*SubscriptionCacheData, error) {
	sub, ok := s.subscriptions[groupID]
	if !ok {
		return nil, ErrSubscriptionNotFound
	}
	cp := sub
	return &cp, nil
}

func (s *autoSwitchBillingCacheStub) SetSubscriptionCache(_ context.Context, _, groupID int64, data *SubscriptionCacheData) error {
	if data == nil {
		delete(s.subscriptions, groupID)
		return nil
	}
	s.subscriptions[groupID] = *data
	return nil
}

func (s *autoSwitchBillingCacheStub) UpdateSubscriptionUsage(_ context.Context, _, groupID int64, cost float64) error {
	sub := s.subscriptions[groupID]
	sub.DailyUsage += cost
	sub.WeeklyUsage += cost
	sub.MonthlyUsage += cost
	s.subscriptions[groupID] = sub
	return nil
}

func (s *autoSwitchBillingCacheStub) InvalidateSubscriptionCache(_ context.Context, _, groupID int64) error {
	delete(s.subscriptions, groupID)
	return nil
}

func (s *autoSwitchBillingCacheStub) GetAPIKeyRateLimit(context.Context, int64) (*APIKeyRateLimitCacheData, error) {
	return nil, ErrSubscriptionNotFound
}

func (s *autoSwitchBillingCacheStub) SetAPIKeyRateLimit(context.Context, int64, *APIKeyRateLimitCacheData) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) UpdateAPIKeyRateLimitUsage(context.Context, int64, float64) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) InvalidateAPIKeyRateLimit(context.Context, int64) error {
	return nil
}

func TestResolveUsableSubscriptionForAPIKey_ResetsCandidateWindowSynchronously(t *testing.T) {
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	expiredWindow := now.Add(-31 * 24 * time.Hour)
	limit := 10.0
	userID := int64(1001)

	currentGroup := &Group{
		ID:               1,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	candidateGroup := &Group{
		ID:               2,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	currentSub := UserSubscription{
		ID:                 11,
		UserID:             userID,
		GroupID:            currentGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(24 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    limit + 1,
		Group:              currentGroup,
	}
	candidateSub := UserSubscription{
		ID:                 22,
		UserID:             userID,
		GroupID:            candidateGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &expiredWindow,
		MonthlyUsageUSD:    limit + 1,
		Group:              candidateGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{
			currentGroup.ID: currentSub,
		},
		list: []UserSubscription{currentSub, candidateSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKey(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	})

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.True(t, candidate.Switched)
	require.Equal(t, candidateGroup.ID, candidate.ToGroupID)
	require.Equal(t, 1, repo.resetMonthlyCalls)
	require.Equal(t, candidateSub.ID, repo.resetMonthlyID)
	require.Zero(t, candidate.Subscription.MonthlyUsageUSD)
	require.NotNil(t, candidate.Subscription.MonthlyWindowStart)
	require.True(t, candidate.Subscription.MonthlyWindowStart.After(expiredWindow))
}

func TestResolveUsableSubscriptionForAPIKey_UsesBillingCacheUsageView(t *testing.T) {
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	limit := 10.0
	userID := int64(1001)

	currentGroup := &Group{
		ID:               1,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	candidateGroup := &Group{
		ID:               2,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	currentSub := UserSubscription{
		ID:                 11,
		UserID:             userID,
		GroupID:            currentGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(24 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              currentGroup,
	}
	candidateSub := UserSubscription{
		ID:                 22,
		UserID:             userID,
		GroupID:            candidateGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              candidateGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{
			currentGroup.ID: currentSub,
		},
		list: []UserSubscription{currentSub, candidateSub},
	}
	cache := &autoSwitchBillingCacheStub{
		subscriptions: map[int64]SubscriptionCacheData{
			currentGroup.ID: {
				Status:       SubscriptionStatusActive,
				ExpiresAt:    currentSub.ExpiresAt,
				MonthlyUsage: limit,
				Version:      now.UnixMicro(),
			},
			candidateGroup.ID: {
				Status:       SubscriptionStatusActive,
				ExpiresAt:    candidateSub.ExpiresAt,
				MonthlyUsage: 0,
				Version:      now.UnixMicro(),
			},
		},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, &BillingCacheService{cache: cache}, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKey(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	})

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.True(t, candidate.Switched)
	require.Equal(t, candidateGroup.ID, candidate.ToGroupID)
	require.Equal(t, 0.0, candidate.Subscription.MonthlyUsageUSD)

	needsMaintenance, validateErr := svc.ValidateAndCheckLimits(candidate.Subscription, candidate.Group)
	require.NoError(t, validateErr)
	require.False(t, needsMaintenance)
}
