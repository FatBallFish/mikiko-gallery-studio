package entstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaprocessingjob"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/objectdeletionjob"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

func TestDeleteMediaAssetEnqueuesOriginalAndDerivativeCleanup(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-delete-cleanup-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(44).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	asset, err := client.MediaAsset.Create().SetUserID(44).SetProjectID(project.ID).SetName("clip.mp4").SetNameKey("clip.mp4").SetMediaType("video").
		SetSourceType("generated").SetStatus("ready").SetStorageDriver("local").SetObjectKey("media/original/44/clip.mp4").
		SetMimeType("video/mp4").SetFileSizeBytes(100).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.MediaDerivative.Create().SetAssetID(asset.ID).SetKind("proxy").SetStatus("ready").SetStorageDriver("local").
		SetObjectKey("media/derivatives/44/clip.mp4").SetMimeType("video/mp4").Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMediaStore(client).DeleteAsset(ctx, mediaassetservice.DeleteAssetRequest{UserID: 44, AssetID: asset.ID, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	jobs, err := client.ObjectDeletionJob.Query().Where(objectdeletionjob.StateEQ("pending")).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("cleanup jobs=%d want 2", len(jobs))
	}
}

func TestMediaUploadServiceIsIdempotentAndCompletesAssetTransaction(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-upload-complete?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(41).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	observer := &mediaUploadObserverSpy{}
	service := mediaassetservice.NewService(NewMediaStore(client), storage.NewStaticRouter(backend), mediaassetservice.Options{
		Policy: domainmedia.DefaultPolicy(), UserQuotaBytes: 1 << 30, PartSize: 8 << 20,
		UploadTTL: time.Hour, Now: func() time.Time { return time.Date(2026, 8, 12, 6, 0, 0, 0, time.UTC) },
		Observer: observer,
	})
	request := mediaassetservice.InitUploadRequest{
		UserID: 41, ProjectID: project.ID, Filename: "voice.wav", MediaType: domainmedia.MediaTypeAudio,
		MIMEType: "audio/wav", SizeBytes: 10, IdempotencyKey: "upload-one",
	}
	session, err := service.InitUpload(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := service.InitUpload(ctx, request)
	if err != nil || replayed.ID != session.ID {
		t.Fatalf("idempotent init session=%s replay=%s err=%v", session.ID, replayed.ID, err)
	}
	changed := request
	changed.Filename = "different.wav"
	if _, err := service.InitUpload(ctx, changed); err == nil {
		t.Fatal("same idempotency key with different request must conflict")
	}

	content := []byte("0123456789")
	checksum := sha256.Sum256(content)
	part, err := service.UploadLocalPart(ctx, 41, session.ID, 1, bytes.NewReader(content), int64(len(content)), hex.EncodeToString(checksum[:]))
	if err != nil {
		t.Fatal(err)
	}
	asset, err := service.CompleteUpload(ctx, 41, session.ID, []storage.CompletedPart{part})
	if err != nil {
		t.Fatal(err)
	}
	if asset.UserID != 41 || asset.ProjectID != project.ID || asset.MediaType != domainmedia.MediaTypeAudio || asset.Status != "processing" || asset.FileSizeBytes != 10 {
		t.Fatalf("unexpected completed asset: %#v", asset)
	}
	completedAgain, err := service.CompleteUpload(ctx, 41, session.ID, []storage.CompletedPart{part})
	if err != nil || completedAgain.ID != asset.ID {
		t.Fatalf("replayed complete asset=%#v err=%v", completedAgain, err)
	}
	if got := strings.Join(observer.events, ","); got != "initialize:success:10,initialize:failed:10,complete:success:10" {
		t.Fatalf("upload observations = %q", got)
	}
	jobs, err := client.MediaProcessingJob.Query().Where(mediaprocessingjob.AssetIDEQ(asset.ID)).All(ctx)
	if err != nil || len(jobs) != 1 || jobs[0].JobType != "probe" {
		t.Fatalf("processing jobs=%#v err=%v", jobs, err)
	}
}

type mediaUploadObserverSpy struct{ events []string }

func (spy *mediaUploadObserverSpy) RecordUpload(stage, result string, bytes int64) {
	spy.events = append(spy.events, stage+":"+result+":"+strconv.FormatInt(bytes, 10))
}

func TestMediaUploadStoreEnforcesProjectOwnerAndReservedQuota(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-upload-quota?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(42).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backend := storage.NewLocalBackend(t.TempDir())
	service := mediaassetservice.NewService(NewMediaStore(client), storage.NewStaticRouter(backend), mediaassetservice.Options{
		Policy: domainmedia.DefaultPolicy(), UserQuotaBytes: 15, PartSize: 8 << 20, UploadTTL: time.Hour,
	})
	base := mediaassetservice.InitUploadRequest{
		UserID: 42, ProjectID: project.ID, Filename: "one.mp3", MediaType: domainmedia.MediaTypeAudio,
		MIMEType: "audio/mpeg", SizeBytes: 10, IdempotencyKey: "quota-one",
	}
	if _, err := service.InitUpload(ctx, base); err != nil {
		t.Fatal(err)
	}
	second := base
	second.Filename = "two.mp3"
	second.IdempotencyKey = "quota-two"
	if _, err := service.InitUpload(ctx, second); err == nil {
		t.Fatal("expected active upload reservation to enforce user quota")
	}
	intruder := base
	intruder.UserID = 99
	intruder.IdempotencyKey = "intruder"
	if _, err := service.InitUpload(ctx, intruder); err == nil {
		t.Fatal("expected project owner isolation")
	}
	count, err := client.MediaUploadSession.Query().Count(ctx)
	if err != nil || count != 1 {
		t.Fatalf("persisted upload count=%d err=%v", count, err)
	}
}

func TestMediaUploadAbortReleasesQuotaAndIsIdempotent(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:media-upload-abort?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(43).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	service := mediaassetservice.NewService(NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{
		Policy: domainmedia.DefaultPolicy(), UserQuotaBytes: 10, PartSize: 8 << 20, UploadTTL: time.Hour,
	})
	request := mediaassetservice.InitUploadRequest{
		UserID: 43, ProjectID: project.ID, Filename: "cancel.mp3", MediaType: domainmedia.MediaTypeAudio,
		MIMEType: "audio/mpeg", SizeBytes: 10, IdempotencyKey: uuid.NewString(),
	}
	session, err := service.InitUpload(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AbortUpload(ctx, 43, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.AbortUpload(ctx, 43, session.ID); err != nil {
		t.Fatalf("idempotent abort: %v", err)
	}
	request.IdempotencyKey = uuid.NewString()
	if _, err := service.InitUpload(ctx, request); err != nil {
		t.Fatalf("released quota must allow replacement upload: %v", err)
	}
}
