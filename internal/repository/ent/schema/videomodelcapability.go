package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type VideoModelCapability struct{ ent.Schema }

func (VideoModelCapability) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (VideoModelCapability) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_model_id"),
		field.Int("schema_version").Default(1),
		field.String("capability_version").MaxLen(64).NotEmpty(),
		field.JSON("capability_json", map[string]any{}),
		field.String("validation_status").MaxLen(16).Default("untested"),
		field.Time("last_tested_at").Optional().Nillable(),
		field.Bool("enabled").Default(false),
	}
}

func (VideoModelCapability) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_model_id").Unique(),
		index.Fields("capability_version"),
		index.Fields("enabled", "validation_status"),
	}
}
