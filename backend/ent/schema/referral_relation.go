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

// ReferralRelation holds the schema definition for the ReferralRelation entity.
type ReferralRelation struct {
	ent.Schema
}

func (ReferralRelation) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_relations"},
	}
}

func (ReferralRelation) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (ReferralRelation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("referrer_user_id"),
		field.String("bind_source").
			MaxLen(32),
		field.String("bind_code").
			Optional().
			Nillable().
			MaxLen(64),
		field.Time("locked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("notes").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}

func (ReferralRelation) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("referral_relation").
			Field("user_id").
			Required().
			Unique(),
		edge.From("referrer", User.Type).
			Ref("referral_referrals").
			Field("referrer_user_id").
			Required().
			Unique(),
	}
}

func (ReferralRelation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
		index.Fields("referrer_user_id"),
		index.Fields("bind_source"),
	}
}
