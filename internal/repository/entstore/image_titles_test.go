package entstore

import (
	"entgo.io/ent/dialect"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
	"strings"
	"testing"
)

func TestImageTitlesDuplicateRenameAndReplay(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:title-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(88).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task := domainimagetask.Task{ID: uuid.NewString(), UserID: 88, ProjectID: project.ID.String(), Status: domainimagetask.StatusSucceeded, AbstractModel: "basic", TaskType: "text_to_image", Prompt: "prompt"}
	for i := range 2 {
		task.Results = append(task.Results, provider.ImageResult{ID: uuid.NewString(), Name: "相同标题", ObjectKey: "generated/" + uuid.NewString() + ".png", MimeType: "image/png", StorageDriver: "local", SHA256: strings.Repeat("d", 64), Width: 512 + i})
	}
	store := NewImageTaskStore(client)
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	firstID := uuid.MustParse(task.Results[0].ID)
	if err := client.MediaAsset.UpdateOneID(firstID).SetName("historical renamed.png").SetNameKey("historical renamed.png").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetByID(ctx, 88, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, result := range got.Results {
		if result.ID == firstID.String() {
			found = true
			if result.Name != "historical renamed.png" || result.MediaAssetID != result.ID {
				t.Fatalf("%+v", result)
			}
		}
	}
	if !found {
		t.Fatal("result missing")
	}
	alias, err := client.ReferenceAsset.Create().SetUserID(88).SetName("stale").SetNameNormalized("stale").SetStorageDriver("local").SetObjectKey("alias/image.png").SetMimeType("image/png").SetSha256(strings.Repeat("a", 64)).SetMediaAssetID(firstID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := NewAssetsStore(client).GetByUserAndID(ctx, 88, alias.ID.String())
	if err != nil || ref.Name != "stale" || ref.DisplayName != "historical renamed.png" {
		t.Fatalf("ref=%+v err=%v", ref, err)
	}
	items := store.decorateNamedGalleryImages(ctx, []domainimagetask.GalleryImage{{ID: firstID.String(), ReferenceAssets: []domainimagetask.GalleryReferenceAsset{{ID: alias.ID.String()}}}})
	if items[0].ReferenceAssets[0].Name != "historical renamed.png" {
		t.Fatalf("%+v", items)
	}
	// Duplicate display titles containing template syntax must never become bindings.
	secondID := uuid.MustParse(task.Results[1].ID)
	for _, id := range []uuid.UUID{firstID, secondID} {
		if err := client.MediaAsset.UpdateOneID(id).SetName("{{duplicate}}").SetNameKey("{{duplicate}}").Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	otherAlias, err := client.ReferenceAsset.Create().SetUserID(88).SetName("stable2").SetNameNormalized("stable2").SetStorageDriver("local").SetObjectKey("alias/other.png").SetMimeType("image/png").SetSha256(strings.Repeat("b", 64)).SetMediaAssetID(secondID).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := NewAssetsStore(client).GetManyByUserAndIDs(ctx, 88, []string{alias.ID.String(), otherAlias.ID.String()})
	if err != nil || len(refs) != 2 {
		t.Fatalf("refs=%+v err=%v", refs, err)
	}
	if refs[0].Name != "stale" || refs[1].Name != "stable2" || refs[0].DisplayName != "{{duplicate}}" || refs[1].DisplayName != "{{duplicate}}" {
		t.Fatalf("binding/display separation: %+v", refs)
	}
	if _, err := domainassets.NormalizeReferenceName(refs[0].Name); err != nil {
		t.Fatal(err)
	}
	if _, err := domainassets.NormalizeReferenceName(refs[1].Name); err != nil {
		t.Fatal(err)
	}
	imported, err := NewAssetsStore(client).ImportGalleryAlias(ctx, 88, task.Results[0])
	if err != nil || imported.DisplayName != "{{duplicate}}" {
		t.Fatalf("initial gallery import=%+v err=%v", imported, err)
	}
	if _, err := domainassets.NormalizeReferenceName(imported.Name); err != nil {
		t.Fatal(err)
	}
	thirdID := uuid.New()
	canonical, err := client.MediaAsset.Create().SetID(thirdID).SetUserID(88).SetProjectID(project.ID).SetName("{{duplicate}}").SetNameKey("{{duplicate}}").SetMediaType("image").SetSourceType("local_upload").SetStorageDriver("local").SetObjectKey("media/third.png").SetMimeType("image/png").SetFileSizeBytes(10).SetSha256(strings.Repeat("c", 64)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := NewAssetsStore(client).ImportMediaAssetAlias(ctx, 88, mapMediaAsset(canonical))
	if err != nil || initial.DisplayName != "{{duplicate}}" {
		t.Fatalf("initial media import=%+v err=%v", initial, err)
	}
	if _, err := domainassets.NormalizeReferenceName(initial.Name); err != nil {
		t.Fatal(err)
	}
	gallery, err := store.GetOwnedGalleryImage(ctx, 88, firstID.String())
	if err != nil {
		t.Fatal(err)
	}
	if gallery.Name != "{{duplicate}}" || gallery.MediaAssetID != firstID.String() {
		t.Fatalf("%+v", gallery)
	}
}
