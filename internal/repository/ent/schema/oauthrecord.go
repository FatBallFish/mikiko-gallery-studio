package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OAuthRecord stores versioned OAuth clients, authorization requests and grant
// families. Family mutations use a version compare-and-swap in one SQL update.
type OAuthRecord struct{ ent.Schema }

func (OAuthRecord) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").MaxLen(512).Immutable(),
		field.String("kind").MaxLen(32).Immutable(),
		field.Int64("user_id").Default(0).Immutable(),
		field.Bytes("payload"),
		field.Int64("version").Default(1),
	}
}
func (OAuthRecord) Indexes() []ent.Index { return []ent.Index{index.Fields("kind", "user_id")} }
