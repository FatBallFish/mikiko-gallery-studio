package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelAccountModel struct{ ent.Schema }

func (ModelAccountModel) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (ModelAccountModel) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.String("model_code").MaxLen(128).NotEmpty(),
		field.String("display_name").MaxLen(128).Default(""),
		field.JSON("task_types", []string{}).Optional(),
		field.JSON("qualities", []string{}).Optional(),
		field.String("cost_per_image").SchemaType(map[string]string{dialect.Postgres: "numeric(18,5)"}).Default("0.00000"),
		field.String("currency").MaxLen(16).Default("USD"),
		field.Bool("enabled").Default(true),
		field.JSON("extra", map[string]any{}).Optional(),
	}
}

func (ModelAccountModel) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id"),
		index.Fields("account_id", "model_code").Unique(),
		index.Fields("enabled"),
	}
}

func (ModelAccountModel) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "model_account_models"}}
}
