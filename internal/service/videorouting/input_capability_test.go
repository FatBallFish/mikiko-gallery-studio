package videorouting

import (
	"context"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"testing"
)

type inputCapabilityStore struct{ group Group }

func (s inputCapabilityStore) GetVideoGroup(context.Context, string, []string) (Group, error) {
	return s.group, nil
}
func (s inputCapabilityStore) ListVideoGroups(context.Context, []string) ([]Group, error) {
	return []Group{s.group}, nil
}

func TestPublicInputCapabilitiesUseRoutableCandidateConstraints(t *testing.T) {
	candidate := func(id int64, count int, bytes int64, formats []string) Candidate {
		return Candidate{AccountModelID: id, Capability: domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
			domainvideo.TaskTypeImageToVideo: {Durations: domainvideo.IntValues{Values: []int{5}}, Resolutions: []domainvideo.Resolution{domainvideo.Resolution720P}, AspectRatios: []domainvideo.AspectRatio{"16:9"}, AudioModes: []domainvideo.AudioMode{"silent"}, Inputs: map[domainvideo.InputRole]domainvideo.InputCapability{
				domainvideo.InputRoleFirstFrame: {Required: true, MaxCount: count, MaxBytes: bytes, MediaTypes: []string{"image"}, Formats: formats},
			}},
		}}}
	}
	group := Group{Code: "video", MaxOutputCount: 1, TaskTypes: []domainvideo.TaskType{domainvideo.TaskTypeImageToVideo}, Candidates: []Candidate{candidate(1, 4, 1000, []string{"jpg", "png"}), candidate(2, 2, 500, []string{"png", "webp"})}}
	response, err := NewService(inputCapabilityStore{group}).Capabilities(context.Background(), "video", nil)
	if err != nil {
		t.Fatal(err)
	}
	input := response.InputCapabilitiesByTaskType[domainvideo.TaskTypeImageToVideo][domainvideo.InputRoleFirstFrame]
	if input.MaxCount != 2 || input.MaxBytes != 500 || !input.Required || len(input.Formats) != 1 || input.Formats[0] != "png" || len(input.MediaTypes) != 1 || input.MediaTypes[0] != "image" {
		t.Fatalf("unexpected projection: %#v", input)
	}
	if len(response.Combinations) != 1 {
		t.Fatalf("combination projection changed: %#v", response.Combinations)
	}
	// A candidate with no public resolution cannot contribute guessed zero limits.
	broken := candidate(3, 0, 1, []string{"gif"})
	task := broken.Capability.TaskTypes[domainvideo.TaskTypeImageToVideo]
	task.Resolutions = nil
	broken.Capability.TaskTypes[domainvideo.TaskTypeImageToVideo] = task
	group.Candidates = append(group.Candidates, broken)
	response, err = NewService(inputCapabilityStore{group}).Capabilities(context.Background(), "video", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.InputCapabilitiesByTaskType[domainvideo.TaskTypeImageToVideo][domainvideo.InputRoleFirstFrame].MaxCount != 2 {
		t.Fatal("unroutable candidate altered public input count")
	}
}
