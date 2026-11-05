package imagetask

import (
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"testing"
)

// MCP/开放平台 native 请求可省略 size_mode；通用归一化把空 mode 填成
// legacy ratio（aspect_ratio 1:1、base_resolution auto），native 候选拒绝任何
// aspect_ratio，导致 100% 失败。显式 native 意图必须跳过这些默认值。
func TestNormalizeCreateRequestKeepsExplicitNativeRequestSparse(t *testing.T) {
	req, err := normalizeCreateRequest(image.CreateRequest{
		TaskType:         "image_edit",
		Operation:        nativeimage.OperationEdit,
		Prompt:           "edit this",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.AspectRatio != "" || req.BaseResolution != "" || req.SizeMode != "" {
		t.Fatalf("explicit native request must not inherit generic ratio defaults: %+v", req)
	}
	if req.Operation != nativeimage.OperationEdit {
		t.Fatalf("operation must survive normalization: %q", req.Operation)
	}

	// The tier combination (auto + base_resolution) must stay reachable for native routes.
	req, err = normalizeCreateRequest(image.CreateRequest{
		TaskType:         "image_edit",
		Operation:        nativeimage.OperationEdit,
		Prompt:           "edit this",
		SizeMode:         "auto",
		BaseResolution:   "1k",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatalf("native tier combination rejected by generic contract: %v", err)
	}
	if req.BaseResolution != "1k" {
		t.Fatalf("native tier base_resolution must survive: %+v", req)
	}
}

// 普通请求（无显式 operation/native_options）继续获得默认值填充。
func TestNormalizeCreateRequestStillAppliesGenericDefaults(t *testing.T) {
	req, err := normalizeCreateRequest(image.CreateRequest{
		TaskType:         "image_edit",
		Prompt:           "edit this",
		OutputImageCount: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.AspectRatio != "1:1" || req.BaseResolution != "auto" || req.Quality != "auto" || req.OutputFormat != "png" || req.Moderation != "auto" {
		t.Fatalf("generic defaults must keep applying: %+v", req)
	}
}
