package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// AssetGroup is an owner/project-scoped single-assignment media group.
type AssetGroup struct{ ent.Schema }

func (AssetGroup) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AssetGroup) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"), field.UUID("project_id", uuid.UUID{}),
		field.String("name").MaxLen(64).NotEmpty(), field.String("name_key").MaxLen(64).NotEmpty(),
		field.Int64("version").Default(1),
	}
}
func (AssetGroup) Indexes() []ent.Index {
	return []ent.Index{index.Fields("user_id", "project_id", "name_key").Unique()}
}
