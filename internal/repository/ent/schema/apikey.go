package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type APIKey struct{ ent.Schema }

func (APIKey) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }
func (APIKey) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("user_id"),
		field.String("access_key").MaxLen(64).NotEmpty(),
		field.String("secret_hash").MaxLen(128).NotEmpty(),
		field.String("name").MaxLen(64).NotEmpty(),
		field.String("status").MaxLen(32).Default("active"),
		field.String("group_code").MaxLen(32).Default("default"),
		field.String("total_quota_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Optional().Nillable(),
		field.String("daily_quota_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Optional().Nillable(),
		field.Int("rpm_limit").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.Time("last_used_at").Optional().Nillable(),
	}
}
func (APIKey) Indexes() []ent.Index {
	return []ent.Index{index.Fields("access_key").Unique(), index.Fields("user_id"), index.Fields("status"), index.Fields("group_code")}
}
