package repository

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type usageBillingRepository struct {
	db *sql.DB
}

func NewUsageBillingRepository(_ *dbent.Client, sqlDB *sql.DB) service.UsageBillingRepository {
	return &usageBillingRepository{db: sqlDB}
}

func (r *usageBillingRepository) Apply(ctx context.Context, cmd *service.UsageBillingCommand) (_ *service.UsageBillingApplyResult, err error) {
	if cmd == nil {
		return &service.UsageBillingApplyResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}

	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := r.claimUsageBillingKey(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if !applied {
		billingSource, lookupErr := lookupUsageBillingSource(ctx, tx, cmd.RequestID, cmd.APIKeyID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		return &service.UsageBillingApplyResult{Applied: false, BillingSource: billingSource}, nil
	}

	result := &service.UsageBillingApplyResult{Applied: true}
	if err := r.applyUsageBillingEffects(ctx, tx, cmd, result); err != nil {
		return nil, err
	}
	result.BillingSource = service.ResolveUsageBillingSourceFromApplyResult(
		cmd.BillingType,
		cmd.SubscriptionID,
		cmd.EntitlementID,
		result,
	)
	if err := persistUsageBillingSource(ctx, tx, cmd.RequestID, cmd.APIKeyID, result.BillingSource); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) claimUsageBillingKey(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand) (bool, error) {
	return r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
}

func (r *usageBillingRepository) claimUsageBillingRequest(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64, requestFingerprint string) (bool, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO usage_billing_dedup (request_id, api_key_id, request_fingerprint)
		VALUES ($1, $2, $3)
		ON CONFLICT (request_id, api_key_id) DO NOTHING
		RETURNING id
	`, requestID, apiKeyID, requestFingerprint).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existingFingerprint string
		if err := tx.QueryRowContext(ctx, `
			SELECT request_fingerprint
			FROM usage_billing_dedup
			WHERE request_id = $1 AND api_key_id = $2
		`, requestID, apiKeyID).Scan(&existingFingerprint); err != nil {
			return false, err
		}
		if strings.TrimSpace(existingFingerprint) != strings.TrimSpace(requestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var archivedFingerprint string
	err = tx.QueryRowContext(ctx, `
		SELECT request_fingerprint
		FROM usage_billing_dedup_archive
		WHERE request_id = $1 AND api_key_id = $2
	`, requestID, apiKeyID).Scan(&archivedFingerprint)
	if err == nil {
		if strings.TrimSpace(archivedFingerprint) != strings.TrimSpace(requestFingerprint) {
			return false, service.ErrUsageBillingRequestConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	return true, nil
}

func lookupUsageBillingSource(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64) (string, error) {
	var dedupSource sql.NullString
	var usageLogSource sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT candidates.billing_source, usage_logs.billing_source
		FROM (
			SELECT billing_source, 0 AS source_priority
			FROM usage_billing_dedup_archive
			WHERE request_id = $1 AND api_key_id = $2
			UNION ALL
			SELECT billing_source, 1 AS source_priority
			FROM usage_billing_dedup
			WHERE request_id = $1 AND api_key_id = $2
		) candidates
		LEFT JOIN usage_logs
			ON usage_logs.request_id = $1 AND usage_logs.api_key_id = $2
		ORDER BY source_priority
		LIMIT 1
	`, requestID, apiKeyID).Scan(&dedupSource, &usageLogSource)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for _, source := range []sql.NullString{dedupSource, usageLogSource} {
		billingSource := strings.TrimSpace(source.String)
		if source.Valid && service.IsValidUsageBillingSource(billingSource) {
			return billingSource, nil
		}
	}
	return "", nil
}

func persistUsageBillingSource(ctx context.Context, tx *sql.Tx, requestID string, apiKeyID int64, billingSource string) error {
	billingSource = strings.TrimSpace(billingSource)
	if !service.IsValidUsageBillingSource(billingSource) {
		return errors.New("usage billing source is invalid")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE usage_billing_dedup
		SET billing_source = $1
		WHERE request_id = $2 AND api_key_id = $3
	`, billingSource, requestID, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return errors.New("usage billing dedup row not found while persisting billing source")
	}
	return nil
}

func (r *usageBillingRepository) ReserveBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, reserveUsageBillingBatchImage)
}

func (r *usageBillingRepository) CaptureBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, captureUsageBillingBatchImage)
}

func (r *usageBillingRepository) ReleaseBatchImageBalance(ctx context.Context, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	return r.applyBatchImageBalanceHold(ctx, cmd, releaseUsageBillingBatchImage)
}

func (r *usageBillingRepository) applyBatchImageBalanceHold(
	ctx context.Context,
	cmd *service.BatchImageBalanceHoldCommand,
	apply func(context.Context, *sql.Tx, *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error),
) (_ *service.BatchImageBalanceHoldResult, err error) {
	if cmd == nil {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if r == nil || r.db == nil {
		return nil, errors.New("usage billing repository db is nil")
	}
	cmd.Normalize()
	if cmd.RequestID == "" {
		return nil, service.ErrUsageBillingRequestIDRequired
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	applied, err := r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	if !applied {
		billingSource, lookupErr := lookupUsageBillingSource(ctx, tx, cmd.RequestID, cmd.APIKeyID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		return &service.BatchImageBalanceHoldResult{Applied: false, BillingSource: billingSource}, nil
	}

	result, err := apply(ctx, tx, cmd)
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &service.BatchImageBalanceHoldResult{}
	}
	result.Applied = true
	if !service.IsValidUsageBillingSource(strings.TrimSpace(result.BillingSource)) {
		result.BillingSource = cmd.BillingSource
	}
	if err := persistUsageBillingSource(ctx, tx, cmd.RequestID, cmd.APIKeyID, result.BillingSource); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx = nil
	return result, nil
}

func (r *usageBillingRepository) applyUsageBillingEffects(ctx context.Context, tx *sql.Tx, cmd *service.UsageBillingCommand, result *service.UsageBillingApplyResult) error {
	if cmd.SubscriptionCost > 0 && cmd.EntitlementID != nil {
		entitlementVersion, usedFallback, err := applyUsageBillingEntitlement(ctx, tx, cmd.UserID, *cmd.EntitlementID, cmd.SubscriptionCost, cmd.EntitlementBalanceFallback, cmd.AllowEntitlementOverage)
		if err != nil {
			return err
		}
		if usedFallback {
			newBalance, balanceErr := deductUsageBillingBalanceStrict(ctx, tx, cmd.UserID, cmd.SubscriptionCost)
			switch {
			case balanceErr == nil:
				result.NewBalance = &newBalance
			case cmd.AllowEntitlementOverage && errors.Is(balanceErr, service.ErrInsufficientBalance):
				// The response has already been delivered on final-settlement paths. If
				// balance fallback cannot cover it, persist the overage against the
				// entitlement so the next preflight sees the exhausted quota.
				entitlementVersion, _, err = applyUsageBillingEntitlement(ctx, tx, cmd.UserID, *cmd.EntitlementID, cmd.SubscriptionCost, false, true)
				if err != nil {
					return err
				}
				usedFallback = false
			default:
				return balanceErr
			}
		}
		if !usedFallback {
			result.EntitlementVersion = entitlementVersion
			// Linked entitlement billing writes the same absolute usage snapshot to
			// its legacy alias in this transaction. PostgreSQL NOW() is stable for
			// the transaction, so the entitlement version is also the alias version.
			result.SubscriptionVersion = entitlementVersion
		}
	} else if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
		updatedAt, err := incrementUsageBillingSubscription(ctx, tx, *cmd.SubscriptionID, cmd.SubscriptionCost)
		if err != nil {
			return err
		}
		result.SubscriptionVersion = updatedAt.UnixMicro()
	}

	if cmd.BalanceCost > 0 {
		newBalance, sufficient, err := deductUsageBillingBalance(ctx, tx, cmd.UserID, cmd.BalanceCost)
		if err != nil {
			return err
		}
		result.NewBalance = &newBalance
		result.BalanceOverdrafted = !sufficient
	}

	if cmd.APIKeyQuotaCost > 0 {
		exhausted, err := incrementUsageBillingAPIKeyQuota(ctx, tx, cmd.APIKeyID, cmd.APIKeyQuotaCost)
		if err != nil {
			return err
		}
		result.APIKeyQuotaExhausted = exhausted
	}

	if cmd.APIKeyRateLimitCost > 0 {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, cmd.APIKeyID, cmd.APIKeyRateLimitCost); err != nil {
			return err
		}
	}

	if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) || strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
		quotaState, err := incrementUsageBillingAccountQuota(ctx, tx, cmd.AccountID, cmd.AccountQuotaCost)
		if err != nil {
			return err
		}
		result.QuotaState = quotaState
	}

	return nil
}

func incrementUsageBillingSubscription(ctx context.Context, tx *sql.Tx, subscriptionID int64, costUSD float64) (time.Time, error) {
	const updateSQL = `
		UPDATE user_subscriptions us
		SET
			daily_usage_usd = us.daily_usage_usd + $1,
			weekly_usage_usd = us.weekly_usage_usd + $1,
			monthly_usage_usd = us.monthly_usage_usd + $1,
			updated_at = NOW()
		FROM groups g
		WHERE us.id = $2
			AND us.deleted_at IS NULL
			AND us.group_id = g.id
			AND g.deleted_at IS NULL
		RETURNING us.updated_at
	`
	var updatedAt time.Time
	if err := tx.QueryRowContext(ctx, updateSQL, costUSD, subscriptionID).Scan(&updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, service.ErrSubscriptionNotFound
		}
		return time.Time{}, err
	}
	return updatedAt, nil
}

func deductUsageBillingBalance(ctx context.Context, tx *sql.Tx, userID int64, amount float64) (float64, bool, error) {
	var newBalance float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance >= $1
		RETURNING balance
	`, amount, userID).Scan(&newBalance)
	if err == nil {
		return newBalance, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, err
	}

	err = tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING balance
	`, amount, userID).Scan(&newBalance)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, service.ErrUserNotFound
	}
	if err != nil {
		return 0, false, err
	}
	return newBalance, false, nil
}

type usageBillingEntitlementState struct {
	ID                   int64
	UserID               int64
	LegacySubscriptionID sql.NullInt64
	Status               string
	StartsAt             time.Time
	ExpiresAt            time.Time
	DailyWindowStart     sql.NullTime
	WeeklyWindowStart    sql.NullTime
	MonthlyWindowStart   sql.NullTime
	DailyLimitUSD        sql.NullFloat64
	WeeklyLimitUSD       sql.NullFloat64
	MonthlyLimitUSD      sql.NullFloat64
	DailyUsageUSD        float64
	WeeklyUsageUSD       float64
	MonthlyUsageUSD      float64
	OveragePolicy        string
}

func applyUsageBillingEntitlement(ctx context.Context, tx *sql.Tx, userID, entitlementID int64, costUSD float64, allowBalanceFallback bool, allowOverage bool) (int64, bool, error) {
	if costUSD < 0 {
		return 0, false, service.ErrSubscriptionEntitlementInvalidUsage
	}
	if err := lockSubscriptionEntitlementUserMutation(ctx, tx, userID); err != nil {
		return 0, false, err
	}

	state, err := lockUsageBillingEntitlement(ctx, tx, entitlementID)
	if err != nil {
		return 0, false, err
	}
	now := time.Now().UTC()
	if state.UserID != userID {
		return 0, false, service.ErrSubscriptionEntitlementNotFound
	}
	if state.Status != service.SubscriptionStatusActive {
		return 0, false, service.ErrSubscriptionEntitlementInactive
	}
	if now.Before(state.StartsAt) || !now.Before(state.ExpiresAt) {
		return 0, false, service.ErrSubscriptionEntitlementExpired
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

	limitExceeded := usageBillingLimitExceeded(usage.dailyUsageUSD, state.DailyLimitUSD, costUSD) ||
		usageBillingLimitExceeded(usage.weeklyUsageUSD, state.WeeklyLimitUSD, costUSD) ||
		usageBillingLimitExceeded(usage.monthlyUsageUSD, state.MonthlyLimitUSD, costUSD)
	if limitExceeded {
		if allowBalanceFallback && state.OveragePolicy == service.SubscriptionEntitlementOverageBalanceFallback {
			return 0, true, nil
		}
		if !allowOverage {
			return 0, false, service.ErrSubscriptionEntitlementQuotaExceeded
		}
	}

	usage.dailyUsageUSD += costUSD
	usage.weeklyUsageUSD += costUSD
	usage.monthlyUsageUSD += costUSD
	linkedAlias, err := lockUsageBillingLinkedLegacyAlias(ctx, tx, state.LegacySubscriptionID, userID)
	if err != nil {
		return 0, false, err
	}

	var updatedAt time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE subscription_entitlements
		SET daily_window_start = $1,
			weekly_window_start = $2,
			monthly_window_start = $3,
			daily_usage_usd = $4,
			weekly_usage_usd = $5,
			monthly_usage_usd = $6,
			updated_at = NOW()
		WHERE id = $7 AND deleted_at IS NULL
		RETURNING updated_at
	`,
		timePtrArg(usage.dailyWindowStart),
		timePtrArg(usage.weeklyWindowStart),
		timePtrArg(usage.monthlyWindowStart),
		usage.dailyUsageUSD,
		usage.weeklyUsageUSD,
		usage.monthlyUsageUSD,
		entitlementID,
	).Scan(&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, service.ErrSubscriptionEntitlementNotFound
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := syncUsageBillingLinkedLegacyAlias(ctx, tx, linkedAlias, usage); err != nil {
		return 0, false, err
	}
	return updatedAt.UnixMicro(), false, nil
}

type usageBillingLinkedLegacyAlias struct {
	ID     int64
	UserID int64
}

func lockUsageBillingLinkedLegacyAlias(
	ctx context.Context,
	tx *sql.Tx,
	legacySubscriptionID sql.NullInt64,
	userID int64,
) (*usageBillingLinkedLegacyAlias, error) {
	return lockUsageBillingLinkedLegacyAliasRow(ctx, tx, legacySubscriptionID, userID, false)
}

func lockUsageBillingLinkedLegacyAliasIncludingDeleted(
	ctx context.Context,
	tx *sql.Tx,
	legacySubscriptionID sql.NullInt64,
	userID int64,
) (*usageBillingLinkedLegacyAlias, error) {
	return lockUsageBillingLinkedLegacyAliasRow(ctx, tx, legacySubscriptionID, userID, true)
}

func lockUsageBillingLinkedLegacyAliasRow(
	ctx context.Context,
	tx *sql.Tx,
	legacySubscriptionID sql.NullInt64,
	userID int64,
	includeDeleted bool,
) (*usageBillingLinkedLegacyAlias, error) {
	if !legacySubscriptionID.Valid {
		return nil, nil
	}
	if legacySubscriptionID.Int64 <= 0 || userID <= 0 {
		return nil, service.ErrSubscriptionEntitlementAliasUnavailable
	}
	alias := &usageBillingLinkedLegacyAlias{}
	query := `
		SELECT id, user_id
		FROM user_subscriptions
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`
	if includeDeleted {
		query = `
			SELECT id, user_id
			FROM user_subscriptions
			WHERE id = $1
			FOR UPDATE
		`
	}
	err := tx.QueryRowContext(ctx, query, legacySubscriptionID.Int64).Scan(&alias.ID, &alias.UserID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionEntitlementAliasUnavailable
	}
	if err != nil {
		return nil, err
	}
	if alias.UserID != userID {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	return alias, nil
}

func syncUsageBillingLinkedLegacyAlias(
	ctx context.Context,
	tx *sql.Tx,
	alias *usageBillingLinkedLegacyAlias,
	usage usageBillingEntitlementWindowUsage,
) (time.Time, error) {
	if alias == nil {
		return time.Time{}, nil
	}
	var updatedAt time.Time
	err := tx.QueryRowContext(ctx, `
		UPDATE user_subscriptions
		SET daily_window_start = $1,
			weekly_window_start = $2,
			monthly_window_start = $3,
			daily_usage_usd = $4,
			weekly_usage_usd = $5,
			monthly_usage_usd = $6,
			updated_at = NOW()
		WHERE id = $7 AND user_id = $8
		RETURNING updated_at
	`,
		timePtrArg(usage.dailyWindowStart),
		timePtrArg(usage.weeklyWindowStart),
		timePtrArg(usage.monthlyWindowStart),
		usage.dailyUsageUSD,
		usage.weeklyUsageUSD,
		usage.monthlyUsageUSD,
		alias.ID,
		alias.UserID,
	).Scan(&updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, service.ErrSubscriptionEntitlementAliasUnavailable
	}
	if err != nil {
		return time.Time{}, err
	}
	return updatedAt, nil
}

func lockUsageBillingEntitlement(ctx context.Context, tx *sql.Tx, entitlementID int64) (*usageBillingEntitlementState, error) {
	var state usageBillingEntitlementState
	err := tx.QueryRowContext(ctx, `
		SELECT id, user_id, legacy_subscription_id, status, starts_at, expires_at,
			daily_window_start, weekly_window_start, monthly_window_start,
			daily_limit_usd, weekly_limit_usd, monthly_limit_usd,
			daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
			overage_policy
		FROM subscription_entitlements
		WHERE id = $1 AND deleted_at IS NULL
		FOR UPDATE
	`, entitlementID).Scan(
		&state.ID,
		&state.UserID,
		&state.LegacySubscriptionID,
		&state.Status,
		&state.StartsAt,
		&state.ExpiresAt,
		&state.DailyWindowStart,
		&state.WeeklyWindowStart,
		&state.MonthlyWindowStart,
		&state.DailyLimitUSD,
		&state.WeeklyLimitUSD,
		&state.MonthlyLimitUSD,
		&state.DailyUsageUSD,
		&state.WeeklyUsageUSD,
		&state.MonthlyUsageUSD,
		&state.OveragePolicy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrSubscriptionEntitlementNotFound
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

type usageBillingEntitlementWindowUsage struct {
	dailyWindowStart   *time.Time
	weeklyWindowStart  *time.Time
	monthlyWindowStart *time.Time
	dailyUsageUSD      float64
	weeklyUsageUSD     float64
	monthlyUsageUSD    float64
}

func (u *usageBillingEntitlementWindowUsage) activateAndReset(startsAt, expiresAt, now time.Time) {
	if u.dailyWindowStart == nil && u.weeklyWindowStart == nil && u.monthlyWindowStart == nil {
		periodicStart := startsAt
		if periodicStart.IsZero() {
			periodicStart = now
		}
		dailyStart := timezone.StartOfDay(now)
		u.dailyWindowStart = &dailyStart
		u.weeklyWindowStart = &periodicStart
		u.monthlyWindowStart = &periodicStart
		u.dailyUsageUSD = 0
		u.weeklyUsageUSD = 0
		u.monthlyUsageUSD = 0
	}
	if u.dailyWindowStart != nil &&
		!usageBillingOneTimeDailyQuota(startsAt, expiresAt) &&
		timezone.StartOfDay(now).After(timezone.StartOfDay(*u.dailyWindowStart)) {
		windowStart := timezone.StartOfDay(now)
		u.dailyWindowStart = &windowStart
		u.dailyUsageUSD = 0
	}
	if usageBillingNeedsWindowResetAt(u.weeklyWindowStart, startsAt, 7*24*time.Hour, now) {
		windowStart := usageBillingResolvedWindowResetStart(u.weeklyWindowStart, startsAt, 7*24*time.Hour, now)
		u.weeklyWindowStart = &windowStart
		u.weeklyUsageUSD = 0
	}
	if usageBillingNeedsWindowResetAt(u.monthlyWindowStart, startsAt, 30*24*time.Hour, now) {
		windowStart := usageBillingResolvedWindowResetStart(u.monthlyWindowStart, startsAt, 30*24*time.Hour, now)
		u.monthlyWindowStart = &windowStart
		u.monthlyUsageUSD = 0
	}
}

func usageBillingLimitExceeded(current float64, limit sql.NullFloat64, additional float64) bool {
	return limit.Valid && limit.Float64 > 0 && current+additional > limit.Float64
}

func nullTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	out := v.Time
	return &out
}

func timePtrArg(v *time.Time) any {
	if v == nil {
		return nil
	}
	return *v
}

func deductUsageBillingBalanceStrict(ctx context.Context, tx *sql.Tx, userID int64, amount float64) (float64, error) {
	var newBalance float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			updated_at = NOW()
		WHERE id = $2
			AND deleted_at IS NULL
			AND balance >= $1
		RETURNING balance
	`, amount, userID).Scan(&newBalance)
	if err == nil {
		return newBalance, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}

	var currentBalance float64
	checkErr := tx.QueryRowContext(ctx, `
		SELECT balance
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&currentBalance)
	if errors.Is(checkErr, sql.ErrNoRows) {
		return 0, service.ErrUserNotFound
	}
	if checkErr != nil {
		return 0, checkErr
	}
	return 0, service.ErrInsufficientBalance
}

func usageBillingNeedsWindowResetAt(windowStart *time.Time, startsAt time.Time, cycle time.Duration, now time.Time) bool {
	if windowStart == nil {
		return false
	}
	start := usageBillingEffectiveWindowStartAt(windowStart, startsAt, cycle, now)
	if start == nil {
		return false
	}
	if !start.After(*windowStart) {
		return false
	}
	return start.Sub(*windowStart) >= cycle
}

func usageBillingResolvedWindowResetStart(windowStart *time.Time, startsAt time.Time, cycle time.Duration, now time.Time) time.Time {
	start := usageBillingEffectiveWindowStartAt(windowStart, startsAt, cycle, now)
	if start == nil {
		return now
	}
	return *start
}

func usageBillingEffectiveWindowStartAt(windowStart *time.Time, startsAt time.Time, cycle time.Duration, now time.Time) *time.Time {
	if windowStart == nil {
		return nil
	}
	windowBased := usageBillingAdvanceWindowStart(*windowStart, cycle, now)
	if windowStart.After(now) {
		return &windowBased
	}
	if aligned, ok := usageBillingAlignedCycleStart(startsAt, cycle, now); ok {
		if usageBillingIsLegacyWindowAnchor(*windowStart, startsAt, cycle) || usageBillingIsAlignedWindowAnchor(*windowStart, startsAt, cycle) {
			return &aligned
		}
	}
	return &windowBased
}

func usageBillingAdvanceWindowStart(windowStart time.Time, cycle time.Duration, now time.Time) time.Time {
	if cycle <= 0 {
		return windowStart
	}
	start := windowStart
	for !start.Add(cycle).After(now) {
		start = start.Add(cycle)
	}
	return start
}

func usageBillingAlignedCycleStart(startsAt time.Time, cycle time.Duration, now time.Time) (time.Time, bool) {
	if startsAt.IsZero() || cycle <= 0 {
		return time.Time{}, false
	}
	if now.Before(startsAt) {
		return startsAt, true
	}
	elapsed := now.Sub(startsAt)
	steps := elapsed / cycle
	return startsAt.Add(steps * cycle), true
}

func usageBillingIsAlignedWindowAnchor(windowStart, startsAt time.Time, cycle time.Duration) bool {
	if cycle <= 0 || windowStart.IsZero() || startsAt.IsZero() || windowStart.Before(startsAt) {
		return false
	}
	return windowStart.Sub(startsAt)%cycle == 0
}

func usageBillingIsLegacyWindowAnchor(windowStart, startsAt time.Time, cycle time.Duration) bool {
	if !usageBillingIsStartOfDay(windowStart) || startsAt.IsZero() || windowStart.IsZero() || cycle <= 0 {
		return false
	}
	if windowStart.Before(startsAt) {
		return true
	}
	return !usageBillingIsAlignedWindowAnchor(windowStart, startsAt, cycle)
}

func usageBillingIsStartOfDay(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

func usageBillingOneTimeDailyQuota(startsAt, expiresAt time.Time) bool {
	if startsAt.IsZero() || expiresAt.IsZero() {
		return false
	}
	return !expiresAt.After(startsAt.AddDate(0, 0, 1))
}

func reserveUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	var balance, frozen float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance - $1,
			frozen_balance = COALESCE(frozen_balance, 0) + $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND balance >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.UserID).Scan(&balance, &frozen)
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, service.ErrBatchImageInsufficientBalance
}

func captureUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 && cmd.ActualAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	if cmd.ActualAmount-cmd.HoldAmount > 0.00000001 {
		return nil, service.ErrBatchImageSettlementCostExceedsHold
	}
	var balance, frozen float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance
				+ CASE WHEN $1 > $2 THEN $1 - $2 ELSE 0 END
				- CASE WHEN $2 > $1 THEN $2 - $1 ELSE 0 END,
			frozen_balance = COALESCE(frozen_balance, 0) - $1,
			updated_at = NOW()
		WHERE id = $3 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.ActualAmount, cmd.UserID).Scan(&balance, &frozen)
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("batch image frozen balance is insufficient")
}

