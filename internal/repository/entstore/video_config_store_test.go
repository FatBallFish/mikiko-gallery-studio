package entstore

import (
	"reflect"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
)

func TestVideoConfigStoreRouteVisibilityUsesEnabledUserGroups(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:video-route-visibility-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("group-video").SetName("Group video").SetMediaType("video").SetVisibility("groups").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := client.UserGroup.Create().SetGroupCode("studio").SetGroupName("Studio").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := client.UserGroup.Create().SetGroupCode("legacy").SetGroupName("Legacy").SetStatus("disabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, groupID := range []int64{int64(enabled.ID), int64(disabled.ID)} {
		if _, err := client.RouteModelVisibilityGroup.Create().SetRouteModelID(int64(route.ID)).SetGroupID(groupID).Save(ctx); err != nil {
			t.Fatal(err)
		}
	}
	store := NewVideoConfigStore(client)

	tests := []struct {
		name       string
		visibility string
		codes      []string
		want       bool
	}{
		{name: "public", visibility: "public", want: true},
		{name: "hidden", visibility: "hidden", codes: []string{"studio"}, want: false},
		{name: "matching enabled group", visibility: "groups", codes: []string{" STUDIO ", "studio"}, want: true},
		{name: "different group", visibility: "groups", codes: []string{"other"}, want: false},
		{name: "disabled group", visibility: "groups", codes: []string{"legacy"}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := store.videoRouteVisible(ctx, int64(route.ID), test.visibility, test.codes)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("visible = %t, want %t", got, test.want)
			}
		})
	}
}

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
