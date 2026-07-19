package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type TextModel struct{ ent.Schema }

func (TextModel) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (TextModel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.String("model_code").MaxLen(128).NotEmpty(),
		field.String("display_name").MaxLen(128).NotEmpty(),
		field.String("input_price_per_million_tokens").SchemaType(map[string]string{dialect.Postgres: "numeric(20,6)"}).Default("0.000000"),
		field.String("output_price_per_million_tokens").SchemaType(map[string]string{dialect.Postgres: "numeric(20,6)"}).Default("0.000000"),
		field.String("currency").MaxLen(8).Default("USD"),
		field.Bool("enabled").Default(true),
		field.Bool("is_default").Default(false),
		field.Int64("version").Default(1),
	}
}

func (TextModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "model_code").Unique(),
		index.Fields("is_default", "enabled"),
		index.Fields("deleted_at"),
	}
}

func (TextModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "text_models"}}
}
