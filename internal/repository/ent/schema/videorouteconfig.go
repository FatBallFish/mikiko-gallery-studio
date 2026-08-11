package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VideoRouteConfig struct{ ent.Schema }

func (VideoRouteConfig) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoRouteConfig) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("route_model_id"),
		field.JSON("task_types", []string{}),
		field.JSON("visible_options", map[string]any{}),
		field.JSON("defaults", map[string]any{}),
		field.Int("max_output_count").Default(1).Range(1, 4),
		field.Int64("pricing_strategy_id"),
		field.String("config_version").MaxLen(64).NotEmpty(),
		field.Bool("enabled").Default(false),
	}
}

func (VideoRouteConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("route_model_id").Unique(),
		index.Fields("pricing_strategy_id"),
		index.Fields("enabled"),
	}
}
