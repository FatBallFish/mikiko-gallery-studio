package modeladmin

import (
	"encoding/json"
	"testing"
)

func TestModelAccountModelJSONIncludesRemediationCapabilities(t *testing.T) {
	payload := marshalModelAdminJSON(t, ModelAccountModel{
		SupportsCustomRatio:  true,
		MinWidth:             512,
		MaxWidth:             4096,
		MinHeight:            512,
		MaxHeight:            4096,
		SupportedBackgrounds: []string{"auto", "transparent"},
	})
	for _, key := range []string{"supports_custom_ratio", "min_width", "max_width", "min_height", "max_height", "supported_backgrounds"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("ModelAccountModel JSON is missing %q: %#v", key, payload)
		}
	}
}

func TestModelAccountModelWriteRequestCarriesRemediationCapabilities(t *testing.T) {
	request := ModelAccountModelWriteRequest{
		SupportsCustomRatio:  true,
		MinWidth:             512,
		MaxWidth:             4096,
		MinHeight:            512,
		MaxHeight:            4096,
		SupportedBackgrounds: []string{"opaque"},
	}
	if !request.SupportsCustomRatio || request.MinWidth != 512 || request.MaxWidth != 4096 || request.MinHeight != 512 || request.MaxHeight != 4096 || len(request.SupportedBackgrounds) != 1 {
		t.Fatalf("write request lost remediation capabilities: %#v", request)
	}
}

func marshalModelAdminJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode %T JSON: %v", value, err)
	}
	return payload
}
