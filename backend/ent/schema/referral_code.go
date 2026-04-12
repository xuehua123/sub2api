package schema

import (
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ReferralCode holds the schema definition for the ReferralCode entity.
type ReferralCode struct {
	ent.Schema
}

func (ReferralCode) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_codes"},
	}
}

func (ReferralCode) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ReferralCode) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("code").
			MaxLen(64).
			NotEmpty().
			Unique(),
		field.String("status").
			MaxLen(20).
			Default("active"),
		field.Bool("is_default").
			Default(false),
	}
}

func (ReferralCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("referral_codes").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (ReferralCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("status"),
		index.Fields("is_default"),
	}
}
