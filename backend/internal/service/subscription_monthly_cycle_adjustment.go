package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

type MonthlyCycleAdjustmentMode string

const (
	MonthlyCycleAdjustmentAdvanceNextCycle MonthlyCycleAdjustmentMode = "advance_next_cycle"
	MonthlyCycleAdjustmentCompensateReset  MonthlyCycleAdjustmentMode = "compensate_reset"
	MonthlyCycleAdjustmentAlignToReset     MonthlyCycleAdjustmentMode = "align_to_reset"
	MonthlyCycleAdjustmentAlignToExpiry    MonthlyCycleAdjustmentMode = "align_to_expiry"
	MonthlyCycleAdjustmentCustom           MonthlyCycleAdjustmentMode = "custom"

	MonthlyCycleAdjustmentTargetSubscription = "subscription"
	MonthlyCycleAdjustmentTargetEntitlement  = "entitlement"

	monthlyCycleAdjustmentReasonMaxLength = 500
	maxMonthlyCycleAdjustmentCycleCount   = (MaxValidityDays + 29) / 30
)

type MonthlyCycleAdjustmentInput struct {
	Mode                     MonthlyCycleAdjustmentMode `json:"mode"`
	CycleCount               int                        `json:"cycle_count,omitempty"`
	CustomMonthlyWindowStart *time.Time                 `json:"custom_monthly_window_start,omitempty"`
	CustomExpiresAt          *time.Time                 `json:"custom_expires_at,omitempty"`
	ResetMonthlyUsage        *bool                      `json:"reset_monthly_usage,omitempty"`
	Reason                   string                     `json:"reason,omitempty"`

	AdminID int64     `json:"-"`
	Now     time.Time `json:"-"`
}

type MonthlyCycleAdjustmentPreview struct {
	Mode           MonthlyCycleAdjustmentMode `json:"mode"`
	TargetType     string                     `json:"target_type"`
	SubscriptionID int64                      `json:"subscription_id"`
	EntitlementID  *int64                     `json:"entitlement_id,omitempty"`
	UserID         int64                      `json:"user_id"`
	GroupID        int64                      `json:"group_id,omitempty"`
	PlanID         *int64                     `json:"plan_id,omitempty"`
	Status         string                     `json:"status"`

	CurrentExpiresAt          time.Time  `json:"current_expires_at"`
	NewExpiresAt              time.Time  `json:"new_expires_at"`
	CurrentMonthlyWindowStart *time.Time `json:"current_monthly_window_start,omitempty"`
	NewMonthlyWindowStart     time.Time  `json:"new_monthly_window_start"`
	CurrentResetAt            *time.Time `json:"current_reset_at,omitempty"`
	NewResetAt                *time.Time `json:"new_reset_at,omitempty"`

	MonthlyLimitUSD        *float64 `json:"monthly_limit_usd,omitempty"`
	CurrentMonthlyUsageUSD float64  `json:"current_monthly_usage_usd"`
	NewMonthlyUsageUSD     float64  `json:"new_monthly_usage_usd"`
	ResetMonthlyUsage      bool     `json:"reset_monthly_usage"`

	DeductedDays    int   `json:"deducted_days"`
	DeductedSeconds int64 `json:"deducted_seconds"`
	CycleCount      int   `json:"cycle_count"`
	FullCycles      int   `json:"full_cycles"`
	TailSeconds     int64 `json:"tail_seconds"`

	CanApply          bool     `json:"can_apply"`
	UnavailableReason string   `json:"unavailable_reason,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`
	Reason            string   `json:"reason,omitempty"`
}

type monthlyCycleAdjustmentTarget struct {
	targetType     string
	subscriptionID int64
	entitlementID  *int64
	userID         int64
	groupID        int64
	planID         *int64
	status         string
	startsAt       time.Time
	expiresAt      time.Time

	monthlyWindowStart *time.Time
	monthlyUsageUSD    float64
	monthlyLimitUSD    *float64

	entitlement  *SubscriptionEntitlement
	subscription *UserSubscription
}

