package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaDerivative struct{ ent.Schema }

func (MediaDerivative) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (MediaDerivative) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("asset_id", uuid.UUID{}),
		field.String("kind").MaxLen(32).NotEmpty(),
		field.Int("transform_version").Default(1).Positive(),
		field.String("status").MaxLen(16).Default("pending"),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("storage_driver").MaxLen(16).Default("local"),
		field.String("bucket").MaxLen(255).Default(""),
		field.String("object_key").MaxLen(512).NotEmpty(),
		field.String("mime_type").MaxLen(128).Default(""),
		field.Int64("file_size_bytes").Default(0).NonNegative(),
		field.Int("width").Optional().Nillable(),
		field.Int("height").Optional().Nillable(),
		field.Int64("duration_ms").Optional().Nillable(),
		field.Int64("bitrate").Optional().Nillable(),
		field.String("sha256").MaxLen(64).Default(""),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
	}
}

func (MediaDerivative) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("asset", MediaAsset.Type).Ref("derivatives").Field("asset_id").Required().Unique(),
	}
}

func (MediaDerivative) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("asset_id", "kind", "transform_version").Unique(),
		index.Fields("status"),
		index.Fields("storage_config_id", "object_key").Unique(),
	}
}
