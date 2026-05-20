package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserGroup struct{ ent.Schema }

func (UserGroup) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (UserGroup) Fields() []ent.Field {
	return []ent.Field{
		field.String("group_code").MaxLen(32).NotEmpty(),
		field.String("group_name").MaxLen(64).NotEmpty(),
		field.String("multiplier").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("1.00000"),
		field.String("status").MaxLen(16).Default("active"),
		field.String("description").MaxLen(255).Optional().Nillable(),
	}
}
func (UserGroup) Indexes() []ent.Index {
	return []ent.Index{index.Fields("group_code").Unique(), index.Fields("status")}
}
