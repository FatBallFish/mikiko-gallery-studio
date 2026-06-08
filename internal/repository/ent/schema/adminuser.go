package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AdminUser struct{ ent.Schema }

func (AdminUser) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AdminUser) Fields() []ent.Field {
	return []ent.Field{
		field.String("email").MaxLen(255).NotEmpty(),
		field.String("password_hash").MaxLen(255).NotEmpty(),
		field.String("role").MaxLen(32).Default("admin"),
		field.String("status").MaxLen(32).Default("active"),
	}
}
func (AdminUser) Indexes() []ent.Index {
	return []ent.Index{index.Fields("email").Unique(), index.Fields("role"), index.Fields("status")}
}
