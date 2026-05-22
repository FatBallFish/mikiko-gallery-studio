package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ProviderModel struct{ ent.Schema }

func (ProviderModel) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (ProviderModel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("provider_id"),
		field.String("model_code").MaxLen(128).NotEmpty(),
		field.String("compat_mode").MaxLen(64).Default(""),
		field.Bool("supports_image_input").Default(false),
		field.Bool("supports_mask").Default(false),
		field.JSON("supported_qualities", []string{}).Optional(),
		field.JSON("supported_ratios", []string{}).Optional(),
		field.Int("max_image_count").Default(1),
		field.Int("max_reference_image_count").Default(0),
		field.Int("timeout_ms").Default(60000),
		field.String("input_cost").MaxLen(32).Default("0"),
		field.String("output_cost").MaxLen(32).Default("0"),
		field.String("currency").MaxLen(16).Default("CNY"),
		field.String("health_status").MaxLen(32).Default("unknown"),
		field.Time("last_health_checked_at").Optional().Nillable(),
		field.Bool("enabled").Default(true),
	}
}

func (ProviderModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_id"),
		index.Fields("provider_id", "model_code").Unique(),
		index.Fields("health_status"),
		index.Fields("enabled"),
	}
}

func (ProviderModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_models"}}
}
