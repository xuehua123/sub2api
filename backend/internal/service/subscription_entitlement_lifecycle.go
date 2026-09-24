package service

import (
	"context"
	"time"
)

type SubscriptionEntitlementEvent struct {
	ID                int64      `json:"id"`
	EntitlementID     int64      `json:"entitlement_id"`
	Kind              string     `json:"kind"`
	SourceType        string     `json:"source_type"`
	PreviousExpiresAt *time.Time `json:"previous_expires_at"`
	NewExpiresAt      time.Time  `json:"new_expires_at"`
	ValiditySeconds   int64      `json:"validity_seconds"`
	CreatedAt         time.Time  `json:"created_at"`
}

// Lifecycle storage is separate from request billing and source fulfillment deduplication.
type SubscriptionEntitlementLifecycleRepository interface {
	SetAutoAdvanceMonthly(context.Context, int64, bool) error
	AcknowledgeEntitlementEvent(context.Context, int64, int64, int64) error
	ListEntitlementEvents(context.Context, int64, int64, int64, int) ([]SubscriptionEntitlementEvent, error)
	LatestEntitlementEvents(context.Context, int64) ([]SubscriptionEntitlementEvent, error)
	InsertEntitlementEvent(context.Context, int64, SubscriptionEntitlementEvent) error
}

type EntitlementMonthlyCyclePreview struct {
	CanAdvance         bool       `json:"can_advance"`
	Reason             string     `json:"reason"`
	HasFutureCycle     bool       `json:"has_future_cycle"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`
	CurrentExpiresAt   time.Time  `json:"current_expires_at"`
	NewExpiresAt       *time.Time `json:"new_expires_at"`
	NextResetAt        *time.Time `json:"next_reset_at"`
	DeductedSeconds    int64      `json:"deducted_seconds"`
	RemainingQuota     float64    `json:"remaining_quota"`
	MonthlyLimit       float64    `json:"monthly_limit"`
}

func PreviewEntitlementMonthlyCycle(ent *SubscriptionEntitlement, now time.Time) EntitlementMonthlyCyclePreview {
	p := EntitlementMonthlyCyclePreview{Reason: "inactive"}
	if ent == nil {
		return p
	}
	p.MonthlyWindowStart = cloneTimePtr(ent.MonthlyWindowStart)
	p.CurrentExpiresAt = ent.ExpiresAt
	if !ent.IsActiveAt(now) {
		return p
	}
	if ent.MonthlyLimitUSD == nil || *ent.MonthlyLimitUSD <= 0 {
		p.Reason = "no_monthly_limit"
		return p
	}
	p.MonthlyLimit = *ent.MonthlyLimitUSD
	used := ent.MonthlyUsageUSD
	if ent.NeedsMonthlyResetAt(now) {
		used = 0
	}
	p.RemainingQuota = p.MonthlyLimit - used
	if p.RemainingQuota < 0 {
		p.RemainingQuota = 0
	}
	resetAt := monthlyCycleResetAt(ent.MonthlyWindowStart, ent.StartsAt, now)
	p.HasFutureCycle = canAdvanceMonthlyCycleByValidity(ent.StartsAt, ent.ExpiresAt, resetAt)
	if !p.HasFutureCycle {
		p.Reason = "no_future_cycle"
		return p
	}
	if !canAdvanceMonthlyCycleByUsage(used, p.MonthlyLimit) {
		p.Reason = "below_threshold"
		return p
	}
	p.DeductedSeconds = ceilDurationSeconds(resetAt.Sub(now))
	next := now.Add(monthlyCycleDuration)
	expires := ent.ExpiresAt.Add(-time.Duration(p.DeductedSeconds) * time.Second)
	p.NewExpiresAt, p.NextResetAt = &expires, &next
	p.CanAdvance, p.Reason = true, "available"
	return p
}

func (s *SubscriptionEntitlementService) SetAutoAdvanceMonthly(ctx context.Context, userID, id int64, enabled bool) (*SubscriptionEntitlement, error) {
	store, ok := s.entitlementRepo.(SubscriptionEntitlementLifecycleRepository)
	if !ok {
		return nil, ErrSubscriptionMaintenance
	}
	err := s.withLockedEntitlement(ctx, id, userID, func(txCtx context.Context, ent *SubscriptionEntitlement) error {
		if enabled {
			if err := validateEntitlementAvailabilityAt(ent, s.inputNow(time.Time{})); err != nil {
				return err
			}
			if ent.MonthlyLimitUSD == nil || *ent.MonthlyLimitUSD <= 0 {
				return ErrMonthlyCycleNotExhausted
			}
		}
		return store.SetAutoAdvanceMonthly(txCtx, id, enabled)
	})
	if err != nil {
		return nil, err
	}
	return s.GetUserEntitlementByID(ctx, userID, id)
}

func (s *SubscriptionEntitlementService) EntitlementEvents(ctx context.Context, userID, id, before int64) ([]SubscriptionEntitlementEvent, error) {
	ent, err := s.entitlementRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if ent.UserID != userID {
		return nil, ErrSubscriptionEntitlementNotFound
	}
	store, ok := s.entitlementRepo.(SubscriptionEntitlementLifecycleRepository)
	if !ok {
		return []SubscriptionEntitlementEvent{}, nil
	}
	return store.ListEntitlementEvents(ctx, userID, id, before, 20)
}

