package entstore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	canvasservice "github.com/fatballfish/pic-gallery/internal/service/canvas"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	assetsservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

func unifiedAssetFixture(t *testing.T) (*repoent.Client, *AssetsStore, *repoent.MediaAsset) {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, fmt.Sprintf("file:%s?mode=memory&cache=shared&_fk=1", t.Name()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	p, err := createDefaultProjectInTest(t, client, 901)
	if err != nil {
		t.Fatal(err)
	}
	row, err := client.MediaAsset.Create().SetUserID(901).SetProjectID(p.ID).SetName("canonical").SetNameKey("canonical").SetMediaType("image").SetSourceType("upload").SetStatus("ready").SetStorageDriver("local").SetObjectKey("shared.png").SetMimeType("image/png").SetFileSizeBytes(5).SetSha256("same-hash").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return client, NewAssetsStore(client), row
}
func createDefaultProjectInTest(t *testing.T, c *repoent.Client, u int64) (*repoent.Project, error) {
	t.Helper()
	return c.Project.Create().SetUserID(u).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(t.Context())
}
func requireMediaError(t *testing.T, err error, code string) {
	t.Helper()
	var e *errs.Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("error=%v wanted %s", err, code)
	}
}

func TestUnifiedReferenceIDsAliasesAndAvailability(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	alias, err := c.ReferenceAsset.Create().SetUserID(901).SetMediaAssetID(m.ID).SetStatus("ready").SetName("old name").SetNameNormalized("old name").SetStorageDriver("local").SetObjectKey("stale.png").SetMimeType("image/png").SetSha256("old").SetOwnsObject(false).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{m.ID, alias.ID} {
		got, err := s.GetByUserAndID(ctx, 901, id.String())
		if err != nil || got.ID != id.String() || got.MediaAssetID != m.ID.String() || got.ObjectKey != m.ObjectKey || got.DisplayName != m.Name {
			t.Fatalf("got=%+v err=%v", got, err)
		}
	}
	for i := 0; i < 3; i++ {
		got, err := s.ImportMediaAssetAlias(ctx, 901, mediaassetservice.Asset{ID: m.ID})
		if err != nil || got.ID != m.ID.String() {
			t.Fatalf("import=%+v err=%v", got, err)
		}
	}
	if n, _ := c.ReferenceAsset.Query().Count(ctx); n != 1 {
		t.Fatalf("imports created aliases: %d", n)
	}
	if _, err := s.GetByUserAndID(ctx, 902, m.ID.String()); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("foreign=%v", err)
	}
	svc := assetsservice.NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{}, s, storage.NewLocalBackend(t.TempDir()))
	for _, status := range []string{"processing", "failed", "processing_failed", "ready_original", "deleting", "deleted"} {
		if err := c.MediaAsset.UpdateOneID(m.ID).SetStatus(status).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		for _, id := range []uuid.UUID{m.ID, alias.ID} {
			err := svc.ValidateReferencesWithContext(ctx, 901, []string{id.String()})
			if status == "ready_original" {
				if err != nil {
					t.Fatal(err)
				}
				continue
			}
			code := map[string]string{"processing": "MEDIA_ASSET_NOT_READY", "failed": "MEDIA_ASSET_PROCESSING_FAILED", "processing_failed": "MEDIA_ASSET_PROCESSING_FAILED", "deleting": "MEDIA_ASSET_DELETED", "deleted": "MEDIA_ASSET_DELETED"}[status]
			requireMediaError(t, err, code)
			_, err = svc.LoadInput(901, id.String())
			requireMediaError(t, err, code)
		}
	}
	if err := c.MediaAsset.UpdateOneID(m.ID).SetStatus("ready").SetMediaType("video").SetMimeType("video/mp4").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetByUserAndID(ctx, 901, alias.ID.String())
	requireMediaError(t, err, "MEDIA_ASSET_TYPE_MISMATCH")
}

