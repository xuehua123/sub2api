//go:build unit

package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

type batchImageUsageLogRepoStub struct {
	UsageLogRepository
	lastLog         *UsageLog
	createErr       error
	bestEffortErr   error
	createCalls     int
	bestEffortCalls int
}

func (s *batchImageUsageLogRepoStub) Create(_ context.Context, log *UsageLog) (bool, error) {
	s.createCalls++
	s.lastLog = log
	return s.createErr == nil, s.createErr
}

func (s *batchImageUsageLogRepoStub) CreateBestEffort(_ context.Context, log *UsageLog) error {
	s.bestEffortCalls++
	s.lastLog = log
	return s.bestEffortErr
}

func TestBatchImageSettlementService_RecordsPersistedBillingIdentity(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_usage_identity")
	groupID := int64(7)
	subscriptionID := int64(41)
	entitlementID := int64(51)
	job.GroupID = &groupID
	job.SubscriptionID = &subscriptionID
	job.EntitlementID = &entitlementID
	job.BillingType = BillingTypeSubscription
	job.BillingSource = BillingSourceEntitlementBalanceFallback
	job.EntitlementBalanceFallback = true
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{}
	usageLogs := &batchImageUsageLogRepoStub{}
	svc := &BatchImageSettlementService{
		Repo:         repo,
		BillingRepo:  billing,
		UsageLogRepo: usageLogs,
		Pricing:      &fakeBatchImagePricingResolver{unitPrice: 0.25},
	}

	_, err := svc.Settle(context.Background(), job.BatchID)

	require.NoError(t, err)
	require.NotNil(t, usageLogs.lastLog)
	require.Equal(t, &groupID, usageLogs.lastLog.GroupID)
	require.Equal(t, &subscriptionID, usageLogs.lastLog.SubscriptionID)
	require.Equal(t, &entitlementID, usageLogs.lastLog.EntitlementID)
	require.Equal(t, BillingTypeSubscription, usageLogs.lastLog.BillingType)
	require.NotNil(t, usageLogs.lastLog.BillingSource)
	require.Equal(t, BillingSourceEntitlementBalanceFallback, *usageLogs.lastLog.BillingSource)
}

func TestBatchImageSettlementService_UsageLogFailurePersistsCaptureForRetry(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_usage_retry")
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{}
	usageLogs := &batchImageUsageLogRepoStub{
		bestEffortErr: errors.New("best-effort unavailable"),
		createErr:     errors.New("sync fallback unavailable"),
	}
	svc := &BatchImageSettlementService{
		Repo:         repo,
		BillingRepo:  billing,
		UsageLogRepo: usageLogs,
		Pricing:      &fakeBatchImagePricingResolver{unitPrice: 0.25},
	}

	_, err := svc.Settle(context.Background(), job.BatchID)

	require.ErrorIs(t, err, ErrBatchImageSettlementBillingFailed)
	require.Equal(t, BatchImageJobStatusSettling, repo.jobs[job.BatchID].Status)
	require.NotNil(t, repo.jobs[job.BatchID].ActualCost)
	require.InDelta(t, 0.5, *repo.jobs[job.BatchID].ActualCost, 1e-12)
	require.Len(t, billing.captures, 1)
	require.Empty(t, billing.releases)
	require.Equal(t, 1, usageLogs.bestEffortCalls)
	require.Equal(t, 1, usageLogs.createCalls)
}

func TestBatchImageSettlementService_UsageLogRetryDoesNotCaptureAgain(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_usage_retry_recovers")
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{}
	usageLogs := &batchImageUsageLogRepoStub{
		bestEffortErr: errors.New("best-effort unavailable"),
		createErr:     errors.New("sync fallback unavailable"),
	}
	pricing := &fakeBatchImagePricingResolver{unitPrice: 0.25}
	svc := &BatchImageSettlementService{
		Repo:         repo,
		BillingRepo:  billing,
		UsageLogRepo: usageLogs,
		Pricing:      pricing,
	}

	_, err := svc.Settle(context.Background(), job.BatchID)
	require.ErrorIs(t, err, ErrBatchImageSettlementBillingFailed)
	require.Equal(t, BatchImageJobStatusSettling, repo.jobs[job.BatchID].Status)
	require.Len(t, billing.captures, 1)

	usageLogs.bestEffortErr = nil
	usageLogs.createErr = nil
	svc.BillingRepo = nil
	svc.Pricing = nil
	result, err := svc.Settle(context.Background(), job.BatchID)

	require.NoError(t, err)
	require.Equal(t, 0.5, result.ActualCost)
	require.Equal(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	require.Len(t, billing.captures, 1)
	require.Empty(t, billing.releases)
	require.Equal(t, 1, pricing.calls)
	require.Equal(t, 2, usageLogs.bestEffortCalls)
	require.Equal(t, 1, usageLogs.createCalls)
}

func TestBatchImageSettlementService_UsageLogRetryExhaustionCompletesWithoutRelease(t *testing.T) {
	repo := newFakeBatchImageRepository()
	job := testSettlingBatchImageJob("imgbatch_usage_retry_exhausted")
	job.RetryCount = batchImageSettlementMaxRetries - 1
	repo.jobs[job.BatchID] = job
	billing := &fakeBatchImageBillingRepo{}
	usageLogs := &batchImageUsageLogRepoStub{
		bestEffortErr: errors.New("best-effort unavailable"),
		createErr:     errors.New("sync fallback unavailable"),
	}
	svc := &BatchImageSettlementService{
		Repo:         repo,
		BillingRepo:  billing,
		UsageLogRepo: usageLogs,
		Pricing:      &fakeBatchImagePricingResolver{unitPrice: 0.25},
	}

	result, err := svc.Settle(context.Background(), job.BatchID)

	require.NoError(t, err)
	require.Equal(t, 0.5, result.ActualCost)
	require.Equal(t, BatchImageJobStatusCompleted, repo.jobs[job.BatchID].Status)
	require.Equal(t, batchImageSettlementMaxRetries, repo.jobs[job.BatchID].RetryCount)
	require.Equal(t, "SETTLEMENT_USAGE_LOG_FAILED_AFTER_CAPTURE", batchImageDerefString(repo.jobs[job.BatchID].LastErrorCode))
	require.Len(t, billing.captures, 1)
	require.Empty(t, billing.releases)
	require.Equal(t, 1, usageLogs.bestEffortCalls)
	require.Equal(t, 1, usageLogs.createCalls)
}
