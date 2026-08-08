package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Project struct{ ent.Schema }

func (Project) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (Project) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.String("name").MaxLen(128).NotEmpty(),
		field.String("name_key").MaxLen(128).NotEmpty(),
		field.Bool("is_default").Default(false),
		field.String("status").MaxLen(16).Default("active"),
		field.Int64("version").Default(int64(1)),
	}
}

func (Project) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("image_tasks", ImageTask.Type),
		edge.To("image_results", ImageResult.Type),
	}
}

func (Project) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
		index.Fields("user_id", "status", "updated_at"),
		index.Fields("user_id").StorageKey("project_active_default").Unique().Annotations(entsql.IndexWhere("is_default = true AND status = 'active' AND deleted_at IS NULL")),
		index.Fields("user_id", "name_key").StorageKey("project_active_name").Unique().Annotations(entsql.IndexWhere("status = 'active' AND deleted_at IS NULL")),
	}
}
