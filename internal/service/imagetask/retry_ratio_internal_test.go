package imagetask

import (
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"testing"
)

func TestOpenPlatformRatioRetryDropsDerivedPixelSize(t *testing.T) {
	original := image.Task{SizeMode: "ratio", RequestedSize: "1024x1024", BaseResolution: "1k", AspectRatio: "1:1", TaskType: "text_to_image", Prompt: "retry", OutputImageCount: 1}
	req := imageRetryRequest(original)
	if req.RequestedSize != "" || req.AspectRatio != "1:1" || req.BaseResolution != "1k" {
		t.Fatalf("retry changed ratio inputs: %#v", req)
	}
	if _, err := normalizeCreateRequest(req); err != nil {
		t.Fatalf("ratio retry invalid: %v", err)
	}
	original.SizeMode = "pixel"
	original.AspectRatio = ""
	req = imageRetryRequest(original)
	if req.RequestedSize != "1024x1024" {
		t.Fatal("pixel size lost")
	}
}