func (s *SubscriptionService) PreviewMonthlyCycleAdjustment(ctx context.Context, subscriptionID int64, input MonthlyCycleAdjustmentInput) (*MonthlyCycleAdjustmentPreview, error) {
	target, err := s.resolveMonthlyCycleAdjustmentTarget(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	return buildMonthlyCycleAdjustmentPreview(target, input, monthlyCycleAdjustmentNow(input))
}

func (s *SubscriptionService) ApplyMonthlyCycleAdjustment(ctx context.Context, subscriptionID int64, input MonthlyCycleAdjustmentInput) (*MonthlyCycleAdjustmentPreview, error) {
	target, err := s.resolveMonthlyCycleAdjustmentTarget(ctx, subscriptionID)
	if err != nil {
		return nil, err
	}
	if target.entitlementID != nil {
		return s.applyEntitlementMonthlyCycleAdjustment(ctx, target, input)
	}
	return s.applySubscriptionMonthlyCycleAdjustment(ctx, target, input)
}

func (s *SubscriptionService) resolveMonthlyCycleAdjustmentTarget(ctx context.Context, subscriptionID int64) (*monthlyCycleAdjustmentTarget, error) {
	if entitlementID, ok := syntheticEntitlementIDFromSubscriptionID(subscriptionID); ok {
		ent, err := s.getAdminEntitlementForMonthlyCycleAdjustment(ctx, entitlementID)
		if err != nil {
			return nil, err
		}
		return monthlyCycleAdjustmentTargetFromEntitlement(subscriptionID, ent), nil
	}

	if s == nil || s.userSubRepo == nil {
		return nil, ErrSubscriptionNotFound
	}
	sub, err := s.userSubRepo.GetByID(ctx, subscriptionID)
	if err != nil {
		return nil, ErrSubscriptionNotFound
	}
	if sub.EntitlementLink != nil && sub.EntitlementLink.EntitlementID > 0 {
		ent, err := s.getAdminEntitlementForMonthlyCycleAdjustment(ctx, sub.EntitlementLink.EntitlementID)
		if err != nil {
			return nil, err
		}
		target := monthlyCycleAdjustmentTargetFromEntitlement(subscriptionID, ent)
		target.subscription = sub
		return target, nil
	}

	monthlyLimit := monthlyLimitForSubscription(sub)
	if monthlyLimit == nil && sub.Group == nil && s.groupRepo != nil {
		if group, err := s.groupRepo.GetByID(ctx, sub.GroupID); err == nil {
			sub.Group = group
			monthlyLimit = monthlyLimitForSubscription(sub)
		}
	}
	return monthlyCycleAdjustmentTargetFromSubscription(sub, monthlyLimit), nil
}

func (s *SubscriptionService) getAdminEntitlementForMonthlyCycleAdjustment(ctx context.Context, entitlementID int64) (*SubscriptionEntitlement, error) {
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return nil, err
	}
	ent, err := entitlementSvc.entitlementRepo.GetByID(ctx, entitlementID)
	if err != nil {
		return nil, err
	}
	entitlementSvc.attachEntitlementEconomics(ctx, ent)
	return ent, nil
}

func monthlyCycleAdjustmentTargetFromEntitlement(subscriptionID int64, ent *SubscriptionEntitlement) *monthlyCycleAdjustmentTarget {
	entitlementID := ent.ID
	groupID := entitlementAdminGroupID(ent)
	return &monthlyCycleAdjustmentTarget{
		targetType:         MonthlyCycleAdjustmentTargetEntitlement,
		subscriptionID:     subscriptionID,
		entitlementID:      &entitlementID,
		userID:             ent.UserID,
		groupID:            groupID,
		planID:             cloneInt64PtrForMonthlyCycleAdjustment(ent.PlanID),
		status:             ent.Status,
		startsAt:           ent.StartsAt,
		expiresAt:          ent.ExpiresAt,
		monthlyWindowStart: cloneTimePtrForMonthlyCycleAdjustment(ent.MonthlyWindowStart),
		monthlyUsageUSD:    ent.MonthlyUsageUSD,
		monthlyLimitUSD:    cloneFloat64PtrForMonthlyCycleAdjustment(ent.MonthlyLimitUSD),
		entitlement:        ent,
	}
}

func monthlyCycleAdjustmentTargetFromSubscription(sub *UserSubscription, monthlyLimit *float64) *monthlyCycleAdjustmentTarget {
	return &monthlyCycleAdjustmentTarget{
		targetType:         MonthlyCycleAdjustmentTargetSubscription,
		subscriptionID:     sub.ID,
		userID:             sub.UserID,
		groupID:            sub.GroupID,
		status:             sub.Status,
		startsAt:           sub.StartsAt,
		expiresAt:          sub.ExpiresAt,
		monthlyWindowStart: cloneTimePtrForMonthlyCycleAdjustment(sub.MonthlyWindowStart),
		monthlyUsageUSD:    sub.MonthlyUsageUSD,
		monthlyLimitUSD:    cloneFloat64PtrForMonthlyCycleAdjustment(monthlyLimit),
		subscription:       sub,
	}
}