func (s *SubscriptionEntitlementService) AcknowledgeEntitlementEvent(ctx context.Context, userID, id, eventID int64) error {
	store, ok := s.entitlementRepo.(SubscriptionEntitlementLifecycleRepository)
	if !ok {
		return ErrSubscriptionMaintenance
	}
	if eventID <= 0 {
		return ErrSubscriptionEntitlementNotFound
	}
	return s.withLockedEntitlement(ctx, id, userID, func(txCtx context.Context, _ *SubscriptionEntitlement) error {
		return store.AcknowledgeEntitlementEvent(txCtx, userID, id, eventID)
	})
}

func (s *SubscriptionEntitlementService) attachEntitlementEvents(ctx context.Context, userID int64, ents []SubscriptionEntitlement) error {
	store, ok := s.entitlementRepo.(SubscriptionEntitlementLifecycleRepository)
	if !ok || len(ents) == 0 {
		return nil
	}
	events, err := store.LatestEntitlementEvents(ctx, userID)
	if err != nil {
		return err
	}
	byID := make(map[int64]*SubscriptionEntitlement, len(ents))
	for i := range ents {
		byID[ents[i].ID] = &ents[i]
	}
	for i := range events {
		event := &events[i]
		if ent := byID[event.EntitlementID]; ent != nil {
			if event.Kind == "manual_advance" || event.Kind == "automatic_advance" {
				ent.LatestCycle = event
			} else {
				ent.LatestRenewal = event
			}
		}
	}
	return nil
}

// TryAutoAdvanceMonthlyCycle is used only by billable request admission. A stale
// request may observe a completed transition, but must never advance its new cycle.
func (s *SubscriptionEntitlementService) TryAutoAdvanceMonthlyCycle(ctx context.Context, ent *SubscriptionEntitlement, expectedCost ...float64) (*SubscriptionEntitlement, error) {
	if ent == nil || !ent.AutoAdvanceMonthly || ent.MonthlyLimitUSD == nil || *ent.MonthlyLimitUSD <= 0 || ent.MonthlyUsageUSD < *ent.MonthlyLimitUSD {
		return ent, nil
	}
	store, ok := s.entitlementRepo.(subscriptionEntitlementMonthlyCycleStore)
	if !ok {
		return nil, ErrSubscriptionMaintenance
	}
	expectedWindow := cloneTimePtr(ent.MonthlyWindowStart)
	var refreshed *SubscriptionEntitlement
	changed := false
	err := s.withLockedEntitlement(ctx, ent.ID, ent.UserID, func(txCtx context.Context, current *SubscriptionEntitlement) error {
		now := s.inputNow(time.Time{}).Truncate(time.Microsecond)
		if err := validateEntitlementAvailabilityAt(current, now); err != nil {
			return err
		}
		if err := s.CheckAndResetWindows(txCtx, current, now); err != nil {
			return err
		}
		refreshed = current
		sameWindow := expectedWindow == nil && current.MonthlyWindowStart == nil || expectedWindow != nil && current.MonthlyWindowStart != nil && expectedWindow.Equal(*current.MonthlyWindowStart)
		if !sameWindow || !current.AutoAdvanceMonthly {
			return nil
		}
		if current.MonthlyLimitUSD == nil || *current.MonthlyLimitUSD <= 0 || current.MonthlyUsageUSD < *current.MonthlyLimitUSD {
			return nil
		}
		if current.DailyLimitUSD != nil && *current.DailyLimitUSD > 0 && current.DailyUsageUSD >= *current.DailyLimitUSD {
			return nil
		}
		if current.WeeklyLimitUSD != nil && *current.WeeklyLimitUSD > 0 && current.WeeklyUsageUSD >= *current.WeeklyLimitUSD {
			return nil
		}
		// A batch hold larger than the next quota must not consume a cycle only
		// to fail immediately. All monetary checks use the locked current state.
		if len(expectedCost) > 0 {
			cost := expectedCost[0]
			if cost < 0 {
				return ErrSubscriptionEntitlementInvalidUsage
			}
			if cost > *current.MonthlyLimitUSD || !current.CheckDailyLimit(cost) || !current.CheckWeeklyLimit(cost) {
				return ErrSubscriptionEntitlementQuotaExceeded
			}
		}
		resetAt := monthlyCycleResetAt(current.MonthlyWindowStart, current.StartsAt, now)
		if !canAdvanceMonthlyCycleByValidity(current.StartsAt, current.ExpiresAt, resetAt) {
			return nil
		}
		snapshot := &SubscriptionEntitlementMonthlyCycleSnapshot{
			ID: current.ID, UserID: current.UserID, PlanID: current.PlanID, Status: current.Status,
			StartsAt: current.StartsAt, ExpiresAt: current.ExpiresAt, MonthlyLimitUSD: current.MonthlyLimitUSD,
			MonthlyUsageUSD: current.MonthlyUsageUSD, MonthlyWindowStart: current.MonthlyWindowStart,
		}
		if _, err := advanceEntitlementMonthlyCycleLocked(txCtx, store, snapshot, now, true); err != nil {
			return err
		}
		var err error
		refreshed, err = s.entitlementRepo.GetByID(txCtx, current.ID)
		if err != nil {
			return err
		}
		if err := syncLinkedLegacySubscriptionLifecycle(txCtx, s.legacySubscriptionRepo, refreshed); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if changed {
		s.invalidateLinkedLegacyAlias(refreshed)
	}
	return refreshed, nil
}
