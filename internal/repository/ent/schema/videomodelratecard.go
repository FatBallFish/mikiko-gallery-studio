package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VideoModelRateCard struct{ ent.Schema }

func (VideoModelRateCard) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoModelRateCard) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_model_id"),
		field.String("provider_code").MaxLen(32).NotEmpty(),
		field.String("pricing_schema").MaxLen(64).NotEmpty(),
		field.Int("rate_version").Positive(),
		field.String("currency").MaxLen(16).Default("CNY"),
		field.JSON("rate_config", map[string]any{}),
		field.String("source_reference").MaxLen(512).Default(""),
		field.Time("effective_at"),
		field.Bool("enabled").Default(false),
	}
}

func (VideoModelRateCard) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_model_id", "rate_version").Unique(),
		index.Fields("account_model_id", "enabled", "effective_at"),
		index.Fields("provider_code", "pricing_schema"),
	}
}
