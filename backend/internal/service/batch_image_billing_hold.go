package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"go.uber.org/zap"
)

const (
	batchImageHoldRequestPrefix                  = "batch_image_hold:"
	batchImageCaptureRequestPrefix               = "batch_image_capture:"
	batchImageReleaseRequestPrefix               = "batch_image_release:"
	batchImageSubscriptionCacheInvalidateTimeout = 5 * time.Second
)

type BatchImageSubscriptionCacheInvalidator interface {
	InvalidateSubscription(context.Context, int64, int64) error
}

func BatchImageHoldRequestID(batchID string) string {
	return batchImageHoldRequestPrefix + strings.TrimSpace(batchID)
}

func BatchImageCaptureRequestID(batchID string) string {
	return batchImageCaptureRequestPrefix + strings.TrimSpace(batchID)
}

func BatchImageReleaseRequestID(batchID string) string {
	return batchImageReleaseRequestPrefix + strings.TrimSpace(batchID)
}

func buildBatchImageHoldCommand(job *BatchImageJob, requestID string, actualAmount float64, payloadHash string) (*BatchImageBalanceHoldCommand, error) {
	if job == nil {
		return nil, ErrBatchImageBillingHoldFailed
	}
	if job.APIKeyID == nil || *job.APIKeyID <= 0 {
		return nil, ErrBatchImageSettlementMissingAPIKeyID
	}
	holdAmount := job.EstimatedCost
	if job.HoldAmount != nil {
		holdAmount = *job.HoldAmount
	}
	if holdAmount < 0 {
		holdAmount = 0
	}
	if actualAmount < 0 {
		actualAmount = 0
	}
	return &BatchImageBalanceHoldCommand{
		RequestID:                  requestID,
		APIKeyID:                   *job.APIKeyID,
		UserID:                     job.UserID,
		BatchID:                    job.BatchID,
		GroupID:                    job.GroupID,
		SubscriptionID:             job.SubscriptionID,
		EntitlementID:              job.EntitlementID,
		BillingType:                job.BillingType,
		BillingSource:              job.BillingSource,
		EntitlementBalanceFallback: job.EntitlementBalanceFallback,
		HeldDailyWindowStart:       job.HeldDailyWindowStart,
		HeldWeeklyWindowStart:      job.HeldWeeklyWindowStart,
		HeldMonthlyWindowStart:     job.HeldMonthlyWindowStart,
		HoldAmount:                 holdAmount,
		ActualAmount:               actualAmount,
		RequestPayloadHash:         strings.TrimSpace(payloadHash),
	}, nil
}

func reserveBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob, payloadHash string) error {
	if repo == nil {
		return ErrBatchImageBillingHoldFailed.WithCause(errors.New("batch image billing repository is not configured"))
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageHoldRequestID(job.BatchID), 0, payloadHash)
	if err != nil {
		return err
	}
	// Reservation determines the effective billing source and quota windows. Keep
	// its idempotency fingerprint based on the immutable pre-reservation intent.
	cmd.BillingSource = ResolveUsageBillingSource(cmd.BillingType, cmd.SubscriptionID, cmd.EntitlementID, false)
	cmd.HeldDailyWindowStart = nil
	cmd.HeldWeeklyWindowStart = nil
	cmd.HeldMonthlyWindowStart = nil
	cmd.Normalize()
	if cmd.HoldAmount <= 0 {
		return nil
	}
	result, err := repo.ReserveBatchImageBalance(ctx, cmd)
	if err != nil {
		if isBatchImageBillingDomainError(err) {
			return err
		}
		return ErrBatchImageBillingHoldFailed.WithCause(err)
	}
	if result != nil {
		if strings.TrimSpace(result.BillingSource) != "" {
			job.BillingSource = strings.TrimSpace(result.BillingSource)
		}
		if result.Applied {
			job.HeldDailyWindowStart = result.HeldDailyWindowStart
			job.HeldWeeklyWindowStart = result.HeldWeeklyWindowStart
			job.HeldMonthlyWindowStart = result.HeldMonthlyWindowStart
		}
	}
	bestEffortInvalidateBatchImageCompatibilitySubscription(ctx, cache, job, "reserve")
	return nil
}

