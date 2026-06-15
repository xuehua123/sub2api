package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionEntitlement holds a user's purchased package quota ledger.
type SubscriptionEntitlement struct {
	ent.Schema
}

func (SubscriptionEntitlement) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "subscription_entitlements"},
	}
}

func (SubscriptionEntitlement) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
		mixins.SoftDeleteMixin{},
	}
}

func (SubscriptionEntitlement) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("plan_id").
			Optional().
			Nillable(),
		field.Int64("legacy_subscription_id").
			Optional().
			Nillable().
			Unique(),
		field.Int64("primary_group_id").
			Optional().
			Nillable(),
		field.String("name").
			MaxLen(120).
			Default(""),
		field.String("source_type").
			MaxLen(32).
			Default("unknown"),
		field.String("status").
			MaxLen(20).
			Default(domain.SubscriptionStatusActive),
		field.Time("starts_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("expires_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("daily_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("weekly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("monthly_window_start").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Float("daily_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("weekly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("monthly_limit_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Float("daily_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("weekly_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("monthly_usage_usd").
			Default(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.String("overage_policy").
			MaxLen(32).
			Default("block"),
		field.JSON("plan_snapshot", map[string]any{}).
			Optional().
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
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
	}
}

func (SubscriptionEntitlement) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("subscription_entitlements").
			Field("user_id").
			Unique().
			Required().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("plan", SubscriptionPlan.Type).
			Ref("entitlements").
			Field("plan_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("legacy_subscription", UserSubscription.Type).
			Ref("legacy_entitlement").
			Field("legacy_subscription_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("primary_group", Group.Type).
			Ref("primary_subscription_entitlements").
			Field("primary_group_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("source_redeem_code", RedeemCode.Type).
			Ref("source_subscription_entitlements").
			Field("source_redeem_code_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.From("assigned_by_user", User.Type).
			Ref("assigned_subscription_entitlements").
			Field("assigned_by").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("groups", Group.Type).
			Through("subscription_entitlement_groups", SubscriptionEntitlementGroup.Type),
		edge.To("api_keys", APIKey.Type),
		edge.To("usage_logs", UsageLog.Type),
		edge.To("payment_orders", PaymentOrder.Type),
		edge.To("redeem_codes", RedeemCode.Type),
		edge.To("fulfillments", SubscriptionEntitlementFulfillment.Type),
	}
}

func (SubscriptionEntitlement) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "status", "expires_at").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("plan_id").
			Annotations(entsql.IndexWhere("deleted_at IS NULL")),
		index.Fields("source_redeem_code_id").
			Unique().
			Annotations(entsql.IndexWhere("source_redeem_code_id IS NOT NULL AND deleted_at IS NULL")),
		index.Fields("source_type", "source_id").
			Unique().
			Annotations(entsql.IndexWhere("source_id IS NOT NULL AND deleted_at IS NULL")),
		index.Fields("source_type", "source_external_id").
			Unique().
			Annotations(entsql.IndexWhere("source_external_id IS NOT NULL AND deleted_at IS NULL")),
		index.Fields("legacy_subscription_id").
			Unique(),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
