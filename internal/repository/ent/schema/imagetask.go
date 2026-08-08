package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type ImageTask struct{ ent.Schema }

func (ImageTask) Mixin() []ent.Mixin { return []ent.Mixin{TimeMixin{}, SoftDeleteMixin{}} }
func (ImageTask) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
		field.Int64("user_id"),
		field.UUID("project_id", uuid.UUID{}).Optional().Nillable(),
		field.Int64("api_key_id").Optional().Nillable(),
		field.String("source_channel").MaxLen(16).Default("web"),
		field.String("task_type").MaxLen(32).NotEmpty(),
		field.String("status").MaxLen(32).Default("queued"),
		field.String("progress_stage").MaxLen(32).Default(""),
		field.Text("progress_message").Default(""),
		field.Text("prompt"),
		field.Text("negative_prompt").Optional().Nillable(),
		field.String("abstract_model").MaxLen(64).NotEmpty(),
		field.String("size_mode").MaxLen(16).Default("ratio"),
		field.String("base_resolution").MaxLen(16).Default("auto"),
		field.String("quality").MaxLen(16).Default("auto"),
		field.String("requested_size").MaxLen(32).Optional().Nillable(),
		field.Int("resolved_width").Optional().Nillable(),
		field.Int("resolved_height").Optional().Nillable(),
		field.String("aspect_ratio").MaxLen(16).Default("1:1"),
		field.String("output_format").MaxLen(16).Default("png"),
		field.String("background").MaxLen(16).Optional().Nillable(),
		field.Int("output_compression").Default(100),
		field.String("moderation").MaxLen(16).Default("auto"),
		field.Int("requested_output_image_count").Default(1),
		field.Int("success_output_image_count").Default(0),
		field.Int("reference_image_count").Default(0),
		field.Bool("mask_present").Default(false),
		field.Int("reference_strength").Optional().Nillable(),
		field.Int64("seed").Optional().Nillable(),
		field.String("response_mode").MaxLen(16).Default("async"),
		field.String("save_policy").MaxLen(16).Default("private"),
		field.String("estimated_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("actual_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Int64("route_model_id").Optional().Nillable(),
		field.String("route_model_code").MaxLen(64).Default(""),
		field.Int64("account_model_id").Optional().Nillable(),
		field.Int64("model_account_id").Optional().Nillable(),
		field.String("upstream_model_code").MaxLen(128).Default(""),
		field.String("effective_multiplier").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("1.00000"),
		field.String("charged_points").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Int64("provider_model_id").Optional().Nillable(),
		field.String("provider_cost").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.String("gross_margin").SchemaType(map[string]string{dialect.Postgres: "numeric(20,5)"}).Default("0.00000"),
		field.Int("fallback_count").Default(0),
		field.String("route_snapshot_version").MaxLen(64).Default(""),
		field.JSON("pricing_snapshot", map[string]any{}).Optional(),
		field.JSON("routing_snapshot", map[string]any{}).Optional(),
		field.JSON("error_policy_snapshot", map[string]any{}).Optional(),
		field.JSON("provider_trace", map[string]any{}).Optional(),
		field.String("provider_request_id").MaxLen(128).Optional().Nillable(),
		field.Time("upstream_succeeded_at").Optional().Nillable(),
		field.String("artifact_recovery_status").MaxLen(32).Default(""),
		field.Text("artifact_recovery_payload").Optional().Nillable(),
		field.Int("artifact_attempt_count").Default(0),
		field.Time("artifact_next_retry_at").Optional().Nillable(),
		field.JSON("artifact_last_diagnostic", map[string]any{}).Optional(),
		field.UUID("artifact_storage_config_id", uuid.UUID{}).Optional().Nillable(),
		field.Int64("artifact_storage_version").Default(0),
		field.String("lease_owner").MaxLen(64).Optional().Nillable(),
		field.Time("lease_expires_at").Optional().Nillable(),
		field.String("error_code").MaxLen(64).Optional().Nillable(),
		field.Text("error_message").Optional().Nillable(),
		field.Time("started_at").Optional().Nillable(),
		field.Time("finished_at").Optional().Nillable(),
	}
}
func (ImageTask) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"), index.Fields("project_id"), index.Fields("user_id", "project_id", "created_at"), index.Fields("api_key_id"), index.Fields("source_channel"), index.Fields("task_type"),
		index.Fields("status"), index.Fields("abstract_model"), index.Fields("route_model_code"), index.Fields("base_resolution"), index.Fields("provider_model_id"), index.Fields("account_model_id"), index.Fields("model_account_id"), index.Fields("lease_owner"),
		index.Fields("lease_expires_at"), index.Fields("artifact_recovery_status", "artifact_next_retry_at"), index.Fields("error_code"), index.Fields("created_at"), index.Fields("deleted_at"),
	}
}
