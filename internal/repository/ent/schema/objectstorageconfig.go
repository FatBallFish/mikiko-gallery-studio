package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ObjectStorageConfig struct{ ent.Schema }

func (ObjectStorageConfig) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }

func (ObjectStorageConfig) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("code").MaxLen(64).NotEmpty(),
		field.String("name").MaxLen(128).NotEmpty(),
		field.String("driver").MaxLen(16).Default("local"),
		field.String("provider").MaxLen(32).Default("local"),
		field.String("status").MaxLen(32).Default("enabled"),
		field.Bool("read_enabled").Default(true),
		field.Bool("write_enabled").Default(true),
		field.Bool("is_default").Default(false),
		field.String("endpoint").MaxLen(255).Optional().Nillable(),
		field.String("region").MaxLen(64).Optional().Nillable(),
		field.String("bucket").MaxLen(128).Optional().Nillable(),
		field.String("prefix").MaxLen(255).Default(""),
		field.Bool("force_path_style").Default(false),
		field.String("public_base_url").MaxLen(255).Optional().Nillable(),
		field.String("local_root").MaxLen(255).Optional().Nillable(),
		field.JSON("public_value", map[string]any{}).Optional(),
		field.JSON("secret_encrypted", map[string]any{}).Optional(),
		field.String("secret_fingerprint").MaxLen(128).Default(""),
		field.JSON("secret_fields", []string{}).Optional(),
		field.String("last_probe_status").MaxLen(32).Default("never"),
		field.String("last_probe_message").MaxLen(512).Default(""),
		field.Time("last_probe_at").Optional().Nillable(),
		field.Int64("version").Default(1),
		field.Int64("updated_by").Default(0),
	}
}

func (ObjectStorageConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("is_default", "status", "write_enabled"),
		index.Fields("status"),
	}
}

func (ObjectStorageConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "object_storage_configs"}}
}
