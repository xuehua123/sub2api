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

// SubscriptionPlanGroup holds the plan-to-group grant configuration.
type SubscriptionPlanGroup struct {
	ent.Schema
}

func (SubscriptionPlanGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_plan_groups"},
		field.ID("plan_id", "group_id"),
	}
}

func (SubscriptionPlanGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("plan_id"),
		field.Int64("group_id"),
		field.Int("sort_order").
			Default(0),
		field.Bool("enabled").
			Default(true),
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

func (SubscriptionPlanGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("plan", SubscriptionPlan.Type).
			Unique().
			Required().
			Field("plan_id").
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("group", Group.Type).
			Unique().
			Required().
			Field("group_id").
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SubscriptionPlanGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "enabled"),
	}
}
