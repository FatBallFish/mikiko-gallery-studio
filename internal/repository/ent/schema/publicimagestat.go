package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PublicImageStat struct{ ent.Schema }

func (PublicImageStat) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (PublicImageStat) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("image_id", uuid.UUID{}).Immutable(),
		field.Int("like_count").Default(0),
		field.Int("favorite_count").Default(0),
		field.Int("comment_count").Default(0),
	}
}
func (PublicImageStat) Indexes() []ent.Index {
	return []ent.Index{index.Fields("image_id").Unique(), index.Fields("like_count"), index.Fields("favorite_count"), index.Fields("comment_count")}
}
func (PublicImageStat) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "public_image_stats"}}
}