func releaseUsageBillingBatchImageBalance(ctx context.Context, tx *sql.Tx, cmd *service.BatchImageBalanceHoldCommand) (*service.BatchImageBalanceHoldResult, error) {
	if cmd.HoldAmount <= 0 {
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	// 释放前校验该 job 确实预留过 hold（hold request id 已被 claim），
	// 防止从未成功冻结的 job 触发"幻影释放"，从其他用户的冻结资金池中凭空生成余额。
	held, heldErr := batchImageHoldClaimExists(ctx, tx, service.BatchImageHoldRequestID(cmd.BatchID), cmd.APIKeyID)
	if heldErr != nil {
		return nil, heldErr
	}
	if !held {
		logger.LegacyPrintf("repository.usage_billing", "[BatchImage] release skipped, hold was never reserved: batch=%s", cmd.BatchID)
		return &service.BatchImageBalanceHoldResult{}, nil
	}
	var balance, frozen float64
	err := tx.QueryRowContext(ctx, `
		UPDATE users
		SET balance = balance + $1,
			frozen_balance = COALESCE(frozen_balance, 0) - $1,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL AND COALESCE(frozen_balance, 0) >= $1
		RETURNING balance, frozen_balance
	`, cmd.HoldAmount, cmd.UserID).Scan(&balance, &frozen)
	if err == nil {
		return &service.BatchImageBalanceHoldResult{NewBalance: &balance, FrozenBalance: &frozen}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if exists, existsErr := userExistsForBilling(ctx, tx, cmd.UserID); existsErr != nil {
		return nil, existsErr
	} else if !exists {
		return nil, service.ErrUserNotFound
	}
	return nil, errors.New("batch image frozen balance is insufficient")
}

// batchImageHoldClaimExists 检查 hold request id 是否已在 dedup（或归档）表中被 claim，
// 即该 batch 的冻结操作确实成功提交过。
func batchImageHoldClaimExists(ctx context.Context, tx *sql.Tx, holdRequestID string, apiKeyID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup
		WHERE request_id = $1 AND api_key_id = $2
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT 1
		FROM usage_billing_dedup_archive
		WHERE request_id = $1 AND api_key_id = $2
	`, holdRequestID, apiKeyID).Scan(&exists)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return false, err
}

func userExistsForBilling(ctx context.Context, tx *sql.Tx, userID int64) (bool, error) {
	var exists int
	err := tx.QueryRowContext(ctx, `
		SELECT 1
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`, userID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func incrementUsageBillingAPIKeyQuota(ctx context.Context, tx *sql.Tx, apiKeyID int64, amount float64) (bool, error) {
	var exhausted bool
	err := tx.QueryRowContext(ctx, `
		UPDATE api_keys
		SET quota_used = quota_used + $1,
			status = CASE
				WHEN quota > 0
					AND status = $3
					AND quota_used < quota
					AND quota_used + $1 >= quota
				THEN $4
				ELSE status
			END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING quota > 0 AND quota_used >= quota AND quota_used - $1 < quota
	`, amount, apiKeyID, service.StatusAPIKeyActive, service.StatusAPIKeyQuotaExhausted).Scan(&exhausted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrAPIKeyNotFound
	}
	if err != nil {
		return false, err
	}
	return exhausted, nil
}

func incrementUsageBillingAPIKeyRateLimit(ctx context.Context, tx *sql.Tx, apiKeyID int64, cost float64) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE api_keys SET
			usage_5h = CASE WHEN window_5h_start IS NOT NULL AND window_5h_start + INTERVAL '5 hours' <= NOW() THEN $1 ELSE usage_5h + $1 END,
			usage_1d = CASE WHEN window_1d_start IS NOT NULL AND window_1d_start + INTERVAL '24 hours' <= NOW() THEN $1 ELSE usage_1d + $1 END,
			usage_7d = CASE WHEN window_7d_start IS NOT NULL AND window_7d_start + INTERVAL '7 days' <= NOW() THEN $1 ELSE usage_7d + $1 END,
			window_5h_start = CASE WHEN window_5h_start IS NULL OR window_5h_start + INTERVAL '5 hours' <= NOW() THEN NOW() ELSE window_5h_start END,
			window_1d_start = CASE WHEN window_1d_start IS NULL OR window_1d_start + INTERVAL '24 hours' <= NOW() THEN date_trunc('day', NOW()) ELSE window_1d_start END,
			window_7d_start = CASE WHEN window_7d_start IS NULL OR window_7d_start + INTERVAL '7 days' <= NOW() THEN date_trunc('day', NOW()) ELSE window_7d_start END,
			updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`, cost, apiKeyID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrAPIKeyNotFound
	}
	return nil
}

func incrementUsageBillingAccountQuota(ctx context.Context, tx *sql.Tx, accountID int64, amount float64) (*service.AccountQuotaState, error) {
	rows, err := tx.QueryContext(ctx,
		`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			|| jsonb_build_object('quota_used', COALESCE((extra->>'quota_used')::numeric, 0) + $1)
			|| CASE WHEN COALESCE((extra->>'quota_daily_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_daily_used',
					CASE WHEN `+dailyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_daily_used')::numeric, 0) + $1 END,
					'quota_daily_start',
					CASE WHEN `+dailyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_daily_start', `+nowUTC+`) END
				)
				|| CASE WHEN `+dailyExpiredExpr+` AND `+nextDailyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_daily_reset_at', `+nextDailyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
			|| CASE WHEN COALESCE((extra->>'quota_weekly_limit')::numeric, 0) > 0 THEN
				jsonb_build_object(
					'quota_weekly_used',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN $1
					ELSE COALESCE((extra->>'quota_weekly_used')::numeric, 0) + $1 END,
					'quota_weekly_start',
					CASE WHEN `+weeklyExpiredExpr+`
					THEN `+nowUTC+`
					ELSE COALESCE(extra->>'quota_weekly_start', `+nowUTC+`) END
				)
				|| CASE WHEN `+weeklyExpiredExpr+` AND `+nextWeeklyResetAtExpr+` IS NOT NULL
				   THEN jsonb_build_object('quota_weekly_reset_at', `+nextWeeklyResetAtExpr+`)
				   ELSE '{}'::jsonb END
			ELSE '{}'::jsonb END
		), updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
		RETURNING
			COALESCE((extra->>'quota_used')::numeric, 0),
			COALESCE((extra->>'quota_limit')::numeric, 0),
			COALESCE((extra->>'quota_daily_used')::numeric, 0),
			COALESCE((extra->>'quota_daily_limit')::numeric, 0),
			COALESCE((extra->>'quota_weekly_used')::numeric, 0),
			COALESCE((extra->>'quota_weekly_limit')::numeric, 0)`,
		amount, accountID)
	if err != nil {
		return nil, err
	}

	var state service.AccountQuotaState
	if rows.Next() {
		if err := rows.Scan(
			&state.TotalUsed, &state.TotalLimit,
			&state.DailyUsed, &state.DailyLimit,
			&state.WeeklyUsed, &state.WeeklyLimit,
		); err != nil {
			_ = rows.Close()
			return nil, err
		}
	} else {
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, err
		}
		_ = rows.Close()
		return nil, service.ErrAccountNotFound
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	// 必须在执行下一条 SQL 前显式关闭 rows：pq 驱动在同一连接上
	// 不允许前一条查询的结果集未耗尽时启动新查询，否则会返回
	// "unexpected Parse response" 错误。
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// 任意维度额度在本次递增中从"未超"跨越到"已超"时，必须刷新调度快照，
	// 否则 Redis 中缓存的 Account 仍显示旧的 used 值，后续请求会继续选中本账号，
	// 最终观察到 daily_used / weekly_used 大幅超过配置的 limit。
	// 对于日/周额度，即使本次触发了周期重置（pre=0、post=amount），
	// 判定式 (post-amount) < limit 同样成立，逻辑与总额度保持一致。
	crossedTotal := state.TotalLimit > 0 && state.TotalUsed >= state.TotalLimit && (state.TotalUsed-amount) < state.TotalLimit
	crossedDaily := state.DailyLimit > 0 && state.DailyUsed >= state.DailyLimit && (state.DailyUsed-amount) < state.DailyLimit
	crossedWeekly := state.WeeklyLimit > 0 && state.WeeklyUsed >= state.WeeklyLimit && (state.WeeklyUsed-amount) < state.WeeklyLimit
	if crossedTotal || crossedDaily || crossedWeekly {
		if err := enqueueSchedulerOutbox(ctx, tx, service.SchedulerOutboxEventAccountChanged, &accountID, nil, nil); err != nil {
			logger.LegacyPrintf("repository.usage_billing", "[SchedulerOutbox] enqueue quota exceeded failed: account=%d err=%v", accountID, err)
			return nil, err
		}
	}
	return &state, nil
}
