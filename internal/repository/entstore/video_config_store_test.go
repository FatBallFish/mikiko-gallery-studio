package entstore

import (
	"testing"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
)

func TestVideoResolutionMappingsDecodePerCandidate(t *testing.T) {
	mappings := videoResolutionMappings(map[string]any{
		"42": map[string]any{"resolutions": map[string]any{"720p": "768p"}},
	}, 42)
	if mappings[domainvideo.Resolution720P] != domainvideo.Resolution768P {
		t.Fatalf("resolution mappings = %#v", mappings)
	}
}
