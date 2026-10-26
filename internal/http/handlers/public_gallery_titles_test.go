package handlers

import (
	"encoding/json"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"testing"
)

func TestPublicGalleryProjectionRetainsCanonicalTitle(t *testing.T) {
	image := domainimagetask.GalleryImage{ID: "2335d87b", MediaAssetID: "canonical-image", Name: "狗狗猫猫睡觉", Prompt: "public prompt", UserID: 88, ProjectID: "private-project"}
	for name, item := range map[string]publicGalleryListImage{"list": publicGalleryListItems([]domainimagetask.GalleryImage{image})[0], "detail": publicGalleryDetailItem(image)} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(item)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if fields["name"] != image.Name || fields["media_asset_id"] != image.MediaAssetID {
				t.Fatalf("canonical public title lost: %s", raw)
			}
			if _, ok := fields["user_id"]; ok {
				t.Fatalf("private owner exposed: %s", raw)
			}
			if _, ok := fields["project_id"]; ok {
				t.Fatalf("private project exposed: %s", raw)
			}
		})
	}
}
