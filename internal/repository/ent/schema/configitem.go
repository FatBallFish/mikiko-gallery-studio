package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ConfigItem struct{ ent.Schema }

func (ConfigItem) Fields() []ent.Field {
	return []ent.Field{
		field.String("config_category").MaxLen(32).NotEmpty(),
		field.String("config_key").MaxLen(64).NotEmpty(),
		field.JSON("config_value", map[string]any{}),
		field.String("scope").MaxLen(16).Default("global"),
		field.Int64("version").Default(1),
		field.Int64("updated_by").Default(0),
		field.Time("updated_at"),
	}
}
func (ConfigItem) Indexes() []ent.Index {
	return []ent.Index{index.Fields("config_category", "config_key", "scope").Unique()}
}
func (ConfigItem) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "system_configs"}}
}
