package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ImageResult struct{ ent.Schema }

func (ImageResult) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }
func (ImageResult) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("task_id", uuid.UUID{}),
		field.Int64("user_id"),
		field.String("image_role").MaxLen(16).Default("output"),
		field.String("storage_driver").MaxLen(16).Default("local"),
		field.String("object_key").MaxLen(255).NotEmpty(),
		field.String("mime_type").MaxLen(64).NotEmpty(),
		field.Int64("file_size_bytes").Default(0),
		field.Int("width").Default(0),
		field.Int("height").Default(0),
		field.String("sha256").MaxLen(64).NotEmpty(),
		field.String("image_group").MaxLen(64).Default(""),
		field.String("visibility_status").MaxLen(32).Default("private"),
		field.String("review_reason").MaxLen(255).Optional().Nillable(),
		field.Time("published_at").Optional().Nillable(),
	}
}
func (ImageResult) Indexes() []ent.Index {
	return []ent.Index{index.Fields("task_id"), index.Fields("user_id"), index.Fields("image_role"), index.Fields("object_key").Unique(), index.Fields("sha256"), index.Fields("visibility_status")}
}
func (ImageResult) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "task_images"}}
}
