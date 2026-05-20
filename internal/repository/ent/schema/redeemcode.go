package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RedeemCode struct{ ent.Schema }

func (RedeemCode) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (RedeemCode) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("batch_id").Default(0),
		field.String("code").MaxLen(64).NotEmpty(),
		field.String("status").MaxLen(32).Default("inactive"),
		field.String("reward_type").MaxLen(16).Default("points"),
		field.String("reward_value").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Time("valid_from"),
		field.Time("valid_until"),
		field.Int("max_redemptions").Default(1),
		field.Int("redeemed_count").Default(0),
		field.Int64("last_redeemed_by").Optional().Nillable(),
	}
}
func (RedeemCode) Indexes() []ent.Index {
	return []ent.Index{index.Fields("code").Unique(), index.Fields("status"), index.Fields("valid_until")}
}
