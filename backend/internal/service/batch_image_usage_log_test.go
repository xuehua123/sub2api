//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type batchImageUsageLogRepoStub struct {
	UsageLogRepository
	lastLog *UsageLog
}

func (s *batchImageUsageLogRepoStub) Create(_ context.Context, log *UsageLog) (bool, error) {
	s.lastLog = log
	return true, nil
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
