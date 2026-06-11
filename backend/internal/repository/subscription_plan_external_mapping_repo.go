package repository

import (
	"context"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionplanexternalmapping"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type subscriptionPlanExternalMappingRepository struct {
	client *dbent.Client
}

func NewSubscriptionPlanExternalMappingRepository(client *dbent.Client) service.SubscriptionPlanExternalMappingRepository {
	return &subscriptionPlanExternalMappingRepository{client: client}
}

func (r *subscriptionPlanExternalMappingRepository) FindEnabled(ctx context.Context, source string, legacyGroupID int64, legacyValidityDays int, legacyValue float64) (*service.SubscriptionPlanExternalMapping, error) {
	client := clientFromContext(ctx, r.client)
	m, err := client.SubscriptionPlanExternalMapping.Query().
		Where(
			subscriptionplanexternalmapping.SourceEQ(source),
			subscriptionplanexternalmapping.LegacyGroupIDEQ(legacyGroupID),
			subscriptionplanexternalmapping.LegacyValidityDaysEQ(legacyValidityDays),
			subscriptionplanexternalmapping.LegacyValueEQ(legacyValue),
			subscriptionplanexternalmapping.EnabledEQ(true),
			subscriptionplanexternalmapping.DeletedAtIsNil(),
		).
		Order(dbent.Desc(subscriptionplanexternalmapping.FieldPriority), dbent.Asc(subscriptionplanexternalmapping.FieldID)).
		First(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, service.ErrSubscriptionPlanExternalMappingNotFound, nil)
	}
	return subscriptionPlanExternalMappingEntityToService(m), nil
}

func subscriptionPlanExternalMappingEntityToService(m *dbent.SubscriptionPlanExternalMapping) *service.SubscriptionPlanExternalMapping {
	if m == nil {
		return nil
	}
	return &service.SubscriptionPlanExternalMapping{
		ID:                 m.ID,
		Source:             m.Source,
		LegacyGroupID:      m.LegacyGroupID,
		LegacyValidityDays: m.LegacyValidityDays,
		LegacyValue:        m.LegacyValue,
		PlanID:             m.PlanID,
		Enabled:            m.Enabled,
		Priority:           m.Priority,
		Notes:              derefString(m.Notes),
		CreatedAt:          m.CreatedAt,
		UpdatedAt:          m.UpdatedAt,
		DeletedAt:          m.DeletedAt,
	}
}
