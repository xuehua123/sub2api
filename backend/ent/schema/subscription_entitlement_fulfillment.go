package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionEntitlementFulfillment records every source event that grants or renews an entitlement.
type SubscriptionEntitlementFulfillment struct {
	ent.Schema
}

func (SubscriptionEntitlementFulfillment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_entitlement_fulfillments"},
	}
}

func (SubscriptionEntitlementFulfillment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("entitlement_id"),
		field.Int64("user_id"),
		field.Int64("plan_id").
			Optional().
			Nillable(),
		field.String("source_type").
			MaxLen(32).
			Default("unknown"),
		field.Int64("source_id").
			Optional().
			Nillable(),
		field.String("source_external_id").
			MaxLen(128).
			Optional().
			Nillable(),
		field.Int64("source_redeem_code_id").
			Optional().
			Nillable(),
		field.Int("validity_days").
			Default(0),
		field.Time("starts_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("assigned_by").
			Optional().
			Nillable(),
		field.Time("assigned_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionEntitlementFulfillment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("entitlement", SubscriptionEntitlement.Type).
			Ref("fulfillments").
			Field("entitlement_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SubscriptionEntitlementFulfillment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("entitlement_id"),
		index.Fields("user_id", "plan_id"),
		index.Fields("source_redeem_code_id").
			Unique().
			Annotations(entsql.IndexWhere("source_redeem_code_id IS NOT NULL")),
		index.Fields("source_type", "source_id").
			Unique().
			Annotations(entsql.IndexWhere("source_id IS NOT NULL")),
		index.Fields("source_type", "source_external_id").
			Unique().
			Annotations(entsql.IndexWhere("source_external_id IS NOT NULL")),
	}
}
