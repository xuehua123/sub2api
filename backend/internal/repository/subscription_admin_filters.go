package repository

import (
	"fmt"
	"strings"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// Use the same preferred linked entitlement as the admin DTO. NULL limits on an
// entitlement mean unlimited, so they must never fall back to the legacy group.
func applySubscriptionAdminFilters(s *entsql.Selector, f service.SubscriptionAdminFilters, now time.Time, native bool) {
	if f.PlanID == nil && f.Source == "" && f.MonthlyQuota == "" && f.ExpiresWithinDays == 0 {
		return
	}
	var derived string
	if native {
		derived = fmt.Sprintf("SELECT e.id AS entitlement_id, e.plan_id, e.expires_at, e.starts_at, e.status, e.monthly_limit_usd AS quota_limit, e.monthly_usage_usd AS used, e.monthly_window_start AS window_start FROM subscription_entitlements e WHERE e.id = %s", s.C("id"))
	} else {
		derived = fmt.Sprintf(`SELECT e.id AS entitlement_id,
  CASE WHEN e.id IS NOT NULL THEN e.plan_id ELSE
    (SELECT min(p.id) FROM subscription_plans p WHERE
      p.group_id=u.group_id OR EXISTS (SELECT 1 FROM subscription_plan_groups pg WHERE pg.plan_id=p.id AND pg.group_id=u.group_id AND pg.enabled)
      HAVING count(*)=1)
  END AS plan_id,
  CASE WHEN e.id IS NOT NULL THEN e.starts_at ELSE u.starts_at END AS starts_at,
  CASE WHEN e.id IS NOT NULL THEN e.status ELSE u.status END AS status,
  CASE WHEN e.id IS NOT NULL THEN e.expires_at ELSE u.expires_at END AS expires_at,
  CASE WHEN e.id IS NOT NULL THEN e.monthly_limit_usd ELSE g.monthly_limit_usd END AS quota_limit,
  CASE WHEN e.id IS NOT NULL THEN e.monthly_usage_usd ELSE u.monthly_usage_usd END AS used,
  CASE WHEN e.id IS NOT NULL THEN e.monthly_window_start ELSE u.monthly_window_start END AS window_start
  FROM user_subscriptions u
  LEFT JOIN groups g ON g.id = u.group_id
  LEFT JOIN LATERAL (SELECT e.* FROM subscription_entitlements e
    WHERE e.legacy_subscription_id = u.id AND e.deleted_at IS NULL
    ORDER BY (e.status = 'active') DESC, e.updated_at DESC, e.id DESC LIMIT 1) e ON TRUE
  WHERE u.id = %s`, s.C("id"))
	}
	clauses := []string{"TRUE"}
	zone := timezone.Name()
	zoneSQL := "clock.zone"
	if zone == "Local" {
		zone = timezone.UTCOffset()
		zoneSQL = "CAST(clock.zone AS interval)"
	}
	args := []any{now, zone}
	if f.PlanID != nil {
		clauses = append(clauses, "f.plan_id = ?")
		args = append(args, *f.PlanID)
	}
	switch f.Source {
	case "entitlement":
		clauses = append(clauses, "f.entitlement_id IS NOT NULL")
	case "legacy":
		clauses = append(clauses, "f.entitlement_id IS NULL")
	}
	if f.ExpiresWithinDays > 0 {
		clauses = append(clauses, "f.expires_at > ? AND f.expires_at <= ?")
		args = append(args, now, now.Add(time.Duration(f.ExpiresWithinDays)*24*time.Hour))
	}
	if f.MonthlyQuota != "" {
		clauses = append(clauses, "f.quota_limit > 0")
		// Mirror entitlement legacy-midnight alignment and the legacy subscription
		// expiry guard. Merely exceeding 30 days is not always a quota reset.
		startDay := "(date_trunc('day', f.starts_at AT TIME ZONE " + zoneSQL + ") AT TIME ZONE " + zoneSQL + ")"
		windowDay := "(date_trunc('day', f.window_start AT TIME ZONE " + zoneSQL + ") AT TIME ZONE " + zoneSQL + ")"
		legacyNext := "(CASE WHEN f.window_start = " + startDay + " AND f.window_start < f.starts_at THEN f.starts_at ELSE f.window_start END + interval '720 hours')"
		entitlementReset := "(CASE WHEN f.window_start = " + windowDay + " AND (f.window_start < f.starts_at OR mod(extract(epoch from (f.window_start-f.starts_at)), 2592000) <> 0) THEN f.starts_at + floor(extract(epoch from (clock.now-f.starts_at))/2592000) * interval '720 hours' >= f.window_start + interval '720 hours' ELSE f.window_start + interval '720 hours' <= clock.now END)"
		reset := "(CASE WHEN f.entitlement_id IS NOT NULL THEN f.status='active' AND f.starts_at<=clock.now AND f.expires_at>clock.now AND " + entitlementReset + " ELSE " + legacyNext + "<=clock.now AND " + legacyNext + "<f.expires_at END)"
		used := "(CASE WHEN f.window_start IS NOT NULL AND " + reset + " THEN 0 ELSE f.used END)"
		switch f.MonthlyQuota {
		case "available":
			clauses = append(clauses, used+" < f.quota_limit * 0.9")
		case "near_exhausted":
			clauses = append(clauses, used+" >= f.quota_limit * 0.9 AND f.used < f.quota_limit")
		case "exhausted":
			clauses = append(clauses, used+" >= f.quota_limit")
		}
	}
	query := "EXISTS (SELECT 1 FROM (" + derived + ") f CROSS JOIN (SELECT ?::timestamptz AS now, ?::text AS zone) clock WHERE " + strings.Join(clauses, " AND ") + ")"
	// Let Ent number bound arguments for the current SQL dialect and outer query.
	s.Where(entsql.P(func(b *entsql.Builder) {
		parts := strings.Split(query, "?")
		for i, part := range parts {
			b.WriteString(part)
			if i < len(args) {
				b.Arg(args[i])
			}
		}
	}))
}
