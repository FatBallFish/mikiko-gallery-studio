package schema

import (
	"fmt"
	"regexp"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Installation records the database identity and the versions that may use it.
// singleton_key has one CHECK-constrained legal value and a unique constraint, making
// the singleton invariant enforceable by PostgreSQL rather than convention.
type Installation struct{ ent.Schema }

var setupRequestDigestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (Installation) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (Installation) Fields() []ent.Field {
	return []ent.Field{
		field.String("singleton_key").MaxLen(32).NotEmpty().Immutable().Unique().Validate(func(value string) error {
			if value != "installation" {
				return fmt.Errorf("singleton_key must be installation")
			}
			return nil
		}),
		field.String("installation_id").MaxLen(128).NotEmpty().Match(stableIdentifierPattern).Immutable(),
		field.Int("config_schema_version").Positive(),
		field.Int("database_schema_version").Positive(),
		field.String("app_version").MaxLen(128).NotEmpty(),
		field.String("setup_operation_id").MaxLen(36).Match(stableIdentifierPattern).Optional().Nillable(),
		field.Int64("setup_admin_id").Positive().Optional().Nillable(),
		field.Int("setup_config_revision").Positive().Optional().Nillable(),
		field.String("setup_request_digest").MaxLen(64).Match(setupRequestDigestPattern).Optional().Nillable().Sensitive(),
		field.Time("initialized_at").Default(time.Now).Immutable(),
		field.Time("migrated_at").Default(time.Now),
	}
}

func (Installation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("installation_id").Unique(),
		index.Fields("setup_operation_id").Unique(),
		index.Fields("database_schema_version", "config_schema_version"),
	}
}

func (Installation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{
		Table: "installations",
		Check: "singleton_key = 'installation'",
	}}
}
