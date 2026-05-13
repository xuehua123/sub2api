package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SupportIssue holds the schema definition for public support issues.
type SupportIssue struct {
	ent.Schema
}

func (SupportIssue) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "support_issues"},
	}
}

func (SupportIssue) Fields() []ent.Field {
	return []ent.Field{
		field.String("public_id").
			MaxLen(32).
			NotEmpty().
			Comment("Public display ID, e.g. ISS-000123"),
		field.String("title").
			MaxLen(160).
			NotEmpty(),
		field.String("description").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.String("account_email").
			MaxLen(255).
			NotEmpty().
			Sensitive(),
		field.String("account_email_normalized").
			MaxLen(255).
			NotEmpty().
			Sensitive(),
		field.String("account_email_masked").
			MaxLen(255).
			NotEmpty(),
		field.Time("occurred_at").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("screenshot_text").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			NotEmpty(),
		field.String("screenshot_language").
			MaxLen(16).
			Default(domain.SupportIssueScreenshotLanguageUnknown),
		field.String("category").
			MaxLen(32).
			Default(domain.SupportIssueCategoryOther),
		field.String("severity").
			MaxLen(32).
			Default(domain.SupportIssueSeverityQuestion),
		field.String("status").
			MaxLen(32).
			Default(domain.SupportIssueStatusOpen),
		field.String("model_name").
			MaxLen(255).
			Default(""),
		field.String("client_name").
			MaxLen(120).
			Default(""),
		field.Int("http_status").
			Optional().
			Nillable(),
		field.String("error_code").
			MaxLen(120).
			Default(""),
		field.String("api_key_suffix").
			MaxLen(16).
			Default("").
			Sensitive(),
		field.Int64("created_by_user_id").
			Optional().
			Nillable(),
		field.Int64("resolved_by_user_id").
			Optional().
			Nillable(),
		field.Time("resolved_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("locked_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_comment_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int("comment_count").
			Default(0),
		field.Int("hidden_comment_count").
			Default(0),
		field.Int("attachment_count").
			Default(0),
		field.String("search_text").
			SchemaType(map[string]string{dialect.Postgres: "text"}).
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

func (SupportIssue) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("comments", SupportIssueComment.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("attachments", SupportIssueAttachment.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("events", SupportIssueEvent.Type).
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (SupportIssue) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("public_id").Unique(),
		index.Fields("status", "updated_at"),
		index.Fields("status", "last_comment_at"),
		index.Fields("category", "status"),
		index.Fields("created_by_user_id", "created_at"),
		index.Fields("account_email_normalized"),
		index.Fields("occurred_at"),
		index.Fields("http_status"),
		index.Fields("error_code"),
	}
}
