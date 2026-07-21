//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestReferralSettlementService_SettlePendingRewards(t *testing.T) {
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
				AvailableAt:     timeValuePtr(time.Now().Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{
				RewardID: int64ValuePtr(1),
				Bucket:   CommissionLedgerBucketPending,
				Amount:   10,
				Currency: ReferralSettlementCurrencyCNY,
			},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{ID: 10, UserID: 100, Provider: "provider", ExternalOrderID: "order-1", PaidAmount: 100}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, settled[0].Status)
	require.Len(t, commissionRepo.ledgers, 3)
	require.Equal(t, -10.0, commissionRepo.ledgers[1].Amount)
	require.Equal(t, 10.0, commissionRepo.ledgers[2].Amount)
}

func TestReferralSettlementService_SettlesRemainingAfterPartialRefund(t *testing.T) {
	// Partial refund must NOT permanently block remaining pending commission.
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
				AvailableAt:     timeValuePtr(time.Now().Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{
				RewardID: int64ValuePtr(1),
				Bucket:   CommissionLedgerBucketPending,
				Amount:   10,
				Currency: ReferralSettlementCurrencyCNY,
			},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID:              10,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-1",
		PaidAmount:      100,
		RefundedAmount:  20,
		Status:          RechargeOrderStatusPartiallyRefunded,
	}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, commissionRepo.rewards[0].Status)
}

func TestReferralSettlementService_BlocksFullyRefundedOrders(t *testing.T) {
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
				AvailableAt:     timeValuePtr(time.Now().Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{
				RewardID: int64ValuePtr(1),
				Bucket:   CommissionLedgerBucketPending,
				Amount:   10,
				Currency: ReferralSettlementCurrencyCNY,
			},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID:              10,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-1",
		PaidAmount:      100,
		RefundedAmount:  100,
		Status:          RechargeOrderStatusRefunded,
	}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Empty(t, settled)
	// Pending residual is reversed so it no longer appears as "待生效".
	require.Equal(t, CommissionRewardStatusSettlementBlocked, commissionRepo.rewards[0].Status)
	require.GreaterOrEqual(t, len(commissionRepo.ledgers), 2)
	require.Equal(t, CommissionLedgerEntryRefundReverse, commissionRepo.ledgers[len(commissionRepo.ledgers)-1].EntryType)
	require.Equal(t, CommissionLedgerBucketPending, commissionRepo.ledgers[len(commissionRepo.ledgers)-1].Bucket)
	require.Equal(t, -10.0, commissionRepo.ledgers[len(commissionRepo.ledgers)-1].Amount)
}

func TestReferralSettlementService_SettlesRemainingPendingAmountAfterPartialReversal(t *testing.T) {
	now := time.Now()
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPartiallyReversed,
				AvailableAt:     timeValuePtr(now.Add(-time.Hour)),
				ReversedAt:      timeValuePtr(now.Add(-30 * time.Minute)),
			},
		},
		ledgers: []CommissionLedger{
			{
				RewardID: int64ValuePtr(1),
				Bucket:   CommissionLedgerBucketPending,
				Amount:   10,
				Currency: ReferralSettlementCurrencyCNY,
			},
			{
				RewardID: int64ValuePtr(1),
				Bucket:   CommissionLedgerBucketPending,
				Amount:   -4,
				Currency: ReferralSettlementCurrencyCNY,
			},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{ID: 10, UserID: 100, Provider: "provider", ExternalOrderID: "order-1", PaidAmount: 100}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, settled[0].Status)
	require.Len(t, commissionRepo.ledgers, 4)
	require.Equal(t, -6.0, commissionRepo.ledgers[2].Amount)
	require.Equal(t, 6.0, commissionRepo.ledgers[3].Amount)
}

