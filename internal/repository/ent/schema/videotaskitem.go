package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type VideoTaskItem struct{ ent.Schema }

func (VideoTaskItem) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (VideoTaskItem) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("task_id", uuid.UUID{}),
		field.Int("ordinal").NonNegative(),
		field.String("status").MaxLen(32).Default("queued"),
		field.String("stage").MaxLen(32).Default("queued"),
		field.UUID("result_asset_id", uuid.UUID{}).Optional().Nillable(),
		field.String("actual_output_seconds").SchemaType(map[string]string{dialect.Postgres: "numeric(12,3)"}).Default("0.000"),
		field.String("actual_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("provider_cost").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.JSON("artifact_snapshot", map[string]any{}).Optional(),
		field.Int("artifact_attempts").NonNegative().Default(0),
		field.Int("max_artifact_attempts").Positive().Default(3),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
		field.Time("next_action_at").Optional().Nillable(),
		field.String("lease_owner").MaxLen(64).Optional().Nillable(),
		field.Time("lease_expires_at").Optional().Nillable(),
		field.Int64("version").Default(1),
	}
}

func (VideoTaskItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("task", VideoTask.Type).Ref("items").Field("task_id").Required().Unique(),
		edge.To("attempts", VideoTaskAttempt.Type),
	}
}

func (VideoTaskItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("task_id", "ordinal").Unique(),
		index.Fields("status", "next_action_at"),
		index.Fields("lease_expires_at"),
		index.Fields("result_asset_id"),
	}
}