func isBatchImageBillingDomainError(err error) bool {
	known := []error{
		ErrBatchImageInsufficientBalance,
		ErrSubscriptionNotFound,
		ErrSubscriptionExpired,
		ErrSubscriptionSuspended,
		ErrDailyLimitExceeded,
		ErrWeeklyLimitExceeded,
		ErrMonthlyLimitExceeded,
		ErrSubscriptionEntitlementNotFound,
		ErrSubscriptionEntitlementInactive,
		ErrSubscriptionEntitlementExpired,
		ErrSubscriptionEntitlementQuotaExceeded,
	}
	for _, target := range known {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func captureBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob, actualAmount float64, payloadHash string) error {
	if repo == nil {
		return ErrBatchImageSettlementBillingFailed.WithCause(errors.New("batch image billing repository is not configured"))
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageCaptureRequestID(job.BatchID), actualAmount, payloadHash)
	if err != nil {
		return err
	}
	if _, err := repo.CaptureBatchImageBalance(ctx, cmd); err != nil {
		return ErrBatchImageSettlementBillingFailed.WithCause(err)
	}
	bestEffortInvalidateBatchImageCompatibilitySubscription(ctx, cache, job, "capture")
	return nil
}

func releaseBatchImageBalanceHold(ctx context.Context, repo UsageBillingRepository, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob, payloadHash string) error {
	if repo == nil || job == nil {
		return nil
	}
	cmd, err := buildBatchImageHoldCommand(job, BatchImageReleaseRequestID(job.BatchID), 0, payloadHash)
	if err != nil {
		return err
	}
	if cmd.HoldAmount <= 0 {
		return nil
	}
	if _, err := repo.ReleaseBatchImageBalance(ctx, cmd); err != nil {
		// 同一 release request id 出现指纹冲突，说明此前已有一次携带不同
		// payloadHash 的释放成功提交（资金已归还）。视为幂等成功，
		// 避免历史指纹不一致的 job 永远卡在释放失败的毒消息循环里。
		if errors.Is(err, ErrUsageBillingRequestConflict) {
			logger.L().Warn("batch_image.release_fingerprint_conflict_treated_as_released",
				zap.String("batch_id", job.BatchID),
			)
			bestEffortInvalidateBatchImageCompatibilitySubscription(ctx, cache, job, "release")
			return nil
		}
		return ErrBatchImageBillingHoldFailed.WithCause(err)
	}
	bestEffortInvalidateBatchImageCompatibilitySubscription(ctx, cache, job, "release")
	return nil
}

func bestEffortInvalidateBatchImageCompatibilitySubscription(ctx context.Context, cache BatchImageSubscriptionCacheInvalidator, job *BatchImageJob, action string) {
	if cache == nil || job == nil || job.UserID <= 0 || job.GroupID == nil || *job.GroupID <= 0 {
		return
	}
	mutatedLegacySubscription := job.BillingSource == BillingSourceLegacySubscription ||
		(job.BillingSource == BillingSourceEntitlementQuota && job.SubscriptionID != nil && *job.SubscriptionID > 0 && job.EntitlementID != nil && *job.EntitlementID > 0)
	if !mutatedLegacySubscription {
		return
	}
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), batchImageSubscriptionCacheInvalidateTimeout)
	defer cancel()
	if err := cache.InvalidateSubscription(cacheCtx, job.UserID, *job.GroupID); err != nil {
		logger.L().Warn("batch_image.subscription_cache_invalidation_failed",
			zap.String("action", action),
			zap.String("batch_id", job.BatchID),
			zap.Int64("user_id", job.UserID),
			zap.Int64("group_id", *job.GroupID),
			zap.Error(err),
		)
	}
}