func buildMonthlyCycleAdjustmentPreview(target *monthlyCycleAdjustmentTarget, input MonthlyCycleAdjustmentInput, now time.Time) (*MonthlyCycleAdjustmentPreview, error) {
	if target == nil || target.userID <= 0 {
		return nil, ErrSubscriptionNotFound
	}
	input = normalizeMonthlyCycleAdjustmentInput(input)
	if input.Mode == "" {
		return nil, ErrMonthlyCycleAdjustmentInvalid.WithCause(fmt.Errorf("invalid monthly cycle adjustment mode"))
	}

	currentResetAt := monthlyCycleResetAt(target.monthlyWindowStart, target.startsAt, now)
	preview := &MonthlyCycleAdjustmentPreview{
		Mode:                      input.Mode,
		TargetType:                target.targetType,
		SubscriptionID:            target.subscriptionID,
		EntitlementID:             cloneInt64PtrForMonthlyCycleAdjustment(target.entitlementID),
		UserID:                    target.userID,
		GroupID:                   target.groupID,
		PlanID:                    cloneInt64PtrForMonthlyCycleAdjustment(target.planID),
		Status:                    target.status,
		CurrentExpiresAt:          target.expiresAt,
		NewExpiresAt:              target.expiresAt,
		CurrentMonthlyWindowStart: cloneTimePtrForMonthlyCycleAdjustment(target.monthlyWindowStart),
		NewMonthlyWindowStart:     monthlyCycleAdjustmentCurrentAnchor(target, now),
		CurrentResetAt:            cloneTimeValueForMonthlyCycleAdjustment(currentResetAt),
		MonthlyLimitUSD:           cloneFloat64PtrForMonthlyCycleAdjustment(target.monthlyLimitUSD),
		CurrentMonthlyUsageUSD:    target.monthlyUsageUSD,
		NewMonthlyUsageUSD:        target.monthlyUsageUSD,
		ResetMonthlyUsage:         monthlyCycleAdjustmentResetUsage(input),
		CanApply:                  true,
		Reason:                    input.Reason,
	}

	if reason := validateMonthlyCycleAdjustmentInput(input); reason != "" {
		preview.markUnavailable(reason)
		return preview, nil
	}

	if reason := monthlyCycleAdjustmentInactiveReason(target, now); reason != "" {
		preview.markUnavailable(reason)
		return preview, nil
	}

	switch input.Mode {
	case MonthlyCycleAdjustmentAdvanceNextCycle:
		applyAdvanceNextCyclePreview(preview, target, now)
	case MonthlyCycleAdjustmentCompensateReset:
		applyCompensateResetPreview(preview, input, now)
	case MonthlyCycleAdjustmentAlignToReset:
		applyAlignToResetPreview(preview, target, input, now)
	case MonthlyCycleAdjustmentAlignToExpiry:
		applyAlignToExpiryPreview(preview, target, input, now)
	case MonthlyCycleAdjustmentCustom:
		if err := applyCustomMonthlyCyclePreview(preview, input, now); err != nil {
			return nil, err
		}
	default:
		return nil, ErrMonthlyCycleAdjustmentInvalid.WithCause(fmt.Errorf("invalid monthly cycle adjustment mode"))
	}

	finalizeMonthlyCycleAdjustmentPreview(preview, target, now)
	return preview, nil
}

