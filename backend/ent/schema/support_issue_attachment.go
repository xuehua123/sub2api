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

// SupportIssueAttachment holds the schema definition for issue attachments.
type SupportIssueAttachment struct {
	ent.Schema
}

func (SupportIssueAttachment) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_issue_attachments"},
	}
}

func (SupportIssueAttachment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("issue_id").
			Optional().
			Nillable(),
		field.Int64("uploaded_by_user_id").
			Optional().
			Nillable(),
		field.String("file_path").
			MaxLen(512).
			NotEmpty().
			Sensitive(),
		field.String("file_url").
			MaxLen(512).
			NotEmpty(),
		field.String("file_name").
			MaxLen(255).
			NotEmpty(),
		field.String("mime_type").
			MaxLen(100).
			NotEmpty(),
		field.Int64("size_bytes"),
		field.String("ocr_text").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Default(""),
		field.String("visibility").
			MaxLen(16).
			Default("public"),
		field.Time("hidden_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("hidden_by_user_id").
			Optional().
			Nillable(),
		field.Time("created_at").
			Immutable().
			Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SupportIssueAttachment) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("issue", SupportIssue.Type).
			Ref("attachments").
			Field("issue_id").
			Unique(),
	}
}

func (SupportIssueAttachment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("issue_id", "visibility"),
	}
}
