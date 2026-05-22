package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PaymentOrder struct{ ent.Schema }

func (PaymentOrder) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (PaymentOrder) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("plan_id"),
		field.String("order_no").MaxLen(64).Unique().NotEmpty(),
		field.String("provider").MaxLen(32).NotEmpty(),
		field.String("status").MaxLen(32).Default("pending"),
		field.String("currency").MaxLen(16).Default("CNY"),
		field.String("amount_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("bonus_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("trade_no").MaxLen(128).Optional().Nillable(),
		field.String("payment_url").MaxLen(2048).Optional().Nillable(),
		field.String("qr_code").MaxLen(4096).Optional().Nillable(),
		field.String("client_token").MaxLen(4096).Optional().Nillable(),
		field.String("failure_reason").MaxLen(255).Optional().Nillable(),
		field.Time("expires_at"),
		field.Time("paid_at").Optional().Nillable(),
		field.Time("closed_at").Optional().Nillable(),
		field.Time("refunded_at").Optional().Nillable(),
		field.JSON("provider_payload", map[string]any{}).Optional(),
	}
}

func (PaymentOrder) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("plan_id"),
		index.Fields("provider"),
		index.Fields("status"),
		index.Fields("trade_no"),
		index.Fields("expires_at"),
	}
}
