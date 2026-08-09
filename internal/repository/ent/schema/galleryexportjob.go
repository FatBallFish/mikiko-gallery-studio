package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type GalleryExportJob struct{ ent.Schema }

func (GalleryExportJob) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (GalleryExportJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.UUID("project_id", uuid.UUID{}),
		field.JSON("image_ids", []string{}),
		field.String("state").MaxLen(16).Default("queued"),
		field.Int64("estimated_bytes").Default(0),
		field.Int64("archive_size_bytes").Default(0),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("storage_driver").MaxLen(16).Default(""),
		field.String("bucket").MaxLen(255).Default(""),
		field.String("object_key").MaxLen(255).Default(""),
		field.Int("attempt_count").Default(0),
		field.String("lease_owner").MaxLen(128).Optional().Nillable(),
		field.Time("lease_expires_at").Optional().Nillable(),
		field.Time("next_attempt_at").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable(),
		field.String("last_error_code").MaxLen(64).Optional().Nillable(),
		field.String("last_error_message").MaxLen(512).Optional().Nillable(),
	}
}

func (GalleryExportJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "created_at"),
		index.Fields("state", "next_attempt_at"),
		index.Fields("state", "expires_at"),
		index.Fields("storage_config_id", "object_key"),
	}
}

func (GalleryExportJob) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "gallery_export_jobs"}}
}
