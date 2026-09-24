package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlement"
	"github.com/Wei-Shaw/sub2api/ent/subscriptionentitlementevent"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r *subscriptionEntitlementRepository) SetAutoAdvanceMonthly(ctx context.Context, id int64, enabled bool) error {
	return clientFromContext(ctx, r.client).SubscriptionEntitlement.UpdateOneID(id).SetAutoAdvanceMonthly(enabled).Exec(ctx)
}

func (r *subscriptionEntitlementRepository) AcknowledgeEntitlementEvent(ctx context.Context, userID, id, eventID int64) error {
	c := clientFromContext(ctx, r.client)
	exists, err := c.SubscriptionEntitlementEvent.Query().Where(
		subscriptionentitlementevent.IDEQ(eventID), subscriptionentitlementevent.UserIDEQ(userID), subscriptionentitlementevent.EntitlementIDEQ(id),
	).Exist(ctx)
	if err != nil {
		return err
	}
	if !exists {
		return service.ErrSubscriptionEntitlementNotFound
	}
	return c.SubscriptionEntitlement.Update().Where(subscriptionentitlement.IDEQ(id), subscriptionentitlement.UserIDEQ(userID), subscriptionentitlement.LastSeenEventIDLT(eventID)).SetLastSeenEventID(eventID).Exec(ctx)
}

func (r *subscriptionEntitlementRepository) ListEntitlementEvents(ctx context.Context, userID, id, before int64, limit int) ([]service.SubscriptionEntitlementEvent, error) {
	q := clientFromContext(ctx, r.client).SubscriptionEntitlementEvent.Query().Where(
		subscriptionentitlementevent.UserIDEQ(userID), subscriptionentitlementevent.EntitlementIDEQ(id),
	)
	if before > 0 {
		q = q.Where(subscriptionentitlementevent.IDLT(before))
	}
	rows, err := q.Order(dbent.Desc(subscriptionentitlementevent.FieldID)).Limit(limit).All(ctx)
	if err != nil {
		return nil, err
	}
	return entitlementEventsToService(rows), nil
}

func (r *subscriptionEntitlementRepository) LatestEntitlementEvents(ctx context.Context, userID int64) ([]service.SubscriptionEntitlementEvent, error) {
	c := clientFromContext(ctx, r.client)
	// One receipt per category and entitlement; no per-card queries or unbounded history loads.
	rows, err := c.QueryContext(ctx, `SELECT MAX(id) FROM subscription_entitlement_events WHERE user_id = $1
 GROUP BY entitlement_id, CASE WHEN kind IN ('manual_advance','automatic_advance') THEN 'cycle' ELSE 'term' END`, userID)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []service.SubscriptionEntitlementEvent{}, nil
	}
	events, err := c.SubscriptionEntitlementEvent.Query().Where(subscriptionentitlementevent.UserIDEQ(userID), subscriptionentitlementevent.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, err
	}
	return entitlementEventsToService(events), nil
}

func (r *subscriptionEntitlementRepository) InsertEntitlementEvent(ctx context.Context, userID int64, event service.SubscriptionEntitlementEvent) error {
	return insertEntitlementEvent(ctx, clientFromContext(ctx, r.client), userID, event)
}

func insertEntitlementEvent(ctx context.Context, c *dbent.Client, userID int64, event service.SubscriptionEntitlementEvent) error {
	return c.SubscriptionEntitlementEvent.Create().SetUserID(userID).SetEntitlementID(event.EntitlementID).
		SetKind(event.Kind).SetSourceType(event.SourceType).SetNillablePreviousExpiresAt(event.PreviousExpiresAt).
		SetNewExpiresAt(event.NewExpiresAt).SetValiditySeconds(event.ValiditySeconds).Exec(ctx)
}

func entitlementEventsToService(rows []*dbent.SubscriptionEntitlementEvent) []service.SubscriptionEntitlementEvent {
	out := make([]service.SubscriptionEntitlementEvent, 0, len(rows))
	for _, e := range rows {
		out = append(out, service.SubscriptionEntitlementEvent{
			ID: e.ID, EntitlementID: e.EntitlementID, Kind: e.Kind, SourceType: e.SourceType,
			PreviousExpiresAt: e.PreviousExpiresAt, NewExpiresAt: e.NewExpiresAt, ValiditySeconds: e.ValiditySeconds, CreatedAt: e.CreatedAt,
		})
	}
	return out
}

func recordEntitlementFulfillmentEvent(ctx context.Context, c *dbent.Client, fulfillment *service.SubscriptionEntitlementFulfillment, previous *time.Time, base time.Time) error {
	kind := "granted"
	if previous != nil {
		kind = "renewed"
		if previous.Before(base) {
			kind = "reactivated"
		}
	}
	return insertEntitlementEvent(ctx, c, fulfillment.UserID, service.SubscriptionEntitlementEvent{
		EntitlementID: fulfillment.EntitlementID, Kind: kind, SourceType: fulfillment.SourceType,
		PreviousExpiresAt: previous, NewExpiresAt: fulfillment.ExpiresAt, ValiditySeconds: int64(fulfillment.ExpiresAt.Sub(base) / time.Second),
	})
}
