package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// RouteModelGroupMember links a route model into a route model group.
// A model may appear in several groups; ordering inside a group is kept
// explicit via sort_order.
type RouteModelGroupMember struct{ ent.Schema }

func (RouteModelGroupMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("group_id"),
		field.Int64("route_model_id"),
		field.Int("sort_order").Default(0),
	}
}

func (RouteModelGroupMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "route_model_id").Unique(),
		index.Fields("route_model_id"),
	}
}

func (RouteModelGroupMember) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "route_model_group_members"}}
}
