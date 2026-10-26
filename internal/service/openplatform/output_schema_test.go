package openplatform

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	group "github.com/fatballfish/pic-gallery/internal/domain/assetgroup"
	auth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	billing "github.com/fatballfish/pic-gallery/internal/domain/billing"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	project "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func resolveContract(t *testing.T, schema map[string]any) *jsonschema.Resolved {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
func validateContract(t *testing.T, schema map[string]any, sample any) {
	t.Helper()
	raw, err := json.Marshal(sample)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if err := resolveContract(t, schema).Validate(value); err != nil {
		t.Fatalf("sample rejected: %v\n%s", err, raw)
	}
}

func TestOutputContractsCoverActualJSONTypes(t *testing.T) {
	samples := map[string]any{
		"get_current_user": currentUserOutput{}, "get_balance": billing.BalanceSummary{},
		"list_image_models": capabilities.Response{}, "list_video_models": videorouting.CapabilityListResponse{},
		"list_image_tasks": imageTaskPageOutput{}, "get_image_task": image.Task{}, "create_image_task": image.Task{},
		"preflight_image_task": imagePreflightOutput{}, "get_image_task_layers": imageLayersOutput{},
		"list_video_tasks": videotask.Page{}, "get_video_task": videotask.Task{}, "cancel_video_task": videotask.Task{},
		"create_video_task": videoCreateOutput{}, "preflight_video_task": videotask.Estimate{},
		"list_assets": mediaasset.AssetPage{}, "list_projects": itemsOutput[project.Project]{}, "list_groups": itemsOutput[group.Group]{},
		"delete_project": project.DeleteResult{}, "delete_group": map[string]any{"deleted": true}, "abort_upload": map[string]any{"aborted": true},
		"get_upload_part_url": storage.MultipartPartTarget{},
	}
	for _, op := range []string{"get_asset", "update_asset", "delete_asset", "complete_upload"} {
		samples[op] = mediaasset.Asset{}
	}
	for _, op := range []string{"get_project", "create_project", "update_project"} {
		samples[op] = project.Project{}
	}
	for _, op := range []string{"get_group", "create_group", "update_group"} {
		samples[op] = group.Group{}
	}
	for _, op := range []string{"get_upload", "create_upload"} {
		samples[op] = mediaasset.UploadSession{}
	}
	for _, op := range []string{"get_asset_access", "get_image_result_access", "get_video_result_access"} {
		samples[op] = mediaasset.AccessResult{}
	}
	schemas := OutputSchemas()
	if len(samples) != len(Specs()) || len(schemas) != len(samples) {
		t.Fatalf("incomplete output coverage: samples=%d specs=%d schemas=%d", len(samples), len(Specs()), len(schemas))
	}
	for _, spec := range Specs() {
		t.Run(spec.Name, func(t *testing.T) {
			resolveContract(t, spec.InputSchema)
			if len(spec.OutputSchema["properties"].(map[string]any)) == 0 {
				t.Fatal("opaque object schema")
			}
			sample, ok := samples[spec.Name]
			if !ok {
				t.Fatal("missing sample")
			}
			validateContract(t, spec.OutputSchema, sample)
			// Fill optional fields and nested items too, catching custom JSON types,
			// maps, hidden fields and arrays that empty async responses don't exercise.
			populated := reflect.ValueOf(sample)
			if populated.Kind() != reflect.Map {
				populated = populateJSONSample(reflect.TypeOf(sample))
			}
			validateContract(t, spec.OutputSchema, populated.Interface())
			// Exercise the exact structured-output envelope used by MCP Catalogue.
			wrapped := objectSchema(map[string]any{"data": spec.OutputSchema}, "data")
			validateContract(t, wrapped, map[string]any{"data": populated.Interface()})
		})
	}
}

func populateJSONSample(typ reflect.Type) reflect.Value {
	switch typ {
	case reflect.TypeFor[time.Time]():
		return reflect.ValueOf(time.Date(2026, 10, 27, 1, 2, 3, 0, time.UTC))
	case reflect.TypeFor[uuid.UUID]():
		return reflect.ValueOf(uuid.MustParse("4fe131a5-6108-498b-a58c-a4aec9b306bb"))
	case reflect.TypeFor[decimal.Decimal]():
		return reflect.ValueOf(decimal.RequireFromString("0.0123456789"))
	case reflect.TypeFor[json.RawMessage]():
		return reflect.ValueOf(json.RawMessage(`{"arbitrary":[1,null,"value"]}`))
	}
	v := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.Pointer:
		v.Set(reflect.New(typ.Elem()))
		v.Elem().Set(populateJSONSample(typ.Elem()))
	case reflect.Struct:
		for i := 0; i < typ.NumField(); i++ {
			if v.Field(i).CanSet() {
				v.Field(i).Set(populateJSONSample(typ.Field(i).Type))
			}
		}
	case reflect.Slice:
		v.Set(reflect.MakeSlice(typ, 1, 1))
		v.Index(0).Set(populateJSONSample(typ.Elem()))
	case reflect.Array:
		for i := 0; i < typ.Len(); i++ {
			v.Index(i).Set(populateJSONSample(typ.Elem()))
		}
	case reflect.Map:
		v.Set(reflect.MakeMap(typ))
		key := reflect.New(typ.Key()).Elem()
		key.SetString("future_field")
		v.SetMapIndex(key, populateJSONSample(typ.Elem()))
	case reflect.Interface:
		v.Set(reflect.ValueOf(map[string]any{"future": []any{nil, "value", 1.25}}))
	case reflect.String:
		v.SetString("fixture")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(0.125)
	}
	return v
}

