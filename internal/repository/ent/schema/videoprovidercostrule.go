package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VideoProviderCostRule struct{ ent.Schema }

func (VideoProviderCostRule) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoProviderCostRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_model_id"),
		field.String("billing_mode").MaxLen(32).NotEmpty(),
		field.Int("rule_version").Default(1).Positive(),
		field.String("currency").MaxLen(16).Default("CNY"),
		field.JSON("rates_json", map[string]any{}),
		field.Int("supported_currency_scale").Default(5).Range(0, 10),
		field.String("cost_reserve_markup").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("1.00000"),
		field.String("source_type").MaxLen(16).Default("manual"),
		field.String("source_reference").MaxLen(255).Default(""),
		field.String("validation_status").MaxLen(16).Default("untested"),
		field.Time("last_tested_at").Optional().Nillable(),
		field.Time("effective_at"),
		field.Time("expires_at").Optional().Nillable(),
		field.Bool("enabled").Default(false),
	}
}

func (VideoProviderCostRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_model_id"),
		index.Fields("account_model_id", "rule_version").Unique(),
		index.Fields("enabled", "effective_at"),
	}
}
