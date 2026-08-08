package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	entgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementfulfillment"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementgroup"
	entuser "github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionEntitlementRepository struct {
	client *dbent.Client
}

type subscriptionEntitlementUserMutationQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func NewSubscriptionEntitlementRepository(client *dbent.Client) service.SubscriptionEntitlementRepository {
	return &subscriptionEntitlementRepository{client: client}
}

func subscriptionEntitlementUserMutationLockKey(userID int64) int64 {
	return advisoryLockHash(fmt.Sprintf("subscription-entitlement:user:%d", userID))
}

func lockSubscriptionEntitlementUserMutation(
	ctx context.Context,
	querier subscriptionEntitlementUserMutationQuerier,
	userID int64,
) error {
	if querier == nil || userID <= 0 {
		return service.ErrSubscriptionEntitlementNotFound
	}
	rows, err := querier.QueryContext(ctx, "SELECT pg_advisory_xact_lock($1)", subscriptionEntitlementUserMutationLockKey(userID))
	if err != nil {
		return err
	}
	return rows.Close()
}

func (r *subscriptionEntitlementRepository) WithUserEntitlementMutationTx(ctx context.Context, userID int64, fn func(context.Context) error) error {
	if userID <= 0 || fn == nil {
		return service.ErrSubscriptionEntitlementNotFound
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		// Serialize entitlement mutations without locking the users row. Usage billing
		// may lock an entitlement before updating balance; a users FOR UPDATE lock here
		// would create the inverse lock order and permit a PostgreSQL deadlock.
		if err := lockSubscriptionEntitlementUserMutation(txCtx, txClient, userID); err != nil {
			return err
		}
		exists, err := txClient.User.Query().
			Where(entuser.IDEQ(userID), entuser.DeletedAtIsNil()).
			Exist(txCtx)
		if err != nil {
			return err
		}
		if !exists {
			return service.ErrSubscriptionEntitlementNotFound
		}
		return fn(txCtx)
	})
}

func (r *subscriptionEntitlementRepository) Create(ctx context.Context, ent *service.SubscriptionEntitlement, groupIDs []int64) error {
	if ent == nil {
		return service.ErrSubscriptionEntitlementNilInput
	}

	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		return r.createWithClient(txCtx, txClient, ent, groupIDs)
	})
}

func (r *subscriptionEntitlementRepository) CreateTx(ctx context.Context, ent *service.SubscriptionEntitlement, groupIDs []int64) error {
	return r.Create(ctx, ent, groupIDs)
}

func (r *subscriptionEntitlementRepository) CreateWithFulfillment(ctx context.Context, ent *service.SubscriptionEntitlement, groupIDs []int64, fulfillment *service.SubscriptionEntitlementFulfillment) error {
	if ent == nil {
		return service.ErrSubscriptionEntitlementNilInput
	}
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if err := r.createWithClient(txCtx, txClient, ent, groupIDs); err != nil {
			return err
		}
		if fulfillment == nil {
			return nil
		}
		fulfillment.EntitlementID = ent.ID
		if fulfillment.UserID == 0 {
			fulfillment.UserID = ent.UserID
		}
		if fulfillment.PlanID == nil {
			fulfillment.PlanID = ent.PlanID
		}
		return createSubscriptionEntitlementFulfillmentWithClient(txCtx, txClient, fulfillment)
	})
}

