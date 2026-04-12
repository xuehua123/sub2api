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
