package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RouteModelCandidate struct{ ent.Schema }

func (RouteModelCandidate) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("route_model_id"),
		field.Int64("account_model_id"),
		field.Int("priority").Default(0),
		field.Int("weight").Default(100),
		field.Int("fallback_order").Default(0),
		field.Bool("enabled").Default(true),
	}
}

func (RouteModelCandidate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("route_model_id", "enabled"),
		index.Fields("route_model_id", "account_model_id").Unique(),
	}
}

func (RouteModelCandidate) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "route_model_candidates"}}
}
