package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type VideoTask struct{ ent.Schema }

func (VideoTask) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoTask) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.UUID("project_id", uuid.UUID{}),
		field.Int64("api_key_id").Optional().Nillable(),
		field.String("source_channel").MaxLen(16).Default("web"),
		field.UUID("source_canvas_id", uuid.UUID{}).Optional().Nillable(),
		field.String("source_canvas_node_id").MaxLen(64).Optional().Nillable(),
		field.String("task_type").MaxLen(32).NotEmpty(),
		field.String("status").MaxLen(32).Default("queued"),
		field.String("progress_stage").MaxLen(32).Default("queued"),
		field.Text("progress_message").Default(""),
		field.Text("prompt_template"),
		field.JSON("prompt_binding_snapshot", map[string]any{}),
		field.Text("execution_prompt"),
		field.Int64("route_model_id"),
		field.String("route_model_code").MaxLen(64).NotEmpty(),
		field.Int("duration_seconds").Positive(),
		field.String("resolution").MaxLen(16).NotEmpty(),
		field.String("aspect_ratio").MaxLen(16).NotEmpty(),
		field.Bool("generate_audio").Default(false),
		field.Int("requested_output_count").Default(1).Range(1, 4),
		field.Int("success_output_count").Default(0).Range(0, 4),
		field.String("estimated_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("reserved_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("actual_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.JSON("pricing_snapshot", map[string]any{}),
		field.JSON("routing_snapshot", map[string]any{}),
		field.String("settlement_status").MaxLen(32).Default("reserved"),
		field.String("idempotency_key").MaxLen(128).NotEmpty(),
		field.String("request_fingerprint").MaxLen(64).NotEmpty(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("finished_at").Optional().Nillable(),
	}
}

func (VideoTask) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("video_tasks").Field("project_id").Required().Unique(),
		edge.To("items", VideoTaskItem.Type),
		edge.To("inputs", VideoTaskInput.Type),
	}
}

func (VideoTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("project_id"),
		index.Fields("user_id", "project_id", "created_at", "id"),
		index.Fields("user_id", "idempotency_key").Unique(),
		index.Fields("status", "updated_at"),
		index.Fields("source_canvas_id", "source_canvas_node_id"),
	}
}
