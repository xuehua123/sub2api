package service

import (
	"context"
	"fmt"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
)

// Settlement batching bounds so a single run never FOR UPDATE-locks the entire
// ready queue. The background runner re-enters every few minutes to drain more.
const (
	referralSettlementBatchSize        = 100
	referralSettlementUserBatchSize    = 500
	referralSettlementMaxBatchesPerRun = 10
)

type ReferralSettlementService struct {
	commissionRepo   CommissionRepository
	rechargeRepo     RechargeOrderRepository
	entClient        *dbent.Client
	settingService   *SettingService
	backgroundRunner *referralSettlementRunner
}

func NewReferralSettlementService(
	commissionRepo CommissionRepository,
	rechargeRepo RechargeOrderRepository,
	entClient *dbent.Client,
	settingService *SettingService,
) *ReferralSettlementService {
	return &ReferralSettlementService{
		commissionRepo: commissionRepo,
		rechargeRepo:   rechargeRepo,
		entClient:      entClient,
		settingService: settingService,
	}
}

func (s *ReferralSettlementService) SettlePendingRewards(ctx context.Context, readyAt time.Time) ([]CommissionReward, error) {
	return s.settlePendingRewards(ctx, readyAt, 0)
}

// SettlePendingRewardsForUser settles ready pending rewards for a single user only.
// Prefer this on user-facing read/write paths to avoid global FOR UPDATE scans.
func (s *ReferralSettlementService) SettlePendingRewardsForUser(ctx context.Context, userID int64, readyAt time.Time) ([]CommissionReward, error) {
	if userID <= 0 {
		return nil, nil
	}
	return s.settlePendingRewards(ctx, readyAt, userID)
}

func (s *ReferralSettlementService) settlePendingRewards(ctx context.Context, readyAt time.Time, userID int64) ([]CommissionReward, error) {
	refundReverseEnabled := s.refundReverseEnabled(ctx)

	batchSize := referralSettlementBatchSize
	maxBatches := referralSettlementMaxBatchesPerRun
	if userID > 0 {
		batchSize = referralSettlementUserBatchSize
		// A single user's ready queue is expected to be small; still bound work.
		maxBatches = referralSettlementMaxBatchesPerRun
	}

	var allSettled []CommissionReward
	var afterAvailableAt *time.Time
	var afterID int64
	for batch := 0; batch < maxBatches; batch++ {
		if err := ctx.Err(); err != nil {
			if len(allSettled) > 0 {
				return allSettled, nil
			}
			return nil, err
		}
		settled, lastAvailableAt, lastID, fetched, err := s.settleOneBatch(ctx, readyAt, userID, afterAvailableAt, afterID, batchSize, refundReverseEnabled)
		if err != nil {
			return allSettled, err
		}
		allSettled = append(allSettled, settled...)
		if fetched == 0 {
			break
		}
		afterAvailableAt = lastAvailableAt
		afterID = lastID
		if fetched < batchSize {
			break
		}
	}
	return allSettled, nil
}

func (s *ReferralSettlementService) settleOneBatch(
	ctx context.Context,
	readyAt time.Time,
	userID int64,
	afterAvailableAt *time.Time,
	afterID int64,
	limit int,
	refundReverseEnabled bool,
) (settled []CommissionReward, lastAvailableAt *time.Time, lastID int64, fetched int, err error) {
	lastID = afterID
	var lastAvailCopy time.Time
	hasLastAvail := afterAvailableAt != nil
	if hasLastAvail {
		lastAvailCopy = *afterAvailableAt
	}

	apply := func(txCtx context.Context) error {
		var rewards []CommissionReward
		var listErr error
		if userID > 0 {
			rewards, listErr = s.commissionRepo.ListPendingRewardsReadyForUser(txCtx, userID, readyAt, afterAvailableAt, afterID, limit)
		} else {
			rewards, listErr = s.commissionRepo.ListPendingRewardsReady(txCtx, readyAt, afterAvailableAt, afterID, limit)
		}
		if listErr != nil {
			return listErr
		}
		fetched = len(rewards)
		settled = make([]CommissionReward, 0, len(rewards))
		for i := range rewards {
			reward := rewards[i]
			// Advance keyset to the last fetched row (list is ordered by available_at, id).
			if reward.AvailableAt != nil {
				lastAvailCopy = *reward.AvailableAt
				hasLastAvail = true
			}
			lastID = reward.ID
			order, loadErr := s.rechargeRepo.GetByID(txCtx, reward.RechargeOrderID)
			if loadErr != nil {
				// Do not permanently block or settle without a known order state.
				// Abort the batch so a transient read failure cannot leave rewards
				// in settlement_blocked or incorrectly available.
				return fmt.Errorf("load recharge order %d for settlement: %w", reward.RechargeOrderID, loadErr)
			}
			// Temporary hold: refund still in progress — leave pending, no permanent block.
			if order.Status == RechargeOrderStatusRefundPending {
				continue
			}
			if isRechargeOrderFullyClosedForCommission(order) {
				if !refundReverseEnabled {
					// Admin disabled refund commission reverse: keep rewarding as configured
					// and allow normal pending → available settlement.
					if applyErr := s.applySettlement(txCtx, &reward); applyErr != nil {
						return applyErr
					}
					settled = append(settled, reward)
					continue
				}
				if blockErr := s.blockSettlement(txCtx, &reward); blockErr != nil {
					return blockErr
				}
				continue
			}
			// No refund, partial refund, or residual after reverse: settle remaining pending.
			if applyErr := s.applySettlement(txCtx, &reward); applyErr != nil {
				return applyErr
			}
			settled = append(settled, reward)
		}
		return nil
	}

	if s.entClient == nil || dbent.TxFromContext(ctx) != nil {
		if applyErr := apply(ctx); applyErr != nil {
			return settled, nil, lastID, fetched, applyErr
		}
	} else {
		tx, txErr := s.entClient.Tx(ctx)
		if txErr != nil {
			return nil, afterAvailableAt, afterID, 0, txErr
		}
		defer func() { _ = tx.Rollback() }()

		if applyErr := apply(dbent.NewTxContext(ctx, tx)); applyErr != nil {
			return settled, nil, lastID, fetched, applyErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return settled, nil, lastID, fetched, commitErr
		}
	}
	if hasLastAvail {
		lastAvailableAt = &lastAvailCopy
	}
	return settled, lastAvailableAt, lastID, fetched, nil
}

