package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	entsubscriptionplan "github.com/Wei-Shaw/sub2api/ent/subscriptionplan"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplangroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionEntitlementPlanRepository struct {
	client *dbent.Client
}

func NewSubscriptionEntitlementPlanRepository(client *dbent.Client) service.SubscriptionEntitlementPlanRepository {
	return &subscriptionEntitlementPlanRepository{client: client}
}

func (r *subscriptionEntitlementPlanRepository) GetEntitlementPlan(ctx context.Context, planID int64) (*service.SubscriptionEntitlementPlan, error) {
	client := clientFromContext(ctx, r.client)
	plan, err := client.SubscriptionPlan.Query().
		Where(entsubscriptionplan.IDEQ(planID)).
		WithSubscriptionPlanGroups(func(q *dbent.SubscriptionPlanGroupQuery) {
			q.Where(subscriptionplangroup.EnabledEQ(true)).
				WithGroup().
				Order(dbent.Asc(subscriptionplangroup.FieldSortOrder), dbent.Asc(subscriptionplangroup.FieldGroupID))
		}).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionEntitlementPlanNotFound, nil)
	}

	return subscriptionPlanEntityToEntitlementPlan(ctx, client, plan), nil
}

func subscriptionPlanEntityToEntitlementPlan(ctx context.Context, client *dbent.Client, plan *dbent.SubscriptionPlan) *service.SubscriptionEntitlementPlan {
	if plan == nil {
		return nil
	}
	out := &service.SubscriptionEntitlementPlan{
		ID:               plan.ID,
		GroupID:          plan.GroupID,
		Name:             plan.Name,
		Description:      plan.Description,
		Price:            plan.Price,
		ValidityDays:     plan.ValidityDays,
		ValidityUnit:     plan.ValidityUnit,
		AccessScope:      plan.AccessScope,
		AllowedPlatforms: append([]string(nil), plan.AllowedPlatforms...),
		DailyLimitUSD:    plan.DailyLimitUsd,
		WeeklyLimitUSD:   plan.WeeklyLimitUsd,
		MonthlyLimitUSD:  plan.MonthlyLimitUsd,
		OveragePolicy:    plan.OveragePolicy,
		Features:         plan.Features,
		ProductName:      plan.ProductName,
		ForSale:          plan.ForSale,
		SortOrder:        plan.SortOrder,
	}

	for _, grant := range plan.Edges.SubscriptionPlanGroups {
		if grant == nil || !grant.Enabled {
			continue
		}
		var groupOut *service.Group
		if sg := groupEntityToService(grant.Edges.Group); sg != nil {
			groupOut = sg
		}
		out.Groups = append(out.Groups, service.SubscriptionEntitlementPlanGroup{
			GroupID:   grant.GroupID,
			SortOrder: grant.SortOrder,
			Enabled:   grant.Enabled,
			Group:     groupOut,
		})
	}

	if len(out.Groups) == 0 && plan.GroupID > 0 {
		if group, err := client.Group.Get(ctx, plan.GroupID); err == nil {
			if groupOut := groupEntityToService(group); groupOut != nil {
				out.Groups = append(out.Groups, service.SubscriptionEntitlementPlanGroup{
					GroupID:   plan.GroupID,
					SortOrder: 0,
					Enabled:   true,
					Group:     groupOut,
				})
			}
		}
	}

	return out
}
