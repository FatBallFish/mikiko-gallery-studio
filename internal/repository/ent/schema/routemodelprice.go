package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type RouteModelPrice struct{ ent.Schema }

func (RouteModelPrice) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("route_model_id"),
		field.String("task_type").MaxLen(64).NotEmpty(),
		field.String("quality").MaxLen(32).NotEmpty(),
		field.String("base_points").SchemaType(map[string]string{dialect.Postgres: "numeric(18,5)"}).Default("0.00000"),
		field.String("reference_multiplier").SchemaType(map[string]string{dialect.Postgres: "numeric(18,5)"}).Default("1.00000"),
		field.Bool("enabled").Default(true),
	}
}

func (RouteModelPrice) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("route_model_id", "task_type", "quality").Unique(),
	}
}

func (RouteModelPrice) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "route_model_prices"}}
}
