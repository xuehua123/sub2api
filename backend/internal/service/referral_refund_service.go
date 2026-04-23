package service

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

type RechargeRefundInput struct {
	RechargeOrderID  int64
	RefundedAmount   float64
	ChargebackAmount float64
}

type ReferralRefundService struct {
	rechargeRepo   RechargeOrderRepository
	commissionRepo CommissionRepository
	entClient      *dbent.Client
	settingService *SettingService
}

func NewReferralRefundService(rechargeRepo RechargeOrderRepository, commissionRepo CommissionRepository, entClient *dbent.Client, settingService *SettingService) *ReferralRefundService {
	return &ReferralRefundService{
		rechargeRepo:   rechargeRepo,
		commissionRepo: commissionRepo,
		entClient:      entClient,
		settingService: settingService,
	}
}

func (s *ReferralRefundService) ApplyRefund(ctx context.Context, input *RechargeRefundInput) (*RechargeOrder, []CommissionReward, error) {
	if input == nil || input.RechargeOrderID <= 0 {
		return nil, nil, infraerrors.BadRequest("RECHARGE_REFUND_INVALID", "invalid refund request")
	}

	// Load settings and check refund reversal flags
	settings, err := s.loadSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !settings.ReferralRefundReverseEnabled {
		// Refund reversal is disabled globally; still update the order status but skip commission reversal
		order, err := s.rechargeRepo.GetByID(ctx, input.RechargeOrderID)
		if err != nil {
			return nil, nil, err
		}
		now := time.Now()
		order.RefundedAmount = input.RefundedAmount
		order.ChargebackAmount = input.ChargebackAmount
		if input.ChargebackAmount > 0 {
			order.Status = RechargeOrderStatusChargeback
			order.ChargebackAt = &now
		} else if input.RefundedAmount >= order.PaidAmount {
			order.Status = RechargeOrderStatusRefunded
			order.RefundedAt = &now
		} else {
			order.Status = RechargeOrderStatusPartiallyRefunded
			order.RefundedAt = &now
		}
		if err := s.rechargeRepo.Update(ctx, order); err != nil {
			return nil, nil, err
		}
		rewards, listErr := s.commissionRepo.ListRewardsByRechargeOrder(ctx, order.ID)
		return order, rewards, listErr
	}

	order, err := s.rechargeRepo.GetByID(ctx, input.RechargeOrderID)
	if err != nil {
		return nil, nil, err
	}

	existingTotal := order.RefundedAmount + order.ChargebackAmount
	newTotal := input.RefundedAmount + input.ChargebackAmount
	delta := newTotal - existingTotal
	if delta <= 0 {
		rewards, listErr := s.commissionRepo.ListRewardsByRechargeOrder(ctx, order.ID)
		return order, rewards, listErr
	}

	negativeCarryEnabled := settings.ReferralNegativeCarryEnabled

	apply := func(txCtx context.Context) ([]CommissionReward, error) {
		now := time.Now()
		order.RefundedAmount = input.RefundedAmount
		order.ChargebackAmount = input.ChargebackAmount
		if input.ChargebackAmount > 0 {
			order.Status = RechargeOrderStatusChargeback
			order.ChargebackAt = &now
		} else if input.RefundedAmount >= order.PaidAmount {
			order.Status = RechargeOrderStatusRefunded
			order.RefundedAt = &now
		} else {
			order.Status = RechargeOrderStatusPartiallyRefunded
			order.RefundedAt = &now
		}
		if err := s.rechargeRepo.Update(txCtx, order); err != nil {
			return nil, err
		}

		rewards, err := s.commissionRepo.ListRewardsByRechargeOrder(txCtx, order.ID)
		if err != nil {
			return nil, err
		}
		ratio := delta / order.PaidAmount
		if ratio > 1 {
			ratio = 1
		}
		for i := range rewards {
			reward := &rewards[i]
			reverseAmount := roundMoney(reward.RewardAmount * ratio)
			if reverseAmount <= 0 {
				continue
			}

			ledgers, appliedAmount, err := s.buildRefundReversalLedgers(txCtx, reward, reverseAmount, negativeCarryEnabled)
			if err != nil {
				return nil, err
			}
			if appliedAmount <= 0 || len(ledgers) == 0 {
				continue
			}
			if err := s.commissionRepo.CreateLedgerEntries(txCtx, ledgers); err != nil {
				return nil, err
			}
			if input.RefundedAmount+input.ChargebackAmount >= order.PaidAmount && reverseAmount-appliedAmount <= amountToleranceCNY {
				reward.Status = CommissionRewardStatusReversed
			} else {
				reward.Status = CommissionRewardStatusPartiallyReversed
			}
			reward.ReversedAt = &now
			if err := s.commissionRepo.UpdateReward(txCtx, reward); err != nil {
				return nil, err
			}
		}
		return rewards, nil
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		rewards, err := apply(ctx)
		return order, rewards, err
	}

	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rewards, err := apply(dbent.NewTxContext(ctx, tx))
	if err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return order, rewards, nil
}

