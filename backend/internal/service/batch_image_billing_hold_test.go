//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestReserveBatchImageBalanceHold_PreservesKnownDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "insufficient balance", err: ErrBatchImageInsufficientBalance},
		{name: "subscription not found", err: ErrSubscriptionNotFound},
		{name: "subscription expired", err: ErrSubscriptionExpired},
		{name: "subscription suspended", err: ErrSubscriptionSuspended},
		{name: "daily limit", err: ErrDailyLimitExceeded},
		{name: "weekly limit", err: ErrWeeklyLimitExceeded},
		{name: "monthly limit", err: ErrMonthlyLimitExceeded},
		{name: "entitlement not found", err: ErrSubscriptionEntitlementNotFound},
		{name: "entitlement inactive", err: ErrSubscriptionEntitlementInactive},
		{name: "entitlement expired", err: ErrSubscriptionEntitlementExpired},
		{name: "entitlement quota exceeded", err: ErrSubscriptionEntitlementQuotaExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := testSettlingBatchImageJob("imgbatch_domain_error")
			repo := &fakeBatchImageBillingRepo{reserveErr: tt.err}

			err := reserveBatchImageBalanceHold(context.Background(), repo, nil, job, "payload")

			require.ErrorIs(t, err, tt.err)
			require.Equal(t, infraerrors.Reason(tt.err), infraerrors.Reason(err))
			require.Equal(t, infraerrors.Code(tt.err), infraerrors.Code(err))
		})
	}
}

func TestReserveBatchImageBalanceHold_WrapsUnknownRepositoryError(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_unknown_error")
	repo := &fakeBatchImageBillingRepo{reserveErr: errors.New("database unavailable")}

	err := reserveBatchImageBalanceHold(context.Background(), repo, nil, job, "payload")

	require.ErrorIs(t, err, ErrBatchImageBillingHoldFailed)
}
func TestReserveBatchImageBalanceHold_IdempotentRetryPreservesQuotaWindows(t *testing.T) {
	job := legacySubscriptionBatchImageJob("imgbatch_reserve_retry_windows")
	repo := &batchImageReserveIdempotencyRepo{fakeBatchImageBillingRepo: &fakeBatchImageBillingRepo{}}
	daily := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.UTC)
	weekly := daily.Add(-48 * time.Hour)
	monthly := daily.Add(-9 * 24 * time.Hour)
	repo.firstResult = &BatchImageBalanceHoldResult{
		Applied:                true,
		BillingSource:          BillingSourceLegacySubscription,
		HeldDailyWindowStart:   &daily,
		HeldWeeklyWindowStart:  &weekly,
		HeldMonthlyWindowStart: &monthly,
	}

	require.NoError(t, reserveBatchImageBalanceHold(context.Background(), repo, nil, job, "payload"))
	require.NoError(t, reserveBatchImageBalanceHold(context.Background(), repo, nil, job, "payload"))

	require.Equal(t, &daily, job.HeldDailyWindowStart)
	require.Equal(t, &weekly, job.HeldWeeklyWindowStart)
	require.Equal(t, &monthly, job.HeldMonthlyWindowStart)
	require.Equal(t, 2, repo.reserveCalls)
}

func TestReserveBatchImageBalanceHold_IdempotentRetryUsesInitialEntitlementSource(t *testing.T) {
	job := testSettlingBatchImageJob("imgbatch_reserve_retry_fallback")
	entitlementID := int64(81)
	job.EntitlementID = &entitlementID
	job.BillingType = BillingTypeSubscription
	job.BillingSource = BillingSourceEntitlementQuota
	job.EntitlementBalanceFallback = true
	repo := &batchImageReserveIdempotencyRepo{
		fakeBatchImageBillingRepo: &fakeBatchImageBillingRepo{},
		firstResult: &BatchImageBalanceHoldResult{
			Applied:       true,
			BillingSource: BillingSourceEntitlementBalanceFallback,
		},
	}

	require.NoError(t, reserveBatchImageBalanceHold(context.Background(), repo, nil, job, "payload"))
	require.Equal(t, BillingSourceEntitlementBalanceFallback, job.BillingSource)
	require.NoError(t, reserveBatchImageBalanceHold(context.Background(), repo, nil, job, "payload"))
	require.Equal(t, 2, repo.reserveCalls)
}

type batchImageReserveIdempotencyRepo struct {
	*fakeBatchImageBillingRepo
	firstFingerprint string
	firstResult      *BatchImageBalanceHoldResult
	reserveCalls     int
}

