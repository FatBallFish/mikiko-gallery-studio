package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type VideoProviderCallbackEvent struct{ ent.Schema }

func (VideoProviderCallbackEvent) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (VideoProviderCallbackEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.String("provider_code").MaxLen(64).NotEmpty(),
		field.Int64("model_account_id").Positive(),
		field.String("provider_event_id").MaxLen(192).NotEmpty(),
		field.String("provider_job_id").MaxLen(192).NotEmpty(),
		field.String("status").MaxLen(24).Default("received"),
		field.JSON("payload_snapshot", map[string]any{}),
		field.Time("received_at"),
		field.Time("processed_at").Optional().Nillable(),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
	}
}

func (VideoProviderCallbackEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("model_account_id", "provider_event_id").Unique(),
		index.Fields("model_account_id", "provider_job_id"),
		index.Fields("status", "received_at"),
	}
}
