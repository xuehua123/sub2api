package service

import "time"

const subscriptionDayDuration = 24 * time.Hour

type UserSubscription struct {
	ID      int64
	UserID  int64
	GroupID int64

	StartsAt  time.Time
	ExpiresAt time.Time
	Status    string

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	AssignedBy *int64
	AssignedAt time.Time
	Notes      string

	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time

	User           *User
	Group          *Group
	AssignedByUser *User

	EntitlementOnly bool
	EntitlementLink *UserSubscriptionEntitlementLink
}

type UserSubscriptionEntitlementLink struct {
	EntitlementID int64
	PlanID        *int64
	PlanName      *string
	Status        string
	ExpiresAt     time.Time

	DailyWindowStart   *time.Time
	WeeklyWindowStart  *time.Time
	MonthlyWindowStart *time.Time

	DailyLimitUSD   *float64
	WeeklyLimitUSD  *float64
	MonthlyLimitUSD *float64

	DailyUsageUSD   float64
	WeeklyUsageUSD  float64
	MonthlyUsageUSD float64

	PrimaryGroupID *int64
	OveragePolicy  string
}

func (s *UserSubscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive && time.Now().Before(s.ExpiresAt)
}

func (s *UserSubscription) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

func (s *UserSubscription) DaysRemaining() int {
	return s.daysRemainingAt(time.Now())
}

func (s *UserSubscription) daysRemainingAt(now time.Time) int {
	remaining := s.ExpiresAt.Sub(now)
	if remaining <= 0 {
		return 0
	}

	days := int(remaining / subscriptionDayDuration)
	if remaining%subscriptionDayDuration != 0 {
		days++
	}
	return days
}

func (s *UserSubscription) IsWindowActivated() bool {
	return s.DailyWindowStart != nil || s.WeeklyWindowStart != nil || s.MonthlyWindowStart != nil
}

func (s *UserSubscription) HasOneTimeDailyQuota() bool {
	if s == nil || s.StartsAt.IsZero() || s.ExpiresAt.IsZero() {
		return false
	}
	return !s.ExpiresAt.After(s.StartsAt.AddDate(0, 0, 1))
}

func (s *UserSubscription) NeedsDailyReset() bool {
	return s.NeedsDailyResetAt(time.Now())
}

func (s *UserSubscription) NeedsDailyResetAt(now time.Time) bool {
	if s.DailyWindowStart == nil {
		return false
	}
	if s.HasOneTimeDailyQuota() {
		return false
	}
	return needsWindowResetAt(s.DailyWindowStart, s.StartsAt, 24*time.Hour, now)
}

func (s *UserSubscription) NeedsWeeklyReset() bool {
	return needsWindowResetAt(s.WeeklyWindowStart, s.StartsAt, 7*24*time.Hour, time.Now())
}

func (s *UserSubscription) NeedsMonthlyReset() bool {
	return needsWindowResetAt(s.MonthlyWindowStart, s.StartsAt, 30*24*time.Hour, time.Now())
}

func (s *UserSubscription) DailyResetTime() *time.Time {
	if s.HasOneTimeDailyQuota() {
		t := s.ExpiresAt
		return &t
	}
	start := effectiveWindowStartAt(s.DailyWindowStart, s.StartsAt, 24*time.Hour, time.Now())
	if start == nil {
		return nil
	}
	t := start.Add(24 * time.Hour)
	return &t
}

func (s *UserSubscription) WeeklyResetTime() *time.Time {
	start := effectiveWindowStartAt(s.WeeklyWindowStart, s.StartsAt, 7*24*time.Hour, time.Now())
	if start == nil {
		return nil
	}
	t := start.Add(7 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) MonthlyResetTime() *time.Time {
	start := effectiveWindowStartAt(s.MonthlyWindowStart, s.StartsAt, 30*24*time.Hour, time.Now())
	if start == nil {
		return nil
	}
	t := start.Add(30 * 24 * time.Hour)
	return &t
}

func (s *UserSubscription) CheckDailyLimit(group *Group, additionalCost float64) bool {
	if !group.HasDailyLimit() {
		return true
	}
	return s.DailyUsageUSD+additionalCost <= *group.DailyLimitUSD
}

func (s *UserSubscription) CheckWeeklyLimit(group *Group, additionalCost float64) bool {
	if !group.HasWeeklyLimit() {
		return true
	}
	return s.WeeklyUsageUSD+additionalCost <= *group.WeeklyLimitUSD
}

func (s *UserSubscription) CheckMonthlyLimit(group *Group, additionalCost float64) bool {
	if !group.HasMonthlyLimit() {
		return true
	}
	return s.MonthlyUsageUSD+additionalCost <= *group.MonthlyLimitUSD
}

func (s *UserSubscription) CheckAllLimits(group *Group, additionalCost float64) (daily, weekly, monthly bool) {
	daily = s.CheckDailyLimit(group, additionalCost)
	weekly = s.CheckWeeklyLimit(group, additionalCost)
	monthly = s.CheckMonthlyLimit(group, additionalCost)
	return
}

func needsWindowResetAt(windowStart *time.Time, startsAt time.Time, cycle time.Duration, now time.Time) bool {
	if windowStart == nil {
		return false
	}
	start := effectiveWindowStartAt(windowStart, startsAt, cycle, now)
	if start == nil {
		return false
	}
	if !start.After(*windowStart) {
		return false
	}
	return start.Sub(*windowStart) >= cycle
}

func effectiveWindowStartAt(windowStart *time.Time, startsAt time.Time, cycle time.Duration, now time.Time) *time.Time {
	if windowStart == nil {
		return nil
	}

	windowBased := advanceWindowStart(*windowStart, cycle, now)
	if windowStart.After(now) {
		return &windowBased
	}
	if aligned, ok := alignedCycleStart(startsAt, cycle, now); ok {
		if isLegacyWindowAnchor(*windowStart, startsAt, cycle) || isAlignedWindowAnchor(*windowStart, startsAt, cycle) {
			return &aligned
		}
	}
	return &windowBased
}

func alignedCycleStart(startsAt time.Time, cycle time.Duration, now time.Time) (time.Time, bool) {
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

func isAlignedWindowAnchor(windowStart, startsAt time.Time, cycle time.Duration) bool {
	if cycle <= 0 || windowStart.IsZero() || startsAt.IsZero() || windowStart.Before(startsAt) {
		return false
	}
	return windowStart.Sub(startsAt)%cycle == 0
}

func isLegacyWindowAnchor(windowStart, startsAt time.Time, cycle time.Duration) bool {
	if !isStartOfDay(windowStart) || startsAt.IsZero() || windowStart.IsZero() || cycle <= 0 {
		return false
	}
	if windowStart.Before(startsAt) {
		return true
	}
	return !isAlignedWindowAnchor(windowStart, startsAt, cycle)
}

func isStartOfDay(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}
