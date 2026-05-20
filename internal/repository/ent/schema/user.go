package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type User struct{ ent.Schema }

func (User) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").MaxLen(255).NotEmpty(),
		field.String("password_hash").MaxLen(255).Optional().Nillable(),
		field.String("nickname").MaxLen(64).Default(""),
		field.String("bio").MaxLen(255).Default(""),
		field.String("avatar_object_key").MaxLen(255).Optional().Nillable(),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("user_group_id").Default(0),
		field.Int("token_version").Default(0),
		field.String("default_locale").MaxLen(16).Default("zh-CN"),
		field.String("theme").MaxLen(16).Default("system"),
	}
}
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("email").Unique(),
		index.Fields("status"),
		index.Fields("user_group_id"),
	}
}
func (User) Annotations() []schema.Annotation { return nil }
