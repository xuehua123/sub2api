package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

type ReferralSettlementService struct {
	commissionRepo CommissionRepository
	rechargeRepo   RechargeOrderRepository
	entClient      *dbent.Client
}

func NewReferralSettlementService(commissionRepo CommissionRepository, rechargeRepo RechargeOrderRepository, entClient *dbent.Client) *ReferralSettlementService {
	return &ReferralSettlementService{
		commissionRepo: commissionRepo,
		rechargeRepo:   rechargeRepo,
		entClient:      entClient,
	}
}

func (s *ReferralSettlementService) SettlePendingRewards(ctx context.Context, readyAt time.Time) ([]CommissionReward, error) {
	var settled []CommissionReward

	apply := func(txCtx context.Context) error {
		rewards, err := s.commissionRepo.ListPendingRewardsReady(txCtx, readyAt)
		if err != nil {
			return err
		}

		settled = make([]CommissionReward, 0, len(rewards))
		for i := range rewards {
			reward := rewards[i]
			hasRisk, err := s.rechargeRepo.HasRefundOrChargeback(txCtx, reward.RechargeOrderID)
			if err != nil {
				return err
			}
			if hasRisk {
				continue
			}
			if err := s.applySettlement(txCtx, &reward); err != nil {
				return err
			}
			settled = append(settled, reward)
		}
		return nil
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		if err := apply(ctx); err != nil {
			return nil, err
		}
		return settled, nil
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := apply(dbent.NewTxContext(ctx, tx)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return settled, nil
}

func (s *ReferralSettlementService) applySettlement(ctx context.Context, reward *CommissionReward) error {
	now := time.Now()
	pendingAmount, err := s.commissionRepo.SumRewardBucketAmountForUpdate(ctx, reward.ID, CommissionLedgerBucketPending, true)
	if err != nil {
		return err
	}
	pendingAmount = roundMoney(pendingAmount)
	if pendingAmount <= 0 {
		return nil
	}

	ledgers := []CommissionLedger{
		{
			UserID:          reward.UserID,
			RewardID:        int64ValuePtr(reward.ID),
			RechargeOrderID: int64ValuePtr(reward.RechargeOrderID),
			EntryType:       CommissionLedgerEntryRewardPendingToAvailable,
			Bucket:          CommissionLedgerBucketPending,
			Amount:          -pendingAmount,
			Currency:        reward.Currency,
		},
		{
			UserID:          reward.UserID,
			RewardID:        int64ValuePtr(reward.ID),
			RechargeOrderID: int64ValuePtr(reward.RechargeOrderID),
			EntryType:       CommissionLedgerEntryRewardPendingToAvailable,
			Bucket:          CommissionLedgerBucketAvailable,
			Amount:          pendingAmount,
			Currency:        reward.Currency,
		},
	}

	if err := s.commissionRepo.CreateLedgerEntries(ctx, ledgers); err != nil {
		return err
	}
	reward.Status = CommissionRewardStatusAvailable
	reward.UpdatedAt = now
	return s.commissionRepo.UpdateReward(ctx, reward)
}
