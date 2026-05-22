package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserSubscription struct{ ent.Schema }

func (UserSubscription) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (UserSubscription) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("plan_id"),
		field.Int64("wallet_grant_id").Optional().Nillable(),
		field.Int64("payment_order_id").Optional().Nillable(),
		field.String("status").MaxLen(32).Default("active"),
		field.Time("started_at"),
		field.Time("current_period_start"),
		field.Time("current_period_end"),
		field.Time("expired_at").Optional().Nillable(),
		field.Time("canceled_at").Optional().Nillable(),
	}
}

func (UserSubscription) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("plan_id"),
		index.Fields("wallet_grant_id"),
		index.Fields("payment_order_id"),
		index.Fields("status"),
		index.Fields("current_period_end"),
	}
}