func (r *subscriptionEntitlementRepository) GetByID(ctx context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	m, err := entitlementQueryWithGroups(client).
		Where(subscriptionentitlement.IDEQ(id), subscriptionentitlement.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetByIDForUpdate(ctx context.Context, id int64) (*service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	lockedID, err := client.SubscriptionEntitlement.Query().
		Where(subscriptionentitlement.IDEQ(id), subscriptionentitlement.DeletedAtIsNil()).
		ForUpdate().
		OnlyID(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return r.GetByID(ctx, lockedID)
}

func (r *subscriptionEntitlementRepository) GetBySourceID(ctx context.Context, sourceType string, sourceID int64) (*service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	m, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.SourceTypeEQ(sourceType),
			subscriptionentitlement.SourceIDEQ(sourceID),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetBySourceExternalID(ctx context.Context, sourceType, sourceExternalID string) (*service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	m, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.SourceTypeEQ(sourceType),
			subscriptionentitlement.SourceExternalIDEQ(sourceExternalID),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetFulfillmentBySourceID(ctx context.Context, sourceType string, sourceID int64) (*service.SubscriptionEntitlementFulfillment, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.SubscriptionEntitlementFulfillment.Query().
		Where(
			subscriptionentitlementfulfillment.SourceTypeEQ(sourceType),
			subscriptionentitlementfulfillment.SourceIDEQ(sourceID),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementFulfillmentEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetFulfillmentBySourceExternalID(ctx context.Context, sourceType, sourceExternalID string) (*service.SubscriptionEntitlementFulfillment, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.SubscriptionEntitlementFulfillment.Query().
		Where(
			subscriptionentitlementfulfillment.SourceTypeEQ(sourceType),
			subscriptionentitlementfulfillment.SourceExternalIDEQ(sourceExternalID),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementFulfillmentEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetFulfillmentBySourceRedeemCodeID(ctx context.Context, redeemCodeID int64) (*service.SubscriptionEntitlementFulfillment, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.SubscriptionEntitlementFulfillment.Query().
		Where(subscriptionentitlementfulfillment.SourceRedeemCodeIDEQ(redeemCodeID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementFulfillmentEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetBySourceRedeemCodeID(ctx context.Context, redeemCodeID int64) (*service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	m, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.SourceRedeemCodeIDEQ(redeemCodeID),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	return subscriptionEntitlementEntityToService(m), nil
}

func (r *subscriptionEntitlementRepository) GetActiveCoveringGroup(ctx context.Context, userID, groupID int64) ([]service.SubscriptionEntitlement, error) {
	return r.ListActiveCoveringGroupForUser(ctx, userID, groupID)
}

func (r *subscriptionEntitlementRepository) ListByUserID(ctx context.Context, userID int64) ([]service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	ms, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.UserIDEQ(userID),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Order(dbent.Desc(subscriptionentitlement.FieldExpiresAt), dbent.Asc(subscriptionentitlement.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return subscriptionEntitlementEntitiesToService(ms), nil
}

func (r *subscriptionEntitlementRepository) ListByUserPlanID(ctx context.Context, userID, planID int64) ([]service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	ms, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.UserIDEQ(userID),
			subscriptionentitlement.PlanIDEQ(planID),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Order(dbent.Desc(subscriptionentitlement.FieldExpiresAt), dbent.Asc(subscriptionentitlement.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return subscriptionEntitlementEntitiesToService(ms), nil
}

func (r *subscriptionEntitlementRepository) ListByUserPlanIDForUpdate(ctx context.Context, userID, planID int64) ([]service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	ids, err := client.SubscriptionEntitlement.Query().
		Where(
			subscriptionentitlement.UserIDEQ(userID),
			subscriptionentitlement.PlanIDEQ(planID),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Order(dbent.Asc(subscriptionentitlement.FieldID)).
		ForUpdate().
		IDs(ctx)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []service.SubscriptionEntitlement{}, nil
	}
	ms, err := entitlementQueryWithGroups(client).
		Where(subscriptionentitlement.IDIn(ids...)).
		Order(dbent.Desc(subscriptionentitlement.FieldExpiresAt), dbent.Asc(subscriptionentitlement.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return subscriptionEntitlementEntitiesToService(ms), nil
}

func (r *subscriptionEntitlementRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	ms, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.UserIDEQ(userID),
			subscriptionentitlement.StatusEQ(service.SubscriptionStatusActive),
			subscriptionentitlement.StartsAtLTE(now),
			subscriptionentitlement.ExpiresAtGT(now),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		Order(dbent.Asc(subscriptionentitlement.FieldExpiresAt), dbent.Asc(subscriptionentitlement.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return subscriptionEntitlementEntitiesToService(ms), nil
}

func (r *subscriptionEntitlementRepository) ListActiveCoveringGroupForUser(ctx context.Context, userID, groupID int64) ([]service.SubscriptionEntitlement, error) {
	client := clientFromContext(ctx, r.client)
	now := time.Now()
	ms, err := entitlementQueryWithGroups(client).
		Where(
			subscriptionentitlement.UserIDEQ(userID),
			subscriptionentitlement.StatusEQ(service.SubscriptionStatusActive),
			subscriptionentitlement.StartsAtLTE(now),
			subscriptionentitlement.ExpiresAtGT(now),
			subscriptionentitlement.DeletedAtIsNil(),
			subscriptionentitlement.HasSubscriptionEntitlementGroupsWith(
				subscriptionentitlementgroup.GroupIDEQ(groupID),
				subscriptionentitlementgroup.EnabledEQ(true),
			),
		).
		Order(dbent.Asc(subscriptionentitlement.FieldExpiresAt), dbent.Asc(subscriptionentitlement.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	return subscriptionEntitlementEntitiesToService(ms), nil
}

func (r *subscriptionEntitlementRepository) UpdateTerm(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string) error {
	client := clientFromContext(ctx, r.client)
	update := client.SubscriptionEntitlement.UpdateOneID(id).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetNotes(notes)
	if status != "" {
		update.SetStatus(status)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, service.ErrSubscriptionEntitlementAlreadyExists)
}

func (r *subscriptionEntitlementRepository) UpdateTermAndSource(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source service.SubscriptionEntitlementSourceRef) error {
	client := clientFromContext(ctx, r.client)
	err := updateSubscriptionEntitlementTermAndSourceWithClient(ctx, client, id, startsAt, expiresAt, status, notes, source, false, time.Time{}, time.Time{})
	return translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, service.ErrSubscriptionEntitlementAlreadyExists)
}

func (r *subscriptionEntitlementRepository) CompareAndSwapTerm(
	ctx context.Context,
	id int64,
	expectedUpdatedAt time.Time,
	startsAt time.Time,
	expiresAt time.Time,
	status string,
	notes string,
) (time.Time, bool, error) {
	client := clientFromContext(ctx, r.client)
	updatedAt := time.Now()
	if !updatedAt.After(expectedUpdatedAt) {
		updatedAt = expectedUpdatedAt.Add(time.Microsecond)
	}
	update := client.SubscriptionEntitlement.Update().
		Where(
			subscriptionentitlement.IDEQ(id),
			subscriptionentitlement.UpdatedAtEQ(expectedUpdatedAt),
			subscriptionentitlement.DeletedAtIsNil(),
		).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetNotes(notes).
		SetUpdatedAt(updatedAt)
	if status != "" {
		update.SetStatus(status)
	}
	affected, err := update.Save(ctx)
	if err != nil {
		return time.Time{}, false, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, service.ErrSubscriptionEntitlementAlreadyExists)
	}
	return updatedAt, affected == 1, nil
}

func (r *subscriptionEntitlementRepository) ExtendWithFulfillment(ctx context.Context, id int64, startsAt, expiresAt time.Time, status, notes string, source service.SubscriptionEntitlementSourceRef, fulfillment *service.SubscriptionEntitlementFulfillment, resetUsage bool, resetDailyStart, resetPeriodicStart time.Time) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		if err := updateSubscriptionEntitlementTermAndSourceWithClient(txCtx, txClient, id, startsAt, expiresAt, status, notes, source, resetUsage, resetDailyStart, resetPeriodicStart); err != nil {
			return translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, service.ErrSubscriptionEntitlementAlreadyExists)
		}
		if fulfillment == nil {
			return nil
		}
		fulfillment.EntitlementID = id
		return createSubscriptionEntitlementFulfillmentWithClient(txCtx, txClient, fulfillment)
	})
}

func (r *subscriptionEntitlementRepository) ActivateWindows(ctx context.Context, id int64, dailyStart, periodicStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	affected, err := client.SubscriptionEntitlement.Update().
		Where(
			subscriptionentitlement.IDEQ(id),
			subscriptionentitlement.DeletedAtIsNil(),
			subscriptionentitlement.DailyWindowStartIsNil(),
			subscriptionentitlement.WeeklyWindowStartIsNil(),
			subscriptionentitlement.MonthlyWindowStartIsNil(),
		).
		SetDailyUsageUsd(0).
		SetWeeklyUsageUsd(0).
		SetMonthlyUsageUsd(0).
		SetDailyWindowStart(dailyStart).
		SetWeeklyWindowStart(periodicStart).
		SetMonthlyWindowStart(periodicStart).
		SetUpdatedAt(time.Now().Add(time.Millisecond)).
		Save(ctx)
	return r.translateConditionalWindowUpdate(ctx, client, id, affected, err)
}

func (r *subscriptionEntitlementRepository) ResetUsage(ctx context.Context, id int64, resetDaily, resetWeekly, resetMonthly bool, dailyStart, periodicStart time.Time) error {
	if !resetDaily && !resetWeekly && !resetMonthly {
		return service.ErrSubscriptionEntitlementInvalidReset
	}
	now := time.Now()
	if resetDaily && dailyStart.IsZero() {
		dailyStart = timezone.StartOfDay(now)
	}
	if (resetWeekly || resetMonthly) && periodicStart.IsZero() {
		periodicStart = now
	}

	client := clientFromContext(ctx, r.client)
	update := client.SubscriptionEntitlement.UpdateOneID(id)
	if resetDaily {
		update.SetDailyUsageUsd(0).SetDailyWindowStart(dailyStart)
	}
	if resetWeekly {
		update.SetWeeklyUsageUsd(0).SetWeeklyWindowStart(periodicStart)
	}
	if resetMonthly {
		update.SetMonthlyUsageUsd(0).SetMonthlyWindowStart(periodicStart)
	}
	_, err := update.Save(ctx)
	return translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
}

func (r *subscriptionEntitlementRepository) ResetDailyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.SubscriptionEntitlement.Update().Where(
		subscriptionentitlement.IDEQ(id),
		subscriptionentitlement.DeletedAtIsNil(),
	)
	if expectedWindowStart == nil {
		update = update.Where(subscriptionentitlement.DailyWindowStartIsNil())
	} else {
		update = update.Where(subscriptionentitlement.DailyWindowStartEQ(*expectedWindowStart))
	}
	affected, err := update.
		SetDailyUsageUsd(0).
		SetDailyWindowStart(newWindowStart).
		SetUpdatedAt(time.Now().Add(time.Millisecond)).
		Save(ctx)
	return r.translateConditionalWindowUpdate(ctx, client, id, affected, err)
}

func (r *subscriptionEntitlementRepository) ResetWeeklyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.SubscriptionEntitlement.Update().Where(
		subscriptionentitlement.IDEQ(id),
		subscriptionentitlement.DeletedAtIsNil(),
	)
	if expectedWindowStart == nil {
		update = update.Where(subscriptionentitlement.WeeklyWindowStartIsNil())
	} else {
		update = update.Where(subscriptionentitlement.WeeklyWindowStartEQ(*expectedWindowStart))
	}
	affected, err := update.
		SetWeeklyUsageUsd(0).
		SetWeeklyWindowStart(newWindowStart).
		SetUpdatedAt(time.Now().Add(time.Millisecond)).
		Save(ctx)
	return r.translateConditionalWindowUpdate(ctx, client, id, affected, err)
}

func (r *subscriptionEntitlementRepository) ResetMonthlyUsage(ctx context.Context, id int64, expectedWindowStart *time.Time, newWindowStart time.Time) error {
	client := clientFromContext(ctx, r.client)
	update := client.SubscriptionEntitlement.Update().Where(
		subscriptionentitlement.IDEQ(id),
		subscriptionentitlement.DeletedAtIsNil(),
	)
	if expectedWindowStart == nil {
		update = update.Where(subscriptionentitlement.MonthlyWindowStartIsNil())
	} else {
		update = update.Where(subscriptionentitlement.MonthlyWindowStartEQ(*expectedWindowStart))
	}
	affected, err := update.
		SetMonthlyUsageUsd(0).
		SetMonthlyWindowStart(newWindowStart).
		SetUpdatedAt(time.Now().Add(time.Millisecond)).
		Save(ctx)
	return r.translateConditionalWindowUpdate(ctx, client, id, affected, err)
}

func (r *subscriptionEntitlementRepository) translateConditionalWindowUpdate(ctx context.Context, client *dbent.Client, id int64, affected int, err error) error {
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	if affected > 0 {
		return nil
	}

	exists, err := client.SubscriptionEntitlement.Query().
		Where(subscriptionentitlement.IDEQ(id), subscriptionentitlement.DeletedAtIsNil()).
		Exist(ctx)
	if err != nil {
		return translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	if !exists {
		return service.ErrSubscriptionEntitlementNotFound
	}
	return nil
}

func (r *subscriptionEntitlementRepository) LockEntitlementMonthlyCycle(ctx context.Context, userID, entitlementID int64) (*service.SubscriptionEntitlementMonthlyCycleSnapshot, error) {
	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, `
		SELECT id, user_id, plan_id, status, starts_at, expires_at,
			monthly_limit_usd, monthly_usage_usd, monthly_window_start
		FROM subscription_entitlements
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, entitlementID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, service.ErrSubscriptionEntitlementNotFound
	}

	var planID sql.NullInt64
	var monthlyLimit sql.NullFloat64
	var monthlyWindowStart sql.NullTime
	out := &service.SubscriptionEntitlementMonthlyCycleSnapshot{}
	if err := rows.Scan(
		&out.ID,
		&out.UserID,
		&planID,
		&out.Status,
		&out.StartsAt,
		&out.ExpiresAt,
		&monthlyLimit,
		&out.MonthlyUsageUSD,
		&monthlyWindowStart,
	); err != nil {
		return nil, err
	}
	if planID.Valid {
		out.PlanID = &planID.Int64
	}
	if monthlyLimit.Valid {
		out.MonthlyLimitUSD = &monthlyLimit.Float64
	}
	out.MonthlyWindowStart = nullableTimePtr(monthlyWindowStart)
	return out, rows.Err()
}

func (r *subscriptionEntitlementRepository) UpdateEntitlementMonthlyCycle(ctx context.Context, update service.SubscriptionEntitlementMonthlyCycleUpdate) error {
	client := clientFromContext(ctx, r.client)
	result, err := client.ExecContext(ctx, `
		UPDATE subscription_entitlements
		SET monthly_usage_usd = $1,
			monthly_window_start = $2,
			expires_at = $3,
			updated_at = $4
		WHERE id = $5 AND user_id = $6 AND deleted_at IS NULL
	`, update.NewMonthlyUsageUSD, update.NewMonthlyWindowStart, update.NewExpiresAt, update.UpdatedAt, update.EntitlementID, update.UserID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return service.ErrSubscriptionEntitlementNotFound
	}
	return nil
}

func (r *subscriptionEntitlementRepository) InsertEntitlementCycleResetLog(ctx context.Context, log service.SubscriptionEntitlementCycleResetLog) error {
	client := clientFromContext(ctx, r.client)
	mode := string(log.Mode)
	if mode == "" {
		mode = string(service.MonthlyCycleAdjustmentAdvanceNextCycle)
	}
	reason := strings.TrimSpace(log.Reason)
	_, err := client.ExecContext(ctx, `
		INSERT INTO subscription_entitlement_cycle_reset_logs (
			user_id, entitlement_id, plan_id, previous_expires_at, new_expires_at,
			previous_monthly_usage_usd, previous_monthly_window_start, new_monthly_window_start,
			deducted_days, deducted_seconds, mode, reason, admin_id, reset_monthly_usage, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
	`, log.UserID, log.EntitlementID, nullableInt64Arg(log.PlanID), log.PreviousExpiresAt, log.NewExpiresAt, log.PreviousMonthlyUsageUSD, nullableTimePtrArg(log.PreviousMonthlyWindowStart), log.NewMonthlyWindowStart, log.DeductedDays, log.DeductedSeconds, mode, reason, nullableInt64Arg(log.AdminID), log.ResetMonthlyUsage)
	return err
}

func (r *subscriptionEntitlementRepository) ApplyEntitlementUsage(ctx context.Context, id int64, costUSD float64, now time.Time) (*service.EntitlementUsageApplyResult, error) {
	if costUSD < 0 {
		return nil, service.ErrSubscriptionEntitlementInvalidUsage
	}
	if now.IsZero() {
		now = time.Now()
	}

	const updateSQL = `
		UPDATE subscription_entitlements
		SET
			daily_usage_usd = daily_usage_usd + $1,
			weekly_usage_usd = weekly_usage_usd + $1,
			monthly_usage_usd = monthly_usage_usd + $1,
			updated_at = $3
		WHERE id = $2
			AND deleted_at IS NULL
			AND status = 'active'
			AND starts_at <= $3
			AND expires_at > $3
			AND (daily_limit_usd IS NULL OR daily_usage_usd + $1 <= daily_limit_usd)
			AND (weekly_limit_usd IS NULL OR weekly_usage_usd + $1 <= weekly_limit_usd)
			AND (monthly_limit_usd IS NULL OR monthly_usage_usd + $1 <= monthly_limit_usd)
		RETURNING updated_at, daily_usage_usd, weekly_usage_usd, monthly_usage_usd,
			daily_window_start, weekly_window_start, monthly_window_start
	`

	client := clientFromContext(ctx, r.client)
	rows, err := client.QueryContext(ctx, updateSQL, costUSD, id, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	if rows.Next() {
		result := &service.EntitlementUsageApplyResult{}
		var dailyWindow, weeklyWindow, monthlyWindow sql.NullTime
		if err := rows.Scan(
			&result.UpdatedAt,
			&result.DailyUsageUSD,
			&result.WeeklyUsageUSD,
			&result.MonthlyUsageUSD,
			&dailyWindow,
			&weeklyWindow,
			&monthlyWindow,
		); err != nil {
			return nil, err
		}
		result.DailyWindowStart = nullableTimePtr(dailyWindow)
		result.WeeklyWindowStart = nullableTimePtr(weeklyWindow)
		result.MonthlyWindowStart = nullableTimePtr(monthlyWindow)
		return result, rows.Err()
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	current, err := client.SubscriptionEntitlement.Query().
		Where(subscriptionentitlement.IDEQ(id), subscriptionentitlement.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementNotFound, nil)
	}
	if current.Status != service.SubscriptionStatusActive {
		return nil, service.ErrSubscriptionEntitlementInactive
	}
	if now.Before(current.StartsAt) || !now.Before(current.ExpiresAt) {
		return nil, service.ErrSubscriptionEntitlementExpired
	}
	return nil, service.ErrSubscriptionEntitlementQuotaExceeded
}

func (r *subscriptionEntitlementRepository) ReplaceGroups(ctx context.Context, id int64, groupIDs []int64) error {
	return r.withTx(ctx, func(txCtx context.Context, txClient *dbent.Client) error {
		exists, err := txClient.SubscriptionEntitlement.Query().
			Where(subscriptionentitlement.IDEQ(id), subscriptionentitlement.DeletedAtIsNil()).
			Exist(txCtx)
		if err != nil {
			return err
		}
		if !exists {
			return service.ErrSubscriptionEntitlementNotFound
		}

		if _, err := txClient.SubscriptionEntitlementGroup.Delete().
			Where(subscriptionentitlementgroup.EntitlementIDEQ(id)).
			Exec(txCtx); err != nil {
			return err
		}
		return addSubscriptionEntitlementGroups(txCtx, txClient, id, groupIDs)
	})
}

func (r *subscriptionEntitlementRepository) createWithClient(ctx context.Context, client *dbent.Client, ent *service.SubscriptionEntitlement, groupIDs []int64) error {
	now := time.Now()
	if ent.StartsAt.IsZero() {
		ent.StartsAt = now
	}
	if ent.AssignedAt.IsZero() {
		ent.AssignedAt = now
	}
	if ent.Status == "" {
		ent.Status = service.SubscriptionStatusActive
	}
	if ent.SourceType == "" {
		ent.SourceType = service.SubscriptionEntitlementSourceUnknown
	}
	if ent.OveragePolicy == "" {
		ent.OveragePolicy = service.SubscriptionEntitlementOverageBlock
	}
	if ent.PlanSnapshot == nil {
		ent.PlanSnapshot = map[string]any{}
	}

	create := client.SubscriptionEntitlement.Create().
		SetUserID(ent.UserID).
		SetNillablePlanID(ent.PlanID).
		SetNillableLegacySubscriptionID(ent.LegacySubscriptionID).
		SetNillablePrimaryGroupID(ent.PrimaryGroupID).
		SetName(ent.Name).
		SetSourceType(ent.SourceType).
		SetStatus(ent.Status).
		SetStartsAt(ent.StartsAt).
		SetExpiresAt(ent.ExpiresAt).
		SetNillableDailyWindowStart(ent.DailyWindowStart).
		SetNillableWeeklyWindowStart(ent.WeeklyWindowStart).
		SetNillableMonthlyWindowStart(ent.MonthlyWindowStart).
		SetNillableDailyLimitUsd(ent.DailyLimitUSD).
		SetNillableWeeklyLimitUsd(ent.WeeklyLimitUSD).
		SetNillableMonthlyLimitUsd(ent.MonthlyLimitUSD).
		SetDailyUsageUsd(ent.DailyUsageUSD).
		SetWeeklyUsageUsd(ent.WeeklyUsageUSD).
		SetMonthlyUsageUsd(ent.MonthlyUsageUSD).
		SetOveragePolicy(ent.OveragePolicy).
		SetPlanSnapshot(ent.PlanSnapshot).
		SetNillableSourceID(ent.SourceID).
		SetNillableSourceExternalID(ent.SourceExternalID).
		SetNillableSourceRedeemCodeID(ent.SourceRedeemCodeID).
		SetNillableAssignedBy(ent.AssignedBy).
		SetAssignedAt(ent.AssignedAt).
		SetNotes(ent.Notes)

	created, err := create.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrSubscriptionEntitlementAlreadyExists)
	}
	if err := addSubscriptionEntitlementGroups(ctx, client, created.ID, groupIDs); err != nil {
		return translatePersistenceError(err, nil, service.ErrSubscriptionEntitlementAlreadyExists)
	}
	applySubscriptionEntitlementEntityToService(ent, created)
	return nil
}

func updateSubscriptionEntitlementTermAndSourceWithClient(ctx context.Context, client *dbent.Client, id int64, startsAt, expiresAt time.Time, status, notes string, source service.SubscriptionEntitlementSourceRef, resetUsage bool, resetDailyStart, resetPeriodicStart time.Time) error {
	update := client.SubscriptionEntitlement.UpdateOneID(id).
		SetStartsAt(startsAt).
		SetExpiresAt(expiresAt).
		SetNotes(notes)
	if status != "" {
		update.SetStatus(status)
	}
	if source.SourceType != "" {
		update.SetSourceType(source.SourceType)
	}
	if source.SourceID != nil {
		update.SetSourceID(*source.SourceID)
	}
	if source.SourceExternalID != nil {
		update.SetSourceExternalID(*source.SourceExternalID)
	}
	if source.SourceRedeemCodeID != nil {
		update.SetSourceRedeemCodeID(*source.SourceRedeemCodeID)
	}
	if source.LegacySubscriptionID != nil {
		update.SetLegacySubscriptionID(*source.LegacySubscriptionID)
	}
	if source.AssignedBy != nil {
		update.SetAssignedBy(*source.AssignedBy)
	}
	if !source.AssignedAt.IsZero() {
		update.SetAssignedAt(source.AssignedAt)
	}
	if resetUsage {
		now := time.Now()
		if resetDailyStart.IsZero() {
			resetDailyStart = timezone.StartOfDay(now)
		}
		if resetPeriodicStart.IsZero() {
			resetPeriodicStart = now
		}
		update.
			SetDailyUsageUsd(0).
			SetWeeklyUsageUsd(0).
			SetMonthlyUsageUsd(0).
			SetDailyWindowStart(resetDailyStart).
			SetWeeklyWindowStart(resetPeriodicStart).
			SetMonthlyWindowStart(resetPeriodicStart)
	}
	_, err := update.Save(ctx)
	return err
}

func createSubscriptionEntitlementFulfillmentWithClient(ctx context.Context, client *dbent.Client, fulfillment *service.SubscriptionEntitlementFulfillment) error {
	if fulfillment == nil {
		return nil
	}
	create := client.SubscriptionEntitlementFulfillment.Create().
		SetEntitlementID(fulfillment.EntitlementID).
		SetUserID(fulfillment.UserID).
		SetNillablePlanID(fulfillment.PlanID).
		SetSourceType(fulfillment.SourceType).
		SetNillableSourceID(fulfillment.SourceID).
		SetNillableSourceExternalID(fulfillment.SourceExternalID).
		SetNillableSourceRedeemCodeID(fulfillment.SourceRedeemCodeID).
		SetValidityDays(fulfillment.ValidityDays).
		SetStartsAt(fulfillment.StartsAt).
		SetExpiresAt(fulfillment.ExpiresAt).
		SetNillableAssignedBy(fulfillment.AssignedBy).
		SetAssignedAt(fulfillment.AssignedAt).
		SetNotes(fulfillment.Notes)
	created, err := create.Save(ctx)
	if err != nil {
		return translatePersistenceError(err, nil, service.ErrSubscriptionEntitlementAlreadyExists)
	}
	fulfillment.ID = created.ID
	fulfillment.CreatedAt = created.CreatedAt
	fulfillment.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *subscriptionEntitlementRepository) withTx(ctx context.Context, fn func(txCtx context.Context, txClient *dbent.Client) error) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return fn(ctx, tx.Client())
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin subscription entitlement transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	txCtx := dbent.NewTxContext(ctx, tx)
	if err := fn(txCtx, tx.Client()); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subscription entitlement transaction: %w", err)
	}
	return nil
}

func entitlementQueryWithGroups(client *dbent.Client) *dbent.SubscriptionEntitlementQuery {
	return client.SubscriptionEntitlement.Query().
		WithGroups(func(q *dbent.GroupQuery) {
			q.Order(dbent.Asc(entgroup.FieldID))
		}).
		WithSubscriptionEntitlementGroups(func(q *dbent.SubscriptionEntitlementGroupQuery) {
			q.Where(subscriptionentitlementgroup.EnabledEQ(true)).
				WithGroup().
				Order(dbent.Asc(subscriptionentitlementgroup.FieldSortOrder), dbent.Asc(subscriptionentitlementgroup.FieldGroupID))
		})
}

func addSubscriptionEntitlementGroups(ctx context.Context, client *dbent.Client, entitlementID int64, groupIDs []int64) error {
	for i, groupID := range groupIDs {
		if _, err := client.SubscriptionEntitlementGroup.Create().
			SetEntitlementID(entitlementID).
			SetGroupID(groupID).
			SetSortOrder(i).
			SetEnabled(true).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

func subscriptionEntitlementEntityToService(m *dbent.SubscriptionEntitlement) *service.SubscriptionEntitlement {
	if m == nil {
		return nil
	}
	out := &service.SubscriptionEntitlement{
		ID:                   m.ID,
		UserID:               m.UserID,
		PlanID:               m.PlanID,
		LegacySubscriptionID: m.LegacySubscriptionID,
		PrimaryGroupID:       m.PrimaryGroupID,
		Name:                 m.Name,
		SourceType:           m.SourceType,
		Status:               m.Status,
		StartsAt:             m.StartsAt,
		ExpiresAt:            m.ExpiresAt,
		DailyWindowStart:     m.DailyWindowStart,
		WeeklyWindowStart:    m.WeeklyWindowStart,
		MonthlyWindowStart:   m.MonthlyWindowStart,
		DailyLimitUSD:        m.DailyLimitUsd,
		WeeklyLimitUSD:       m.WeeklyLimitUsd,
		MonthlyLimitUSD:      m.MonthlyLimitUsd,
		DailyUsageUSD:        m.DailyUsageUsd,
		WeeklyUsageUSD:       m.WeeklyUsageUsd,
		MonthlyUsageUSD:      m.MonthlyUsageUsd,
		OveragePolicy:        m.OveragePolicy,
		PlanSnapshot:         m.PlanSnapshot,
		SourceID:             m.SourceID,
		SourceExternalID:     m.SourceExternalID,
		SourceRedeemCodeID:   m.SourceRedeemCodeID,
		AssignedBy:           m.AssignedBy,
		AssignedAt:           m.AssignedAt,
		Notes:                derefString(m.Notes),
		CreatedAt:            m.CreatedAt,
		UpdatedAt:            m.UpdatedAt,
	}
	if len(m.Edges.Groups) > 0 {
		out.Groups = make([]service.Group, 0, len(m.Edges.Groups))
		for _, g := range m.Edges.Groups {
			if sg := groupEntityToService(g); sg != nil {
				out.Groups = append(out.Groups, *sg)
			}
		}
	}
	if len(m.Edges.SubscriptionEntitlementGroups) > 0 {
		out.GroupGrants = make([]service.SubscriptionEntitlementGroupGrant, 0, len(m.Edges.SubscriptionEntitlementGroups))
		for _, grant := range m.Edges.SubscriptionEntitlementGroups {
			if grant == nil || !grant.Enabled {
				continue
			}
			var groupOut *service.Group
			if sg := groupEntityToService(grant.Edges.Group); sg != nil {
				groupOut = sg
			}
			out.GroupGrants = append(out.GroupGrants, service.SubscriptionEntitlementGroupGrant{
				GroupID:   grant.GroupID,
				SortOrder: grant.SortOrder,
				Enabled:   grant.Enabled,
				Group:     groupOut,
			})
		}
	}
	return out
}

func subscriptionEntitlementEntitiesToService(models []*dbent.SubscriptionEntitlement) []service.SubscriptionEntitlement {
	out := make([]service.SubscriptionEntitlement, 0, len(models))
	for _, m := range models {
		if ent := subscriptionEntitlementEntityToService(m); ent != nil {
			out = append(out, *ent)
		}
	}
	return out
}

func subscriptionEntitlementFulfillmentEntityToService(m *dbent.SubscriptionEntitlementFulfillment) *service.SubscriptionEntitlementFulfillment {
	if m == nil {
		return nil
	}
	return &service.SubscriptionEntitlementFulfillment{
		ID:                 m.ID,
		EntitlementID:      m.EntitlementID,
		UserID:             m.UserID,
		PlanID:             m.PlanID,
		SourceType:         m.SourceType,
		SourceID:           m.SourceID,
		SourceExternalID:   m.SourceExternalID,
		SourceRedeemCodeID: m.SourceRedeemCodeID,
		ValidityDays:       m.ValidityDays,
		StartsAt:           m.StartsAt,
		ExpiresAt:          m.ExpiresAt,
		AssignedBy:         m.AssignedBy,
		AssignedAt:         m.AssignedAt,
		Notes:              derefString(m.Notes),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
	}
}

func applySubscriptionEntitlementEntityToService(dst *service.SubscriptionEntitlement, src *dbent.SubscriptionEntitlement) {
	if dst == nil || src == nil {
		return
	}
	dst.ID = src.ID
	dst.CreatedAt = src.CreatedAt
	dst.UpdatedAt = src.UpdatedAt
}

func nullableTimePtr(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func nullableTimePtrArg(v *time.Time) any {
	if v == nil {
		return nil
	}
	return *v
}
