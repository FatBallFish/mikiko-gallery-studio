package entstore

import (
	"reflect"
	"testing"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
)

func TestDeriveVideoGroupTaskTypesUsesCandidateCapabilityUnion(t *testing.T) {
	candidates := []videoroutingservice.Candidate{
		{Capability: domainvideo.Capability{TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
			domainvideo.TaskTypeImageToVideo: {},
			domainvideo.TaskTypeTextToVideo:  {},
		}}},
		{Capability: domainvideo.Capability{TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
			domainvideo.TaskTypeTextToVideo: {},
		}}},
	}
	want := []domainvideo.TaskType{domainvideo.TaskTypeImageToVideo, domainvideo.TaskTypeTextToVideo}
	if got := deriveVideoGroupTaskTypes(candidates); !reflect.DeepEqual(got, want) {
		t.Fatalf("task types = %#v, want %#v", got, want)
	}
}

func TestVideoResolutionMappingsDecodePerCandidate(t *testing.T) {
	mappings := videoResolutionMappings(map[string]any{
		"42": map[string]any{"resolutions": map[string]any{"720p": "768p"}},
	}, 42)
	if mappings[domainvideo.Resolution720P] != domainvideo.Resolution768P {
		t.Fatalf("resolution mappings = %#v", mappings)
	}
}
