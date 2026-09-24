//go:build integration

package repository

import (
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"time"
)

func (s *UserSubscriptionRepoSuite) TestAdminFiltersBeforePagination() {
	user := s.mustCreateUser("filters@test.com", service.RoleUser)
	group := s.mustCreateGroup("filters")
	now := time.Now()
	_, err := s.client.Group.UpdateOneID(group.ID).SetMonthlyLimitUsd(1).Save(s.ctx)
	s.Require().NoError(err)
	plan, err := s.client.SubscriptionPlan.Create().SetGroupID(group.ID).SetName("off-sale plan").SetPrice(10).SetValidityDays(30).SetForSale(false).Save(s.ctx)
	s.Require().NoError(err)
	alias := s.mustCreateSubscription(user.ID, group.ID, func(c *dbent.UserSubscriptionCreate) { c.SetMonthlyUsageUsd(200) })
	makeEnt := func(used float64, days int, linked bool, stale bool) *dbent.SubscriptionEntitlement {
		window := now.Add(-time.Hour)
		if stale {
			window = now.Add(-31 * 24 * time.Hour)
		}
		c := s.client.SubscriptionEntitlement.Create().SetUserID(user.ID).SetPlanID(plan.ID).SetPrimaryGroupID(group.ID).
			SetName("filter card").SetSourceType(service.SubscriptionEntitlementSourceAdminAssign).SetStatus(service.SubscriptionStatusActive).
			SetStartsAt(now.Add(-60 * 24 * time.Hour)).SetExpiresAt(now.Add(time.Duration(days) * 24 * time.Hour)).
			SetOveragePolicy(service.SubscriptionEntitlementOverageBlock).SetMonthlyLimitUsd(100).SetMonthlyUsageUsd(used).SetMonthlyWindowStart(window)
		if linked {
			c.SetLegacySubscriptionID(alias.ID)
		}
		e, err := c.Save(s.ctx)
		s.Require().NoError(err)
		return e
	}
	linked := makeEnt(95, 3, true, false)
	native := makeEnt(95, 5, false, false)
	_ = makeEnt(100, 40, false, false)
	stale := makeEnt(100, 20, false, true)
	unlimited := makeEnt(200, 20, false, false)
	_, err = s.client.SubscriptionEntitlement.UpdateOneID(unlimited.ID).ClearMonthlyLimitUsd().Save(s.ctx)
	s.Require().NoError(err)
	// Same group, another plan, must not match by group membership.
	other := makeEnt(95, 5, false, false)
	_, err = s.client.SubscriptionEntitlement.UpdateOneID(other.ID).ClearPlanID().Save(s.ctx)
	s.Require().NoError(err)
	f := service.SubscriptionAdminFilters{PlanID: &plan.ID, Source: "entitlement", MonthlyQuota: "near_exhausted", ExpiresWithinDays: 7}
	ids := []int64{}
	for page := 1; page <= 2; page++ {
		rows, total, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: page, PageSize: 1}, &user.ID, nil, "active", "", "created_at", "desc", f)
		s.Require().NoError(err)
		s.Require().Equal(int64(2), total.Total)
		s.Require().Len(rows, 1)
		ids = append(ids, rows[0].ID)
	}
	s.Require().ElementsMatch([]int64{alias.ID, -native.ID}, ids)
	rows, total, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &user.ID, nil, "active", "", "created_at", "desc", service.SubscriptionAdminFilters{Source: "legacy"})
	s.Require().NoError(err)
	s.Require().Empty(rows)
	s.Require().Zero(total.Total)
	rows, total, err = s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &user.ID, nil, "active", "", "created_at", "desc", service.SubscriptionAdminFilters{MonthlyQuota: "available"})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), total.Total)
	s.Require().Equal(-stale.ID, rows[0].ID)
	// An unlimited linked card must not inherit the group's much smaller limit.
	_, err = s.client.SubscriptionEntitlement.UpdateOneID(linked.ID).ClearMonthlyLimitUsd().Save(s.ctx)
	s.Require().NoError(err)
	rows, _, err = s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &user.ID, nil, "active", "", "created_at", "desc", service.SubscriptionAdminFilters{MonthlyQuota: "exhausted"})
	s.Require().NoError(err)
	s.Require().Len(rows, 1)
	s.Require().NotEqual(alias.ID, rows[0].ID)
}

func (s *UserSubscriptionRepoSuite) TestAdminMonthlyFiltersKeepFinalAndMidnightUsage() {
	user := s.mustCreateUser("filter-boundaries@test.com", service.RoleUser)
	now := time.Now()
	g := s.mustCreateGroup("filter-boundaries")
	_, err := s.client.Group.UpdateOneID(g.ID).SetMonthlyLimitUsd(100).Save(s.ctx)
	s.Require().NoError(err)
	// An expired one-month subscription never earned a second quota window.
	final := s.mustCreateSubscription(user.ID, g.ID, func(c *dbent.UserSubscriptionCreate) {
		start := now.Add(-31 * 24 * time.Hour)
		c.SetStartsAt(start).SetExpiresAt(start.Add(30 * 24 * time.Hour)).SetMonthlyWindowStart(start).SetMonthlyUsageUsd(100)
	})
	rows, _, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &user.ID, nil, "expired", "", "created_at", "desc", service.SubscriptionAdminFilters{Source: "legacy", MonthlyQuota: "exhausted"})
	s.Require().NoError(err)
	s.Require().Len(rows, 1)
	s.Require().Equal(final.ID, rows[0].ID)
	// The historical midnight anchor must not reset before the subscription's time of day.
	start := now.Add(-30*24*time.Hour + time.Minute)
	midnight := timezone.StartOfDay(start)
	if !midnight.Add(30 * 24 * time.Hour).Before(now) {
		return
	}
	g2 := s.mustCreateGroup("filter-midnight")
	_, err = s.client.Group.UpdateOneID(g2.ID).SetMonthlyLimitUsd(100).Save(s.ctx)
	s.Require().NoError(err)
	legacy := s.mustCreateSubscription(user.ID, g2.ID, func(c *dbent.UserSubscriptionCreate) {
		c.SetStartsAt(start).SetExpiresAt(now.Add(30 * 24 * time.Hour)).SetMonthlyWindowStart(midnight).SetMonthlyUsageUsd(100)
	})
	native, err := s.client.SubscriptionEntitlement.Create().SetUserID(user.ID).SetPrimaryGroupID(g.ID).
		SetName("midnight entitlement").SetSourceType(service.SubscriptionEntitlementSourceAdminAssign).SetStatus("active").
		SetStartsAt(start).SetExpiresAt(now.Add(30 * 24 * time.Hour)).SetMonthlyWindowStart(midnight).SetMonthlyLimitUsd(100).SetMonthlyUsageUsd(100).
		SetOveragePolicy(service.SubscriptionEntitlementOverageBlock).Save(s.ctx)
	s.Require().NoError(err)
	rows, _, err = s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &user.ID, nil, "active", "", "created_at", "desc", service.SubscriptionAdminFilters{MonthlyQuota: "exhausted"})
	s.Require().NoError(err)
	ids := []int64{}
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	s.Require().ElementsMatch([]int64{legacy.ID, -native.ID}, ids)
}
