package openplatform

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

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
		// Preflight/create and CRUD callers share property maps. Each spec must
		// own its schema before operation-specific constraints are applied.
		raw, _ := json.Marshal(props)
		_ = json.Unmarshal(raw, &props)
		read := strings.HasPrefix(name, "get_") || strings.HasPrefix(name, "list_") || strings.HasPrefix(name, "preflight_")
		specs = append(specs, OperationSpec{Name: name, Description: strings.ReplaceAll(name, "_", " "), Scope: scope, InputSchema: objectSchema(props, required...), OutputSchema: map[string]any{"type": "object"}, ReadOnly: read, Destructive: strings.HasPrefix(name, "delete_") || strings.HasPrefix(name, "abort_") || strings.HasPrefix(name, "cancel_"), Idempotent: read || name == "create_image_task" || name == "create_video_task" || name == "create_upload" || name == "create_project"})
	}
	for _, name := range []string{"get_current_user", "get_balance"} {
		add(name, "account:read", map[string]any{})
	}
	for _, kind := range []string{"image", "video"} {
		add("list_"+kind+"_models", "models:read", map[string]any{})
		plural := kind + "s"
		add("list_"+kind+"_tasks", plural+":read", map[string]any{"project_id": stringProperty(), "cursor": stringProperty(), "limit": map[string]any{"type": "integer", "description": "Page size; omitted or non-positive uses the server default, values above 100 are clamped to 100."}, "status": stringProperty()})
		add("get_"+kind+"_task", plural+":read", map[string]any{"id": stringProperty()}, "id")
		props := taskProperties(kind)
		add("preflight_"+kind+"_task", plural+":write", props, "project_id", "route_model_code")
		add("create_"+kind+"_task", plural+":write", props, "project_id", "route_model_code", "idempotency_key")
	}
	add("get_image_task_layers", "images:read", map[string]any{"id": stringProperty()}, "id")
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
	outputs := OutputSchemas()
	for i := range specs {
		spec := &specs[i]
		spec.OutputSchema = outputs[spec.Name]
		if spec.OutputSchema == nil {
			panic("missing output schema: " + spec.Name)
		}
		constrainInput(spec)
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
func nativeImageOptionsSchema() map[string]any {
	nullable := func(s map[string]any) map[string]any {
		return map[string]any{"anyOf": []any{s, map[string]any{"type": "null"}}}
	}
	boolean := func() map[string]any { return nullable(map[string]any{"type": "boolean"}) }
	coords := func(n int) map[string]any {
		return map[string]any{"type": "array", "minItems": n, "maxItems": n, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": 999}}
	}
	properties := map[string]any{
		"seed": map[string]any{"type": "integer", "minimum": 0, "maximum": 2147483647}, "negative_prompt": stringProperty(), "watermark": boolean(),
		"qwen":        objectSchema(map[string]any{"prompt_extend": boolean(), "prompt_extend_mode": map[string]any{"type": "string", "enum": []string{"direct", "agent"}}, "enable_thinking": boolean()}),
		"seedream":    objectSchema(map[string]any{"optimize_prompt_mode": map[string]any{"type": "string", "enum": []string{"standard", "fast"}}}),
		"annotations": map[string]any{"type": "array", "items": objectSchema(map[string]any{"reference_index": map[string]any{"type": "integer", "minimum": 1, "maximum": 10}, "label": stringProperty(), "point": nullable(coords(2)), "bbox": nullable(coords(4))}, "reference_index")},
	}
	for _, key := range []string{"seed", "qwen", "seedream", "annotations"} {
		properties[key] = nullable(properties[key].(map[string]any))
	}
	return objectSchema(properties)
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
		p["operation"] = map[string]any{"type": "string", "enum": []string{"", "generate", "edit", "layer_decomposition"}}
		p["native_options"] = nativeImageOptionsSchema()
		p["reference_asset_ids"] = map[string]any{"type": "array", "items": stringProperty()}
	} else {
		p["inputs"] = map[string]any{"type": "array", "items": objectSchema(map[string]any{"asset_id": stringProperty(), "role": stringProperty(), "ordinal": map[string]any{"type": "integer"}}, "asset_id", "role", "ordinal")}
	}
	return p
}

// Constraints here are unconditional. Model-dependent limits come from the
// model catalogue and are checked by preflight rather than hardcoded globally.
func constrainInput(spec *OperationSpec) {
	p := spec.InputSchema["properties"].(map[string]any)
	require := func(keys ...string) { spec.InputSchema["required"] = keys }
	integer := func(min int) map[string]any { return map[string]any{"type": "integer", "minimum": min} }
	nullable := func(schema map[string]any) map[string]any {
		return map[string]any{"anyOf": []any{schema, map[string]any{"type": "null"}}}
	}
	if _, ok := p["idempotency_key"]; ok {
		p["idempotency_key"] = map[string]any{"type": "string", "maxLength": 128}
	}
	if _, ok := p["expected_version"]; ok {
		p["expected_version"] = integer(1)
	}
	if strings.HasSuffix(spec.Name, "_task") && (strings.HasPrefix(spec.Name, "create_") || strings.HasPrefix(spec.Name, "preflight_")) {
		if strings.HasPrefix(spec.Name, "create_") {
			require("project_id", "route_model_code", "idempotency_key")
			p["quote_token"] = map[string]any{"type": "string", "description": "Required for new creation; obtain from preflight. A persisted idempotent replay does not require a live quote."}
			p["idempotency_key"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
		}
		if seed, ok := p["seed"]; ok {
			p["seed"] = nullable(seed.(map[string]any))
		}
		if options, ok := p["native_options"]; ok {
			p["native_options"] = nullable(options.(map[string]any))
		}
	}
	switch spec.Name {
	case "list_assets":
		p["visibility_status"] = map[string]any{"type": "string", "description": "Visibility filter; empty/all, private, pending/pending_review/reviewing, public/approved, rejected, offline/unpublished. Case-insensitive; surrounding whitespace is ignored."}
	case "update_project":
		require("id", "name", "expected_version")
	case "delete_project":
		require("id", "expected_version")
	case "get_group":
		require("id")
	case "create_group":
		require("name")
	case "update_group":
		require("id", "name", "expected_version")
	case "delete_group":
		require("id", "expected_version")
	case "update_asset", "delete_asset":
		require("id", "expected_version")
	case "get_upload_part_url":
		p["part_number"] = integer(1)
	case "create_upload":
		p["idempotency_key"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 128}
		p["size_bytes"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 1 << 30}
	case "complete_upload":
		p["parts"] = map[string]any{"type": "array", "items": jsonTypeSchema(reflect.TypeFor[storage.CompletedPart]())}
		item := p["parts"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
		item["part_number"] = integer(1)
	}
	for key, property := range p {
		if schema, ok := property.(map[string]any); ok && schema["type"] == "array" {
			p[key] = nullable(schema)
		}
	}
	// resourceInput uses pointers; null has the same effect as omission.
	for _, key := range []string{"project_id", "group_id", "group_name", "name"} {
		if !strings.Contains(spec.Name, "_task") {
			if property, ok := p[key]; ok {
				p[key] = nullable(property.(map[string]any))
			}
		}
	}
}
