package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type StorageConfig struct{ ent.Schema }

func (StorageConfig) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (StorageConfig) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(64).NotEmpty(),
		field.String("name").MaxLen(100).Default(""),
		field.String("driver").MaxLen(16).Default("s3"),
		field.String("endpoint").MaxLen(255).Default(""),
		field.String("region").MaxLen(64).Default(""),
		field.String("bucket").MaxLen(128).Default(""),
		field.String("prefix").MaxLen(255).Default(""),
		field.Bool("force_path_style").Default(false),
		field.String("access_key_id_encrypted").Default(""),
		field.String("secret_access_key_encrypted").Default(""),
		field.String("status").MaxLen(32).Default("active"),
		field.Bool("is_default_write").Default(false),
		field.String("last_test_status").MaxLen(32).Default("unknown"),
		field.String("last_test_error").MaxLen(255).Default(""),
		field.Time("last_tested_at").Optional().Nillable(),
	}
}

func (StorageConfig) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("code").Unique(),
		index.Fields("driver"),
		index.Fields("status"),
		index.Fields("is_default_write"),
	}
}

func (StorageConfig) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "storage_configs"}}
}
