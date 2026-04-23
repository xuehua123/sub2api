//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReferralRefundService_RefundsPendingReward(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID:              10,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-1",
		PaidAmount:      100,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              1,
				UserID:          200,
				RechargeOrderID: 10,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPending,
			},
		},
	}

	svc := NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil)
	order, rewards, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{
		RechargeOrderID: 10,
		RefundedAmount:  100,
	})
	require.NoError(t, err)
	require.Equal(t, RechargeOrderStatusRefunded, order.Status)
	require.Len(t, rewards, 1)
	require.Equal(t, CommissionRewardStatusReversed, rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionLedgerBucketPending, commissionRepo.ledgers[0].Bucket)
	require.Equal(t, -10.0, commissionRepo.ledgers[0].Amount)
}

func TestReferralRefundService_CreatesNegativeCarryForPaidReward(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-2"] = &RechargeOrder{
		ID:              11,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-2",
		PaidAmount:      100,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              2,
				UserID:          200,
				RechargeOrderID: 11,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPaid,
			},
		},
	}

	svc := NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil)
	_, rewards, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{
		RechargeOrderID: 11,
		RefundedAmount:  50,
	})
	require.NoError(t, err)
	require.Len(t, rewards, 1)
	require.Equal(t, CommissionRewardStatusPartiallyReversed, rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 1)
	require.Equal(t, CommissionLedgerEntryNegativeCarry, commissionRepo.ledgers[0].EntryType)
	require.Equal(t, CommissionLedgerBucketAvailable, commissionRepo.ledgers[0].Bucket)
	require.Equal(t, -5.0, commissionRepo.ledgers[0].Amount)
}

func TestReferralRefundService_ReversesMixedAvailableAndFrozenBuckets(t *testing.T) {
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-3"] = &RechargeOrder{
		ID:              12,
		UserID:          100,
		Provider:        "provider",
		ExternalOrderID: "order-3",
		PaidAmount:      100,
		Currency:        ReferralSettlementCurrencyCNY,
		Status:          RechargeOrderStatusCredited,
	}
	rewardID := int64(3)
	commissionRepo := &commissionRepoStub{
		rewards: []CommissionReward{
			{
				ID:              rewardID,
				UserID:          200,
				RechargeOrderID: 12,
				RewardAmount:    10,
				Currency:        ReferralSettlementCurrencyCNY,
				Status:          CommissionRewardStatusPartiallyFrozen,
			},
		},
		ledgers: []CommissionLedger{
			{
				UserID:          200,
				RewardID:        int64ValuePtr(rewardID),
				RechargeOrderID: int64ValuePtr(12),
				EntryType:       CommissionLedgerEntryRewardPendingToAvailable,
				Bucket:          CommissionLedgerBucketAvailable,
				Amount:          6,
				Currency:        ReferralSettlementCurrencyCNY,
			},
			{
				UserID:          200,
				RewardID:        int64ValuePtr(rewardID),
				RechargeOrderID: int64ValuePtr(12),
				EntryType:       CommissionLedgerEntryWithdrawFreeze,
				Bucket:          CommissionLedgerBucketFrozen,
				Amount:          4,
				Currency:        ReferralSettlementCurrencyCNY,
			},
		},
	}

	svc := NewReferralRefundService(rechargeRepo, commissionRepo, nil, nil)
	_, rewards, err := svc.ApplyRefund(context.Background(), &RechargeRefundInput{
		RechargeOrderID: 12,
		RefundedAmount:  100,
	})
	require.NoError(t, err)
	require.Len(t, rewards, 1)
	require.Equal(t, CommissionRewardStatusReversed, rewards[0].Status)
	require.Len(t, commissionRepo.ledgers, 4)
	require.Equal(t, CommissionLedgerBucketAvailable, commissionRepo.ledgers[2].Bucket)
	require.Equal(t, -6.0, commissionRepo.ledgers[2].Amount)
	require.Equal(t, CommissionLedgerBucketFrozen, commissionRepo.ledgers[3].Bucket)
	require.Equal(t, -4.0, commissionRepo.ledgers[3].Amount)
}
