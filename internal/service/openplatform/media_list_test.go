package openplatform

import (
	"context"
	"encoding/json"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"testing"
)

type assetListProbe struct {
	mediaasset.Store
	request mediaasset.AssetListRequest
}

func (s *assetListProbe) ListAssets(_ context.Context, req mediaasset.AssetListRequest) (mediaasset.AssetPage, error) {
	s.request = req
	return mediaasset.AssetPage{Items: []mediaasset.Asset{}}, nil
}
func TestListAssetsForwardsVisibilityToMediaService(t *testing.T) {
	store := &assetListProbe{}
	service := New(Dependencies{Assets: mediaasset.NewService(store, nil, mediaasset.Options{})})
	_, err := service.mediaOperation(t.Context(), 42, "list_assets", json.RawMessage(`{"visibility_status":"approved","status":"ready","limit":1,"cursor":"cursor"}`))
	if err != nil {
		t.Fatal(err)
	}
	if store.request.VisibilityStatus != "public" || store.request.Status != "ready" || store.request.Limit != 1 || store.request.Cursor != "cursor" || store.request.UserID != 42 {
		t.Fatalf("request = %+v", store.request)
	}
	if _, err := service.mediaOperation(t.Context(), 42, "list_assets", json.RawMessage(`{"visibility_status":"invalid"}`)); err == nil {
		t.Fatal("invalid visibility accepted")
	}
}