func applyAdvanceNextCyclePreview(preview *MonthlyCycleAdjustmentPreview, target *monthlyCycleAdjustmentTarget, now time.Time) {
	if target.monthlyLimitUSD == nil || *target.monthlyLimitUSD <= 0 {
		preview.markUnavailable("monthly_limit_required")
		return
	}
	if !canAdvanceMonthlyCycleByUsage(target.monthlyUsageUSD, *target.monthlyLimitUSD) {
		preview.markUnavailable("monthly_usage_not_exhausted")
		return
	}
	if preview.CurrentResetAt == nil || !preview.CurrentResetAt.After(now) {
		preview.markUnavailable("invalid_reset_window")
		return
	}
	if !canAdvanceMonthlyCycleByValidity(target.startsAt, target.expiresAt, *preview.CurrentResetAt) {
		preview.markUnavailable("no_future_monthly_cycle")
		return
	}

	remaining := preview.CurrentResetAt.Sub(now)
	deductedSeconds := ceilDurationSeconds(remaining)
	if deductedSeconds <= 0 {
		preview.markUnavailable("invalid_reset_window")
		return
	}
	newExpiresAt := target.expiresAt.Add(-time.Duration(deductedSeconds) * time.Second)
	if !newExpiresAt.After(now) {
		preview.markUnavailable("no_future_monthly_cycle")
		return
	}

	preview.NewMonthlyWindowStart = now
	preview.NewExpiresAt = newExpiresAt
	preview.DeductedSeconds = deductedSeconds
	preview.DeductedDays = ceilSecondsToDays(deductedSeconds)
}

func applyCompensateResetPreview(preview *MonthlyCycleAdjustmentPreview, input MonthlyCycleAdjustmentInput, now time.Time) {
	if input.Reason == "" {
		preview.markUnavailable("reason_required")
		return
	}
	preview.NewMonthlyWindowStart = now
	preview.NewExpiresAt = preview.CurrentExpiresAt
	preview.addWarning("compensate_reset_does_not_deduct_validity")
}

func applyAlignToResetPreview(preview *MonthlyCycleAdjustmentPreview, target *monthlyCycleAdjustmentTarget, input MonthlyCycleAdjustmentInput, now time.Time) {
	cycleCount := input.CycleCount
	if cycleCount < 0 {
		preview.markUnavailable("cycle_count_out_of_range")
		return
	}
	if cycleCount <= 0 {
		cycleCount = inferMonthlyCycleCount(target, now)
	}
	duration, ok := monthlyCycleAdjustmentCycleDuration(cycleCount)
	if !ok {
		preview.markUnavailable("cycle_count_out_of_range")
		return
	}
	anchor := monthlyCycleAdjustmentCurrentAnchor(target, now)
	newExpiresAt, ok := monthlyCycleAdjustmentAddWithinMax(anchor, duration)
	if !ok {
		preview.markUnavailable("expires_at_after_max")
		return
	}
	if !newExpiresAt.After(now) {
		preview.markUnavailable("expires_at_not_in_future")
		return
	}
	preview.CycleCount = cycleCount
	preview.NewMonthlyWindowStart = anchor
	preview.NewExpiresAt = newExpiresAt
}

func applyAlignToExpiryPreview(preview *MonthlyCycleAdjustmentPreview, target *monthlyCycleAdjustmentTarget, input MonthlyCycleAdjustmentInput, now time.Time) {
	cycleCount := input.CycleCount
	if cycleCount < 0 {
		preview.markUnavailable("cycle_count_out_of_range")
		return
	}
	if cycleCount <= 0 {
		cycleCount = inferMonthlyCycleCount(target, now)
	}
	duration, ok := monthlyCycleAdjustmentCycleDuration(cycleCount)
	if !ok {
		preview.markUnavailable("cycle_count_out_of_range")
		return
	}
	if target.expiresAt.After(MaxExpiresAt) {
		preview.markUnavailable("expires_at_after_max")
		return
	}
	newWindowStart := target.expiresAt.Add(-duration)
	if newWindowStart.After(now) {
		preview.markUnavailable("monthly_window_start_in_future")
		return
	}
	preview.CycleCount = cycleCount
	preview.NewMonthlyWindowStart = newWindowStart
	preview.NewExpiresAt = target.expiresAt
}

func applyCustomMonthlyCyclePreview(preview *MonthlyCycleAdjustmentPreview, input MonthlyCycleAdjustmentInput, now time.Time) error {
	if input.CustomMonthlyWindowStart == nil || input.CustomExpiresAt == nil {
		preview.markUnavailable("custom_times_required")
		return nil
	}
	newWindowStart := input.CustomMonthlyWindowStart.Truncate(time.Second)
	newExpiresAt := input.CustomExpiresAt.Truncate(time.Second)
	if newWindowStart.IsZero() || newExpiresAt.IsZero() {
		preview.markUnavailable("custom_times_required")
		return nil
	}
	if newWindowStart.After(now) {
		preview.markUnavailable("monthly_window_start_in_future")
		return nil
	}
	if !newExpiresAt.After(now) {
		preview.markUnavailable("expires_at_not_in_future")
		return nil
	}
	if !newExpiresAt.After(newWindowStart) {
		preview.markUnavailable("expires_at_must_be_after_window_start")
		return nil
	}
	if newExpiresAt.After(MaxExpiresAt) {
		preview.markUnavailable("expires_at_after_max")
		return nil
	}
	preview.NewMonthlyWindowStart = newWindowStart
	preview.NewExpiresAt = newExpiresAt
	preview.CycleCount = 0
	return nil
}

