package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type CanvasGenerationRun struct{ ent.Schema }

func (CanvasGenerationRun) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (CanvasGenerationRun) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("canvas_id", uuid.UUID{}),
		field.Int64("user_id"),
		field.String("node_id").MaxLen(64).NotEmpty(),
		field.Int64("submitted_revision").Positive(),
		field.String("task_kind").MaxLen(16).NotEmpty(),
		field.UUID("task_id", uuid.UUID{}),
		field.JSON("node_snapshot", map[string]any{}),
		field.String("status").MaxLen(24).Default("running"),
		field.JSON("result_asset_ids", []uuid.UUID{}).Optional(),
		field.Int64("attached_revision").Optional().Nillable(),
		field.String("idempotency_key").MaxLen(128).NotEmpty(),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
	}
}

func (CanvasGenerationRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("canvas", CreativeCanvas.Type).Ref("generation_runs").Field("canvas_id").Required().Unique(),
	}
}

func (CanvasGenerationRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("canvas_id", "node_id", "created_at"),
		index.Fields("canvas_id", "node_id", "idempotency_key").Unique(),
		index.Fields("task_kind", "task_id"),
		index.Fields("status"),
	}
}
