package compat

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
)

func TestLifecycleURLRenewsAccessWithoutGeneration(t *testing.T) {
	s := NewService(config.Config{})
	calls := 0
	s.SetOperations(operationFunc(func(_ context.Context, _ openplatform.Principal, op string, raw json.RawMessage) (any, error) {
		calls++
		if op != "get_image_result_access" {
			t.Fatal(op)
		}
		var input map[string]string
		_ = json.Unmarshal(raw, &input)
		if input["image_id"] != "image" || input["purpose"] != "download" {
			t.Fatal(input)
		}
		return map[string]string{"url": "https://studio.example/capability"}, nil
	}))
	result, err := s.Result(context.Background(), openplatform.Principal{}, domainimagetask.Task{Results: []provider.ImageResult{{ID: "image", MediaAssetID: "asset", URL: "/api/agent/private"}}}, "url")
	if err != nil || calls != 1 || result.Data[0].URL != "https://studio.example/capability" {
		t.Fatalf("%+v %v calls=%d", result, err, calls)
	}
}
