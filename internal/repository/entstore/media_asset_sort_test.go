package entstore

import (
	"testing"
	"time"

	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/google/uuid"
)

func TestMediaAssetMatchesVisibilityAliases(t *testing.T) {
	for _, pair := range [][2]string{{"public", "approved"}, {"approved", "public"}, {"pending", "pending_review"}, {"pending", "reviewing"}, {"offline", "unpublished"}, {"rejected", "rejected"}, {"private", "private"}} {
		item := mediaassetservice.Asset{VisibilityStatus: pair[1], Status: "ready"}
		req := mediaassetservice.AssetListRequest{VisibilityStatus: pair[0], Status: "ready"}
		if !mediaAssetMatches(item, req) {
			t.Errorf("%q should match %q", pair[0], pair[1])
		}
		req.Status = "processing"
		if mediaAssetMatches(item, req) {
			t.Error("visibility must not bypass processing filter")
		}
	}
	if mediaAssetMatches(mediaassetservice.Asset{VisibilityStatus: "private"}, mediaassetservice.AssetListRequest{VisibilityStatus: "public"}) {
		t.Fatal("private asset matched public")
	}
}

func TestSortMediaAssetsAcceptsPublicFileSizeAndDurationFields(t *testing.T) {
	durationShort, durationLong := int64(1000), int64(9000)
	items := []mediaassetservice.Asset{
		{ID: uuid.MustParse("00000000-0000-4000-8000-000000000001"), Name: "large-short", FileSizeBytes: 900, DurationMS: &durationShort, CreatedAt: time.Unix(1, 0)},
		{ID: uuid.MustParse("00000000-0000-4000-8000-000000000002"), Name: "small-long", FileSizeBytes: 100, DurationMS: &durationLong, CreatedAt: time.Unix(2, 0)},
	}
	sortMediaAssets(items, "file_size_bytes", "desc")
	if items[0].Name != "large-short" {
		t.Fatalf("file_size_bytes sort order = %q first", items[0].Name)
	}
	sortMediaAssets(items, "duration_ms", "desc")
	if items[0].Name != "small-long" {
		t.Fatalf("duration_ms sort order = %q first", items[0].Name)
	}
}
