package openplatform

import "strings"

func stringProperty() map[string]any { return map[string]any{"type": "string"} }
func objectSchema(properties map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}
func Specs() []OperationSpec {
	specs := []OperationSpec{}
	add := func(name, scope string, props map[string]any, required ...string) {
		read := strings.HasPrefix(name, "get_") || strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "preflight_")
		specs = append(specs, OperationSpec{Name: name, Description: strings.ReplaceAll(name, "_", " "), Scope: scope, InputSchema: objectSchema(props, required...), OutputSchema: map[string]any{"type": "object"}, ReadOnly: read, Destructive: strings.HasPrefix(name, "delete_") || strings.HasPrefix(name, "abort_") || strings.HasPrefix(name, "cancel_"), Idempotent: read || name == "create_image_task" || name == "create_video_task" || name == "create_upload" || name == "create_project"})
	}
	for _, name := range []string{"get_current_user", "get_balance"} {
		add(name, "account:read", map[string]any{})
	}
	for _, kind := range []string{"image", "video"} {
		add("list_"+kind+"_models", "models:read", map[string]any{})
		plural := kind + "s"
		add("list_"+kind+"_tasks", plural+":read", map[string]any{"project_id": stringProperty(), "cursor": stringProperty(), "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}, "status": stringProperty()})
		add("get_"+kind+"_task", plural+":read", map[string]any{"id": stringProperty()}, "id")
		props := taskProperties(kind)
		add("preflight_"+kind+"_task", plural+":write", props, "project_id", "route_model_code")
		add("create_"+kind+"_task", plural+":write", props, "project_id", "route_model_code", "idempotency_key")
	}
	add("get_image_result_access", "images:read", map[string]any{"task_id": stringProperty(), "image_id": stringProperty(), "purpose": stringProperty()}, "task_id", "image_id")
	add("get_video_result_access", "videos:read", map[string]any{"task_id": stringProperty(), "asset_id": stringProperty(), "purpose": stringProperty()}, "task_id", "asset_id")
	add("cancel_video_task", "videos:delete", map[string]any{"id": stringProperty(), "idempotency_key": stringProperty()}, "id", "idempotency_key")
	for _, kind := range []string{"asset", "project", "group", "upload"} {
		plural := kind + "s"
		props := map[string]any{"id": stringProperty(), "project_id": stringProperty(), "name": stringProperty(), "expected_version": map[string]any{"type": "integer"}, "group_name": stringProperty(), "group_id": stringProperty(), "target_project_id": stringProperty(), "idempotency_key": stringProperty()}
		add("get_"+kind, plural+":read", map[string]any{"id": stringProperty(), "project_id": stringProperty()}, "id")
		if kind != "upload" {
			add("list_"+plural, plural+":read", listProperties())
		}
		if kind != "asset" && kind != "upload" {
			add("create_"+kind, plural+":write", props, "name")
		}
		if kind != "upload" {
			add("update_"+kind, plural+":write", props, "id")
			add("delete_"+kind, plural+":delete", props, "id")
		}
	}
	add("get_asset_access", "assets:read", map[string]any{"id": stringProperty(), "purpose": stringProperty()}, "id")
	add("create_upload", "uploads:write", map[string]any{"project_id": stringProperty(), "group_name": stringProperty(), "group_id": stringProperty(), "filename": stringProperty(), "media_type": stringProperty(), "mime_type": stringProperty(), "size_bytes": map[string]any{"type": "integer"}, "checksum": stringProperty(), "idempotency_key": stringProperty()}, "project_id", "filename", "media_type", "mime_type", "size_bytes", "idempotency_key")
	add("get_upload_part_url", "uploads:write", map[string]any{"id": stringProperty(), "part_number": map[string]any{"type": "integer"}, "checksum": stringProperty()}, "id", "part_number")
	add("complete_upload", "uploads:write", map[string]any{"id": stringProperty(), "parts": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}}, "id", "parts")
	add("abort_upload", "uploads:delete", map[string]any{"id": stringProperty()}, "id")
	for i := range specs {
		if strings.HasPrefix(specs[i].Name, "list_") && specs[i].Name != "list_image_models" && specs[i].Name != "list_video_models" {
			specs[i].OutputSchema = objectSchema(map[string]any{"items": map[string]any{"type": "array", "items": map[string]any{"type": "object"}}, "next_cursor": stringProperty()}, "items")
		}
	}
	return specs
}
func (s *Service) Specs() []OperationSpec { return Specs() }
func listProperties() map[string]any {
	p := map[string]any{}
	for _, k := range []string{"project_id", "media_type", "source_type", "group_name", "status", "keyword", "sort_by", "sort_order", "cursor"} {
		p[k] = stringProperty()
	}
	p["limit"] = map[string]any{"type": "integer"}
	return p
}
func taskProperties(kind string) map[string]any {
	p := map[string]any{}
	keys := []string{"project_id", "route_model_code", "task_type", "quote_token", "idempotency_key"}
	if kind == "image" {
		keys = append(keys, "abstract_model", "prompt", "negative_prompt", "size_mode", "requested_size", "base_resolution", "quality", "output_format", "background", "moderation", "aspect_ratio", "capability_version", "mask_asset_id")
	} else {
		keys = append(keys, "prompt_template", "resolution", "aspect_ratio", "audio_mode")
	}
	for _, k := range keys {
		p[k] = stringProperty()
	}
	ints := []string{"output_image_count", "output_compression", "reference_strength", "seed"}
	if kind == "video" {
		ints = []string{"duration_seconds", "output_count"}
	}
	for _, k := range ints {
		p[k] = map[string]any{"type": "integer"}
	}
	p["prompt_variables"] = map[string]any{"type": "array", "items": objectSchema(map[string]any{"name": stringProperty(), "value": stringProperty()}, "name", "value")}
	p["reference_bindings"] = map[string]any{"type": "array", "items": objectSchema(map[string]any{"name": stringProperty(), "asset_id": stringProperty()}, "name", "asset_id")}
	if kind == "image" {
		p["reference_asset_ids"] = map[string]any{"type": "array", "items": stringProperty()}
	} else {
		p["inputs"] = map[string]any{"type": "array", "items": objectSchema(map[string]any{"asset_id": stringProperty(), "role": stringProperty(), "ordinal": map[string]any{"type": "integer"}}, "asset_id", "role", "ordinal")}
	}
	return p
}
