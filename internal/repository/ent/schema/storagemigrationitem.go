package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type StorageMigrationItem struct{ ent.Schema }

func (StorageMigrationItem) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (StorageMigrationItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("job_id", uuid.UUID{}),
		field.String("object_kind").MaxLen(32).NotEmpty(),
		field.UUID("object_id", uuid.UUID{}),
		field.String("source_object_key").MaxLen(255).NotEmpty(),
		field.String("target_object_key").MaxLen(255).Default(""),
		field.String("status").MaxLen(32).Default("pending"),
		field.Int64("size_bytes").Default(0),
		field.String("error").MaxLen(255).Default(""),
		field.Time("copied_at").Optional().Nillable(),
		field.Time("record_updated_at").Optional().Nillable(),
	}
}

func (StorageMigrationItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_id"),
		index.Fields("status"),
		index.Fields("object_kind", "object_id"),
	}
}

func (StorageMigrationItem) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "storage_migration_items"}}
}
