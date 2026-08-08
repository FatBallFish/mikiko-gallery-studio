package assets

import (
	"encoding/json"
	"testing"
)

func TestReferenceAssetJSONIncludesAliasOwnershipContract(t *testing.T) {
	asset := ReferenceAsset{SourceImageResultID: "result-1", OwnsObject: false}
	data, err := json.Marshal(asset)
	if err != nil {
		t.Fatalf("marshal ReferenceAsset: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode ReferenceAsset JSON: %v", err)
	}
	if payload["source_image_result_id"] != "result-1" {
		t.Fatalf("source_image_result_id = %#v, want result-1", payload["source_image_result_id"])
	}
	if ownsObject, ok := payload["owns_object"].(bool); !ok || ownsObject {
		t.Fatalf("owns_object = %#v, want false", payload["owns_object"])
	}
}
