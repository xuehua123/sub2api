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

// UpstreamGroup is a discovered group in one upstream connection.
type UpstreamGroup struct {
	ent.Schema
}

func (UpstreamGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upstream_groups"},
	}
}

func (UpstreamGroup) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (UpstreamGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("connection_id"),
		field.String("remote_id").
			Default("").
			MaxLen(128),
		field.String("name").
			NotEmpty().
			MaxLen(128),
		field.Float("rate_multiplier").
			Optional().
			Nillable().
			Min(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("source").
			Default("").
			MaxLen(64),
		field.String("confidence").
			Default("unknown").
			MaxLen(32),
		field.JSON("metadata", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("observed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("fresh_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamGroup) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("connection", UpstreamConnection.Type).
			Ref("groups").
			Field("connection_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (UpstreamGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("connection_id", "name").Unique(),
		index.Fields("connection_id", "remote_id"),
		index.Fields("observed_at"),
		index.Fields("fresh_until"),
	}
}
