package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelProvider struct{ ent.Schema }

func (ModelProvider) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (ModelProvider) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider_code").MaxLen(64).NotEmpty(),
		field.String("provider_type").MaxLen(32).NotEmpty(),
		field.String("auth_config_encrypted").Default(""),
		field.String("health_status").MaxLen(32).Default("unknown"),
		field.Bool("enabled").Default(true),
	}
}
func (ModelProvider) Indexes() []ent.Index {
	return []ent.Index{index.Fields("provider_code").Unique(), index.Fields("provider_type")}
}
func (ModelProvider) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "model_providers"}}
}
