package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	entgroup "github.com/Wei-Shaw/sub2api/ent/group"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
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
		Currency:         plan.Currency,
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

	switch plan.AccessScope {
	case service.PlanAccessScopeAllSubscriptionGroups:
		out.Groups = append(out.Groups, subscriptionEntitlementPlanGroupsFromEnt(loadEntitlementPlanAutoGrantGroups(ctx, client, nil))...)
	case service.PlanAccessScopePlatformSubscriptionGroups:
		out.Groups = append(out.Groups, subscriptionEntitlementPlanGroupsFromEnt(loadEntitlementPlanAutoGrantGroups(ctx, client, plan.AllowedPlatforms))...)
	}

	if plan.AccessScope == "" || plan.AccessScope == service.PlanAccessScopeExplicit {
		for _, grant := range plan.Edges.SubscriptionPlanGroups {
			if grant == nil || !grant.Enabled {
				continue
			}
			groupOut := groupEntityToService(grant.Edges.Group)
			if groupOut == nil {
				continue
			}
			out.Groups = append(out.Groups, service.SubscriptionEntitlementPlanGroup{
				GroupID:   grant.GroupID,
				SortOrder: grant.SortOrder,
				Enabled:   grant.Enabled,
				Group:     groupOut,
			})
		}
	}

	if (plan.AccessScope == "" || plan.AccessScope == service.PlanAccessScopeExplicit) && len(out.Groups) == 0 && plan.GroupID > 0 {
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

func loadEntitlementPlanAutoGrantGroups(ctx context.Context, client *dbent.Client, platforms []string) []*dbent.Group {
	if client == nil {
		return nil
	}
	query := client.Group.Query().
		Where(subscriptionEntitlementPlanAutoGrantGroupPredicates()...).
		Order(entgroup.BySortOrder(), entgroup.ByID())
	if len(platforms) > 0 {
		query.Where(entgroup.PlatformIn(platforms...))
	}
	groups, err := query.All(ctx)
	if err != nil {
		return nil
	}
	return groups
}

func subscriptionEntitlementPlanAutoGrantGroupPredicates() []predicate.Group {
	return []predicate.Group{
		entgroup.StatusEQ(service.StatusActive),
		entgroup.SubscriptionEnabledEQ(true),
		entgroup.PlanAutoGrantEnabledEQ(true),
		entgroup.IsExclusiveEQ(false),
		entgroup.DeletedAtIsNil(),
		entgroup.Not(entgroup.NameContainsFold("test")),
		entgroup.Not(entgroup.NameContainsFold("private")),
		entgroup.Not(entgroup.NameContains("测试")),
		entgroup.Not(entgroup.NameContains("专属")),
	}
}

func subscriptionEntitlementPlanGroupsFromEnt(groups []*dbent.Group) []service.SubscriptionEntitlementPlanGroup {
	out := make([]service.SubscriptionEntitlementPlanGroup, 0, len(groups))
	for _, group := range groups {
		if group == nil {
			continue
		}
		out = append(out, service.SubscriptionEntitlementPlanGroup{
			GroupID:   group.ID,
			SortOrder: group.SortOrder,
			Enabled:   true,
			Group:     groupEntityToService(group),
		})
	}
	return out
}