func TestReferralSettlementService_SkipsRefundPendingWithoutPermanentBlock(t *testing.T) {
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID: 1, UserID: 200, RechargeOrderID: 10, RewardAmount: 10,
				Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending,
				AvailableAt: timeValuePtr(time.Now().Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{RewardID: int64ValuePtr(1), Bucket: CommissionLedgerBucketPending, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID: 10, UserID: 100, Provider: "provider", ExternalOrderID: "order-1",
		PaidAmount: 100, Status: RechargeOrderStatusRefundPending,
	}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Empty(t, settled)
	require.Equal(t, CommissionRewardStatusPending, commissionRepo.rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 1)
}

func TestReferralSettlementService_GetByIDFailureAbortsWithoutBlocking(t *testing.T) {
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID: 1, UserID: 200, RechargeOrderID: 999, RewardAmount: 10,
				Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending,
				AvailableAt: timeValuePtr(time.Now().Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{RewardID: int64ValuePtr(1), Bucket: CommissionLedgerBucketPending, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.Error(t, err)
	require.Empty(t, settled)
	require.Equal(t, CommissionRewardStatusPending, commissionRepo.rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 1)
}

func TestReferralSettlementService_SettlePendingRewardsForUserOnlyTouchesThatUser(t *testing.T) {
	now := time.Now()
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID: 1, UserID: 200, RechargeOrderID: 10, RewardAmount: 10,
				Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending,
				AvailableAt: timeValuePtr(now.Add(-time.Hour)),
			},
			{
				ID: 2, UserID: 300, RechargeOrderID: 11, RewardAmount: 20,
				Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending,
				AvailableAt: timeValuePtr(now.Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{RewardID: int64ValuePtr(1), Bucket: CommissionLedgerBucketPending, Amount: 10, Currency: ReferralSettlementCurrencyCNY},
			{RewardID: int64ValuePtr(2), Bucket: CommissionLedgerBucketPending, Amount: 20, Currency: ReferralSettlementCurrencyCNY},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["p::a"] = &RechargeOrder{ID: 10, UserID: 100, Provider: "p", ExternalOrderID: "a", PaidAmount: 100}
	rechargeRepo.orders["p::b"] = &RechargeOrder{ID: 11, UserID: 101, Provider: "p", ExternalOrderID: "b", PaidAmount: 200}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewardsForUser(context.Background(), 200, now)
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, int64(200), settled[0].UserID)
	require.Equal(t, CommissionRewardStatusAvailable, commissionRepo.rewards[0].Status)
	require.Equal(t, CommissionRewardStatusPending, commissionRepo.rewards[1].Status)
}

func TestReferralSettlementService_RefundReverseDisabledSettlesInsteadOfBlocking(t *testing.T) {
	// When ReferralRefundReverseEnabled=false, full refund must not write refund_reverse
	// or settlement_blocked via the background/global settle path.
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
				AvailableAt:     timeValuePtr(time.Now().Add(-time.Hour)),
			},
		},
		ledgers: []CommissionLedger{
			{
				RewardID: int64ValuePtr(1),
				Bucket:   CommissionLedgerBucketPending,
				Amount:   10,
				Currency: ReferralSettlementCurrencyCNY,
			},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID:              10,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-1",
		PaidAmount:      100,
		RefundedAmount:  100,
		Status:          RechargeOrderStatusRefunded,
	}
	settings := NewSettingService(&settingRepoStub{values: map[string]string{
		SettingKeyReferralRefundReverseEnabled: "false",
	}}, &config.Config{
		Default: config.DefaultConfig{UserBalance: 0, UserConcurrency: 1},
	})

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, settings)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, commissionRepo.rewards[0].Status)
	for _, ledger := range commissionRepo.ledgers {
		require.NotEqual(t, CommissionLedgerEntryRefundReverse, ledger.EntryType)
	}
}

func TestReferralSettlementService_BatchesGlobalSettle(t *testing.T) {
	now := time.Now()
	// Three ready rewards; force batch size via afterID paging in the stub with limit.
	// settleOneBatch uses referralSettlementBatchSize (100); build >0 and verify keyset
	// advances by settling all with distinct IDs in one call (multiple internal batches
	// only kick in above batch size — here we just assert multi-reward settle works).
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{ID: 1, UserID: 200, RechargeOrderID: 10, RewardAmount: 1, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending, AvailableAt: timeValuePtr(now.Add(-time.Hour))},
			{ID: 2, UserID: 201, RechargeOrderID: 11, RewardAmount: 2, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending, AvailableAt: timeValuePtr(now.Add(-time.Hour))},
			{ID: 3, UserID: 202, RechargeOrderID: 12, RewardAmount: 3, Currency: ReferralSettlementCurrencyCNY, Status: CommissionRewardStatusPending, AvailableAt: timeValuePtr(now.Add(-time.Hour))},
		},
		ledgers: []CommissionLedger{
			{RewardID: int64ValuePtr(1), Bucket: CommissionLedgerBucketPending, Amount: 1, Currency: ReferralSettlementCurrencyCNY},
			{RewardID: int64ValuePtr(2), Bucket: CommissionLedgerBucketPending, Amount: 2, Currency: ReferralSettlementCurrencyCNY},
			{RewardID: int64ValuePtr(3), Bucket: CommissionLedgerBucketPending, Amount: 3, Currency: ReferralSettlementCurrencyCNY},
		},
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["p::a"] = &RechargeOrder{ID: 10, UserID: 100, Provider: "p", ExternalOrderID: "a", PaidAmount: 100}
	rechargeRepo.orders["p::b"] = &RechargeOrder{ID: 11, UserID: 101, Provider: "p", ExternalOrderID: "b", PaidAmount: 100}
	rechargeRepo.orders["p::c"] = &RechargeOrder{ID: 12, UserID: 102, Provider: "p", ExternalOrderID: "c", PaidAmount: 100}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, settled, 3)
	for i := range commissionRepo.rewards {
		require.Equal(t, CommissionRewardStatusAvailable, commissionRepo.rewards[i].Status)
	}
}
