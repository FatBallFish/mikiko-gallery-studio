package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserIdentity struct{ ent.Schema }

func (UserIdentity) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (UserIdentity) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("provider").MaxLen(32).NotEmpty(),
		field.String("subject").MaxLen(255).NotEmpty(),
		field.String("email").MaxLen(255).Default(""),
	}
}

func (UserIdentity) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "subject").Unique(),
		index.Fields("user_id"),
	}
}