func finalizeMonthlyCycleAdjustmentPreview(preview *MonthlyCycleAdjustmentPreview, target *monthlyCycleAdjustmentTarget, now time.Time) {
	if preview == nil || !preview.CanApply {
		return
	}
	if preview.NewExpiresAt.After(MaxExpiresAt) {
		preview.markUnavailable("expires_at_after_max")
		return
	}
	if preview.ResetMonthlyUsage {
		preview.NewMonthlyUsageUSD = 0
		preview.addWarning("monthly_usage_will_reset")
	} else {
		preview.NewMonthlyUsageUSD = target.monthlyUsageUSD
	}
	if preview.CycleCount < 0 {
		preview.CycleCount = 0
	}
	newResetAt := monthlyCycleResetAt(&preview.NewMonthlyWindowStart, target.startsAt, now)
	preview.NewResetAt = &newResetAt

	validity := preview.NewExpiresAt.Sub(preview.NewMonthlyWindowStart)
	if validity > 0 {
		preview.FullCycles = int(validity / monthlyCycleDuration)
		preview.TailSeconds = int64((validity % monthlyCycleDuration) / time.Second)
	}
	if preview.NewExpiresAt.Before(MaxExpiresAt) && preview.NewExpiresAt.After(target.expiresAt) {
		preview.addWarning("expires_at_will_extend")
	}
	if preview.NewExpiresAt.Before(target.expiresAt) {
		preview.addWarning("expires_at_will_shorten")
	}
}

