package entstore

import (
	"testing"
	"time"

	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/google/uuid"
)

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
