package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type APIKeyQuotaReservation struct{ ent.Schema }

func (APIKeyQuotaReservation) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (APIKeyQuotaReservation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("api_key_id"),
		field.String("reservation_id").MaxLen(128).NotEmpty(),
		field.String("points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).NotEmpty(),
		field.String("usage_day").MaxLen(10).NotEmpty(),
		field.String("status").MaxLen(16).Default("active"),
	}
}
func (APIKeyQuotaReservation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("api_key_id", "reservation_id").Unique(),
		index.Fields("api_key_id", "status"),
	}
}
