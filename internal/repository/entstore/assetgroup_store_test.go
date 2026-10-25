package entstore

import (
	"testing"
	"time"

	domain "github.com/fatballfish/pic-gallery/internal/domain/assetgroup"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	groups "github.com/fatballfish/pic-gallery/internal/service/assetgroup"
	media "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/google/uuid"
)

func TestAssetGroupLifecycleAndLegacyConsistency(t *testing.T) {
	ctx, c := openProjectStoreTestClient(t, "asset-group-lifecycle")
	p, err := c.Project.Create().SetUserID(44).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	q, err := c.Project.Create().SetUserID(44).SetName("Target").SetNameKey("target").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := groups.NewService(NewAssetGroupStore(c))
	g, err := s.Create(ctx, 44, "", domain.CreateRequest{Name: "  Group  One "})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Create(ctx, 44, p.ID.String(), domain.CreateRequest{Name: "group one"}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err = s.Get(ctx, 99, p.ID.String(), g.ID); err == nil {
		t.Fatal("other owner accepted")
	}
	if _, err = s.Get(ctx, 44, q.ID.String(), g.ID); err == nil {
		t.Fatal("other project accepted")
	}
	names, err := NewMediaStore(c).ListAssetGroups(ctx, 44, &p.ID)
	if err != nil || len(names) != 1 || names[0] != "Group One" {
		t.Fatalf("empty groups: %v %v", names, err)
	}
	task, err := c.ImageTask.Create().SetUserID(44).SetProjectID(p.ID).SetTaskType("text_to_image").SetPrompt("test").SetAbstractModel("test").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	img, err := c.ImageResult.Create().SetUserID(44).SetProjectID(p.ID).SetTaskID(task.ID).SetImageGroup("group one").SetObjectKey("group.png").SetMimeType("image/png").SetSha256("abc").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	name := "group one"
	a, err := NewMediaStore(c).UpdateAsset(ctx, media.UpdateAssetRequest{UserID: 44, AssetID: img.ID, GroupName: &name, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if a.GroupID == nil || a.GroupID.String() != g.ID || a.GroupName != g.Name {
		t.Fatalf("inconsistent asset %+v", a)
	}
	renamed, err := s.Update(ctx, 44, p.ID.String(), g.ID, domain.UpdateRequest{Name: "Renamed", ExpectedVersion: g.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Update(ctx, 44, p.ID.String(), g.ID, domain.UpdateRequest{Name: "Stale", ExpectedVersion: g.Version}); err == nil {
		t.Fatal("stale rename accepted")
	}
	legacy, err := c.ImageResult.Get(ctx, img.ID)
	if err != nil || legacy.ImageGroup != "Renamed" {
		t.Fatalf("legacy rename %v %v", legacy, err)
	}
	target, err := s.Create(ctx, 44, q.ID.String(), domain.CreateRequest{Name: "renamed"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = NewMediaStore(c).GetAsset(ctx, 44, img.ID)
	if err != nil {
		t.Fatal(err)
	}
	a, err = NewMediaStore(c).UpdateAsset(ctx, media.UpdateAssetRequest{UserID: 44, AssetID: img.ID, ProjectID: &q.ID, ExpectedVersion: a.Version})
	if err != nil {
		t.Fatal(err)
	}
	if a.GroupID == nil || a.GroupID.String() != target.ID || a.GroupName != target.Name {
		t.Fatalf("transfer %+v", a)
	}
	if _, err := NewMediaStore(c).UpdateAsset(ctx, media.UpdateAssetRequest{UserID: 44, AssetID: img.ID, GroupID: &q.ID, ExpectedVersion: a.Version}); err == nil {
		t.Fatal("unowned/missing group id accepted")
	}
	gid := uuid.MustParse(target.ID)
	a, err = NewMediaStore(c).UpdateAsset(ctx, media.UpdateAssetRequest{UserID: 44, AssetID: img.ID, GroupID: &gid, ExpectedVersion: a.Version})
	if err != nil || a.GroupID == nil || *a.GroupID != gid {
		t.Fatalf("explicit group id: %+v %v", a, err)
	}
	if err = s.Delete(ctx, 44, p.ID.String(), g.ID, domain.DeleteRequest{ExpectedVersion: g.Version}); err == nil {
		t.Fatal("stale delete accepted")
	}
	if err = s.Delete(ctx, 44, p.ID.String(), g.ID, domain.DeleteRequest{ExpectedVersion: renamed.Version}); err != nil {
		t.Fatal(err)
	}
	if err = s.Delete(ctx, 44, q.ID.String(), target.ID, domain.DeleteRequest{ExpectedVersion: target.Version}); err != nil {
		t.Fatal(err)
	}
	a, err = NewMediaStore(c).GetAsset(ctx, 44, img.ID)
	if err != nil || a.GroupID != nil || a.GroupName != "" || a.Status == "deleted" {
		t.Fatalf("delete destroyed/grouped asset %+v %v", a, err)
	}
	legacy, _ = c.ImageResult.Get(ctx, img.ID)
	if legacy.ImageGroup != "" || legacy.DeletedAt != nil {
		t.Fatalf("legacy deleted/grouped %+v", legacy)
	}
}
func TestAssetGroupPendingUploadRenameAndDelete(t *testing.T) {
	ctx, c := openProjectStoreTestClient(t, "asset-group-pending-upload")
	p, err := c.Project.Create().SetUserID(44).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s := groups.NewService(NewAssetGroupStore(c))
	g, err := s.Create(ctx, 44, p.ID.String(), domain.CreateRequest{Name: "Pending"})
	if err != nil {
		t.Fatal(err)
	}
	upload, err := c.MediaUploadSession.Create().SetUserID(44).SetProjectID(p.ID).SetGroupName(g.Name).SetOriginalFilename("pending.png").SetDeclaredMediaType("image").SetDeclaredMimeType("image/png").SetDeclaredSizeBytes(10).SetStorageDriver("local").SetObjectKey("pending.png").SetPartSize(10).SetPartCount(1).SetReservedBytes(10).SetIdempotencyKey("pending").SetRequestFingerprint("pending").SetExpiresAt(time.Now().Add(time.Hour)).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	g, err = s.Update(ctx, 44, p.ID.String(), g.ID, domain.UpdateRequest{Name: "Renamed", ExpectedVersion: g.Version})
	if err != nil {
		t.Fatal(err)
	}
	upload, err = c.MediaUploadSession.Get(ctx, upload.ID)
	if err != nil || upload.GroupName != g.Name {
		t.Fatalf("upload rename: %v %v", upload, err)
	}
	if err := s.Delete(ctx, 44, p.ID.String(), g.ID, domain.DeleteRequest{ExpectedVersion: g.Version}); err != nil {
		t.Fatal(err)
	}
	upload, err = c.MediaUploadSession.Get(ctx, upload.ID)
	if err != nil || upload.GroupName != "" || upload.Status != "initialized" {
		t.Fatalf("upload delete: %v %v", upload, err)
	}
}

func TestAssetGroupBackfillLegacyAndCanonical(t *testing.T) {
	ctx, c := openProjectStoreTestClient(t, "asset-group-backfill")
	p, err := c.Project.Create().SetUserID(44).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	task, err := c.ImageTask.Create().SetUserID(44).SetProjectID(p.ID).SetTaskType("text_to_image").SetPrompt("test").SetAbstractModel("test").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.ImageResult.Create().SetUserID(44).SetTaskID(task.ID).SetImageGroup("Legacy").SetObjectKey("legacy.png").SetMimeType("image/png").SetSha256("abc").Save(ctx); err != nil {
		t.Fatal(err)
	}
	a, err := c.MediaAsset.Create().SetUserID(44).SetProjectID(p.ID).SetName("a").SetNameKey("a").SetGroupName(" legacy ").SetMediaType("image").SetSourceType("upload").SetObjectKey("a.png").SetMimeType("image/png").SetFileSizeBytes(1).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.BackfillAssetGroups(ctx, c); err != nil {
		t.Fatal(err)
	}
	gs, err := NewAssetGroupStore(c).List(ctx, 44, p.ID.String())
	if err != nil || len(gs) != 1 {
		t.Fatalf("backfill groups %v %v", gs, err)
	}
	a, err = c.MediaAsset.Get(ctx, a.ID)
	if err != nil || a.GroupID == nil || *a.GroupID != uuid.MustParse(gs[0].ID) || a.GroupName != gs[0].Name {
		t.Fatalf("backfill asset %v %v", a, err)
	}
	if n, err := db.BackfillAssetGroups(ctx, c); err != nil || n != 0 {
		t.Fatalf("restart: %d %v", n, err)
	}
}