func (s *ReferralRefundService) buildRefundReversalLedgers(ctx context.Context, reward *CommissionReward, reverseAmount float64, negativeCarryEnabled bool) ([]CommissionLedger, float64, error) {
	if reward == nil || reverseAmount <= 0 {
		return nil, 0, nil
	}

	remaining := roundMoney(reverseAmount)
	ledgers := make([]CommissionLedger, 0, 4)
	appendLedger := func(entryType, bucket string, amount float64) {
		amount = roundMoney(amount)
		if amount <= 0 {
			return
		}
		ledgers = append(ledgers, CommissionLedger{
			UserID:          reward.UserID,
			RewardID:        int64ValuePtr(reward.ID),
			RechargeOrderID: int64ValuePtr(reward.RechargeOrderID),
			EntryType:       entryType,
			Bucket:          bucket,
			Amount:          -amount,
			Currency:        reward.Currency,
		})
		remaining = roundMoney(remaining - amount)
	}

	for _, bucket := range []string{CommissionLedgerBucketAvailable, CommissionLedgerBucketFrozen, CommissionLedgerBucketPending} {
		if remaining <= amountToleranceCNY {
			break
		}
		balance, err := s.commissionRepo.SumRewardBucketAmountForUpdate(ctx, reward.ID, bucket, true)
		if err != nil {
			return nil, 0, err
		}
		balance = roundMoney(balance)
		if balance <= 0 {
			continue
		}
		amount := balance
		if amount > remaining {
			amount = remaining
		}
		appendLedger(CommissionLedgerEntryRefundReverse, bucket, amount)
	}

	if remaining > amountToleranceCNY && len(ledgers) == 0 {
		if fallbackBucket := refundFallbackBucketForSingleState(reward.Status); fallbackBucket != "" {
			appendLedger(CommissionLedgerEntryRefundReverse, fallbackBucket, remaining)
		}
	}

	if remaining > amountToleranceCNY && negativeCarryEnabled && rewardAllowsNegativeCarry(reward.Status) {
		settled, err := s.commissionRepo.SumRewardBucketAmountForUpdate(ctx, reward.ID, CommissionLedgerBucketSettled, true)
		if err != nil {
			return nil, 0, err
		}
		settled = roundMoney(settled)
		if settled <= 0 {
			// Legacy or test fixtures may only carry the settled state in the
			// reward status. Preserve prior behavior for paid rewards while mixed
			// non-settled balances are handled by the actual bucket totals above.
			settled = remaining
		}
		amount := settled
		if amount > remaining {
			amount = remaining
		}
		appendLedger(CommissionLedgerEntryNegativeCarry, CommissionLedgerBucketAvailable, amount)
	}

	appliedAmount := roundMoney(reverseAmount - remaining)
	return ledgers, appliedAmount, nil
}

func rewardAllowsNegativeCarry(status string) bool {
	return status == CommissionRewardStatusPaid || status == CommissionRewardStatusPartiallyPaid
}

func refundFallbackBucketForSingleState(status string) string {
	switch status {
	case CommissionRewardStatusPending:
		return CommissionLedgerBucketPending
	case CommissionRewardStatusFrozen:
		return CommissionLedgerBucketFrozen
	case CommissionRewardStatusAvailable:
		return CommissionLedgerBucketAvailable
	default:
		return ""
	}
}

func (s *ReferralRefundService) loadSettings(ctx context.Context) (*SystemSettings, error) {
	if s.settingService == nil {
		return &SystemSettings{
			ReferralRefundReverseEnabled: true,
			ReferralNegativeCarryEnabled: true,
		}, nil
	}
	return s.settingService.GetAllSettings(ctx)
}
