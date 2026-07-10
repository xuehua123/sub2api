package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const batchImageBillingEpsilon = 0.00000001

func reserveUsageBillingBatchImage(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	var (
		result *service.BatchImageBalanceHoldResult
		err    error
	)
	switch cmd.BillingSource {
	case service.BillingSourceBalance:
		result, err = reserveUsageBillingBatchImageBalance(ctx, tx, cmd)
		if result != nil {
			result.BillingSource = service.BillingSourceBalance
		}
	case service.BillingSourceLegacySubscription:
		result, err = reserveUsageBillingBatchImageLegacySubscription(ctx, tx, cmd)
	case service.BillingSourceEntitlementQuota:
		result, err = reserveUsageBillingBatchImageEntitlement(ctx, tx, cmd)
	case service.BillingSourceEntitlementBalanceFallback:
		if !cmd.EntitlementBalanceFallback {
			return nil, service.ErrSubscriptionEntitlementQuotaExceeded
		}
		result, err = reserveUsageBillingBatchImageBalance(ctx, tx, cmd)
		if result != nil {
			result.BillingSource = service.BillingSourceEntitlementBalanceFallback
		}
	default:
		return nil, errors.New("unsupported batch image billing source")
	}
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &service.BatchImageBalanceHoldResult{BillingSource: cmd.BillingSource}
	}
	if strings.TrimSpace(result.BillingSource) == "" {
		result.BillingSource = cmd.BillingSource
	}
	if err := persistBatchImageBillingReservation(ctx, tx, cmd, result); err != nil {
		return nil, err
	}
	return result, nil
}

func captureUsageBillingBatchImage(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.ActualAmount-cmd.HoldAmount > batchImageBillingEpsilon {
		return nil, service.ErrBatchImageSettlementCostExceedsHold
	}
	switch cmd.BillingSource {
	case service.BillingSourceBalance, service.BillingSourceEntitlementBalanceFallback:
		result, err := captureUsageBillingBatchImageBalance(ctx, tx, cmd)
		if result != nil {
			result.BillingSource = cmd.BillingSource
		}
		return result, err
	case service.BillingSourceLegacySubscription:
		return settleUsageBillingBatchImageLegacySubscription(ctx, tx, cmd, cmd.HoldAmount-cmd.ActualAmount)
	case service.BillingSourceEntitlementQuota:
		return settleUsageBillingBatchImageEntitlement(ctx, tx, cmd, cmd.HoldAmount-cmd.ActualAmount)
	default:
		return nil, errors.New("unsupported batch image billing source")
	}
}

func releaseUsageBillingBatchImage(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	switch cmd.BillingSource {
	case service.BillingSourceBalance, service.BillingSourceEntitlementBalanceFallback:
		result, err := releaseUsageBillingBatchImageBalance(ctx, tx, cmd)
		if result != nil {
			result.BillingSource = cmd.BillingSource
		}
		return result, err
	case service.BillingSourceLegacySubscription:
		return settleUsageBillingBatchImageLegacySubscription(ctx, tx, cmd, cmd.HoldAmount)
	case service.BillingSourceEntitlementQuota:
		return settleUsageBillingBatchImageEntitlement(ctx, tx, cmd, cmd.HoldAmount)
	default:
		return nil, errors.New("unsupported batch image billing source")
	}
}

