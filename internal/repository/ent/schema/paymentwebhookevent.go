package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PaymentWebhookEvent struct{ ent.Schema }

func (PaymentWebhookEvent) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (PaymentWebhookEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider").MaxLen(32).NotEmpty(),
		field.String("trade_no").MaxLen(128).NotEmpty(),
		field.String("event_type").MaxLen(64).Default("payment.succeeded"),
		field.String("status").MaxLen(32).Default("received"),
		field.Int64("payment_order_id").Optional().Nillable(),
		field.String("signature").MaxLen(512).Optional().Nillable(),
		field.JSON("headers", map[string]any{}).Optional(),
		field.Text("payload").Optional(),
		field.Time("processed_at").Optional().Nillable(),
	}
}

func (PaymentWebhookEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider"),
		index.Fields("trade_no"),
		index.Fields("payment_order_id"),
		index.Fields("provider", "trade_no").Unique(),
	}
}