func TestOutputContractRejectsWrongFieldsAndPreservesNullable(t *testing.T) {
	schemas := OutputSchemas()
	access := schemas["get_asset_access"]
	if resolveContract(t, access).Validate(map[string]any{"url": "https://example.test"}) == nil {
		t.Fatal("incomplete access accepted")
	}
	asset := schemas["get_asset"]["properties"].(map[string]any)
	if asset["group_id"] == nil || asset["object_key"] != nil || asset["storage_config_id"] != nil {
		t.Fatal("hidden/nullable fields drifted")
	}
	imagePreflight := schemas["preflight_image_task"]["properties"].(map[string]any)
	validateContract(t, imagePreflight["resolved_size"].(map[string]any), nil)
	if imagePreflight["pricing_snapshot"] != nil {
		t.Fatal("hidden pricing exposed")
	}
	if imagePreflight["estimated_points"].(map[string]any)["type"] != "string" {
		t.Fatal("decimal amount lost precision")
	}
	validateContract(t, jsonTypeSchema(reflect.TypeFor[decimal.Decimal]()), decimal.RequireFromString("0.123456789"))
	validateContract(t, jsonTypeSchema(reflect.TypeFor[json.RawMessage]()), json.RawMessage(`[1,null,{"future":true}]`))
}

func TestActualExecuteLayerManifestAndAccountMatchContracts(t *testing.T) {
	ctx := context.Background()
	store := imagetask.NewMemoryStore()
	task := image.Task{ID: "layer-task", UserID: 7, Operation: nativeimage.OperationLayerDecomposition, Results: []provider.ImageResult{{ID: "empty-layer", Width: 1024, Height: 1024}, {ID: "base", Layer: &nativeimage.LayerMetadata{IsBase: true, BoundingBox: &nativeimage.BoundingBox{Absolute: [4]int{0, 0, 1024, 1024}}}}}}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	service := New(Dependencies{Auth: &testUsers{auth.User{ID: 7, Status: "active"}}, Images: imagetask.NewServiceWithStore(config.Config{}, store)})
	for _, tc := range []struct {
		op    string
		input string
		scope string
	}{{"get_current_user", `{}`, "account:read"}, {"get_image_task", `{"id":"layer-task"}`, "images:read"}, {"get_image_task_layers", `{"id":"layer-task"}`, "images:read"}} {
		result, err := service.Execute(ctx, Principal{UserID: 7, Scopes: []string{tc.scope}}, tc.op, json.RawMessage(tc.input))
		if err != nil {
			t.Fatal(err)
		}
		validateContract(t, OutputSchemas()[tc.op], result)
	}
}

func TestInputContractConditionalQuoteAndUploadParts(t *testing.T) {
	specs := map[string]OperationSpec{}
	for _, spec := range Specs() {
		specs[spec.Name] = spec
	}
	// Replay remains legal without a current quote; preflight has no idempotency requirement.
	validateContract(t, specs["create_image_task"].InputSchema, map[string]any{"project_id": "project", "route_model_code": "model", "idempotency_key": "retry", "seed": nil, "native_options": nil})
	validateContract(t, specs["preflight_image_task"].InputSchema, map[string]any{"project_id": "project", "route_model_code": "model"})
	validateContract(t, specs["complete_upload"].InputSchema, map[string]any{"id": uuid.NewString(), "parts": []storage.CompletedPart{{PartNumber: 1, ETag: "etag", Checksum: "hash", SizeBytes: 1024}}})
	for _, tc := range []struct {
		op    string
		input any
	}{
		{"complete_upload", map[string]any{"id": "id", "parts": []any{map[string]any{"part_number": 1}}}},
		{"get_upload_part_url", map[string]any{"id": "id", "part_number": 0}},
		{"update_asset", map[string]any{"id": "id"}},
		{"create_image_task", map[string]any{"project_id": "project", "route_model_code": "model", "idempotency_key": ""}},
	} {
		if resolveContract(t, specs[tc.op].InputSchema).Validate(tc.input) == nil {
			t.Fatalf("%s accepted invalid input %#v", tc.op, tc.input)
		}
	}
	for _, visibility := range []string{"", "all", "private", "pending_review", "reviewing", "public", "approved", "rejected", "offline", "unpublished", " PUBLIC "} {
		validateContract(t, specs["list_assets"].InputSchema, map[string]any{"visibility_status": visibility})
	}
	validateContract(t, specs["list_groups"].InputSchema, map[string]any{})
	validateContract(t, specs["create_group"].InputSchema, map[string]any{"name": "tag"})
	validateContract(t, specs["preflight_image_task"].InputSchema, map[string]any{"project_id": "project", "route_model_code": "model", "prompt_variables": nil, "reference_asset_ids": nil, "native_options": map[string]any{"seed": nil, "watermark": nil, "qwen": nil, "annotations": nil}})
	validateContract(t, specs["list_image_tasks"].InputSchema, map[string]any{"limit": 0})
	validateContract(t, specs["list_image_tasks"].InputSchema, map[string]any{"limit": 101})
	validateContract(t, specs["create_image_task"].InputSchema, ImageInput{ProjectID: "project", RouteModelCode: "model", IdempotencyKey: "replay", Operation: nativeimage.OperationGenerate})
	first := Specs()
	first[0].OutputSchema["properties"].(map[string]any)["injected"] = map[string]any{}
	if Specs()[0].OutputSchema["properties"].(map[string]any)["injected"] != nil {
		t.Fatal("schema maps shared across calls")
	}
}
