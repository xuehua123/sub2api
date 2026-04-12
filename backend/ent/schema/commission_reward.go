package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/consts"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CommissionReward holds the schema definition for the CommissionReward entity.
type CommissionReward struct {
	ent.Schema
}

func (CommissionReward) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "commission_rewards"},
	}
}

func (CommissionReward) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CommissionReward) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("source_user_id"),
		field.Int64("recharge_order_id"),
		field.Int("level"),
		field.Float("rate_snapshot").
			SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).
			Default(0),
		field.Float("base_amount_snapshot").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("reward_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("currency").
			MaxLen(3).
			Default(consts.ReferralSettlementCurrencyCNY),
		field.String("reward_mode_snapshot").
			MaxLen(32),
		field.String("status").
			MaxLen(32).
			Default("pending"),
		field.Time("available_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("frozen_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("paid_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("reversed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("rule_snapshot_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("relation_snapshot_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (CommissionReward) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("commission_rewards").
			Field("user_id").
			Required().
			Unique(),
		edge.From("source_user", User.Type).
			Ref("source_commission_rewards").
			Field("source_user_id").
			Required().
			Unique(),
		edge.From("recharge_order", RechargeOrder.Type).
			Ref("commission_rewards").
			Field("recharge_order_id").
			Required().
			Unique(),
		edge.To("commission_ledgers", CommissionLedger.Type),
		edge.To("withdrawal_items", CommissionWithdrawalItem.Type),
	}
}

func (CommissionReward) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("recharge_order_id", "user_id", "level").Unique(),
		index.Fields("user_id"),
		index.Fields("source_user_id"),
		index.Fields("status"),
		index.Fields("available_at"),
	}
}
