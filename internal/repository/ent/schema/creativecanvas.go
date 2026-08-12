package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type CreativeCanvas struct{ ent.Schema }

func (CreativeCanvas) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (CreativeCanvas) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.UUID("project_id", uuid.UUID{}),
		field.String("name").MaxLen(128).NotEmpty(),
		field.String("name_key").MaxLen(128).NotEmpty(),
		field.Int("schema_version").Default(1),
		field.Int64("revision").Default(1),
		field.Int64("metadata_version").Default(1),
		field.JSON("document_json", map[string]any{}),
		field.Int("document_bytes").Default(0).NonNegative(),
		field.Int("node_count").Default(0).NonNegative(),
		field.Int("edge_count").Default(0).NonNegative(),
		field.UUID("preview_derivative_id", uuid.UUID{}).Optional().Nillable(),
		field.Int("running_task_count").Default(0).NonNegative(),
		field.Int("failed_task_count").Default(0).NonNegative(),
		field.String("status").MaxLen(16).Default("active"),
		field.Time("last_transferred_at").Optional().Nillable(),
		field.Time("last_saved_at").Default(time.Now),
	}
}

func (CreativeCanvas) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("creative_canvases").Field("project_id").Required().Unique(),
		edge.To("revisions", CreativeCanvasRevision.Type),
		edge.To("generation_runs", CanvasGenerationRun.Type),
	}
}

func (CreativeCanvas) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "project_id", "updated_at", "id"),
		index.Fields("user_id", "project_id", "name_key"),
		index.Fields("status"),
		index.Fields("running_task_count"),
	}
}
