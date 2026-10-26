package entstore

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

func TestVideoPublicationLifecycle(t *testing.T) {
	ctx := t.Context()
	client, e := repoent.Open(dialect.SQLite, "file:video-publication-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = client.Close() })
	if e = client.Schema.Create(ctx); e != nil {
		t.Fatal(e)
	}
	testVideoPublicationLifecycle(t, client)
}

func TestVideoPublicationLifecyclePostgres(t *testing.T) {
	_, _, client, _ := openProjectTaskPostgres(t)
	testVideoPublicationLifecycle(t, client)
}

func testVideoPublicationLifecycle(t *testing.T, client *repoent.Client) {
	t.Helper()
	ctx := t.Context()
	p, e := client.Project.Create().SetUserID(44).SetName("Secret project").SetNameKey("secret").Save(ctx)
	if e != nil {
		t.Fatal(e)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	media := mediaasset.NewService(NewMediaStore(client), storage.NewStaticRouter(backend), mediaasset.Options{})
	store := NewVideoPublicationStore(client)
	service := videopublication.NewService(store, media)
	for _, source := range []string{"upload", "generated"} {
		t.Run(source, func(t *testing.T) {
			asset, e := client.MediaAsset.Create().SetUserID(44).SetProjectID(p.ID).SetName("clip-" + source).SetNameKey("clip-" + source).SetMediaType("video").SetSourceType(source).SetStatus("ready").SetStorageDriver("local").SetObjectKey(source + "/original.mp4").SetMimeType("video/mp4").SetFileSizeBytes(10).Save(ctx)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = service.Submit(ctx, 45, asset.ID); e == nil {
				t.Fatal("non-owner submitted")
			}
			if _, e = service.Submit(ctx, 44, asset.ID); e == nil {
				t.Fatal("missing derivatives allowed")
			}
			for _, kind := range []string{"proxy", "poster"} {
				key := source + "/" + kind
				mime := "video/mp4"
				if kind == "poster" {
					mime = "image/jpeg"
				}
				if e = backend.Put(ctx, key, mime, []byte("0123456789")); e != nil {
					t.Fatal(e)
				}
				if _, e = client.MediaDerivative.Create().SetAssetID(asset.ID).SetKind(kind).SetStatus("ready").SetStorageDriver("local").SetObjectKey(key).SetMimeType(mime).Save(ctx); e != nil {
					t.Fatal(e)
				}
			}
			pending, e := service.Submit(ctx, 44, asset.ID)
			if e != nil || pending.Status != "pending_review" {
				t.Fatalf("submit %+v %v", pending, e)
			}
			if _, e = service.Open(ctx, asset.ID, false, false); e == nil {
				t.Fatal("pending publicly readable")
			}
			page, e := service.List(ctx, "", 1, 20, false)
			if e != nil || page.Total != 0 {
				t.Fatalf("pending listed %+v %v", page, e)
			}
			rejected, e := service.Review(ctx, asset.ID, "rejected", "  copyright  ")
			if e != nil || rejected.Status != videopublication.Rejected || rejected.PublishedAt != nil || rejected.ReviewReason != "copyright" || rejected.Version != pending.Version+1 {
				t.Fatalf("reject %+v %v", rejected, e)
			}
			for _, decision := range []string{"approved", "rejected"} {
				_, e = service.Review(ctx, asset.ID, decision, "copyright")
				var appErr *errs.Error
				if !errors.As(e, &appErr) || appErr.StatusCode != 409 || appErr.Code != errs.CodeConflict {
					t.Fatalf("stale %s: want 409, got %v", decision, e)
				}
			}
			if _, e = service.Get(ctx, asset.ID, false); e == nil {
				t.Fatal("rejected detail exposed")
			}
			if _, e = service.Submit(ctx, 44, asset.ID); e != nil {
				t.Fatal(e)
			}
			approved, e := service.Review(ctx, asset.ID, "approved", "")
			if e != nil || approved.Status != videopublication.Approved || approved.PublishedAt == nil || approved.ReviewReason != "" || approved.Version != rejected.Version+2 {
				t.Fatalf("approve %+v %v", approved, e)
			}
			for _, decision := range []string{"approved", "rejected"} {
				_, e = service.Review(ctx, asset.ID, decision, "copyright")
				var appErr *errs.Error
				if !errors.As(e, &appErr) || appErr.StatusCode != 409 {
					t.Fatalf("repeat %s: want 409, got %v", decision, e)
				}
			}
			for _, poster := range []bool{true, false} {
				stream, e := service.Open(ctx, asset.ID, poster, false)
				if e != nil {
					t.Fatal(e)
				}
				_ = stream.Reader.Close()
			}
			data, _ := json.Marshal(approved.Video)
			for _, secret := range []string{"project_id", "user_id", "source_task", "access_token", "object_key", "review_reason"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("public leak %s", data)
				}
			}
			if _, e = service.Withdraw(ctx, 45, asset.ID); e == nil {
				t.Fatal("non-owner withdrew")
			}
			if _, e = service.Withdraw(ctx, 44, asset.ID); e != nil {
				t.Fatal(e)
			}
			if _, e = service.Open(ctx, asset.ID, true, false); e == nil {
				t.Fatal("withdrawn poster exposed")
			}
			if _, e = service.Get(ctx, asset.ID, false); e == nil {
				t.Fatal("withdrawn detail exposed")
			}
		})
	}
}
