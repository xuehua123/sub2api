package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

type autoSwitchUserSubRepoStub struct {
	userSubRepoNoop

	activeByGroup     map[int64]UserSubscription
	list              []UserSubscription
	listCalls         int
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

func (r *autoSwitchUserSubRepoStub) GetByID(_ context.Context, id int64) (*UserSubscription, error) {
	for _, sub := range r.activeByGroup {
		if sub.ID == id {
			cp := sub
			return &cp, nil
		}
	}
	for _, sub := range r.list {
		if sub.ID == id {
			cp := sub
			return &cp, nil
		}
	}
	return nil, ErrSubscriptionNotFound
}

func (r *autoSwitchUserSubRepoStub) ListActiveByUserID(_ context.Context, userID int64) ([]UserSubscription, error) {
	r.listCalls++
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

func (r *autoSwitchUserSubRepoStub) ResetMonthlyUsage(_ context.Context, id int64, _ *time.Time, windowStart time.Time) error {
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

func (s *autoSwitchBillingCacheStub) GetUserPlatformQuotaCache(context.Context, int64, string) (*UserPlatformQuotaCacheEntry, bool, error) {
	return nil, false, nil
}

func (s *autoSwitchBillingCacheStub) SetUserPlatformQuotaCache(context.Context, int64, string, *UserPlatformQuotaCacheEntry, time.Duration) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) DeleteUserPlatformQuotaCache(context.Context, int64, string) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) IncrUserPlatformQuotaUsageCache(context.Context, int64, string, float64, time.Duration, bool) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) PopDirtyUserPlatformQuotaKeys(context.Context, int) ([]UserPlatformQuotaKey, error) {
	return nil, nil
}

func (s *autoSwitchBillingCacheStub) ReaddDirtyUserPlatformQuotaKeys(context.Context, []UserPlatformQuotaKey) error {
	return nil
}

