package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type VideoTaskInput struct{ ent.Schema }

func (VideoTaskInput) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (VideoTaskInput) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("task_id", uuid.UUID{}),
		field.UUID("asset_id", uuid.UUID{}),
		field.String("role").MaxLen(32).NotEmpty(),
		field.Int("ordinal").NonNegative(),
		field.JSON("asset_snapshot", map[string]any{}),
	}
}

func (VideoTaskInput) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", VideoTask.Type).Ref("inputs").Field("task_id").Required().Unique(),
		edge.From("asset", MediaAsset.Type).Ref("video_task_inputs").Field("asset_id").Required().Unique(),
	}
}

func (VideoTaskInput) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id"),
		index.Fields("asset_id"),
		index.Fields("task_id", "role", "ordinal").Unique(),
	}
}
