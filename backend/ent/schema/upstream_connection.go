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

// UpstreamConnection stores one reusable management identity for an upstream
// wallet. API-key accounts bind to a connection instead of duplicating its
// management credentials.
type UpstreamConnection struct {
	ent.Schema
}

func (UpstreamConnection) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "upstream_connections"},
	}
}

func (UpstreamConnection) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixins.TimeMixin{},
	}
}

func (UpstreamConnection) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").
			NotEmpty().
			MaxLen(100),
		field.String("provider").
			Default("auto").
			MaxLen(32),
		field.String("auth_mode").
			NotEmpty().
			MaxLen(32),
		field.String("management_base_url").
			NotEmpty().
			MaxLen(500),
		field.String("forwarding_base_url").
			Default("").
			MaxLen(500),
		field.String("credential_encrypted").
			NotEmpty().
			Sensitive().
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("credential_fingerprint").
			Default("").
			MaxLen(128).
			Sensitive(),
		field.String("legacy_migration_key").
			Optional().
			Nillable().
			MaxLen(128).
			Unique().
			Sensitive(),
		field.String("credential_hint").
			Default("").
			MaxLen(100),
		field.String("remote_user_id").
			Default("").
			MaxLen(128),
		field.Int64("proxy_id").
			Optional().
			Nillable(),
		field.JSON("capabilities", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("status").
			Default("pending").
			MaxLen(32),
		field.String("last_error").
			Default("").
			SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Bool("sync_enabled").
			Default(true),
		field.Int("sync_interval_seconds").
			Default(300).
			Range(30, 86400),
		field.Int("sync_failures").
			Default(0).
			NonNegative(),
		field.Int64("version").
			Default(1).
			Positive(),
		field.Float("wallet_amount").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.String("wallet_currency").
			Default("").
			MaxLen(16),
		field.Float("wallet_usd").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "decimal(20,8)"}),
		field.Bool("wallet_unlimited").
			Default(false),
		field.String("wallet_source").
			Default("").
			MaxLen(64),
		field.String("wallet_reliability").
			Default("unknown").
			MaxLen(32),
		field.JSON("wallet_raw", map[string]any{}).
			Default(func() map[string]any { return map[string]any{} }).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("wallet_observed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_discovered_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_synced_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("next_sync_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamConnection) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("proxy", Proxy.Type).
			Field("proxy_id").
			Unique().
			Annotations(entsql.OnDelete(entsql.SetNull)),
		edge.To("groups", UpstreamGroup.Type),
		edge.To("account_bindings", UpstreamAccountBinding.Type),
	}
}

func (UpstreamConnection) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("sync_enabled", "next_sync_at"),
		index.Fields("status"),
		index.Fields("management_base_url"),
		index.Fields("forwarding_base_url"),
		index.Fields("provider"),
		index.Fields("remote_user_id"),
		index.Fields("proxy_id"),
	}
}