func (s *autoSwitchBillingCacheStub) BatchGetUserPlatformQuotaCache(context.Context, []UserPlatformQuotaKey) ([]*UserPlatformQuotaCacheEntry, error) {
	return nil, nil
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

func TestResolveUsableSubscriptionForAPIKey_OpenAIResponsesCanFallbackAcrossMessagesDispatchCapability(t *testing.T) {
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	limit := 10.0
	userID := int64(1001)

	codexGroup := &Group{
		ID:                    1,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: false,
	}
	claudeMessagesGroup := &Group{
		ID:                    2,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: true,
	}
	currentSub := UserSubscription{
		ID:                 11,
		UserID:             userID,
		GroupID:            codexGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(24 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    limit + 1,
		Group:              codexGroup,
	}
	claudeMessagesSub := UserSubscription{
		ID:                 22,
		UserID:             userID,
		GroupID:            claudeMessagesGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              claudeMessagesGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{codexGroup.ID: currentSub},
		list:          []UserSubscription{currentSub, claudeMessagesSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKeyWithRequest(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &codexGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  codexGroup,
	}, NewSubscriptionSwitchRequestFromPath("/backend-api/codex/responses"))

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.True(t, candidate.Switched)
	require.Equal(t, claudeMessagesGroup.ID, candidate.ToGroupID)
	require.Equal(t, "monthly_limit_exceeded", candidate.Reason)
}

func TestResolveUsableSubscriptionForAPIKey_SwitchesWithinSameOpenAIBucket(t *testing.T) {
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	limit := 10.0
	userID := int64(1001)

	currentGroup := &Group{
		ID:                    1,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: true,
	}
	wrongBucketGroup := &Group{
		ID:                    2,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: false,
	}
	sameBucketGroup := &Group{
		ID:                    3,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: true,
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
	wrongBucketSub := UserSubscription{
		ID:                 22,
		UserID:             userID,
		GroupID:            wrongBucketGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(36 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              wrongBucketGroup,
	}
	sameBucketSub := UserSubscription{
		ID:                 33,
		UserID:             userID,
		GroupID:            sameBucketGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              sameBucketGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{currentGroup.ID: currentSub},
		list:          []UserSubscription{currentSub, wrongBucketSub, sameBucketSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKeyWithRequest(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	}, NewSubscriptionSwitchRequestFromPath("/v1/messages"))

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.True(t, candidate.Switched)
	require.Equal(t, sameBucketGroup.ID, candidate.ToGroupID)
}

func TestResolveUsableSubscriptionForAPIKey_OpenAIMessagesRequiresMessagesDispatchCandidate(t *testing.T) {
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	limit := 10.0
	userID := int64(1001)

	currentGroup := &Group{
		ID:                    1,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: false,
	}
	messagesGroup := &Group{
		ID:                    2,
		Platform:              PlatformOpenAI,
		Status:                StatusActive,
		SubscriptionType:      SubscriptionTypeSubscription,
		MonthlyLimitUSD:       &limit,
		AllowMessagesDispatch: true,
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
	messagesSub := UserSubscription{
		ID:                 22,
		UserID:             userID,
		GroupID:            messagesGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              messagesGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{currentGroup.ID: currentSub},
		list:          []UserSubscription{currentSub, messagesSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKeyWithRequest(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	}, NewSubscriptionSwitchRequestFromPath("/v1/messages"))

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.True(t, candidate.Switched)
	require.Equal(t, messagesGroup.ID, candidate.ToGroupID)
	require.Equal(t, "endpoint_unsupported", candidate.Reason)
}

func TestResolveUsableSubscriptionForAPIKey_KeepsCurrentGroupWhenUsable(t *testing.T) {
	now := time.Now()
	currentWindow := now.Add(-time.Hour)
	limit := 10.0
	userID := int64(1001)

	highPriorityGroup := &Group{
		ID:               1,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	currentGroup := &Group{
		ID:               2,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	highPrioritySub := UserSubscription{
		ID:                 11,
		UserID:             userID,
		GroupID:            highPriorityGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              highPriorityGroup,
	}
	currentSub := UserSubscription{
		ID:                 22,
		UserID:             userID,
		GroupID:            currentGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(24 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              currentGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{currentGroup.ID: currentSub},
		list:          []UserSubscription{currentSub, highPrioritySub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKeyWithRequest(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	}, SubscriptionSwitchRequest{})

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.False(t, candidate.Switched)
	require.Equal(t, currentGroup.ID, candidate.ToGroupID)
	require.Equal(t, 0, repo.listCalls)
}

func TestListAutoSwitchCandidates_UsesPreferencesOnlyForFallbackOrder(t *testing.T) {
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
	lowPriorityGroup := &Group{
		ID:               2,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	highPriorityGroup := &Group{
		ID:               3,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	disabledGroup := &Group{
		ID:               4,
		Platform:         PlatformOpenAI,
		Status:           StatusActive,
		SubscriptionType: SubscriptionTypeSubscription,
		MonthlyLimitUSD:  &limit,
	}
	currentSub := UserSubscription{
		ID:                 1,
		UserID:             userID,
		GroupID:            currentGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(48 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              currentGroup,
	}
	lowPrioritySub := UserSubscription{
		ID:                 2,
		UserID:             userID,
		GroupID:            lowPriorityGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(72 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              lowPriorityGroup,
	}
	highPrioritySub := UserSubscription{
		ID:                 3,
		UserID:             userID,
		GroupID:            highPriorityGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(96 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              highPriorityGroup,
	}
	disabledSub := UserSubscription{
		ID:                 4,
		UserID:             userID,
		GroupID:            disabledGroup.ID,
		Status:             SubscriptionStatusActive,
		ExpiresAt:          now.Add(12 * time.Hour),
		MonthlyWindowStart: &currentWindow,
		MonthlyUsageUSD:    0,
		Group:              disabledGroup,
	}
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{currentGroup.ID: currentSub},
		list:          []UserSubscription{currentSub, lowPrioritySub, highPrioritySub, disabledSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidates, err := svc.listAutoSwitchCandidates(context.Background(), userID, currentGroup.ID, currentGroup, SubscriptionSwitchRequest{}, map[int64]subscriptionGroupPreferenceRank{
		highPriorityGroup.ID: {SortOrder: 0, Enabled: true},
		lowPriorityGroup.ID:  {SortOrder: 1, Enabled: true},
		disabledGroup.ID:     {SortOrder: 2, Enabled: false},
		currentGroup.ID:      {SortOrder: 3, Enabled: true},
	}, false)

	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.Equal(t, highPriorityGroup.ID, candidates[0].GroupID)
	require.Equal(t, lowPriorityGroup.ID, candidates[1].GroupID)
}

func TestResolveUsableSubscriptionForAPIKey_FallsBackByPreferenceWhenCurrentExhausted(t *testing.T) {
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
		MonthlyUsageUSD:    limit + 1,
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
		activeByGroup: map[int64]UserSubscription{currentGroup.ID: currentSub},
		list:          []UserSubscription{currentSub, candidateSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)
	preferences := map[int64]subscriptionGroupPreferenceRank{
		candidateGroup.ID: {SortOrder: 0, Enabled: true},
		currentGroup.ID:   {SortOrder: 1, Enabled: true},
	}

	candidates, err := svc.listAutoSwitchCandidates(context.Background(), userID, currentGroup.ID, currentGroup, SubscriptionSwitchRequest{}, preferences, false)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, candidateGroup.ID, candidates[0].GroupID)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKeyWithRequest(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: true,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	}, SubscriptionSwitchRequest{})

	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.True(t, candidate.Switched)
	require.Equal(t, candidateGroup.ID, candidate.ToGroupID)
	require.Equal(t, "monthly_limit_exceeded", candidate.Reason)
}

func TestResolveUsableSubscriptionForAPIKey_DisabledAutoSwitchStaysFixed(t *testing.T) {
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
	repo := &autoSwitchUserSubRepoStub{
		activeByGroup: map[int64]UserSubscription{currentGroup.ID: currentSub},
		list:          []UserSubscription{currentSub},
	}
	svc := NewSubscriptionService(groupRepoNoop{}, repo, nil, nil, nil)

	candidate, err := svc.ResolveUsableSubscriptionForAPIKeyWithRequest(context.Background(), &APIKey{
		ID:                     101,
		UserID:                 userID,
		GroupID:                &currentGroup.ID,
		AutoSwitchGroupEnabled: false,
		User:                   &User{ID: userID},
		Group:                  currentGroup,
	}, SubscriptionSwitchRequest{})

	require.Nil(t, candidate)
	require.True(t, errors.Is(err, ErrMonthlyLimitExceeded))
	require.Equal(t, 0, repo.listCalls)
}
