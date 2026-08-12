package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaProcessingJob struct{ ent.Schema }

func (MediaProcessingJob) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}} }

func (MediaProcessingJob) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.UUID("asset_id", uuid.UUID{}),
		field.String("job_type").MaxLen(32).NotEmpty(),
		field.Int("transform_version").Default(1).Positive(),
		field.String("status").MaxLen(24).Default("pending"),
		field.Int("attempt_count").Default(0).NonNegative(),
		field.Int("max_attempts").Default(5).Positive(),
		field.Time("next_retry_at").Optional().Nillable(),
		field.String("lease_owner").MaxLen(64).Optional().Nillable(),
		field.Time("lease_expires_at").Optional().Nillable(),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
		field.String("requested_by_type").MaxLen(16).Default("system"),
		field.String("requested_by_id").MaxLen(64).Default(""),
	}
}

func (MediaProcessingJob) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("asset", MediaAsset.Type).Ref("processing_jobs").Field("asset_id").Required().Unique(),
	}
}

func (MediaProcessingJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("asset_id", "job_type", "transform_version").Unique(),
		index.Fields("status", "next_retry_at"),
		index.Fields("lease_expires_at"),
	}
}
