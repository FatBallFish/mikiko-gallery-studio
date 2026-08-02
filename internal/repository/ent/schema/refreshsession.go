package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type RefreshSession struct{ ent.Schema }

func (RefreshSession) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }
func (RefreshSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.Int("token_version").Default(0),
		field.UUID("session_family_id", uuid.UUID{}).Default(uuid.New),
		field.String("refresh_token_hash").MaxLen(128).NotEmpty(),
		field.String("status").MaxLen(32).Default("active"),
		field.String("user_agent").MaxLen(255).Default(""),
		field.String("ip_addr").MaxLen(64).Default(""),
		field.Time("expires_at").Default(func() time.Time { return time.Now().Add(2 * time.Hour) }),
		field.Time("last_rotated_at").Optional().Nillable(),
		field.UUID("replaced_by_session_id", uuid.UUID{}).Optional().Nillable(),
	}
}
func (RefreshSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("refresh_token_hash").Unique(),
		index.Fields("user_id"),
		index.Fields("session_family_id"),
		index.Fields("status"),
		index.Fields("expires_at"),
	}
}
