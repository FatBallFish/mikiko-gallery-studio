package openplatform

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	group "github.com/fatballfish/pic-gallery/internal/domain/assetgroup"
	billing "github.com/fatballfish/pic-gallery/internal/domain/billing"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	project "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// These envelopes mirror the map/anonymous-struct returns in Execute. Resource
// bodies below use their actual JSON types, including hidden and optional fields.
type currentUserOutput struct {
	ID        int64  `json:"id"`
	Email     string `json:"email"`
	Nickname  string `json:"nickname"`
	Status    string `json:"status"`
	GroupCode string `json:"group_code"`
}
type imagePreflightOutput struct {
	billing.EstimateResult
	QuoteToken string    `json:"quote_token"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type videoCreateOutput struct {
	Task     videotask.Task `json:"task"`
	Replayed bool           `json:"replayed"`
}
type imageTaskPageOutput struct {
	Items      []image.Task `json:"items"`
	NextCursor string       `json:"next_cursor"`
}
type itemsOutput[T any] struct {
	Items []T `json:"items"`
}
type imageLayerAccessInput struct {
	TaskID  string `json:"task_id"`
	ImageID string `json:"image_id"`
	Purpose string `json:"purpose"`
}
type imageLayerOutput struct {
	ImageID         string                     `json:"image_id"`
	Width           int                        `json:"width"`
	Height          int                        `json:"height"`
	Format          string                     `json:"format"`
	Layer           *nativeimage.LayerMetadata `json:"layer"`
	AccessOperation string                     `json:"access_operation"`
	AccessInput     imageLayerAccessInput      `json:"access_input"`
}
type imageLayersOutput struct {
	Version   int                   `json:"version"`
	TaskID    string                `json:"task_id"`
	Operation nativeimage.Operation `json:"operation"`
	Layers    []imageLayerOutput    `json:"layers"`
}

// jsonTypeSchema follows encoding/json, rather than Go's underlying kind, for
// UUIDs, raw JSON and decimal. Amounts already exposed as strings stay strings;
// decimal.Decimal can legally marshal as either string or number. Dynamic maps
// and raw JSON deliberately retain their arbitrary keys/values.
func jsonTypeSchema(t reflect.Type) map[string]any {
	overrides := map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[uuid.UUID]():       {Type: "string", Format: "uuid"},
		reflect.TypeFor[decimal.Decimal](): {Types: []string{"string", "number"}},
		reflect.TypeFor[json.RawMessage](): {},
	}
	// jsonschema-go v0.4 infers slices as nullable but maps as object-only.
	// encoding/json marshals every nil map as null, including nested snapshots.
	var collectMaps func(reflect.Type)
	seen := map[reflect.Type]bool{}
	collectMaps = func(typ reflect.Type) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		if _, ok := overrides[typ]; ok {
			return
		}
		switch typ.Kind() {
		case reflect.Map:
			collectMaps(typ.Elem())
			value, err := jsonschema.ForType(typ.Elem(), &jsonschema.ForOptions{TypeSchemas: overrides})
			if err != nil {
				panic(err)
			}
			overrides[typ] = &jsonschema.Schema{Types: []string{"object", "null"}, AdditionalProperties: value}
		case reflect.Pointer, reflect.Slice, reflect.Array:
			collectMaps(typ.Elem())
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				if f.IsExported() && f.Tag.Get("json") != "-" {
					collectMaps(f.Type)
				}
			}
		}
	}
	collectMaps(t)
	schema, err := jsonschema.ForType(t, &jsonschema.ForOptions{TypeSchemas: overrides})
	if err != nil {
		panic(fmt.Sprintf("openplatform schema for %v: %v", t, err))
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	if string(raw) == "true" {
		return map[string]any{}
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		panic(err)
	}
	return result
}
func outputSchema[T any]() map[string]any { return jsonTypeSchema(reflect.TypeFor[T]()) }

// OutputSchemas exports the native operation response schemas, keyed by the
// public operation name. Each call owns its maps. OpenAPI should use these as
// response data schemas; MCP Catalogue adds its existing {data: ...} envelope.
func OutputSchemas() map[string]map[string]any {
	result := map[string]map[string]any{
		"get_current_user":      outputSchema[currentUserOutput](),
		"get_balance":           outputSchema[billing.BalanceSummary](),
		"list_image_models":     outputSchema[capabilities.Response](),
		"list_video_models":     outputSchema[videorouting.CapabilityListResponse](),
		"list_image_tasks":      outputSchema[imageTaskPageOutput](),
		"get_image_task":        outputSchema[image.Task](),
		"create_image_task":     outputSchema[image.Task](),
		"preflight_image_task":  outputSchema[imagePreflightOutput](),
		"get_image_task_layers": outputSchema[imageLayersOutput](),
		"list_video_tasks":      outputSchema[videotask.Page](),
		"get_video_task":        outputSchema[videotask.Task](),
		"cancel_video_task":     outputSchema[videotask.Task](),
		"create_video_task":     outputSchema[videoCreateOutput](),
		"preflight_video_task":  outputSchema[videotask.Estimate](),
		"list_assets":           outputSchema[mediaasset.AssetPage](),
		"list_projects":         outputSchema[itemsOutput[project.Project]](),
		"list_groups":           outputSchema[itemsOutput[group.Group]](),
		"delete_project":        outputSchema[project.DeleteResult](),
		"delete_group": outputSchema[struct {
			Deleted bool `json:"deleted"`
		}](),
		"abort_upload": outputSchema[struct {
			Aborted bool `json:"aborted"`
		}](),
		"get_upload_part_url": outputSchema[storage.MultipartPartTarget](),
	}
	for _, op := range []string{"get_asset", "update_asset", "delete_asset", "complete_upload"} {
		result[op] = outputSchema[mediaasset.Asset]()
	}
	for _, op := range []string{"get_project", "create_project", "update_project"} {
		result[op] = outputSchema[project.Project]()
	}
	for _, op := range []string{"get_group", "create_group", "update_group"} {
		result[op] = outputSchema[group.Group]()
	}
	for _, op := range []string{"create_upload", "get_upload"} {
		result[op] = outputSchema[mediaasset.UploadSession]()
	}
	for _, op := range []string{"get_asset_access", "get_image_result_access", "get_video_result_access"} {
		result[op] = outputSchema[mediaasset.AccessResult]()
	}
	return result
}