func persistBatchImageBillingReservation(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, result *service.BatchImageBalanceHoldResult) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE batch_image_jobs
		SET billing_source = $1,
			held_daily_window_start = $2,
			held_weekly_window_start = $3,
			held_monthly_window_start = $4,
			updated_at = NOW()
		WHERE batch_id = $5
			AND user_id = $6
			AND api_key_id = $7
			AND group_id IS NOT DISTINCT FROM $8
			AND subscription_id IS NOT DISTINCT FROM $9
			AND entitlement_id IS NOT DISTINCT FROM $10
	`,
		result.BillingSource,
		timePtrArg(result.HeldDailyWindowStart),
		timePtrArg(result.HeldWeeklyWindowStart),
		timePtrArg(result.HeldMonthlyWindowStart),
		cmd.BatchID,
		cmd.UserID,
		cmd.APIKeyID,
		cmd.GroupID,
		cmd.SubscriptionID,
		cmd.EntitlementID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrBatchImageJobNotFound
	}
	return nil
}

type batchImageLegacySubscriptionState struct {
	ID                 int64
	UserID             int64
	GroupID            int64
	Status             string
	StartsAt           time.Time
	ExpiresAt          time.Time
	DailyWindowStart   sql.NullTime
	WeeklyWindowStart  sql.NullTime
	MonthlyWindowStart sql.NullTime
	DailyUsageUSD      float64
	WeeklyUsageUSD     float64
	MonthlyUsageUSD    float64
	DailyLimitUSD      sql.NullFloat64
	WeeklyLimitUSD     sql.NullFloat64
	MonthlyLimitUSD    sql.NullFloat64
}

func reserveUsageBillingBatchImageLegacySubscription(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.SubscriptionID == nil {
		return nil, service.ErrSubscriptionNotFound
	}
	state, err := lockBatchImageLegacySubscription(ctx, tx, *cmd.SubscriptionID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if state.UserID != cmd.UserID || (cmd.GroupID != nil && state.GroupID != *cmd.GroupID) {
		return nil, service.ErrSubscriptionNotFound
	}
	if state.Status != service.SubscriptionStatusActive {
		return nil, service.ErrSubscriptionSuspended
	}
	if now.Before(state.StartsAt) || !now.Before(state.ExpiresAt) {
		return nil, service.ErrSubscriptionExpired
	}

	usage := usageBillingEntitlementWindowUsage{
		dailyWindowStart:   nullTimePtr(state.DailyWindowStart),
		weeklyWindowStart:  nullTimePtr(state.WeeklyWindowStart),
		monthlyWindowStart: nullTimePtr(state.MonthlyWindowStart),
		dailyUsageUSD:      state.DailyUsageUSD,
		weeklyUsageUSD:     state.WeeklyUsageUSD,
		monthlyUsageUSD:    state.MonthlyUsageUSD,
	}
	usage.activateAndReset(state.StartsAt, state.ExpiresAt, now)
	if usageBillingLimitExceeded(usage.dailyUsageUSD, state.DailyLimitUSD, cmd.HoldAmount) {
		return nil, service.ErrDailyLimitExceeded
	}
	if usageBillingLimitExceeded(usage.weeklyUsageUSD, state.WeeklyLimitUSD, cmd.HoldAmount) {
		return nil, service.ErrWeeklyLimitExceeded
	}
	if usageBillingLimitExceeded(usage.monthlyUsageUSD, state.MonthlyLimitUSD, cmd.HoldAmount) {
		return nil, service.ErrMonthlyLimitExceeded
	}
	usage.dailyUsageUSD += cmd.HoldAmount
	usage.weeklyUsageUSD += cmd.HoldAmount
	usage.monthlyUsageUSD += cmd.HoldAmount

	res, err := tx.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET daily_window_start = $1,
			weekly_window_start = $2,
			monthly_window_start = $3,
			daily_usage_usd = $4,
			weekly_usage_usd = $5,
			monthly_usage_usd = $6,
			updated_at = NOW()
		WHERE id = $7 AND user_id = $8
	`,
		timePtrArg(usage.dailyWindowStart),
		timePtrArg(usage.weeklyWindowStart),
		timePtrArg(usage.monthlyWindowStart),
		usage.dailyUsageUSD,
		usage.weeklyUsageUSD,
		usage.monthlyUsageUSD,
		state.ID,
		cmd.UserID,
	)
	if err != nil {
		return nil, err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, service.ErrSubscriptionNotFound
	}
	return &service.BatchImageBalanceHoldResult{
		BillingSource:          service.BillingSourceLegacySubscription,
		HeldDailyWindowStart:   usage.dailyWindowStart,
		HeldWeeklyWindowStart:  usage.weeklyWindowStart,
		HeldMonthlyWindowStart: usage.monthlyWindowStart,
	}, nil
}

