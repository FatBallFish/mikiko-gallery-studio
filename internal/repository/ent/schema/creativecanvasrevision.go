package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type CreativeCanvasRevision struct{ ent.Schema }

func (CreativeCanvasRevision) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (CreativeCanvasRevision) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("canvas_id", uuid.UUID{}),
		field.Int64("revision").Positive(),
		field.Int("schema_version").Default(1),
		field.JSON("document_json", map[string]any{}),
		field.String("reason").MaxLen(24).Default("periodic"),
		field.String("created_by").MaxLen(16).Default("system"),
		field.Int("document_bytes").Default(0).NonNegative(),
	}
}

func (CreativeCanvasRevision) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("canvas", CreativeCanvas.Type).Ref("revisions").Field("canvas_id").Required().Unique(),
	}
}

func (CreativeCanvasRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("canvas_id", "revision").Unique(),
		index.Fields("canvas_id", "created_at"),
	}
}
