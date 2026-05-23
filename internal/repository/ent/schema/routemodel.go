package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RouteModel struct{ ent.Schema }

func (RouteModel) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (RouteModel) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(64).NotEmpty(),
		field.String("name").MaxLen(128).NotEmpty(),
		field.Text("description").Default(""),
		field.String("visibility").MaxLen(32).Default("hidden"),
		field.Bool("enabled").Default(false),
		field.Int("sort_order").Default(0),
	}
}

func (RouteModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("visibility", "enabled"),
	}
}

func (RouteModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "route_models"}}
}
