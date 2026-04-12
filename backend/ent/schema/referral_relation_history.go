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

// ReferralRelationHistory holds the schema definition for the ReferralRelationHistory entity.
type ReferralRelationHistory struct {
	ent.Schema
}

func (ReferralRelationHistory) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "referral_relation_histories"},
	}
}

func (ReferralRelationHistory) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("old_referrer_user_id").
			Optional().
			Nillable(),
		field.Int64("new_referrer_user_id").
			Optional().
			Nillable(),
		field.String("old_bind_code").
			Optional().
			Nillable().
			MaxLen(64),
		field.String("new_bind_code").
			Optional().
			Nillable().
			MaxLen(64),
		field.String("change_source").
			MaxLen(32),
		field.Int64("changed_by").
			Optional().
			Nillable(),
		field.String("reason").
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

func (ReferralRelationHistory) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).
			Ref("referral_relation_histories").
			Field("user_id").
			Required().
			Unique(),
	}
}

func (ReferralRelationHistory) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("changed_by"),
		index.Fields("created_at"),
	}
}
