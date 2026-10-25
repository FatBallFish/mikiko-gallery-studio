package openplatform

import (
	"encoding/json"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"testing"
)

func TestNativeImageInputsAffectIdempotencyAndMCP(t *testing.T) {
	no := false
	yes := true
	input := ImageInput{RouteModelCode: "native", Prompt: "fixture", Operation: nativeimage.OperationGenerate, NativeOptions: &nativeimage.Options{Watermark: &no}}
	before := imageInputFingerprint(input)
	input.NativeOptions = &nativeimage.Options{Watermark: &yes}
	if imageInputFingerprint(input) == before {
		t.Fatal("native option omitted from idempotency")
	}
	input.Operation = nativeimage.OperationLayerDecomposition
	if imageInputFingerprint(input) == before {
		t.Fatal("operation omitted from idempotency")
	}
	found := false
	for _, spec := range Specs() {
		if spec.Name == "get_image_task_layers" {
			found = true
			if spec.Scope != "images:read" || !spec.ReadOnly {
				t.Fatalf("unsafe layer tool: %#v", spec)
			}
		}
		if spec.Name == "create_image_task" {
			raw, _ := json.Marshal(spec.InputSchema)
			var schema map[string]any
			_ = json.Unmarshal(raw, &schema)
			props := schema["properties"].(map[string]any)
			if props["operation"] == nil || props["native_options"] == nil {
				t.Fatal("MCP schema lost native fields")
			}
		}
	}
	if !found {
		t.Fatal("missing layer manifest tool")
	}
}
