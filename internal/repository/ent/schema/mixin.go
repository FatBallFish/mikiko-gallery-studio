package schema

import (
	"regexp"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/mixin"
)

var (
	stableIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	sha256HexPattern        = regexp.MustCompile(`^[a-f0-9]{64}$`)
	base64URL32Pattern      = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)
)

type TimeMixin struct{ mixin.Schema }

func (TimeMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

type SoftDeleteMixin struct{ mixin.Schema }

func (SoftDeleteMixin) Fields() []ent.Field {
	return []ent.Field{
		field.Time("deleted_at").Optional().Nillable(),
	}
}

func numericField(name string) ent.Field {
	return field.String(name).SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"})
}
