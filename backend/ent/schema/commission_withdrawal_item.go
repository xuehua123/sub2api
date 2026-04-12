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

// CommissionWithdrawalItem holds the schema definition for the CommissionWithdrawalItem entity.
type CommissionWithdrawalItem struct {
	ent.Schema
}

func (CommissionWithdrawalItem) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "commission_withdrawal_items"},
	}
}

func (CommissionWithdrawalItem) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CommissionWithdrawalItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("withdrawal_id"),
		field.Int64("user_id"),
		field.Int64("reward_id"),
		field.Int64("recharge_order_id"),
		field.Float("allocated_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("fee_allocated_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("net_allocated_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("currency").
			MaxLen(3).
			Default(consts.ReferralSettlementCurrencyCNY),
		field.String("status").
			MaxLen(32).
			Default("frozen"),
		field.Int64("freeze_ledger_id").
			Optional().
			Nillable(),
		field.Int64("return_ledger_id").
			Optional().
			Nillable(),
		field.Int64("paid_ledger_id").
			Optional().
			Nillable(),
		field.Int64("reverse_ledger_id").
			Optional().
			Nillable(),
	}
}

func (CommissionWithdrawalItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("withdrawal", CommissionWithdrawal.Type).
			Ref("items").
			Field("withdrawal_id").
			Required().
			Unique(),
		edge.From("user", User.Type).
			Ref("commission_withdrawal_items").
			Field("user_id").
			Required().
			Unique(),
		edge.From("reward", CommissionReward.Type).
			Ref("withdrawal_items").
			Field("reward_id").
			Required().
			Unique(),
		edge.From("recharge_order", RechargeOrder.Type).
			Ref("commission_withdrawal_items").
			Field("recharge_order_id").
			Required().
			Unique(),
		edge.To("commission_ledgers", CommissionLedger.Type),
	}
}

func (CommissionWithdrawalItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("withdrawal_id", "reward_id").Unique(),
		index.Fields("user_id"),
		index.Fields("reward_id"),
		index.Fields("status"),
	}
}
