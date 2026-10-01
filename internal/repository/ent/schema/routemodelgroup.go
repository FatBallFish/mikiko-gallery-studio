package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RouteModelGroup is a presentation-level bucket users pick from before
// choosing a concrete route model: e.g. "即梦视频" grouping Seedance routes.
// Membership is many-to-many via route_model_group_members.
type RouteModelGroup struct{ ent.Schema }

func (RouteModelGroup) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (RouteModelGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(64).NotEmpty(),
		field.String("name").MaxLen(128).NotEmpty(),
		field.Text("description").Default(""),
		field.String("media_type").MaxLen(16).Default("image"),
		// icon_key selects a built-in vendor icon; icon_svg holds an admin
		// uploaded SVG document (inline storage, max 64KB). Empty both →
		// render the site favicon.
		field.String("icon_key").MaxLen(64).Default(""),
		field.Text("icon_svg").MaxLen(65536).Default(""),
		field.Int("sort_order").Default(0),
		field.Bool("enabled").Default(false),
	}
}

func (RouteModelGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("media_type", "enabled"),
	}
}

func (RouteModelGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "route_model_groups"}}
}
