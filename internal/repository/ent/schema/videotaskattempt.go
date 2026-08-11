package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type VideoTaskAttempt struct{ ent.Schema }

func (VideoTaskAttempt) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (VideoTaskAttempt) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("item_id", uuid.UUID{}),
		field.Int("attempt_no").Positive(),
		field.Int64("route_candidate_id"),
		field.Int64("account_model_id"),
		field.Int64("model_account_id"),
		field.String("provider_code").MaxLen(64).NotEmpty(),
		field.String("model_code").MaxLen(128).NotEmpty(),
		field.String("provider_job_id").MaxLen(192).Optional().Nillable(),
		field.String("provider_idempotency_key").MaxLen(128).NotEmpty(),
		field.String("status").MaxLen(32).Default("submitting"),
		field.JSON("request_snapshot", map[string]any{}),
		field.JSON("provider_status_snapshot", map[string]any{}).Optional(),
		field.JSON("usage_raw", map[string]any{}).Optional(),
		field.JSON("usage_normalized", map[string]any{}).Optional(),
		field.JSON("cost_snapshot", map[string]any{}).Optional(),
		field.String("provider_cost").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Bool("platform_absorbed").Default(false),
		field.Time("artifact_url_expires_at").Optional().Nillable(),
		field.String("error_category").MaxLen(32).Optional().Nillable(),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("finished_at").Optional().Nillable(),
	}
}

func (VideoTaskAttempt) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", VideoTaskItem.Type).Ref("attempts").Field("item_id").Required().Unique(),
	}
}

func (VideoTaskAttempt) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("item_id", "attempt_no").Unique(),
		index.Fields("model_account_id", "provider_job_id").Unique(),
		index.Fields("status"),
		index.Fields("artifact_url_expires_at"),
	}
}
