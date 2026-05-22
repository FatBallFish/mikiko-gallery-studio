package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type WalletReservationAllocation struct{ ent.Schema }

func (WalletReservationAllocation) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (WalletReservationAllocation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("wallet_grant_id"),
		field.UUID("task_id", uuid.UUID{}),
		field.Int("reservation_cycle").Default(0),
		field.String("status").MaxLen(32).Default("reserved"),
		field.String("reserved_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("consumed_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("refunded_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
	}
}

func (WalletReservationAllocation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("wallet_grant_id"),
		index.Fields("task_id"),
		index.Fields("task_id", "reservation_cycle"),
		index.Fields("status"),
	}
}
