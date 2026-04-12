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

// CommissionWithdrawal holds the schema definition for the CommissionWithdrawal entity.
type CommissionWithdrawal struct {
	ent.Schema
}

func (CommissionWithdrawal) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "commission_withdrawals"},
	}
}

func (CommissionWithdrawal) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CommissionWithdrawal) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("withdrawal_no").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.Float("amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("fee_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.Float("net_amount").
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}).
			Default(0),
		field.String("currency").
			MaxLen(3).
			Default(consts.ReferralSettlementCurrencyCNY),
		field.String("status").
			MaxLen(32).
			Default("pending_review"),
		field.String("payout_method").
			MaxLen(32),
		field.String("payout_account_snapshot_json").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("reviewed_by").
			Optional().
			Nillable(),
		field.Time("reviewed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("paid_by").
			Optional().
			Nillable(),
		field.Time("paid_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("reject_reason").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("remark").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (CommissionWithdrawal) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("commission_withdrawals").
			Field("user_id").
			Required().
			Unique(),
		edge.To("items", CommissionWithdrawalItem.Type),
		edge.To("commission_ledgers", CommissionLedger.Type),
	}
}

func (CommissionWithdrawal) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("created_at"),
	}
}
