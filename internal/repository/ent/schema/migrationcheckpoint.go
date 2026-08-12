package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// MigrationCheckpoint persists restart-safe progress for bounded data migrations.
type MigrationCheckpoint struct{ ent.Schema }

func (MigrationCheckpoint) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (MigrationCheckpoint) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(128).NotEmpty().Immutable(),
		field.String("phase").MaxLen(16).Default("users"),
		field.Int("after_user_id").Default(0),
		field.UUID("after_task_id", uuid.UUID{}).Optional().Nillable(),
		field.UUID("after_result_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("after_created_at").Optional().Nillable(),
		field.Int("processed_rows").Default(0),
		field.Bool("completed").Default(false),
	}
}

func (MigrationCheckpoint) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name").Unique()}
}
