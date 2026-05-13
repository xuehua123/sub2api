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

// SupportIssueEvent holds the schema definition for issue audit events.
type SupportIssueEvent struct {
	ent.Schema
}

func (SupportIssueEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_issue_events"},
	}
}

func (SupportIssueEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("issue_id"),
		field.Int64("actor_user_id").
			Optional().
			Nillable(),
		field.String("event_type").
			MaxLen(32).
			NotEmpty(),
		field.String("from_status").
			MaxLen(32).
			Optional().
			Nillable(),
		field.String("to_status").
			MaxLen(32).
			Optional().
			Nillable(),
		field.JSON("metadata", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportIssueEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("issue", SupportIssue.Type).
			Ref("events").
			Field("issue_id").
			Unique().
			Required(),
	}
}

func (SupportIssueEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("issue_id", "created_at"),
	}
}
