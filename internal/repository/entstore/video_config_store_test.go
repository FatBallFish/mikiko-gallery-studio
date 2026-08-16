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

func TestVideoRouteMissingPriceRequiresEveryCandidateRateCard(t *testing.T) {
	route := adminvideoservice.RouteConfigSummary{CandidateAccountModelIDs: []int64{21, 22}}
	cards := []adminvideoservice.RateCardSummary{{AccountModelID: 21, Enabled: true}}
	if !videoRouteHasMissingPrice(route, cards) {
		t.Fatal("missing candidate rate card must block readiness")
	}
	cards = append(cards, adminvideoservice.RateCardSummary{AccountModelID: 22, Enabled: true})
	if videoRouteHasMissingPrice(route, cards) {
		t.Fatal("all candidate rate cards should satisfy readiness")
	}
}
