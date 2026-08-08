package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ObjectDeletionJob struct{ ent.Schema }

func (ObjectDeletionJob) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (ObjectDeletionJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("storage_driver").MaxLen(16).Default("local"),
		field.String("bucket").MaxLen(255).Default(""),
		field.String("object_key").MaxLen(255).NotEmpty(),
		field.String("state").MaxLen(16).Default("pending"),
		field.Int("attempt_count").Default(0),
		field.Time("next_attempt_at").Optional().Nillable(),
		field.String("last_error_code").MaxLen(64).Optional().Nillable(),
		field.Text("last_error_message").Optional().Nillable(),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (ObjectDeletionJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("state", "next_attempt_at"),
		index.Fields("storage_config_id", "bucket", "object_key"),
		index.Fields("storage_config_id", "bucket", "object_key").StorageKey("object_cleanup_config_live").Unique().Annotations(entsql.IndexWhere("storage_config_id IS NOT NULL AND state IN ('pending', 'running', 'retry', 'blocked')")),
		index.Fields("storage_driver", "bucket", "object_key").StorageKey("object_cleanup_driver_live").Unique().Annotations(entsql.IndexWhere("storage_config_id IS NULL AND state IN ('pending', 'running', 'retry', 'blocked')")),
	}
}
