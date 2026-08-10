package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SubscriptionPlan struct{ ent.Schema }

func (SubscriptionPlan) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (SubscriptionPlan) Fields() []ent.Field {
	return []ent.Field{
		field.String("plan_code").MaxLen(64).Unique().NotEmpty(),
		field.String("plan_name").MaxLen(128).NotEmpty(),
		field.String("plan_type").MaxLen(32).Default("points_package"),
		field.Bool("purchase_enabled").Default(true),
		field.String("status").MaxLen(32).Default("active"),
		field.String("price_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("bonus_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Bool("credit_expiry_enabled").Default(true),
		field.Int("duration_days").Default(30),
		field.String("currency").MaxLen(16).Default("CNY"),
		field.String("description").MaxLen(255).Default(""),
		field.Int("sort_order").Default(0),
		field.JSON("metadata", map[string]any{}).Optional(),
	}
}

func (SubscriptionPlan) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("sort_order"),
	}
}
