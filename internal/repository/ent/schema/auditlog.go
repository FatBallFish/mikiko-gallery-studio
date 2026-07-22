package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type AuditLog struct{ ent.Schema }

func (AuditLog) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (AuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("actor_type").MaxLen(16).NotEmpty(),
		field.String("actor_id").MaxLen(128).NotEmpty(),
		field.String("action").MaxLen(64).NotEmpty(),
		field.String("target_type").MaxLen(32).NotEmpty(),
		field.String("target_id").MaxLen(128).NotEmpty(),
		field.String("result").MaxLen(16).Default("success"),
		field.JSON("metadata", map[string]any{}).Optional(),
		field.String("ip_addr").MaxLen(64).Default(""),
		field.String("user_agent").MaxLen(255).Default(""),
	}
}
func (AuditLog) Indexes() []ent.Index {
	return []ent.Index{index.Fields("actor_type"), index.Fields("actor_id"), index.Fields("action"), index.Fields("target_type"), index.Fields("target_id"), index.Fields("result"), index.Fields("created_at")}
}
