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

// RechargeOrder holds the schema definition for the RechargeOrder entity.
type RechargeOrder struct {
	ent.Schema
}

func (RechargeOrder) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "recharge_orders"},
	}
}

func (RechargeOrder) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (RechargeOrder) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("external_order_id").
			MaxLen(128).
			NotEmpty(),
		field.String("provider").
			MaxLen(32),
		field.String("channel").
			MaxLen(32).
			Optional().
			Nillable(),
		field.String("currency").
			MaxLen(3).
			Default(consts.ReferralSettlementCurrencyCNY),
		field.Float("gross_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("discount_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("paid_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("gift_balance_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("credited_balance_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("refunded_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("chargeback_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("status").
			MaxLen(32).
			Default("pending"),
		field.Time("paid_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("credited_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("refunded_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("chargeback_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("idempotency_key").
			Optional().
			Nillable().
			MaxLen(128),
		field.String("metadata_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (RechargeOrder) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("recharge_orders").
			Field("user_id").
			Required().
			Unique(),
		edge.To("commission_rewards", CommissionReward.Type),
		edge.To("commission_ledgers", CommissionLedger.Type),
		edge.To("commission_withdrawal_items", CommissionWithdrawalItem.Type),
	}
}

func (RechargeOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("external_order_id", "provider").Unique(),
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("idempotency_key"),
		index.Fields("paid_at"),
		index.Fields("refunded_at"),
	}
}
