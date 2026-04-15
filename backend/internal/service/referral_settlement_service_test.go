//go:build unit

package service

import (
	"context"
	"testing"
	"time"

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

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, settled[0].Status)
	require.Len(t, commissionRepo.ledgers, 3)
	require.Equal(t, -10.0, commissionRepo.ledgers[1].Amount)
	require.Equal(t, 10.0, commissionRepo.ledgers[2].Amount)
}

func TestReferralSettlementService_SkipsRewardsWithRefundRisk(t *testing.T) {
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
	}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Empty(t, settled)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionRewardStatusPending, commissionRepo.rewards[0].Status)
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

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), now)
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, settled[0].Status)
	require.Len(t, commissionRepo.ledgers, 4)
	require.Equal(t, -6.0, commissionRepo.ledgers[2].Amount)
	require.Equal(t, 6.0, commissionRepo.ledgers[3].Amount)
}
