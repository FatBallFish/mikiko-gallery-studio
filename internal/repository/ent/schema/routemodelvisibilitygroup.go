package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RouteModelVisibilityGroup struct{ ent.Schema }

func (RouteModelVisibilityGroup) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("route_model_id"),
		field.Int64("group_id"),
	}
}

func (RouteModelVisibilityGroup) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("route_model_id", "group_id").Unique(),
		index.Fields("group_id"),
	}
}

func (RouteModelVisibilityGroup) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "route_model_visibility_groups"}}
}
