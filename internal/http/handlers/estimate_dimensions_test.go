package handlers

import (
	"bytes"
	"context"
	"github.com/fatballfish/pic-gallery/internal/config"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	"image"
	"image/png"
	"net/http/httptest"
	"testing"
)

func TestEstimateTrustedReferenceDimensions(t *testing.T) {
	svc := assetservice.NewService(config.StorageConfig{LocalRoot: t.TempDir()}, config.GenerationLimitsConfig{ReferenceImageMaxMB: 1})
	var data bytes.Buffer
	if err := png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 3, 5))); err != nil {
		t.Fatal(err)
	}
	asset, err := svc.Upload(7, "reference.png", "image/png", data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	a := &API{assets: svc}
	r := httptest.NewRequest("GET", "/estimate?reference_asset_ids="+asset.ID, nil)
	dims, count, err := a.estimateInputDimensions(context.Background(), 7, estimateReferenceIDs(r), 0, false)
	if err != nil || count != 1 || len(dims) != 1 || dims[0].Width != 3 || dims[0].Height != 5 {
		t.Fatalf("%+v %d %v", dims, count, err)
	}
	if _, _, err = a.estimateInputDimensions(context.Background(), 8, []string{asset.ID}, 1, true); err == nil {
		t.Fatal("foreign reference accepted")
	}
	if _, _, err = a.estimateInputDimensions(context.Background(), 7, []string{asset.ID}, 2, true); err == nil {
		t.Fatal("mismatching count accepted")
	}
	if _, _, err = a.estimateInputDimensions(context.Background(), 7, []string{asset.ID, asset.ID}, 2, true); err == nil {
		t.Fatal("duplicate reference accepted")
	}
}
