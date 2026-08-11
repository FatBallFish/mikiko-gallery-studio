package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PointLedger struct{ ent.Schema }

func (PointLedger) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PointLedger) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("api_key_id").Optional().Nillable(),
		field.UUID("task_id", uuid.UUID{}).Optional().Nillable(),
		field.String("task_media_type").MaxLen(16).Default("image"),
		field.JSON("usage_summary", map[string]any{}).Optional(),
		field.Int64("order_id").Optional().Nillable(),
		field.Int64("redeem_code_id").Optional().Nillable(),
		field.String("ledger_type").MaxLen(32).NotEmpty(),
		field.String("change_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("balance_after").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("frozen_after").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("balance_bucket").MaxLen(32).Default("recharge"),
		field.String("source_type").MaxLen(32).Default(""),
		field.Int64("source_id").Optional().Nillable(),
		field.String("bucket_balance_after").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Time("expires_at").Optional().Nillable(),
		field.String("reason").MaxLen(255).Default(""),
		field.Int64("operator_admin_id").Optional().Nillable(),
		field.String("idempotency_key").MaxLen(128).Optional().Nillable(),
	}
}
func (PointLedger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("api_key_id"),
		index.Fields("task_id"),
		index.Fields("order_id"),
		index.Fields("redeem_code_id"),
		index.Fields("ledger_type"),
		index.Fields("balance_bucket"),
		index.Fields("source_type", "source_id"),
		index.Fields("created_at"),
		index.Fields("idempotency_key").Unique(),
	}
}
