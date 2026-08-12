package entstore

import (
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestMediaStoreClaimsExpiredUploadAndRecoversStaleExpiryLease(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-upload-reconcile?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(700).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC)
	activeID := uuid.New()
	if _, err := client.MediaUploadSession.Create().SetID(activeID).SetUserID(700).SetProjectID(project.ID).
		SetOriginalFilename("active.mp4").SetDeclaredMediaType("video").SetDeclaredMimeType("video/mp4").SetDeclaredSizeBytes(1024).
		SetStorageDriver("local").SetObjectKey("media/original/700/active.mp4").SetBackendUploadID("upload-active").
		SetPartSize(1024).SetPartCount(1).SetStatus("uploading").SetReservedBytes(1024).SetIdempotencyKey("active").
		SetRequestFingerprint("active").SetExpiresAt(now.Add(-time.Minute)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	staleID := uuid.New()
	if _, err := client.MediaUploadSession.Create().SetID(staleID).SetUserID(700).SetProjectID(project.ID).
		SetOriginalFilename("stale.mp4").SetDeclaredMediaType("video").SetDeclaredMimeType("video/mp4").SetDeclaredSizeBytes(2048).
		SetStorageDriver("local").SetObjectKey("media/original/700/stale.mp4").SetBackendUploadID("upload-stale").
		SetPartSize(2048).SetPartCount(1).SetStatus("expiring").SetReservedBytes(2048).SetIdempotencyKey("stale").
		SetRequestFingerprint("stale").SetExpiresAt(now.Add(-time.Hour)).SetUpdatedAt(now.Add(-10 * time.Minute)).Save(ctx); err != nil {
		t.Fatal(err)
	}

	store := NewMediaStore(client)
	first, ok, err := store.ClaimExpiredUpload(ctx, now, 5*time.Minute)
	if err != nil || !ok || first.ID != activeID || first.Status != "expiring" {
		t.Fatalf("first claim = %#v, %v, %v", first, ok, err)
	}
	second, ok, err := store.ClaimExpiredUpload(ctx, now.Add(time.Minute), 5*time.Minute)
	if err != nil || !ok || second.ID != staleID || second.Status != "expiring" {
		t.Fatalf("stale claim = %#v, %v, %v", second, ok, err)
	}
	third, ok, err := store.ClaimExpiredUpload(ctx, now.Add(6*time.Minute), 5*time.Minute)
	if err != nil || !ok || third.ID != activeID || third.Status != "expiring" {
		t.Fatalf("recovered first claim = %#v, %v, %v", third, ok, err)
	}

	completed, err := store.CompleteExpiredUpload(ctx, second.ID)
	if err != nil || !completed {
		t.Fatalf("CompleteExpiredUpload() = %v, %v", completed, err)
	}
	row, err := client.MediaUploadSession.Get(ctx, staleID)
	if err != nil || row.Status != "expired" || row.ReservedBytes != 0 {
		t.Fatalf("expired row = %#v, %v", row, err)
	}
	completed, err = store.CompleteExpiredUpload(ctx, staleID)
	if err != nil || completed {
		t.Fatalf("idempotent CompleteExpiredUpload() = %v, %v", completed, err)
	}
}
