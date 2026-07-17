package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ReferenceAsset struct{ ent.Schema }

func (ReferenceAsset) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }
func (ReferenceAsset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.Int64("api_key_id").Optional().Nillable(),
		field.String("upload_source").MaxLen(16).Default("web"),
		field.String("status").MaxLen(32).Default("uploading"),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("storage_driver").MaxLen(16).Default("local"),
		field.UUID("storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.String("object_key").MaxLen(255).NotEmpty(),
		field.String("mime_type").MaxLen(64).NotEmpty(),
		field.Int64("file_size_bytes").Default(0),
		field.Int("width").Optional().Nillable(),
		field.Int("height").Optional().Nillable(),
		field.String("sha256").MaxLen(64).NotEmpty(),
		field.UUID("bound_task_id", uuid.UUID{}).Optional().Nillable(),
		field.Time("expires_at").Default(func() time.Time { return time.Now().Add(24 * time.Hour) }),
	}
}
func (ReferenceAsset) Indexes() []ent.Index {
	return []ent.Index{index.Fields("object_key").Unique(), index.Fields("storage_config_id"), index.Fields("user_id"), index.Fields("status"), index.Fields("sha256"), index.Fields("expires_at")}
}