func lockBatchImageLegacySubscription(ctx context.Context, tx *sql.Tx, subscriptionID int64) (*batchImageLegacySubscriptionState, error) {
	var state batchImageLegacySubscriptionState
	err := tx.QueryRowContext(ctx, `
		SELECT us.id, us.user_id, us.group_id, us.status, us.starts_at, us.expires_at,
			us.daily_window_start, us.weekly_window_start, us.monthly_window_start,
			us.daily_usage_usd, us.weekly_usage_usd, us.monthly_usage_usd,
			g.daily_limit_usd, g.weekly_limit_usd, g.monthly_limit_usd
		FROM user_subscriptions us
		JOIN groups g ON g.id = us.group_id AND g.deleted_at IS NULL
		WHERE us.id = $1 AND us.deleted_at IS NULL
		FOR UPDATE OF us, g
	`, subscriptionID).Scan(
		&state.ID,
		&state.UserID,
		&state.GroupID,
		&state.Status,
		&state.StartsAt,
		&state.ExpiresAt,
		&state.DailyWindowStart,
		&state.WeeklyWindowStart,
		&state.MonthlyWindowStart,
		&state.DailyUsageUSD,
		&state.WeeklyUsageUSD,
		&state.MonthlyUsageUSD,
		&state.DailyLimitUSD,
		&state.WeeklyLimitUSD,
		&state.MonthlyLimitUSD,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func reserveUsageBillingBatchImageEntitlement(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.EntitlementID == nil {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	state, err := lockUsageBillingEntitlement(ctx, tx, *cmd.EntitlementID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if state.UserID != cmd.UserID {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	if state.Status != service.SubscriptionStatusActive {
		return nil, service.ErrSubscriptionEntitlementInactive
	}
	if now.Before(state.StartsAt) || !now.Before(state.ExpiresAt) {
		return nil, service.ErrSubscriptionEntitlementExpired
	}

	usage := usageBillingEntitlementWindowUsage{
		dailyWindowStart:   nullTimePtr(state.DailyWindowStart),
		weeklyWindowStart:  nullTimePtr(state.WeeklyWindowStart),
		monthlyWindowStart: nullTimePtr(state.MonthlyWindowStart),
		dailyUsageUSD:      state.DailyUsageUSD,
		weeklyUsageUSD:     state.WeeklyUsageUSD,
		monthlyUsageUSD:    state.MonthlyUsageUSD,
	}
	usage.activateAndReset(state.StartsAt, state.ExpiresAt, now)
	limitExceeded := usageBillingLimitExceeded(usage.dailyUsageUSD, state.DailyLimitUSD, cmd.HoldAmount) ||
		usageBillingLimitExceeded(usage.weeklyUsageUSD, state.WeeklyLimitUSD, cmd.HoldAmount) ||
		usageBillingLimitExceeded(usage.monthlyUsageUSD, state.MonthlyLimitUSD, cmd.HoldAmount)
	if limitExceeded {
		if !cmd.EntitlementBalanceFallback || state.OveragePolicy != service.SubscriptionEntitlementOverageBalanceFallback {
			return nil, service.ErrSubscriptionEntitlementQuotaExceeded
		}
		result, err := reserveUsageBillingBatchImageBalance(ctx, tx, cmd)
		if result != nil {
			result.BillingSource = service.BillingSourceEntitlementBalanceFallback
		}
		return result, err
	}

	usage.dailyUsageUSD += cmd.HoldAmount
	usage.weeklyUsageUSD += cmd.HoldAmount
	usage.monthlyUsageUSD += cmd.HoldAmount
	res, err := tx.ExecContext(ctx, `
		UPDATE subscription_entitlements
		SET daily_window_start = $1,
			weekly_window_start = $2,
			monthly_window_start = $3,
			daily_usage_usd = $4,
			weekly_usage_usd = $5,
			monthly_usage_usd = $6,
			updated_at = NOW()
		WHERE id = $7 AND user_id = $8
	`,
		timePtrArg(usage.dailyWindowStart),
		timePtrArg(usage.weeklyWindowStart),
		timePtrArg(usage.monthlyWindowStart),
		usage.dailyUsageUSD,
		usage.weeklyUsageUSD,
		usage.monthlyUsageUSD,
		state.ID,
		cmd.UserID,
	)
	if err != nil {
		return nil, err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	return &service.BatchImageBalanceHoldResult{
		BillingSource:          service.BillingSourceEntitlementQuota,
		HeldDailyWindowStart:   usage.dailyWindowStart,
		HeldWeeklyWindowStart:  usage.weeklyWindowStart,
		HeldMonthlyWindowStart: usage.monthlyWindowStart,
	}, nil
}

func settleUsageBillingBatchImageLegacySubscription(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, refundAmount float64) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.SubscriptionID == nil {
		return nil, service.ErrSubscriptionNotFound
	}
	if err := ensureBatchImageHoldWasReserved(ctx, tx, cmd); err != nil {
		return nil, err
	}
	if refundAmount <= batchImageBillingEpsilon {
		return &service.BatchImageBalanceHoldResult{BillingSource: service.BillingSourceLegacySubscription}, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET daily_usage_usd = CASE
				WHEN $1::timestamptz IS NOT NULL AND daily_window_start = $1 THEN GREATEST(0, daily_usage_usd - $4)
				ELSE daily_usage_usd END,
			weekly_usage_usd = CASE
				WHEN $2::timestamptz IS NOT NULL AND weekly_window_start = $2 THEN GREATEST(0, weekly_usage_usd - $4)
				ELSE weekly_usage_usd END,
			monthly_usage_usd = CASE
				WHEN $3::timestamptz IS NOT NULL AND monthly_window_start = $3 THEN GREATEST(0, monthly_usage_usd - $4)
				ELSE monthly_usage_usd END,
			updated_at = NOW()
		WHERE id = $5 AND user_id = $6
	`,
		timePtrArg(cmd.HeldDailyWindowStart),
		timePtrArg(cmd.HeldWeeklyWindowStart),
		timePtrArg(cmd.HeldMonthlyWindowStart),
		refundAmount,
		*cmd.SubscriptionID,
		cmd.UserID,
	)
	if err != nil {
		return nil, err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, service.ErrSubscriptionNotFound
	}
	return &service.BatchImageBalanceHoldResult{BillingSource: service.BillingSourceLegacySubscription}, nil
}

func settleUsageBillingBatchImageEntitlement(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand, refundAmount float64) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.EntitlementID == nil {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	if err := ensureBatchImageHoldWasReserved(ctx, tx, cmd); err != nil {
		return nil, err
	}
	if refundAmount <= batchImageBillingEpsilon {
		return &service.BatchImageBalanceHoldResult{BillingSource: service.BillingSourceEntitlementQuota}, nil
	}
	res, err := tx.ExecContext(ctx, `
		UPDATE subscription_entitlements
		SET daily_usage_usd = CASE
				WHEN $1::timestamptz IS NOT NULL AND daily_window_start = $1 THEN GREATEST(0, daily_usage_usd - $4)
				ELSE daily_usage_usd END,
			weekly_usage_usd = CASE
				WHEN $2::timestamptz IS NOT NULL AND weekly_window_start = $2 THEN GREATEST(0, weekly_usage_usd - $4)
				ELSE weekly_usage_usd END,
			monthly_usage_usd = CASE
				WHEN $3::timestamptz IS NOT NULL AND monthly_window_start = $3 THEN GREATEST(0, monthly_usage_usd - $4)
				ELSE monthly_usage_usd END,
			updated_at = NOW()
		WHERE id = $5 AND user_id = $6
	`,
		timePtrArg(cmd.HeldDailyWindowStart),
		timePtrArg(cmd.HeldWeeklyWindowStart),
		timePtrArg(cmd.HeldMonthlyWindowStart),
		refundAmount,
		*cmd.EntitlementID,
		cmd.UserID,
	)
	if err != nil {
		return nil, err
	}
	if affected, err := res.RowsAffected(); err != nil {
		return nil, err
	} else if affected == 0 {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	return &service.BatchImageBalanceHoldResult{BillingSource: service.BillingSourceEntitlementQuota}, nil
}

func ensureBatchImageHoldWasReserved(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) error {
	held, err := batchImageHoldClaimExists(ctx, tx, service.BatchImageHoldRequestID(cmd.BatchID), cmd.APIKeyID)
	if err != nil {
		return err
	}
	if !held {
		return errors.New("batch image hold was never reserved")
	}
	return nil
}
