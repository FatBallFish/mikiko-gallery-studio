package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PublicImageInteraction struct{ ent.Schema }

func (PublicImageInteraction) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PublicImageInteraction) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("image_id", uuid.UUID{}),
		field.Int64("user_id"),
		field.Bool("liked").Default(false),
		field.Bool("favorited").Default(false),
	}
}
func (PublicImageInteraction) Indexes() []ent.Index {
	return []ent.Index{index.Fields("image_id", "user_id").Unique(), index.Fields("user_id", "liked"), index.Fields("user_id", "favorited")}
}
func (PublicImageInteraction) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "public_image_interactions"}}
}