func (r *batchImageReserveIdempotencyRepo) ReserveBatchImageBalance(_ context.Context, cmd *BatchImageBalanceHoldCommand) (*BatchImageBalanceHoldResult, error) {
	r.reserveCalls++
	cmd.Normalize()
	if r.firstFingerprint == "" {
		r.firstFingerprint = cmd.RequestFingerprint
		return r.firstResult, nil
	}
	if cmd.RequestFingerprint != r.firstFingerprint {
		return nil, ErrUsageBillingRequestConflict
	}
	return &BatchImageBalanceHoldResult{Applied: false}, nil
}

type batchImageSubscriptionCacheInvalidatorStub struct {
	calls [][2]int64
	err   error
}

func (s *batchImageSubscriptionCacheInvalidatorStub) InvalidateSubscription(_ context.Context, userID, groupID int64) error {
	s.calls = append(s.calls, [2]int64{userID, groupID})
	return s.err
}

func legacySubscriptionBatchImageJob(batchID string) *BatchImageJob {
	job := testSettlingBatchImageJob(batchID)
	groupID := int64(7)
	subscriptionID := int64(41)
	job.GroupID = &groupID
	job.SubscriptionID = &subscriptionID
	job.BillingType = BillingTypeSubscription
	job.BillingSource = BillingSourceLegacySubscription
	return job
}

func TestBatchImageBillingHold_InvalidatesLegacySubscriptionCacheAfterSuccessfulMutations(t *testing.T) {
	tests := []struct {
		name string
		run  func(context.Context, UsageBillingRepository, BatchImageSubscriptionCacheInvalidator, *BatchImageJob) error
	}{
		{
			name: "reserve",
			run: func(ctx context.Context, repo UsageBillingRepository, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob) error {
				return reserveBatchImageBalanceHold(ctx, repo, cache, job, "payload")
			},
		},
		{
			name: "capture",
			run: func(ctx context.Context, repo UsageBillingRepository, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob) error {
				return captureBatchImageBalanceHold(ctx, repo, cache, job, 0.25, "payload")
			},
		},
		{
			name: "release",
			run: func(ctx context.Context, repo UsageBillingRepository, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob) error {
				return releaseBatchImageBalanceHold(ctx, repo, cache, job, "payload")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			job := legacySubscriptionBatchImageJob("imgbatch_cache_" + tt.name)
			repo := &fakeBatchImageBillingRepo{}
			cache := &batchImageSubscriptionCacheInvalidatorStub{}

			err := tt.run(context.Background(), repo, cache, job)

			require.NoError(t, err)
			require.Equal(t, [][2]int64{{job.UserID, *job.GroupID}}, cache.calls)
		})
	}
}

func TestBatchImageBillingHold_DoesNotInvalidateNonLegacySources(t *testing.T) {
	sources := []string{
		BillingSourceBalance,
		BillingSourceEntitlementQuota,
		BillingSourceEntitlementBalanceFallback,
	}
	for _, source := range sources {
		t.Run(source, func(t *testing.T) {
			job := legacySubscriptionBatchImageJob("imgbatch_cache_nonlegacy_" + source)
			job.BillingSource = source
			cache := &batchImageSubscriptionCacheInvalidatorStub{}

			require.NoError(t, reserveBatchImageBalanceHold(context.Background(), &fakeBatchImageBillingRepo{}, cache, job, "payload"))
			require.NoError(t, captureBatchImageBalanceHold(context.Background(), &fakeBatchImageBillingRepo{}, cache, job, 0.25, "payload"))
			require.NoError(t, releaseBatchImageBalanceHold(context.Background(), &fakeBatchImageBillingRepo{}, cache, job, "payload"))
			require.Empty(t, cache.calls)
		})
	}
}

func TestBatchImageBillingHold_CacheInvalidationFailureIsBestEffort(t *testing.T) {
	job := legacySubscriptionBatchImageJob("imgbatch_cache_failure")
	cache := &batchImageSubscriptionCacheInvalidatorStub{err: errors.New("redis unavailable")}

	err := reserveBatchImageBalanceHold(context.Background(), &fakeBatchImageBillingRepo{}, cache, job, "payload")

	require.NoError(t, err)
	require.Len(t, cache.calls, 1)
}

func TestReleaseBatchImageBalanceHold_InvalidatesLegacyCacheAfterFingerprintConflict(t *testing.T) {
	job := legacySubscriptionBatchImageJob("imgbatch_cache_release_conflict")
	cache := &batchImageSubscriptionCacheInvalidatorStub{}
	repo := &fakeBatchImageBillingRepo{releaseErr: ErrUsageBillingRequestConflict}

	err := releaseBatchImageBalanceHold(context.Background(), repo, cache, job, "payload")

	require.NoError(t, err)
	require.Len(t, cache.calls, 1)
}
