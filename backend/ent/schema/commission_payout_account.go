package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// CommissionPayoutAccount holds the schema definition for the CommissionPayoutAccount entity.
type CommissionPayoutAccount struct {
	ent.Schema
}

func (CommissionPayoutAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "commission_payout_accounts"},
	}
}

func (CommissionPayoutAccount) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (CommissionPayoutAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("method").
			MaxLen(32),
		field.String("account_name").
			MaxLen(128),
		field.String("account_no_masked").
			Optional().
			Nillable().
			MaxLen(128),
		field.String("account_no_encrypted").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("bank_name").
			Optional().
			Nillable().
			MaxLen(128),
		field.String("qr_image_url").
			Optional().
			Nillable().
			MaxLen(512),
		field.Bool("is_default").
			Default(false),
		field.String("status").
			MaxLen(20).
			Default("active"),
	}
}

func (CommissionPayoutAccount) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("commission_payout_accounts").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (CommissionPayoutAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("method"),
		index.Fields("status"),
		index.Fields("is_default"),
	}
}
