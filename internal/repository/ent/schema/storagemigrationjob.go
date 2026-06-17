package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type StorageMigrationJob struct{ ent.Schema }

func (StorageMigrationJob) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (StorageMigrationJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("source_storage_config_id").Optional().Nillable(),
		field.Int64("target_storage_config_id").Optional().Nillable(),
		field.String("source_legacy_driver").MaxLen(16).Default(""),
		field.JSON("scope", map[string]any{}).Optional(),
		field.Bool("dry_run").Default(true),
		field.Bool("update_records").Default(true),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("total_items").Default(0),
		field.Int64("processed_items").Default(0),
		field.Int64("failed_items").Default(0),
		field.Int64("total_bytes").Default(0),
		field.String("last_error").MaxLen(255).Default(""),
		field.Int64("created_by").Default(0),
		field.Time("started_at").Optional().Nillable(),
		field.Time("finished_at").Optional().Nillable(),
	}
}

func (StorageMigrationJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("source_storage_config_id"),
		index.Fields("target_storage_config_id"),
	}
}

func (StorageMigrationJob) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "storage_migration_jobs"}}
}
