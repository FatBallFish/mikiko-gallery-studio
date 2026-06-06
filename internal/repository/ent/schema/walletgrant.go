package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type WalletGrant struct{ ent.Schema }

func (WalletGrant) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (WalletGrant) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("grant_type").MaxLen(32).NotEmpty(),
		field.String("source_type").MaxLen(32).NotEmpty(),
		field.Int64("source_id").Optional().Nillable(),
		field.String("status").MaxLen(32).Default("active"),
		field.String("total_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("available_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("frozen_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("consumed_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Time("expires_at").Optional().Nillable(),
		field.JSON("metadata", map[string]any{}).Optional(),
	}
}

func (WalletGrant) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("grant_type"),
		index.Fields("status"),
		index.Fields("source_type", "source_id"),
		index.Fields("expires_at"),
		index.Fields("user_id", "status", "grant_type", "expires_at"),
	}
}
