package schema

import (
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
)

func TestMultimediaSchemasExposeRequiredContracts(t *testing.T) {
	tests := []struct {
		name     string
		fields   []ent.Field
		required []string
	}{
		{name: "video_model_capabilities", fields: VideoModelCapability{}.Fields(), required: []string{"account_model_id", "schema_version", "capability_version", "capability_json", "validation_status", "enabled"}},
		{name: "video_model_rate_cards", fields: VideoModelRateCard{}.Fields(), required: []string{"account_model_id", "provider_code", "pricing_schema", "rate_version", "currency", "rate_config", "source_reference", "effective_at", "enabled"}},
		{name: "video_route_configs", fields: VideoRouteConfig{}.Fields(), required: []string{"route_model_id", "task_types", "visible_options", "defaults", "max_output_count", "candidate_parameter_mappings", "minimum_task_points", "rounding_step_points", "config_version", "enabled"}},
		{name: "video_pricing_strategies", fields: VideoPricingStrategy{}.Fields(), required: []string{"code", "gross_point_value_cny", "minimum_net_point_income_cny", "target_margin_rate", "provider_cost_buffer_rate", "exact_reserve_markup", "metered_reserve_markup", "strategy_version", "enabled"}},
		{name: "video_price_rules", fields: VideoPriceRule{}.Fields(), required: []string{"pricing_strategy_id", "task_type", "resolution", "audio_mode", "rule_version", "output_second_points", "reserve_markup", "safety_points", "candidate_cost_upper_cny", "safety_snapshot", "enabled"}},
		{name: "video_provider_cost_rules", fields: VideoProviderCostRule{}.Fields(), required: []string{"account_model_id", "billing_mode", "rule_version", "currency", "rates_json", "cost_reserve_markup", "validation_status", "effective_at", "enabled"}},
		{name: "video_tasks", fields: VideoTask{}.Fields(), required: []string{"id", "user_id", "project_id", "source_channel", "task_type", "status", "prompt_template", "execution_prompt", "requested_output_count", "estimated_points", "reserved_points", "actual_points", "settlement_status", "idempotency_key", "request_fingerprint"}},
		{name: "video_task_items", fields: VideoTaskItem{}.Fields(), required: []string{"id", "task_id", "ordinal", "status", "stage", "result_asset_id", "actual_points", "artifact_snapshot", "artifact_attempts", "max_artifact_attempts", "next_action_at", "lease_owner", "lease_expires_at", "version"}},
		{name: "video_task_inputs", fields: VideoTaskInput{}.Fields(), required: []string{"id", "task_id", "asset_id", "role", "ordinal", "asset_snapshot"}},
		{name: "video_task_attempts", fields: VideoTaskAttempt{}.Fields(), required: []string{"id", "item_id", "attempt_no", "provider_code", "model_code", "provider_job_id", "provider_idempotency_key", "status", "usage_raw", "usage_normalized", "cost_snapshot", "provider_cost", "platform_absorbed"}},
		{name: "video_provider_callback_events", fields: VideoProviderCallbackEvent{}.Fields(), required: []string{"id", "provider_code", "model_account_id", "provider_event_id", "provider_job_id", "status", "payload_snapshot", "received_at", "processed_at"}},
		{name: "media_assets", fields: MediaAsset{}.Fields(), required: []string{"id", "user_id", "project_id", "legacy_image_result_id", "name", "name_key", "media_type", "source_type", "status", "object_key", "file_size_bytes", "source_task_kind", "source_task_id", "version"}},
		{name: "media_derivatives", fields: MediaDerivative{}.Fields(), required: []string{"id", "asset_id", "kind", "transform_version", "status", "object_key", "file_size_bytes"}},
		{name: "media_upload_sessions", fields: MediaUploadSession{}.Fields(), required: []string{"id", "user_id", "project_id", "original_filename", "declared_size_bytes", "backend_upload_id", "part_size", "part_count", "status", "reserved_bytes", "idempotency_key", "request_fingerprint", "completed_parts", "asset_id", "expires_at"}},
		{name: "media_processing_jobs", fields: MediaProcessingJob{}.Fields(), required: []string{"id", "asset_id", "job_type", "transform_version", "status", "attempt_count", "max_attempts", "next_retry_at", "lease_owner", "lease_expires_at"}},
		{name: "media_asset_references", fields: MediaAssetReference{}.Fields(), required: []string{"id", "asset_id", "ref_type", "ref_id", "ref_key", "user_id"}},
		{name: "creative_canvases", fields: CreativeCanvas{}.Fields(), required: []string{"id", "user_id", "project_id", "name", "name_key", "schema_version", "revision", "metadata_version", "document_json", "document_bytes", "node_count", "edge_count", "running_task_count", "status", "last_saved_at"}},
		{name: "creative_canvas_revisions", fields: CreativeCanvasRevision{}.Fields(), required: []string{"id", "canvas_id", "revision", "schema_version", "document_json", "reason", "created_by", "document_bytes"}},
		{name: "canvas_generation_runs", fields: CanvasGenerationRun{}.Fields(), required: []string{"id", "canvas_id", "user_id", "node_id", "submitted_revision", "task_kind", "task_id", "node_snapshot", "status", "result_asset_ids", "attached_revision", "idempotency_key"}},
		{name: "migration_checkpoints", fields: MigrationCheckpoint{}.Fields(), required: []string{"after_result_id", "after_created_at"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields := schemaFieldDescriptors(tt.fields)
			for _, name := range tt.required {
				if _, ok := fields[name]; !ok {
					t.Errorf("%s is missing field %s", tt.name, name)
				}
			}
		})
	}
}

