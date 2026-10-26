package entstore

import (
	"context"
	"errors"
	"testing"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	domain "github.com/fatballfish/pic-gallery/internal/domain/publicfeed"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/google/uuid"
)

// Intercept the first detail query after the page has selected its IDs. This
// deterministically reproduces a withdrawal/deletion between selection and
// hydration without sleeps or a production-only test hook.
func TestPublicListsHydrationRace(t *testing.T) {
	for _, feed := range []bool{false, true} {
		name := "videos"
		if feed {
			name = "feed"
		}
		for _, change := range []string{"withdraw", "delete", "storage-error"} {
			t.Run(name+"/"+change, func(t *testing.T) {
				ctx := t.Context()
				client, err := repoent.Open(dialect.SQLite, "file:hydrate-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				if err := client.Schema.Create(ctx); err != nil {
					t.Fatal(err)
				}
				project := client.Project.Create().SetUserID(44).SetName("race").SetNameKey("race").SaveX(ctx)
				videos := make([]*repoent.MediaAsset, 0, 3)
				for i := 0; i < 3; i++ {
					videos = append(videos, client.MediaAsset.Create().SetUserID(44).SetProjectID(project.ID).SetName("clip").SetNameKey(uuid.NewString()).SetMediaType("video").SetSourceType("upload").SetStatus("ready").SetVisibilityStatus("approved").SetPublishedAt(time.Now().Add(-time.Duration(i)*time.Hour)).SetObjectKey("clip.mp4").SetMimeType("video/mp4").SetFileSizeBytes(1).SaveX(ctx))
				}
				injected := errors.New("injected storage read failure")
				fired := false
				client.MediaAsset.Intercept(ent.InterceptFunc(func(next ent.Querier) ent.Querier {
					return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
						if !fired && ent.QueryFromContext(ctx).Op == ent.OpQueryOnly {
							fired = true
							switch change {
							case "withdraw":
								// Perform the actual publication transition before the detail read.
								if _, err := client.MediaAsset.UpdateOneID(videos[0].ID).SetVisibilityStatus("private").ClearPublishedAt().Save(ctx); err != nil {
									return nil, err
								}
							case "delete":
								if err := client.MediaAsset.DeleteOneID(videos[0].ID).Exec(ctx); err != nil {
									return nil, err
								}
							case "storage-error":
								return nil, injected
							}
						}
						return next.Query(ctx, q)
					})
				}))
				var ids []uuid.UUID
				var total, page, size int
				if feed {
					result, e := NewPublicFeedStore(client).List(ctx, domain.Request{Page: 1, PageSize: 2})
					err = e
					total = result.Total
					page = result.Page
					size = result.PageSize
					for _, item := range result.Items {
						if item.Video != nil {
							ids = append(ids, item.Video.ID)
						}
					}
				} else {
					result, e := NewVideoPublicationStore(client).ListPublic(ctx, 0, false, false, "latest", 1, 2)
					err = e
					total = result.Total
					page = result.Page
					size = result.PageSize
					for _, item := range result.Items {
						ids = append(ids, item.ID)
					}
				}
				if !fired {
					t.Fatal("detail interception did not execute")
				}
				if change == "storage-error" {
					if !errors.Is(err, injected) {
						t.Fatalf("storage error swallowed: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(ids) != 1 || ids[0] != videos[1].ID {
					t.Fatalf("visible item lost or window refilled: %v", ids)
				}
				if total != 3 || page != 1 || size != 2 {
					t.Fatalf("pagination changed: total=%d page=%d size=%d", total, page, size)
				}
			})
		}
	}
}