func (s *ReferralSettlementService) refundReverseEnabled(ctx context.Context) bool {
	// Match ReferralRefundService defaults: enabled when settings are unavailable
	// so production misconfig does not silently stop reverse of closed orders.
	if s == nil || s.settingService == nil {
		return true
	}
	settings, err := s.settingService.GetAllSettings(ctx)
	if err != nil || settings == nil {
		return true
	}
	return settings.ReferralRefundReverseEnabled
}

// isRechargeOrderFullyClosedForCommission reports whether the order can no longer
// produce settleable commission (full refund and/or chargeback).
func isRechargeOrderFullyClosedForCommission(order *RechargeOrder) bool {
	if order == nil {
		return false
	}
	switch order.Status {
	case RechargeOrderStatusRefunded, RechargeOrderStatusChargeback:
		return true
	case RechargeOrderStatusRefundPending:
		// Temporary — handled as skip-without-block by caller.
		return false
	}
	closedAmount := roundMoney(order.RefundedAmount + order.ChargebackAmount)
	if order.PaidAmount > 0 && closedAmount+amountToleranceCNY >= order.PaidAmount {
		return true
	}
	return false
}

// blockSettlement marks a reward non-settleable because its source order is fully
// closed (refunded/chargeback). Remaining pending/available/frozen balances are
// reversed via refund_reverse ledgers so users do not keep a phantom "pending"
// amount that can never settle or withdraw. Settled buckets are left untouched
// (already paid/converted — negative carry is the refund path's job when enabled).
func (s *ReferralSettlementService) blockSettlement(ctx context.Context, reward *CommissionReward) error {
	if reward == nil {
		return nil
	}
	now := time.Now()
	ledgers := make([]CommissionLedger, 0, 3)
	for _, bucket := range []string{
		CommissionLedgerBucketAvailable,
		CommissionLedgerBucketFrozen,
		CommissionLedgerBucketPending,
	} {
		balance, err := s.commissionRepo.SumRewardBucketAmountForUpdate(ctx, reward.ID, bucket, true)
		if err != nil {
			return err
		}
		balance = roundMoney(balance)
		if balance <= amountToleranceCNY {
			continue
		}
		ledgers = append(ledgers, CommissionLedger{
			UserID:          reward.UserID,
			RewardID:        int64ValuePtr(reward.ID),
			RechargeOrderID: int64ValuePtr(reward.RechargeOrderID),
			EntryType:       CommissionLedgerEntryRefundReverse,
			Bucket:          bucket,
			Amount:          -balance,
			Currency:        reward.Currency,
			Remark:          stringValuePtr("settlement_blocked: source order fully refunded or chargebacked"),
		})
	}
	if len(ledgers) > 0 {
		if err := s.commissionRepo.CreateLedgerEntries(ctx, ledgers); err != nil {
			return err
		}
	}
	if reward.Status == CommissionRewardStatusSettlementBlocked {
		return nil
	}
	reward.Status = CommissionRewardStatusSettlementBlocked
	reward.UpdatedAt = now
	reward.ReversedAt = timeValuePtr(now)
	return s.commissionRepo.UpdateReward(ctx, reward)
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
