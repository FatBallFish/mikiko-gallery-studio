package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type UserGroupMember struct{ ent.Schema }

func (UserGroupMember) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.Int64("group_id"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (UserGroupMember) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "group_id").Unique(),
		index.Fields("group_id"),
	}
}

func (UserGroupMember) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "user_group_members"}}
}
