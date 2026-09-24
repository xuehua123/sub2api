//go:build integration

package repository

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (s *UserSubscriptionRepoSuite) TestLegacyPlanFilterMatchesDisplayedPlan() {
	u := s.mustCreateUser("review-plan@test.invalid", service.RoleUser)
	g := s.mustCreateGroup("legacy-only-group")
	plan, err := s.client.SubscriptionPlan.Create().SetGroupID(g.ID).SetName("only-plan-for-group").SetPrice(10).SetValidityDays(30).Save(s.ctx)
	s.Require().NoError(err)
	sub := s.mustCreateSubscription(u.ID, g.ID, nil)
	rows, total, err := s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &u.ID, nil, "active", "", "created_at", "desc")
	s.Require().NoError(err)
	s.Require().Equal(int64(1), total.Total)
	s.Require().Equal(sub.ID, rows[0].ID)
	rows, total, err = s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &u.ID, nil, "active", "", "created_at", "desc", service.SubscriptionAdminFilters{PlanID: &plan.ID})
	s.Require().NoError(err)
	s.Require().Equal(int64(1), total.Total, "the UI resolves this row to the only plan on its group, but filtering removes it")
	s.Require().Len(rows, 1)
	_, err = s.client.SubscriptionPlan.Create().SetGroupID(g.ID).SetName("ambiguous-second-plan").SetPrice(15).SetValidityDays(30).Save(s.ctx)
	s.Require().NoError(err)
	rows, total, err = s.repo.List(s.ctx, pagination.PaginationParams{Page: 1, PageSize: 20}, &u.ID, nil, "active", "", "created_at", "desc", service.SubscriptionAdminFilters{PlanID: &plan.ID})
	s.Require().NoError(err)
	s.Require().Empty(rows)
	s.Require().Zero(total.Total, "ambiguous legacy plans must not be guessed")
}
