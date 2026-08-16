package entstore

import (
	"testing"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	adminvideoservice "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
)

func TestVideoResolutionMappingsDecodePerCandidate(t *testing.T) {
	mappings := videoResolutionMappings(map[string]any{
		"42": map[string]any{"resolutions": map[string]any{"720p": "768p"}},
	}, 42)
	if mappings[domainvideo.Resolution720P] != domainvideo.Resolution768P {
		t.Fatalf("resolution mappings = %#v", mappings)
	}
}

func TestVideoRouteMissingPriceRequiresAtLeastOneCandidateRateCard(t *testing.T) {
	route := adminvideoservice.RouteConfigSummary{CandidateAccountModelIDs: []int64{21, 22}}
	if !videoRouteHasMissingPrice(route, nil) {
		t.Fatal("route without any candidate rate card must fail readiness")
	}
	cards := []adminvideoservice.RateCardSummary{{AccountModelID: 21, Enabled: true}}
	if videoRouteHasMissingPrice(route, cards) {
		t.Fatal("one priceable candidate should satisfy mixed-route readiness")
	}
}
