package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelRoute struct{ ent.Schema }

func (ModelRoute) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (ModelRoute) Fields() []ent.Field {
	return []ent.Field{
		field.String("group_code").MaxLen(32).NotEmpty(),
		field.String("task_type").MaxLen(32).NotEmpty(),
		field.Int64("provider_model_id").Default(0),
		field.Int("priority").Default(0),
		field.Int("weight_percent").Default(100),
		field.Int("fallback_order").Default(0),
		field.Bool("enabled").Default(true),
	}
}
func (ModelRoute) Indexes() []ent.Index {
	return []ent.Index{index.Fields("group_code", "task_type"), index.Fields("provider_model_id")}
}
func (ModelRoute) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "model_routes"}}
}
