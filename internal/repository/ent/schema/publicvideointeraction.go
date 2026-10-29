package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// PublicVideoInteraction is separate from image interactions even for equal UUIDs.
type PublicVideoInteraction struct{ ent.Schema }

func (PublicVideoInteraction) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PublicVideoInteraction) Fields() []ent.Field {
	return []ent.Field{field.UUID("video_id", uuid.UUID{}).Immutable(), field.Int64("user_id"), field.Bool("liked").Default(false), field.Bool("favorited").Default(false)}
}
func (PublicVideoInteraction) Indexes() []ent.Index {
	return []ent.Index{index.Fields("video_id", "user_id").Unique(), index.Fields("user_id", "liked"), index.Fields("user_id", "favorited")}
}
func (PublicVideoInteraction) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "public_video_interactions"}}
}
