package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaUploadSession struct{ ent.Schema }

func (MediaUploadSession) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (MediaUploadSession) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.UUID("project_id", uuid.UUID{}),
		field.String("group_name").MaxLen(64).Default(""),
		field.String("original_filename").MaxLen(255).NotEmpty(),
		field.String("declared_media_type").MaxLen(16).NotEmpty(),
		field.String("declared_mime_type").MaxLen(128).NotEmpty(),
		field.Int64("declared_size_bytes").Positive(),
		field.String("declared_checksum").MaxLen(64).Optional().Nillable(),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("storage_driver").MaxLen(16).Default("local"),
		field.String("bucket").MaxLen(255).Default(""),
		field.String("object_key").MaxLen(512).NotEmpty(),
		field.Text("backend_upload_id").Optional().Nillable().Sensitive(),
		field.Int64("part_size").Positive(),
		field.Int("part_count").Positive(),
		field.String("status").MaxLen(24).Default("initialized"),
		field.Int64("reserved_bytes").NonNegative(),
		field.Int64("actual_bytes").Default(0).NonNegative(),
		field.String("idempotency_key").MaxLen(128).NotEmpty(),
		field.String("request_fingerprint").MaxLen(64).NotEmpty(),
		field.JSON("completed_parts", []map[string]any{}).Optional(),
		field.UUID("asset_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("expires_at"),
		field.Time("completed_at").Optional().Nillable(),
	}
}

func (MediaUploadSession) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "idempotency_key").Unique(),
		index.Fields("user_id", "status"),
		index.Fields("project_id"),
		index.Fields("status", "expires_at"),
		index.Fields("asset_id"),
	}
}
