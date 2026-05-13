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

// SupportIssueComment holds the schema definition for issue comments.
type SupportIssueComment struct {
	ent.Schema
}

func (SupportIssueComment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_issue_comments"},
	}
}

func (SupportIssueComment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("issue_id"),
		field.Int64("author_user_id").
			Optional().
			Nillable(),
		field.String("author_role").
			MaxLen(16).
			NotEmpty(),
		field.String("content").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.Time("hidden_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("hidden_by_user_id").
			Optional().
			Nillable(),
		field.String("hide_reason").
			MaxLen(255).
			Default(""),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportIssueComment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("issue", SupportIssue.Type).
			Ref("comments").
			Field("issue_id").
			Unique().
			Required(),
	}
}

func (SupportIssueComment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("issue_id", "created_at"),
		index.Fields("issue_id", "hidden_at"),
	}
}
