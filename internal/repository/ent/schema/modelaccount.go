package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type ModelAccount struct{ ent.Schema }

func (ModelAccount) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (ModelAccount) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(128).NotEmpty(),
		field.String("adapter_type").MaxLen(64).NotEmpty(),
		field.String("auth_type").MaxLen(64).NotEmpty(),
		field.String("base_url").MaxLen(512).NotEmpty(),
		field.JSON("credentials_encrypted", map[string]string{}).Optional(),
		field.String("credentials_fingerprint").MaxLen(128).Default(""),
		field.String("status").MaxLen(32).Default("disabled"),
		field.Int("priority").Default(0),
		field.Int("weight").Default(100),
		field.Int("concurrency_limit").Default(1),
		field.Int("timeout_ms").Default(120000),
		field.Text("error_message").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
		field.JSON("extra", map[string]any{}).Optional(),
	}
}

func (ModelAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("adapter_type", "status"),
		index.Fields("deleted_at"),
	}
}

func (ModelAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "model_accounts"}}
}
