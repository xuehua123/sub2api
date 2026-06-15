//go:build integration

package repository

import (
	"context"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/suite"
)

type SubscriptionEntitlementExternalMappingRepoSuite struct {
	suite.Suite
	ctx    context.Context
	client *dbent.Client
	repo   *subscriptionPlanExternalMappingRepository
}

func (s *SubscriptionEntitlementExternalMappingRepoSuite) SetupTest() {
	tx := testEntTx(s.T())
	s.ctx = dbent.NewTxContext(context.Background(), tx)
	s.client = tx.Client()
	s.repo = NewSubscriptionPlanExternalMappingRepository(integrationEntClient).(*subscriptionPlanExternalMappingRepository)
}

func TestSubscriptionEntitlementExternalMappingRepoSuite(t *testing.T) {
	suite.Run(t, new(SubscriptionEntitlementExternalMappingRepoSuite))
}

func (s *SubscriptionEntitlementExternalMappingRepoSuite) TestFindEnabledMatchesExactLegacyTuple() {
	group := mustCreateGroup(s.T(), s.client, &service.Group{Name: uniqueTestName("mapping-group")})
	plan, err := s.client.SubscriptionPlan.Create().
		SetGroupID(group.ID).
		SetName(uniqueTestName("mapping-plan")).
		SetPrice(29.90).
		SetValidityDays(30).
		SetValidityUnit("day").
		Save(s.ctx)
	s.Require().NoError(err)

	mapping, err := s.client.SubscriptionPlanExternalMapping.Create().
		SetSource(service.SubscriptionPlanExternalMappingSourceSub2PaymentPage).
		SetLegacyGroupID(group.ID).
		SetLegacyValidityDays(30).
		SetLegacyValue(29.90).
		SetPlanID(plan.ID).
		SetEnabled(true).
		SetPriority(10).
		SetNotes("legacy cashier mapping").
		Save(s.ctx)
	s.Require().NoError(err)

	got, err := s.repo.FindEnabled(s.ctx, service.SubscriptionPlanExternalMappingSourceSub2PaymentPage, group.ID, 30, 29.90)
	s.Require().NoError(err)
	s.Require().Equal(mapping.ID, got.ID)
	s.Require().Equal(plan.ID, got.PlanID)
	s.Require().Equal("legacy cashier mapping", got.Notes)

	_, err = s.repo.FindEnabled(s.ctx, service.SubscriptionPlanExternalMappingSourceSub2PaymentPage, group.ID, 30, 39.90)
	s.Require().ErrorIs(err, service.ErrSubscriptionPlanExternalMappingNotFound)
}
