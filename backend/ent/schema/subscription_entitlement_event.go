package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SubscriptionEntitlementEvent is a user-visible receipt, not a billing ledger.
type SubscriptionEntitlementEvent struct{ ent.Schema }

func (SubscriptionEntitlementEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("entitlement_id"),
		field.Int64("user_id"),
		field.String("kind").MaxLen(32),
		field.String("source_type").MaxLen(32).Default(""),
		field.Time("previous_expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("new_expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("validity_seconds").Default(0),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionEntitlementEvent) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id", "entitlement_id", "id")}
}
