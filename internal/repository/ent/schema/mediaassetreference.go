package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaAssetReference struct{ ent.Schema }

func (MediaAssetReference) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (MediaAssetReference) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("asset_id", uuid.UUID{}),
		field.String("ref_type").MaxLen(32).NotEmpty(),
		field.UUID("ref_id", uuid.UUID{}),
		field.String("ref_key").MaxLen(128).NotEmpty(),
		field.Int64("user_id"),
	}
}

func (MediaAssetReference) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("asset", MediaAsset.Type).Ref("references").Field("asset_id").Required().Unique(),
	}
}

func (MediaAssetReference) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("asset_id"),
		index.Fields("user_id", "ref_type", "ref_id"),
		index.Fields("asset_id", "ref_type", "ref_id", "ref_key").StorageKey("media_asset_reference_active").Unique().Annotations(entsql.IndexWhere("deleted_at IS NULL")),
	}
}
