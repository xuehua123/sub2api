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

// SubscriptionPlanExternalMapping maps legacy external cashier tuples to plans.
type SubscriptionPlanExternalMapping struct {
	ent.Schema
}

func (SubscriptionPlanExternalMapping) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_plan_external_mappings"},
	}
}

func (SubscriptionPlanExternalMapping) Fields() []ent.Field {
	return []ent.Field{
		field.String("source").
			MaxLen(64).
			Default("sub2-payment-page"),
		field.Int64("legacy_group_id"),
		field.Int("legacy_validity_days"),
		field.Float("legacy_value").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Int64("plan_id"),
		field.Bool("enabled").
			Default(true),
		field.Int("priority").
			Default(0),
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
		field.Time("deleted_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionPlanExternalMapping) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("legacy_group", Group.Type).
			Ref("subscription_plan_external_mappings").
			Field("legacy_group_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Restrict)),
		edge.From("plan", SubscriptionPlan.Type).
			Ref("external_mappings").
			Field("plan_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SubscriptionPlanExternalMapping) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source", "legacy_group_id", "legacy_validity_days", "legacy_value").
			Unique().
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("plan_id"),
		index.Fields("legacy_group_id"),
		index.Fields("enabled"),
	}
}
