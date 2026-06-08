package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type PaymentProviderInstance struct{ ent.Schema }

func (PaymentProviderInstance) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (PaymentProviderInstance) Fields() []ent.Field {
	return []ent.Field{
		field.String("provider_type").MaxLen(32).NotEmpty(),
		field.String("name").MaxLen(100).Default(""),
		field.JSON("config_encrypted", map[string]any{}).Optional(),
		field.String("credentials_fingerprint").MaxLen(128).Default(""),
		field.JSON("supported_methods", []string{}).Optional(),
		field.Bool("enabled").Default(true),
		field.Int("sort_order").Default(0),
		field.Int("scheduler_weight").Default(100),
		field.JSON("limits", map[string]any{}).Optional(),
		field.Bool("refund_enabled").Default(false),
		field.String("health_status").MaxLen(32).Default("unknown"),
		field.String("last_error").MaxLen(255).Default(""),
		field.Time("last_used_at").Optional().Nillable(),
		field.JSON("metadata", map[string]any{}).Optional(),
	}
}

func (PaymentProviderInstance) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider_type"),
		index.Fields("enabled"),
		index.Fields("provider_type", "enabled"),
		index.Fields("sort_order"),
	}
}
