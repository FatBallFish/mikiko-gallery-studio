package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ProviderErrorPolicy struct{ ent.Schema }

func (ProviderErrorPolicy) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (ProviderErrorPolicy) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider_type").MaxLen(32).NotEmpty(),
		field.Int("http_status").Default(0),
		field.String("provider_error_code").MaxLen(64).Default(""),
		field.String("action").MaxLen(32).NotEmpty(),
		field.String("platform_error_code").MaxLen(64).NotEmpty(),
		field.Int("retry_budget").Default(0),
	}
}
func (ProviderErrorPolicy) Indexes() []ent.Index {
	return []ent.Index{index.Fields("provider_type", "http_status", "provider_error_code")}
}
func (ProviderErrorPolicy) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "provider_error_policies"}}
}
