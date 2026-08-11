package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VideoPriceRule struct{ ent.Schema }

func (VideoPriceRule) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoPriceRule) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("pricing_strategy_id"),
		field.String("task_type").MaxLen(32).NotEmpty(),
		field.String("resolution").MaxLen(16).NotEmpty(),
		field.String("audio_mode").MaxLen(16).Default("silent"),
		field.Int("rule_version").Default(1).Positive(),
		field.Time("effective_at"),
		field.Time("expires_at").Optional().Nillable(),
		field.String("output_second_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("fixed_task_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("reference_image_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("input_video_second_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("reference_audio_second_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("generated_audio_fixed_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("generated_audio_second_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Int("minimum_billable_seconds").Default(0).NonNegative(),
		field.String("minimum_task_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("reserve_markup").SchemaType(map[string]string{dialect.Postgres: "numeric(10,5)"}).Default("1.00000"),
		field.String("safety_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("candidate_cost_upper_cny").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.JSON("safety_snapshot", map[string]any{}),
		field.Bool("enabled").Default(false),
		field.String("internal_note").MaxLen(255).Default(""),
	}
}

func (VideoPriceRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pricing_strategy_id"),
		index.Fields("pricing_strategy_id", "task_type", "resolution", "audio_mode", "rule_version").Unique(),
		index.Fields("enabled", "effective_at"),
	}
}
