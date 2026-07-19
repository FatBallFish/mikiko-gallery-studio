package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PromptOptimizationRun struct{ ent.Schema }

func (PromptOptimizationRun) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (PromptOptimizationRun) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.Int64("account_id"),
		field.Int64("model_id"),
		field.String("model_code").MaxLen(128).NotEmpty(),
		field.String("api_style").MaxLen(32).NotEmpty(),
		field.Int64("config_version"),
		field.String("prompt_sha256").MaxLen(64).NotEmpty(),
		field.String("status").MaxLen(32).NotEmpty(),
		field.Int("input_tokens").Default(0),
		field.Int("output_tokens").Default(0),
		field.String("estimated_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("actual_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("provider_request_id").MaxLen(128).Default(""),
		field.String("error_code").MaxLen(64).Default(""),
		field.Text("error_message").Default(""),
		field.JSON("metadata", map[string]any{}).Optional(),
	}
}

func (PromptOptimizationRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("account_id", "model_id"),
		index.Fields("status"),
	}
}

func (PromptOptimizationRun) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "prompt_optimization_runs"}}
}
