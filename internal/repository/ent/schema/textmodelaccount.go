package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type TextModelAccount struct{ ent.Schema }

func (TextModelAccount) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (TextModelAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(128).NotEmpty(),
		field.String("platform_type").MaxLen(64).NotEmpty(),
		field.String("api_style").MaxLen(32).NotEmpty(),
		field.String("base_url").MaxLen(512).NotEmpty(),
		field.JSON("secret_encrypted", map[string]any{}).Optional(),
		field.String("secret_fingerprint").MaxLen(128).Default(""),
		field.Bool("enabled").Default(false),
		field.Int64("version").Default(1),
	}
}

func (TextModelAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("platform_type", "enabled"),
		index.Fields("deleted_at"),
	}
}

func (TextModelAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "text_model_accounts"}}
}