func TestVideoRouteConfigNoLongerBindsLegacyPricingStrategy(t *testing.T) {
	fields := schemaFieldDescriptors(VideoRouteConfig{}.Fields())
	if _, exists := fields["pricing_strategy_id"]; exists {
		t.Fatal("video_route_configs.pricing_strategy_id must be removed from the active schema")
	}
	if !hasIndexFields(VideoModelRateCard{}.Indexes(), []string{"account_model_id", "rate_version"}, true) {
		t.Fatal("video model rate cards must have immutable versions per account model")
	}
}

func TestMultimediaSchemaDefaultsAndIndexes(t *testing.T) {
	accountFields := schemaFieldDescriptors(ModelAccount{}.Fields())
	publicID := requireSchemaField(t, accountFields, "public_id")
	if !publicID.Unique || publicID.Default == nil {
		t.Fatal("model_accounts.public_id must be a generated unique public callback alias")
	}
	routeFields := schemaFieldDescriptors(RouteModel{}.Fields())
	mediaType := requireSchemaField(t, routeFields, "media_type")
	if defaultValue, ok := mediaType.Default.(string); !ok || defaultValue != "image" {
		t.Fatalf("route_models.media_type default = %#v, want image", mediaType.Default)
	}
	ledgerFields := schemaFieldDescriptors(PointLedger{}.Fields())
	if defaultValue, ok := requireSchemaField(t, ledgerFields, "task_media_type").Default.(string); !ok || defaultValue != "image" {
		t.Fatalf("point_ledgers.task_media_type default = %#v, want image", requireSchemaField(t, ledgerFields, "task_media_type").Default)
	}
	requireSchemaField(t, ledgerFields, "usage_summary")

	maxOutput := requireSchemaField(t, schemaFieldDescriptors(VideoRouteConfig{}.Fields()), "max_output_count")
	if len(maxOutput.Validators) == 0 {
		t.Fatal("video_route_configs.max_output_count must enforce 1..4")
	}
	requestedOutput := requireSchemaField(t, schemaFieldDescriptors(VideoTask{}.Fields()), "requested_output_count")
	if len(requestedOutput.Validators) == 0 {
		t.Fatal("video_tasks.requested_output_count must enforce 1..4")
	}
	if !hasIndexFields(VideoTask{}.Indexes(), []string{"user_id", "idempotency_key"}, true) {
		t.Fatal("video_tasks must uniquely index user_id and idempotency_key")
	}
	if !hasIndexFields(VideoTaskItem{}.Indexes(), []string{"status", "next_action_at"}, false) {
		t.Fatal("video_task_items must index due work")
	}
	if !hasIndexFields(VideoProviderCallbackEvent{}.Indexes(), []string{"model_account_id", "provider_event_id"}, true) {
		t.Fatal("video provider callback events must deduplicate provider event ids per account")
	}
	if !hasIndexFields(MediaDerivative{}.Indexes(), []string{"asset_id", "kind", "transform_version"}, true) {
		t.Fatal("media_derivatives must have an idempotent transform key")
	}
	if !hasIndexFields(CreativeCanvasRevision{}.Indexes(), []string{"canvas_id", "revision"}, true) {
		t.Fatal("creative canvas revisions must be unique per revision")
	}
}

func requireFieldDescriptor(t *testing.T, fields map[string]*field.Descriptor, name string) *field.Descriptor {
	t.Helper()
	fieldDescriptor, ok := fields[name]
	if !ok {
		t.Fatalf("schema field %s is missing", name)
	}
	return fieldDescriptor
}
