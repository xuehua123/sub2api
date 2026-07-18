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

// UpstreamAccountBinding links one local API-key account to a shared upstream
// connection and stores the observed key-to-group resolution.
type UpstreamAccountBinding struct {
	ent.Schema
}

func (UpstreamAccountBinding) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upstream_account_bindings"},
	}
}

func (UpstreamAccountBinding) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (UpstreamAccountBinding) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Int64("connection_id"),
		field.String("key_fingerprint").
			Default("").
			MaxLen(128).
			Sensitive(),
		field.String("remote_token_id").
			Default("").
			MaxLen(128),
		field.String("remote_token_name").
			Default("").
			MaxLen(255),
		field.String("resolution_kind").
			Default("unresolved").
			MaxLen(32),
		field.String("remote_group_id").
			Default("").
			MaxLen(128),
		field.String("remote_group_name").
			Default("").
			MaxLen(128),
		field.JSON("fallback_groups", []string{}).
			Default(func() []string { return []string{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Float("observed_multiplier").
			Optional().
			Nillable().
			Min(0).
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("confidence").
			Default("unknown").
			MaxLen(32),
		field.String("source").
			Default("").
			MaxLen(64),
		field.String("apply_policy").
			Default("observe_only").
			MaxLen(32),
		field.String("status").
			Default("pending").
			MaxLen(32),
		field.Int("sync_failures").
			Default(0).
			NonNegative(),
		field.String("last_error").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.JSON("resolution_details", map[string]any{}).
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
		field.Time("next_sync_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamAccountBinding) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("account", Account.Type).
			Ref("upstream_binding").
			Field("account_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.From("connection", UpstreamConnection.Type).
			Ref("account_bindings").
			Field("connection_id").
			Required().
			Unique().
			Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (UpstreamAccountBinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id").Unique(),
		index.Fields("connection_id", "next_sync_at"),
		index.Fields("status"),
		index.Fields("connection_id", "remote_token_id"),
		index.Fields("resolution_kind"),
		index.Fields("fresh_until"),
	}
}
