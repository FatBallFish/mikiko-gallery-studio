package schema

import (
	"fmt"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
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
		field.JSON("candidate_parameter_mappings", map[string]any{}).Default(map[string]any{}),
		field.String("minimum_task_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Int("rounding_step_points").Default(1).Validate(func(value int) error {
			if value != 1 && value != 5 && value != 10 {
				return fmt.Errorf("rounding step points must be 1, 5, or 10")
			}
			return nil
		}),
		field.String("config_version").MaxLen(64).NotEmpty(),
		field.Bool("enabled").Default(false),
	}
}

func (VideoRouteConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("route_model_id").Unique(),
		index.Fields("enabled"),
	}
}
