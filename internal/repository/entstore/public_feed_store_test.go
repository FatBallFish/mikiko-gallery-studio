package entstore

import (
	"entgo.io/ent/dialect"
	domain "github.com/fatballfish/pic-gallery/internal/domain/publicfeed"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
	"testing"
	"time"
)

func TestPublicFeedUnifiedPaginationAndFilters(t *testing.T) {
	ctx := t.Context()
	client, e := repoent.Open(dialect.SQLite, "file:feed-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	if e = client.Schema.Create(ctx); e != nil {
		t.Fatal(e)
	}
	p := client.Project.Create().SetUserID(44).SetName("p").SetNameKey("p").SaveX(ctx)
	task := client.ImageTask.Create().SetUserID(44).SetProjectID(p.ID).SetTaskType("text_to_image").SetPrompt("matching landscape").SetAbstractModel("test").SaveX(ctx)
	now := time.Now().UTC()
	image := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(44).SetProjectID(p.ID).SetObjectKey("image.png").SetMimeType("image/png").SetSha256("hash").SetVisibilityStatus("approved").SetPublishedAt(now).SaveX(ctx)
	video := client.MediaAsset.Create().SetUserID(44).SetProjectID(p.ID).SetName("matching clip").SetNameKey("clip").SetMediaType("video").SetSourceType("upload").SetStatus("ready").SetVisibilityStatus("approved").SetPublishedAt(now.Add(time.Second)).SetObjectKey("clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1).SaveX(ctx)
	client.MediaAsset.Create().SetUserID(44).SetProjectID(p.ID).SetName("pending").SetNameKey("pending").SetMediaType("video").SetSourceType("upload").SetStatus("ready").SetVisibilityStatus("pending_review").SetObjectKey("pending.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1).SaveX(ctx)
	store := NewPublicFeedStore(client)
	first, e := store.List(ctx, domain.Request{Page: 1, PageSize: 1})
	if e != nil {
		t.Fatal(e)
	}
	if first.Total != 2 || len(first.Items) != 1 || first.Items[0].Video == nil || first.Items[0].Video.ID != video.ID {
		t.Fatalf("first %+v", first)
	}
	second, e := store.List(ctx, domain.Request{Page: 2, PageSize: 1})
	if e != nil {
		t.Fatal(e)
	}
	if len(second.Items) != 1 || second.Items[0].Image == nil || second.Items[0].Image.ID != image.ID.String() {
		t.Fatalf("second %+v", second)
	}
	search, e := store.List(ctx, domain.Request{Page: 1, PageSize: 20, Query: "matching"})
	if e != nil || search.Total != 2 {
		t.Fatalf("search %+v %v", search, e)
	}
	client.PublicImageStat.Create().SetImageID(image.ID).SetLikeCount(2).SaveX(ctx)
	hot, e := store.List(ctx, domain.Request{Page: 1, PageSize: 1, Sort: "hot"})
	if e != nil || len(hot.Items) != 1 || hot.Items[0].Image == nil {
		t.Fatalf("hot %+v %v", hot, e)
	}
	client.PublicImageInteraction.Create().SetImageID(image.ID).SetUserID(44).SetLiked(true).SaveX(ctx)
	liked, e := store.List(ctx, domain.Request{Page: 1, PageSize: 20, Liked: true, ViewerID: 44})
	if e != nil || liked.Total != 1 || liked.Items[0].Image == nil {
		t.Fatalf("liked %+v %v", liked, e)
	}
}
