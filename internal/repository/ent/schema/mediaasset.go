package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaAsset struct{ ent.Schema }

func (MediaAsset) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (MediaAsset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.UUID("project_id", uuid.UUID{}),
		field.UUID("legacy_image_result_id", uuid.UUID{}).Optional().Nillable(),
		field.String("name").MaxLen(255).NotEmpty(),
		field.String("name_key").MaxLen(255).NotEmpty(),
		field.String("group_name").MaxLen(64).Default(""),
		field.String("media_type").MaxLen(16).NotEmpty(),
		field.String("source_type").MaxLen(24).NotEmpty(),
		field.String("status").MaxLen(32).Default("processing"),
		field.String("visibility_status").MaxLen(32).Default("private"),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("storage_driver").MaxLen(16).Default("local"),
		field.String("bucket").MaxLen(255).Default(""),
		field.String("object_key").MaxLen(512).NotEmpty(),
		field.String("mime_type").MaxLen(128).NotEmpty(),
		field.String("container").MaxLen(32).Default(""),
		field.String("codec").MaxLen(32).Default(""),
		field.Int64("file_size_bytes").NonNegative(),
		field.String("sha256").MaxLen(64).Default(""),
		field.Int("width").Optional().Nillable(),
		field.Int("height").Optional().Nillable(),
		field.Int64("duration_ms").Optional().Nillable(),
		field.Int("frame_rate_milli").Optional().Nillable(),
		field.String("audio_codec").MaxLen(32).Default(""),
		field.Int("channels").Optional().Nillable(),
		field.Int("sample_rate").Optional().Nillable(),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.String("source_task_kind").MaxLen(16).Optional().Nillable(),
		field.UUID("source_task_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("source_canvas_id", uuid.UUID{}).Optional().Nillable(),
		field.String("processing_error_code").MaxLen(64).Optional().Nillable(),
		field.Text("processing_error_message").Optional().Nillable(),
		field.Int64("version").Default(1),
		field.Time("processed_at").Optional().Nillable(),
	}
}

func (MediaAsset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("project", Project.Type).Ref("media_assets").Field("project_id").Required().Unique(),
		edge.To("derivatives", MediaDerivative.Type),
		edge.To("processing_jobs", MediaProcessingJob.Type),
		edge.To("references", MediaAssetReference.Type),
		edge.To("video_task_inputs", VideoTaskInput.Type),
	}
}

func (MediaAsset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("project_id"),
		index.Fields("user_id", "project_id", "created_at", "id"),
		index.Fields("user_id", "project_id", "media_type", "source_type", "status"),
		index.Fields("user_id", "project_id", "name_key"),
		index.Fields("storage_config_id", "object_key").Unique(),
		index.Fields("legacy_image_result_id").Unique(),
		index.Fields("source_task_kind", "source_task_id"),
	}
}