func TestLegacyUploadImmediatelyVisibleAndReuploadAfterDelete(t *testing.T) {
	c, s, _ := unifiedAssetFixture(t)
	ctx := t.Context()
	asset := domainassets.ReferenceAsset{ID: uuid.NewString(), Status: "ready", StorageDriver: "local", ObjectKey: "legacy.png", MimeType: "image/png", SHA256: "legacy-hash", FileSizeBytes: 5, CreatedAt: time.Now()}
	saved, err := s.SaveWithGeneratedName(ctx, 901, asset, domainassets.UploadMetadata{UploadSource: "openapi"}, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	row, err := c.MediaAsset.Get(ctx, uuid.MustParse(asset.ID))
	if err != nil || row.ObjectKey != asset.ObjectKey || saved.OwnsObject || saved.MediaAssetID != asset.ID {
		t.Fatalf("row=%+v saved=%+v err=%v", row, saved, err)
	}
	page, err := NewMediaStore(c).ListAssets(ctx, mediaassetservice.AssetListRequest{UserID: 901, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range page.Items {
		if a.ID.String() == asset.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy upload absent from media list")
	}
	if _, err := s.GetByUserAndHash(ctx, 901, asset.SHA256); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteByUserAndID(ctx, 901, asset.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetByUserAndHash(ctx, 901, asset.SHA256); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("deleted hash=%v", err)
	}
	legacy, err := c.ReferenceAsset.Get(ctx, uuid.MustParse(asset.ID))
	if err != nil || legacy.DeletedAt == nil {
		t.Fatalf("legacy not tombstoned: %+v %v", legacy, err)
	}
	asset.ID = uuid.NewString()
	asset.ObjectKey = "reupload.png"
	if _, err := s.SaveWithGeneratedName(ctx, 901, asset, domainassets.UploadMetadata{}, "legacy"); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.MediaAsset.Query().Where(mediaasset.Sha256EQ(asset.SHA256), mediaasset.DeletedAtIsNil()).Count(ctx); n != 1 {
		t.Fatalf("live reuploads=%d", n)
	}
}

func TestBorrowedAliasDeletionDoesNotDeleteCanonical(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	alias, err := c.ReferenceAsset.Create().SetUserID(901).SetMediaAssetID(m.ID).SetStatus("ready").SetStorageDriver("local").SetObjectKey(m.ObjectKey).SetMimeType(m.MimeType).SetSha256("alias").SetOwnsObject(false).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteByUserAndID(ctx, 901, alias.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetByUserAndID(ctx, 901, m.ID.String()); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.ObjectDeletionJob.Query().Count(ctx); n != 0 {
		t.Fatalf("borrowed deletion scheduled %d jobs", n)
	}
	if _, err := NewMediaStore(c).DeleteAsset(ctx, mediaassetservice.DeleteAssetRequest{UserID: 901, AssetID: m.ID, ExpectedVersion: m.Version}); err != nil {
		t.Fatal(err)
	}
	if n, _ := c.ObjectDeletionJob.Query().Count(ctx); n != 1 {
		t.Fatalf("canonical deletion jobs=%d", n)
	}
}

func TestLegacyUploadServiceRechecksPersistentMediaAfterDeletion(t *testing.T) {
	c, s, _ := unifiedAssetFixture(t)
	svc := assetsservice.NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{}, s, storage.NewLocalBackend(t.TempDir()))
	content, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+y1X8AAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.UploadWithMetadataContext(t.Context(), 901, "first.png", "image/png", content, domainassets.UploadMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	row, err := c.MediaAsset.Get(t.Context(), uuid.MustParse(first.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewMediaStore(c).DeleteAsset(t.Context(), mediaassetservice.DeleteAssetRequest{UserID: 901, AssetID: row.ID, ExpectedVersion: row.Version}); err != nil {
		t.Fatal(err)
	}
	second, err := svc.UploadWithMetadataContext(t.Context(), 901, "first.png", "image/png", content, domainassets.UploadMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("stale in-memory hash cache resurrected deleted media")
	}
	input, err := svc.LoadInput(901, second.ID)
	if err != nil || string(input.Data) != string(content) {
		t.Fatalf("input=%+v err=%v", input, err)
	}
}

func TestCanvasRealStoreKeepsUnifiedIDsAndRejectsUnready(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	svc := assetsservice.NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{}, s, storage.NewLocalBackend(t.TempDir()))
	g := canvasservice.NewTaskGenerator(nil, nil, nil)
	g.SetReferenceAssets(svc)
	sub := canvasservice.GenerationSubmission{UserID: 901, Kind: canvasservice.TaskKindImage, Node: domaincanvas.Node{Type: domaincanvas.NodeTypeImageV2, Payload: json.RawMessage(`{"draft":{"route_model_code":"model","prompt_template":"with {{@stable}}"}}`)}, Inputs: []canvasservice.GenerationInput{{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{AssetID: m.ID.String(), Payload: json.RawMessage(`{"name":"stable"}`)}}}}
	req, err := g.PrepareCanvasImageRequest(t.Context(), sub)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.ReferenceAssetIDs) != 1 || req.ReferenceAssetIDs[0] != m.ID.String() || req.ReferenceBindings[0].AssetID != m.ID.String() {
		t.Fatalf("changed IDs %+v", req)
	}
	if n, _ := c.ReferenceAsset.Query().Count(t.Context()); n != 0 {
		t.Fatalf("created aliases %d", n)
	}
	if err := c.MediaAsset.UpdateOneID(m.ID).SetStatus("processing").Exec(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, err = g.PrepareCanvasImageRequest(t.Context(), sub)
	requireMediaError(t, err, "MEDIA_ASSET_NOT_READY")
	var apiErr *errs.Error
	if !errors.As(err, &apiErr) || apiErr.Details["asset_id"] != m.ID.String() {
		t.Fatalf("lost media details %v", err)
	}
}

func TestUnifiedReferenceHistoricalSourceMappingAndRename(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	sourceID := uuid.New()
	if err := c.MediaAsset.UpdateOneID(m.ID).SetLegacyImageResultID(sourceID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	alias, err := c.ReferenceAsset.Create().SetUserID(901).SetSourceImageResultID(sourceID).SetStatus("ready").SetName("stable").SetNameNormalized("stable").SetStorageDriver("local").SetObjectKey("stale.png").SetMimeType("image/png").SetSha256("source-old").SetOwnsObject(false).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByUserAndID(ctx, 901, alias.ID.String())
	if err != nil || got.MediaAssetID != m.ID.String() || got.ObjectKey != m.ObjectKey {
		t.Fatalf("source mapping %+v %v", got, err)
	}
	renamed, err := s.RenameByUserAndID(ctx, 901, alias.ID.String(), "renamed", "renamed")
	if err != nil || renamed.DisplayName != "renamed" || renamed.Name != "renamed" {
		t.Fatalf("rename %+v %v", renamed, err)
	}
	canonical, err := s.GetByUserAndID(ctx, 901, m.ID.String())
	if err != nil || canonical.Name != "renamed" {
		t.Fatalf("canonical %+v %v", canonical, err)
	}
	// A foreign mapping must never expose another owner's canonical metadata.
	foreign, err := c.ReferenceAsset.Create().SetUserID(902).SetMediaAssetID(m.ID).SetStatus("ready").SetStorageDriver("local").SetObjectKey("foreign.png").SetMimeType("image/png").SetSha256("foreign").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetByUserAndID(ctx, 902, foreign.ID.String()); !errors.Is(err, repoerr.ErrNotFound) {
		t.Fatalf("foreign mapping=%v", err)
	}
}

func TestReferenceTupleFallbackUsesExactStorageIdentity(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	configID := uuid.New()
	alias, err := c.ReferenceAsset.Create().SetUserID(901).SetStatus("ready").SetStorageConfigID(configID).SetStorageDriver(m.StorageDriver).SetObjectKey(m.ObjectKey).SetMimeType(m.MimeType).SetSha256("tuple").SetOwnsObject(false).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByUserAndID(ctx, 901, alias.ID.String())
	if err != nil || got.MediaAssetID != alias.ID.String() || got.StorageConfigID != configID.String() {
		t.Fatalf("cross-config association %+v %v", got, err)
	}
	exact, err := c.ReferenceAsset.Create().SetUserID(901).SetStatus("ready").SetStorageDriver(m.StorageDriver).SetObjectKey(m.ObjectKey).SetMimeType(m.MimeType).SetSha256("tuple-exact").SetOwnsObject(false).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err = s.GetByUserAndID(ctx, 901, exact.ID.String())
	if err != nil || got.MediaAssetID != m.ID.String() {
		t.Fatalf("exact tuple %+v %v", got, err)
	}
}

func TestImagePreflightRealStoreRejectsUnreadyReferenceAndMask(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	refs := assetsservice.NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{}, s, storage.NewLocalBackend(t.TempDir()))
	images := imagetaskservice.NewServiceWithProvidersStoreAssetsBillingAndRouter(config.Config{}, nil, nil, refs, nil, nil)
	for _, status := range []string{"processing", "processing_failed", "deleting"} {
		if err := c.MediaAsset.UpdateOneID(m.ID).SetStatus(status).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		code := map[string]string{"processing": "MEDIA_ASSET_NOT_READY", "processing_failed": "MEDIA_ASSET_PROCESSING_FAILED", "deleting": "MEDIA_ASSET_DELETED"}[status]
		for _, mask := range []bool{false, true} {
			req := domainimagetask.CreateRequest{UserID: 901, TaskType: "image_edit", Prompt: "edit", AbstractModel: "plus", SizeMode: "auto", OutputImageCount: 1}
			if mask {
				req.MaskAssetID = m.ID.String()
			} else {
				req.ReferenceAssetIDs = []string{m.ID.String()}
			}
			_, err := images.PreflightMetadata(ctx, req)
			requireMediaError(t, err, code)
			var apiErr *errs.Error
			if !errors.As(err, &apiErr) || apiErr.Details["asset_id"] != m.ID.String() || apiErr.Details["status"] != status {
				t.Fatalf("lost status details %v", err)
			}
			_, err = images.Preflight(ctx, req)
			requireMediaError(t, err, code)
			_, err = images.CreateTask(ctx, req)
			requireMediaError(t, err, code)
		}
	}
}

func TestLateStandaloneReferenceIsAdoptedAndRechecked(t *testing.T) {
	c, s, _ := unifiedAssetFixture(t)
	ctx := t.Context()
	row, err := c.ReferenceAsset.Create().SetUserID(901).SetName("stableLate").SetNameNormalized("stableLate").SetStatus("ready").SetStorageDriver("local").SetObjectKey("late.png").SetMimeType("image/png").SetSha256("late").SetOwnsObject(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		got, err := s.GetByUserAndID(ctx, 901, row.ID.String())
		if err != nil || got.MediaAssetID != row.ID.String() || got.Name != "stableLate" || got.OwnsObject {
			t.Fatalf("adopted %+v %v", got, err)
		}
	}
	mapped, err := c.ReferenceAsset.Get(ctx, row.ID)
	if err != nil || mapped.MediaAssetID == nil || *mapped.MediaAssetID != row.ID || mapped.OwnsObject {
		t.Fatalf("mapping %+v %v", mapped, err)
	}
	if err := c.MediaAsset.UpdateOneID(row.ID).SetStatus("processing_failed").SetName("display changed").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetByUserAndID(ctx, 901, row.ID.String())
	if err != nil || got.Name != "stableLate" || got.DisplayName != "display changed" {
		t.Fatalf("stable binding %+v %v", got, err)
	}
	requireMediaError(t, assetsservice.ValidateReferenceAvailability(got), "MEDIA_ASSET_PROCESSING_FAILED")
}

func TestCanonicalGalleryImportReferenceDeletePreservesSource(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	task, err := c.ImageTask.Create().SetUserID(901).SetProjectID(m.ProjectID).SetTaskType("text_to_image").SetPrompt("source").SetAbstractModel("plus").SetRouteModelCode("route").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.ImageResult.Create().SetUserID(901).SetProjectID(m.ProjectID).SetTaskID(task.ID).SetStorageDriver("local").SetObjectKey("generated/import.png").SetMimeType("image/png").SetSha256("gallery").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := s.ImportGalleryAlias(ctx, 901, provider.ImageResult{ID: result.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if imported.ID != result.ID.String() {
		t.Fatalf("import ID %s", imported.ID)
	}
	if err := s.DeleteByUserAndID(ctx, 901, imported.ID); err != nil {
		t.Fatal(err)
	}
	source, err := c.ImageResult.Get(ctx, result.ID)
	if err != nil || source.DeletedAt != nil {
		t.Fatalf("deleted gallery %+v %v", source, err)
	}
	canonical, err := c.MediaAsset.Get(ctx, result.ID)
	if err != nil || canonical.DeletedAt != nil || canonical.Status == "deleted" {
		t.Fatalf("deleted media %+v %v", canonical, err)
	}
	if n, _ := c.ObjectDeletionJob.Query().Count(ctx); n != 0 {
		t.Fatalf("scheduled shared object cleanup %d", n)
	}
	if _, err := s.GetByUserAndID(ctx, 901, imported.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMediaStore(c).DeleteAsset(ctx, mediaassetservice.DeleteAssetRequest{UserID: 901, AssetID: canonical.ID, ExpectedVersion: canonical.Version}); err != nil {
		t.Fatal(err)
	}
	source, err = c.ImageResult.Get(ctx, result.ID)
	if err != nil || source.DeletedAt == nil {
		t.Fatalf("media DELETE did not delete source %+v %v", source, err)
	}
}

func TestMediaDeleteTombstonesLateUnmappedExactStorageReferences(t *testing.T) {
	c, _, m := unifiedAssetFixture(t)
	ctx := t.Context()
	create := func(user int64, configID *uuid.UUID, driver string) *repoent.ReferenceAsset {
		t.Helper()
		b := c.ReferenceAsset.Create().SetUserID(user).SetStatus("ready").SetStorageDriver(driver).SetObjectKey(m.ObjectKey).SetMimeType(m.MimeType).SetSha256(uuid.NewString()).SetOwnsObject(true)
		if configID != nil {
			b.SetStorageConfigID(*configID)
		}
		r, e := b.Save(ctx)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	late := create(901, nil, m.StorageDriver)
	foreign := create(902, nil, m.StorageDriver)
	configID := uuid.New()
	otherConfig := create(901, &configID, m.StorageDriver)
	otherDriver := create(901, nil, "s3")
	if _, err := NewMediaStore(c).DeleteAsset(ctx, mediaassetservice.DeleteAssetRequest{UserID: 901, AssetID: m.ID, ExpectedVersion: m.Version}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []*repoent.ReferenceAsset{late, foreign, otherConfig, otherDriver} {
		got, err := c.ReferenceAsset.Get(ctx, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		if (got.DeletedAt != nil) != (row.ID == late.ID) {
			t.Fatalf("incorrect tuple tombstone %+v", got)
		}
	}
}

func TestReferenceRenameCollisionRollsBackCanonicalAndOnlyChangesRequestedAlias(t *testing.T) {
	c, s, m := unifiedAssetFixture(t)
	ctx := t.Context()
	create := func(id uuid.UUID, name string, mediaID uuid.UUID) *repoent.ReferenceAsset {
		t.Helper()
		r, e := c.ReferenceAsset.Create().SetID(id).SetUserID(901).SetName(name).SetNameNormalized(name).SetMediaAssetID(mediaID).SetStatus("ready").SetStorageDriver("local").SetObjectKey(m.ObjectKey).SetMimeType(m.MimeType).SetSha256(uuid.NewString()).Save(ctx)
		if e != nil {
			t.Fatal(e)
		}
		return r
	}
	self := create(m.ID, "selfStable", m.ID)
	alias := create(uuid.New(), "aliasStable", m.ID)
	other, err := c.MediaAsset.Create().SetUserID(901).SetProjectID(m.ProjectID).SetName("occupied").SetNameKey("occupied").SetMediaType("image").SetSourceType("local_upload").SetStatus("ready").SetStorageDriver("local").SetObjectKey("other.png").SetMimeType("image/png").SetFileSizeBytes(0).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	create(other.ID, "occupied", other.ID)
	if _, err := s.RenameByUserAndID(ctx, 901, alias.ID.String(), "occupied", "occupied"); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("conflict=%v", err)
	}
	canonical, err := c.MediaAsset.Get(ctx, m.ID)
	if err != nil || canonical.Name != m.Name || canonical.Version != m.Version {
		t.Fatalf("conflict modified canonical %+v %v", canonical, err)
	}
	unchanged, err := c.ReferenceAsset.Get(ctx, alias.ID)
	if err != nil || *unchanged.Name != "aliasStable" {
		t.Fatalf("conflict modified alias %+v %v", unchanged, err)
	}
	if _, err := s.RenameByUserAndID(ctx, 901, alias.ID.String(), "newName", "newName"); err != nil {
		t.Fatal(err)
	}
	preserved, err := c.ReferenceAsset.Get(ctx, self.ID)
	if err != nil || *preserved.Name != "selfStable" {
		t.Fatalf("renamed unrequested self row %+v %v", preserved, err)
	}
}