func (s *SubscriptionService) applyEntitlementMonthlyCycleAdjustment(ctx context.Context, target *monthlyCycleAdjustmentTarget, input MonthlyCycleAdjustmentInput) (*MonthlyCycleAdjustmentPreview, error) {
	entitlementSvc, err := s.adminEntitlementService()
	if err != nil {
		return nil, err
	}
	store, ok := entitlementSvc.entitlementRepo.(subscriptionEntitlementMonthlyCycleStore)
	if !ok {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	if target.entitlementID == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}

	now := monthlyCycleAdjustmentNow(input)
	var applied *MonthlyCycleAdjustmentPreview
	var refreshed *SubscriptionEntitlement
	if err := store.WithUserEntitlementMutationTx(ctx, target.userID, func(txCtx context.Context) error {
		snapshot, err := store.LockEntitlementMonthlyCycle(txCtx, target.userID, *target.entitlementID)
		if err != nil {
			return err
		}
		lockedTarget := monthlyCycleAdjustmentTargetFromSnapshot(target.subscriptionID, target.groupID, snapshot)
		preview, err := buildMonthlyCycleAdjustmentPreview(lockedTarget, input, now)
		if err != nil {
			return err
		}
		if !preview.CanApply {
			return ErrMonthlyCycleAdjustmentUnavailable.WithCause(fmt.Errorf("%s", preview.UnavailableReason))
		}
		if err := store.UpdateEntitlementMonthlyCycle(txCtx, SubscriptionEntitlementMonthlyCycleUpdate{
			EntitlementID:         snapshot.ID,
			UserID:                snapshot.UserID,
			NewExpiresAt:          preview.NewExpiresAt,
			NewMonthlyWindowStart: preview.NewMonthlyWindowStart,
			NewMonthlyUsageUSD:    preview.NewMonthlyUsageUSD,
			UpdatedAt:             now.Add(time.Millisecond),
		}); err != nil {
			return err
		}
		if err := store.InsertEntitlementCycleResetLog(txCtx, SubscriptionEntitlementCycleResetLog{
			UserID:                     snapshot.UserID,
			EntitlementID:              snapshot.ID,
			PlanID:                     cloneInt64PtrForMonthlyCycleAdjustment(snapshot.PlanID),
			PreviousExpiresAt:          snapshot.ExpiresAt,
			NewExpiresAt:               preview.NewExpiresAt,
			PreviousMonthlyUsageUSD:    snapshot.MonthlyUsageUSD,
			PreviousMonthlyWindowStart: cloneTimePtrForMonthlyCycleAdjustment(snapshot.MonthlyWindowStart),
			NewMonthlyWindowStart:      preview.NewMonthlyWindowStart,
			DeductedDays:               preview.DeductedDays,
			DeductedSeconds:            preview.DeductedSeconds,
			ResetMonthlyUsage:          preview.ResetMonthlyUsage,
			Mode:                       preview.Mode,
			Reason:                     preview.Reason,
			AdminID:                    monthlyCycleAdjustmentAdminIDPtr(input.AdminID),
		}); err != nil {
			return err
		}
		refreshed, err = entitlementSvc.entitlementRepo.GetByID(txCtx, *target.entitlementID)
		if err != nil {
			return err
		}
		if err := syncLinkedLegacySubscriptionLifecycle(txCtx, s.userSubRepo, refreshed); err != nil {
			return err
		}
		applied = preview
		return nil
	}); err != nil {
		return nil, err
	}

	if refreshed == nil {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	entitlementSvc.attachEntitlementEconomics(ctx, refreshed)
	s.invalidateEntitlementLegacyAlias(refreshed)
	return applied, nil
}

func monthlyCycleAdjustmentTargetFromSnapshot(subscriptionID, groupID int64, snapshot *SubscriptionEntitlementMonthlyCycleSnapshot) *monthlyCycleAdjustmentTarget {
	entitlementID := snapshot.ID
	return &monthlyCycleAdjustmentTarget{
		targetType:         MonthlyCycleAdjustmentTargetEntitlement,
		subscriptionID:     subscriptionID,
		entitlementID:      &entitlementID,
		userID:             snapshot.UserID,
		groupID:            groupID,
		planID:             cloneInt64PtrForMonthlyCycleAdjustment(snapshot.PlanID),
		status:             snapshot.Status,
		startsAt:           snapshot.StartsAt,
		expiresAt:          snapshot.ExpiresAt,
		monthlyWindowStart: cloneTimePtrForMonthlyCycleAdjustment(snapshot.MonthlyWindowStart),
		monthlyUsageUSD:    snapshot.MonthlyUsageUSD,
		monthlyLimitUSD:    cloneFloat64PtrForMonthlyCycleAdjustment(snapshot.MonthlyLimitUSD),
	}
}

func (s *SubscriptionService) applySubscriptionMonthlyCycleAdjustment(ctx context.Context, target *monthlyCycleAdjustmentTarget, input MonthlyCycleAdjustmentInput) (*MonthlyCycleAdjustmentPreview, error) {
	if s.entClient == nil {
		return nil, ErrSubscriptionMaintenance.WithCause(fmt.Errorf("monthly cycle transaction client is not configured"))
	}
	return s.applySubscriptionMonthlyCycleAdjustmentTx(ctx, target, input)
}

func (s *SubscriptionService) applySubscriptionMonthlyCycleAdjustmentTx(ctx context.Context, target *monthlyCycleAdjustmentTarget, input MonthlyCycleAdjustmentInput) (*MonthlyCycleAdjustmentPreview, error) {
	now := monthlyCycleAdjustmentNow(input)
	tx, err := s.entClient.Tx(ctx)
	if err != nil {
		return nil, err
	}
	txClient := tx.Client()

	var previousUsage float64
	var previousWindowStart sql.NullTime
	var previousStartsAt time.Time
	var previousExpiresAt time.Time
	var previousStatus string
	var previousUpdatedAt sql.NullTime
	rows, err := txClient.QueryContext(ctx, `
		SELECT monthly_usage_usd, monthly_window_start, starts_at, expires_at, status, updated_at
		FROM user_subscriptions
		WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL
		FOR UPDATE
	`, target.subscriptionID, target.userID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if !rows.Next() {
		_ = rows.Close()
		_ = tx.Rollback()
		return nil, ErrSubscriptionNotFound
	}
	if err := rows.Scan(&previousUsage, &previousWindowStart, &previousStartsAt, &previousExpiresAt, &previousStatus, &previousUpdatedAt); err != nil {
		_ = rows.Close()
		_ = tx.Rollback()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		_ = tx.Rollback()
		return nil, err
	}

	var previousWindowStartPtr *time.Time
	if previousWindowStart.Valid {
		previousWindowStartPtr = &previousWindowStart.Time
	}
	lockedTarget := *target
	lockedTarget.status = previousStatus
	lockedTarget.startsAt = previousStartsAt
	lockedTarget.expiresAt = previousExpiresAt
	lockedTarget.monthlyWindowStart = previousWindowStartPtr
	lockedTarget.monthlyUsageUSD = previousUsage

	preview, err := buildMonthlyCycleAdjustmentPreview(&lockedTarget, input, now)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if !preview.CanApply {
		_ = tx.Rollback()
		return nil, ErrMonthlyCycleAdjustmentUnavailable.WithCause(fmt.Errorf("%s", preview.UnavailableReason))
	}
	if s.billingCacheService != nil {
		if err := s.billingCacheService.InvalidateSubscriptionBefore(ctx, target.userID, target.groupID, subscriptionCacheVersionFromNullTime(previousUpdatedAt)); err != nil {
			_ = tx.Rollback()
			return nil, ErrSubscriptionMaintenance.WithCause(fmt.Errorf("invalidate subscription billing cache: %w", err))
		}
	}

	result, err := txClient.ExecContext(ctx, `
		UPDATE user_subscriptions
		SET monthly_usage_usd = $1,
			monthly_window_start = $2,
			expires_at = $3,
			updated_at = $4
		WHERE id = $5 AND user_id = $6 AND deleted_at IS NULL
	`, preview.NewMonthlyUsageUSD, preview.NewMonthlyWindowStart, preview.NewExpiresAt, now.Add(time.Millisecond), target.subscriptionID, target.userID)
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if affected == 0 {
		_ = tx.Rollback()
		return nil, ErrSubscriptionNotFound
	}
	if _, err := txClient.ExecContext(ctx, `
		INSERT INTO subscription_cycle_reset_logs (
			user_id, subscription_id, group_id, previous_expires_at, new_expires_at,
			previous_monthly_usage_usd, previous_monthly_window_start, new_monthly_window_start,
			deducted_days, deducted_seconds, mode, reason, admin_id, reset_monthly_usage, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, NOW())
	`, target.userID, target.subscriptionID, target.groupID, previousExpiresAt, preview.NewExpiresAt, previousUsage, nullableTimeArg(previousWindowStart), preview.NewMonthlyWindowStart, preview.DeductedDays, preview.DeductedSeconds, preview.Mode, preview.Reason, monthlyCycleAdjustmentAdminIDArg(input.AdminID), preview.ResetMonthlyUsage); err != nil {
		_ = tx.Rollback()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	s.InvalidateSubCache(target.userID, target.groupID)
	s.waitSubCacheInvalidation()
	return preview, nil
}

func normalizeMonthlyCycleAdjustmentMode(mode MonthlyCycleAdjustmentMode) MonthlyCycleAdjustmentMode {
	switch mode {
	case MonthlyCycleAdjustmentAdvanceNextCycle,
		MonthlyCycleAdjustmentCompensateReset,
		MonthlyCycleAdjustmentAlignToReset,
		MonthlyCycleAdjustmentAlignToExpiry,
		MonthlyCycleAdjustmentCustom:
		return mode
	default:
		return ""
	}
}

func normalizeMonthlyCycleAdjustmentInput(input MonthlyCycleAdjustmentInput) MonthlyCycleAdjustmentInput {
	input.Mode = normalizeMonthlyCycleAdjustmentMode(input.Mode)
	input.Reason = strings.TrimSpace(input.Reason)
	return input
}

func monthlyCycleAdjustmentResetUsage(input MonthlyCycleAdjustmentInput) bool {
	switch input.Mode {
	case MonthlyCycleAdjustmentAdvanceNextCycle, MonthlyCycleAdjustmentCompensateReset:
		return true
	case MonthlyCycleAdjustmentAlignToReset, MonthlyCycleAdjustmentAlignToExpiry, MonthlyCycleAdjustmentCustom:
		return input.ResetMonthlyUsage != nil && *input.ResetMonthlyUsage
	default:
		return false
	}
}

func validateMonthlyCycleAdjustmentInput(input MonthlyCycleAdjustmentInput) string {
	if utf8.RuneCountInString(input.Reason) > monthlyCycleAdjustmentReasonMaxLength {
		return "reason_too_long"
	}
	return ""
}

func monthlyCycleAdjustmentNow(input MonthlyCycleAdjustmentInput) time.Time {
	if !input.Now.IsZero() {
		return input.Now.Truncate(time.Second)
	}
	return time.Now().Truncate(time.Second)
}

func monthlyCycleAdjustmentInactiveReason(target *monthlyCycleAdjustmentTarget, now time.Time) string {
	switch target.status {
	case SubscriptionStatusActive:
	case SubscriptionStatusSuspended:
		return "inactive_subscription"
	default:
		return "expired_subscription"
	}
	if now.Before(target.startsAt) || !target.expiresAt.After(now) {
		return "expired_subscription"
	}
	return ""
}

func monthlyCycleAdjustmentCurrentAnchor(target *monthlyCycleAdjustmentTarget, now time.Time) time.Time {
	resetAt := monthlyCycleResetAt(target.monthlyWindowStart, target.startsAt, now)
	if resetAt.After(now) {
		return resetAt.Add(-monthlyCycleDuration)
	}
	if target.monthlyWindowStart != nil && !target.monthlyWindowStart.IsZero() {
		return *target.monthlyWindowStart
	}
	if !target.startsAt.IsZero() {
		return target.startsAt
	}
	return now
}

func inferMonthlyCycleCount(target *monthlyCycleAdjustmentTarget, now time.Time) int {
	anchor := monthlyCycleAdjustmentCurrentAnchor(target, now)
	if anchor.IsZero() || !target.expiresAt.After(anchor) {
		return 1
	}
	seconds := ceilDurationSeconds(target.expiresAt.Sub(anchor))
	cycleSeconds := int64(monthlyCycleDuration / time.Second)
	cycles := int((seconds + cycleSeconds - 1) / cycleSeconds)
	if cycles < 1 {
		return 1
	}
	return cycles
}

func monthlyCycleAdjustmentCycleDuration(cycleCount int) (time.Duration, bool) {
	if cycleCount <= 0 || cycleCount > maxMonthlyCycleAdjustmentCycleCount {
		return 0, false
	}
	return time.Duration(cycleCount) * monthlyCycleDuration, true
}

func monthlyCycleAdjustmentAddWithinMax(anchor time.Time, duration time.Duration) (time.Time, bool) {
	if anchor.IsZero() || duration <= 0 {
		return time.Time{}, false
	}
	if anchor.After(MaxExpiresAt.Add(-duration)) {
		return time.Time{}, false
	}
	return anchor.Add(duration), true
}

func monthlyCycleAdjustmentAdminIDPtr(adminID int64) *int64 {
	if adminID <= 0 {
		return nil
	}
	return &adminID
}

func monthlyCycleAdjustmentAdminIDArg(adminID int64) any {
	if adminID <= 0 {
		return nil
	}
	return adminID
}

func ceilSecondsToDays(seconds int64) int {
	if seconds <= 0 {
		return 0
	}
	days := int((time.Duration(seconds)*time.Second + 24*time.Hour - 1) / (24 * time.Hour))
	if days <= 0 {
		return 1
	}
	return days
}

func (p *MonthlyCycleAdjustmentPreview) markUnavailable(reason string) {
	p.CanApply = false
	p.UnavailableReason = reason
}

func (p *MonthlyCycleAdjustmentPreview) addWarning(warning string) {
	for _, existing := range p.Warnings {
		if existing == warning {
			return
		}
	}
	p.Warnings = append(p.Warnings, warning)
}

func monthlyLimitForSubscription(sub *UserSubscription) *float64 {
	if sub == nil || sub.Group == nil || sub.Group.MonthlyLimitUSD == nil || *sub.Group.MonthlyLimitUSD <= 0 {
		return nil
	}
	return sub.Group.MonthlyLimitUSD
}

func cloneInt64PtrForMonthlyCycleAdjustment(v *int64) *int64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFloat64PtrForMonthlyCycleAdjustment(v *float64) *float64 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneTimePtrForMonthlyCycleAdjustment(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	out := v.Truncate(time.Second)
	return &out
}

func cloneTimeValueForMonthlyCycleAdjustment(v time.Time) *time.Time {
	if v.IsZero() {
		return nil
	}
	out := v.Truncate(time.Second)
	return &out
}
