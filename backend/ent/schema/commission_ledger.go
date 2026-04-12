package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/consts"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CommissionLedger holds the schema definition for the CommissionLedger entity.
type CommissionLedger struct {
	ent.Schema
}

func (CommissionLedger) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "commission_ledgers"},
	}
}

func (CommissionLedger) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("reward_id").
			Optional().
			Nillable(),
		field.Int64("recharge_order_id").
			Optional().
			Nillable(),
		field.Int64("withdrawal_id").
			Optional().
			Nillable(),
		field.Int64("withdrawal_item_id").
			Optional().
			Nillable(),
		field.String("entry_type").
			MaxLen(64),
		field.String("bucket").
			MaxLen(32),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("currency").
			MaxLen(3).
			Default(consts.ReferralSettlementCurrencyCNY),
		field.String("idempotency_key").
			Optional().
			Nillable().
			MaxLen(128),
		field.Int64("operator_user_id").
			Optional().
			Nillable(),
		field.String("remark").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("metadata_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (CommissionLedger) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("commission_ledgers").
			Field("user_id").
			Required().
			Unique(),
		edge.From("reward", CommissionReward.Type).
			Ref("commission_ledgers").
			Field("reward_id").
			Unique(),
		edge.From("recharge_order", RechargeOrder.Type).
			Ref("commission_ledgers").
			Field("recharge_order_id").
			Unique(),
		edge.From("withdrawal", CommissionWithdrawal.Type).
			Ref("commission_ledgers").
			Field("withdrawal_id").
			Unique(),
		edge.From("withdrawal_item", CommissionWithdrawalItem.Type).
			Ref("commission_ledgers").
			Field("withdrawal_item_id").
			Unique(),
	}
}

func (CommissionLedger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("reward_id"),
		index.Fields("recharge_order_id"),
		index.Fields("withdrawal_id"),
		index.Fields("withdrawal_item_id"),
		index.Fields("entry_type"),
		index.Fields("bucket"),
		index.Fields("created_at"),
		index.Fields("idempotency_key"),
	}
}
