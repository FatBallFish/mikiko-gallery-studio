package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type SecureConfig struct{ ent.Schema }

func (SecureConfig) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (SecureConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("config_category").MaxLen(32).NotEmpty(),
		field.String("config_key").MaxLen(64).NotEmpty(),
		field.JSON("public_value", map[string]any{}).Optional(),
		field.JSON("secret_encrypted", map[string]any{}).Optional(),
		field.String("secret_fingerprint").MaxLen(128).Default(""),
		field.JSON("secret_fields", []string{}).Optional(),
		field.Int64("version").Default(1),
		field.Int64("updated_by").Default(0),
	}
}

func (SecureConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("config_category", "config_key").Unique(),
	}
}
