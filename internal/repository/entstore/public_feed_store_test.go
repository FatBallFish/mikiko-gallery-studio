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

func TestPublicVideoInteractions(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:video-interactions-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	testPublicVideoInteractions(t, client)
}
func TestPublicVideoInteractionsPostgres(t *testing.T) {
	_, _, client, _ := openProjectTaskPostgres(t)
	testPublicVideoInteractions(t, client)
}
func testPublicVideoInteractions(t *testing.T, client *repoent.Client) {
	ctx := t.Context()
	p := client.Project.Create().SetUserID(44).SetName("interactions").SetNameKey("interactions").SaveX(ctx)
	task := client.ImageTask.Create().SetUserID(44).SetProjectID(p.ID).SetTaskType("text_to_image").SetPrompt("test").SetAbstractModel("test").SaveX(ctx)
	id := uuid.New()
	image := client.ImageResult.Create().SetID(id).SetTaskID(task.ID).SetUserID(44).SetProjectID(p.ID).SetObjectKey("image.png").SetMimeType("image/png").SetSha256("hash").SetVisibilityStatus("approved").SetPublishedAt(time.Now()).SaveX(ctx)
	video := client.MediaAsset.Create().SetID(id).SetUserID(44).SetProjectID(p.ID).SetName("clip").SetNameKey("clip").SetMediaType("video").SetSourceType("upload").SetStatus("ready").SetVisibilityStatus("approved").SetPublishedAt(time.Now().Add(-time.Hour)).SetObjectKey("clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1).SaveX(ctx)
	store := NewVideoPublicationStore(client)
	for i := 0; i < 3; i++ {
		if _, err := store.SetInteraction(ctx, video.ID, 44, "like", true); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.SetInteraction(ctx, video.ID, 44, "favorite", true); err != nil {
		t.Fatal(err)
	}
	v, err := store.SetInteraction(ctx, video.ID, 45, "favorite", true)
	if err != nil {
		t.Fatal(err)
	}
	if v.LikeCount != 1 || v.FavoriteCount != 2 || v.LikedByViewer || !v.FavoritedByViewer {
		t.Fatalf("counts/state %+v", v)
	}
	if count := client.PublicVideoInteraction.Query().CountX(ctx); count != 2 {
		t.Fatalf("rows = %d", count)
	}
	if count := client.PublicImageInteraction.Query().CountX(ctx); count != 0 {
		t.Fatalf("image interactions leaked = %d", count)
	}
	anonymous, err := store.GetForViewer(ctx, id, 0)
	if err != nil || anonymous.LikedByViewer || anonymous.FavoritedByViewer || anonymous.LikeCount != 1 {
		t.Fatalf("anonymous %+v %v", anonymous, err)
	}
	page, err := store.ListPublic(ctx, 44, true, true, "hot", 1, 1)
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("public list %+v %v", page, err)
	}
	feed := NewPublicFeedStore(client)
	for _, req := range []domain.Request{{Page: 1, PageSize: 1, Sort: "hot"}, {Page: 1, PageSize: 10, Liked: true, Favorited: true, ViewerID: 44}} {
		result, err := feed.List(ctx, req)
		if err != nil || len(result.Items) != 1 || result.Items[0].Video == nil || !result.Items[0].Video.LikedByViewer && req.ViewerID > 0 {
			t.Fatalf("feed %+v %v", result, err)
		}
	}
	// Equal UUID image stays independently visible but never inherits video reactions.
	imageResult, err := NewImageTaskStore(client).GetPublicImage(ctx, image.ID.String(), 44)
	if err != nil || imageResult.LikedByViewer || imageResult.FavoritedByViewer {
		t.Fatalf("image leaked %+v %v", imageResult, err)
	}
	if _, err := store.SetInteraction(ctx, id, 44, "like", false); err != nil {
		t.Fatal(err)
	}
	v, err = store.SetInteraction(ctx, id, 44, "like", false)
	if err != nil || v.LikeCount != 0 || v.FavoriteCount != 2 || !v.FavoritedByViewer {
		t.Fatalf("idempotent removal %+v %v", v, err)
	}
	if _, err := store.Transition(ctx, id, 44, "private", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetInteraction(ctx, id, 44, "like", true); err == nil {
		t.Fatal("withdrawn video accepted reaction")
	}
	if _, err := store.GetForViewer(ctx, id, 44); err == nil {
		t.Fatal("withdrawn video visible")
	}
	result, err := feed.List(ctx, domain.Request{Page: 1, PageSize: 10, Favorited: true, ViewerID: 44})
	if err != nil || result.Total != 0 {
		t.Fatalf("withdrawn feed %+v %v", result, err)
	}
}

func TestPublicVideoConcurrentInteractionsPostgres(t *testing.T) {
	ctx, _, client, _ := openProjectTaskPostgres(t)
	p := client.Project.Create().SetUserID(44).SetName("concurrency").SetNameKey("concurrency").SaveX(ctx)
	video := client.MediaAsset.Create().SetUserID(44).SetProjectID(p.ID).SetName("clip").SetNameKey("clip").SetMediaType("video").SetSourceType("upload").SetStatus("ready").SetVisibilityStatus("approved").SetObjectKey("clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1).SaveX(ctx)
	store := NewVideoPublicationStore(client)
	failures := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func(i int) {
			_, err := store.SetInteraction(ctx, video.ID, 44+int64(i%5), "like", true)
			failures <- err
		}(i)
	}
	for i := 0; i < 20; i++ {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	v, err := store.GetForViewer(ctx, video.ID, 44)
	if err != nil || v.LikeCount != 5 || !v.LikedByViewer {
		t.Fatalf("concurrent counts %+v %v", v, err)
	}
	if count := client.PublicVideoInteraction.Query().CountX(ctx); count != 5 {
		t.Fatalf("duplicate rows %d", count)
	}
}
