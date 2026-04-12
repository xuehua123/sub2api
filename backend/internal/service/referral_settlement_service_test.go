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
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{ID: 10, UserID: 100, Provider: "provider", ExternalOrderID: "order-1", PaidAmount: 100}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Len(t, settled, 1)
	require.Equal(t, CommissionRewardStatusAvailable, settled[0].Status)
	require.Len(t, commissionRepo.ledgers, 2)
	require.Equal(t, -10.0, commissionRepo.ledgers[0].Amount)
	require.Equal(t, 10.0, commissionRepo.ledgers[1].Amount)
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
	}
	rechargeRepo := newRechargeOrderRepoStub()
	rechargeRepo.orders["provider::order-1"] = &RechargeOrder{
		ID:             10,
		UserID:         100,
		Provider:       "provider",
		ExternalOrderID: "order-1",
		PaidAmount:     100,
		RefundedAmount: 20,
	}

	svc := NewReferralSettlementService(commissionRepo, rechargeRepo, nil)
	settled, err := svc.SettlePendingRewards(context.Background(), time.Now())
	require.NoError(t, err)
	require.Empty(t, settled)
	require.Empty(t, commissionRepo.ledgers)
	require.Equal(t, CommissionRewardStatusPending, commissionRepo.rewards[0].Status)
}
