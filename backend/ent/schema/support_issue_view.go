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

// SupportIssueView tracks deduplicated public issue views.
type SupportIssueView struct {
	ent.Schema
}

func (SupportIssueView) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_issue_views"},
	}
}

func (SupportIssueView) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("issue_id"),
		field.Int64("viewer_user_id").
			Optional().
			Nillable(),
		field.String("viewer_hash").
			MaxLen(64).
			NotEmpty(),
		field.Time("viewed_at").
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportIssueView) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("issue", SupportIssue.Type).
			Ref("views").
			Field("issue_id").
			Unique().
			Required(),
	}
}

func (SupportIssueView) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("issue_id", "viewed_at"),
		index.Fields("viewer_hash", "issue_id", "viewed_at"),
		index.Fields("viewer_user_id", "issue_id", "viewed_at"),
	}
}
