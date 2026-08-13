package mediaasset

import (
	"context"
	"testing"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

type accessStoreProbe struct {
	asset       Asset
	derivatives []AssetDerivative
}

func (*accessStoreProbe) FindUploadByIdempotency(context.Context, int64, string) (UploadSession, bool, error) {
	return UploadSession{}, false, nil
}
func (*accessStoreProbe) CreateUpload(context.Context, CreateUploadRecord) (UploadSession, error) {
	return UploadSession{}, nil
}
func (*accessStoreProbe) GetUpload(context.Context, int64, uuid.UUID) (UploadSession, error) {
	return UploadSession{}, nil
}
func (*accessStoreProbe) RecordCompletedParts(context.Context, int64, uuid.UUID, []storage.CompletedPart) (UploadSession, error) {
	return UploadSession{}, nil
}
func (*accessStoreProbe) MarkUploadCompleting(context.Context, int64, uuid.UUID, []storage.CompletedPart) (UploadSession, error) {
	return UploadSession{}, nil
}
func (*accessStoreProbe) CompleteUpload(context.Context, CompleteUploadRecord) (Asset, error) {
	return Asset{}, nil
}
func (*accessStoreProbe) AbortUpload(context.Context, int64, uuid.UUID) (UploadSession, error) {
	return UploadSession{}, nil
}
func (*accessStoreProbe) ListAssets(context.Context, AssetListRequest) (AssetPage, error) {
	return AssetPage{}, nil
}
func (s *accessStoreProbe) GetAsset(context.Context, int64, uuid.UUID) (Asset, error) {
	return s.asset, nil
}
func (*accessStoreProbe) UpdateAsset(context.Context, UpdateAssetRequest) (Asset, error) {
	return Asset{}, nil
}
func (*accessStoreProbe) DeleteAsset(context.Context, DeleteAssetRequest) (Asset, error) {
	return Asset{}, nil
}
func (s *accessStoreProbe) ListReadyDerivatives(context.Context, int64, uuid.UUID) ([]AssetDerivative, error) {
	return s.derivatives, nil
}
func (*accessStoreProbe) RetryAssetProcessing(context.Context, int64, uuid.UUID) (Asset, error) {
	return Asset{}, nil
}

func TestAssetObjectUsesOriginalForLegacyImageThumbnailWithoutDerivative(t *testing.T) {
	assetID := uuid.New()
	legacyID := uuid.New()
	store := &accessStoreProbe{asset: Asset{
		ID: assetID, UserID: 7, LegacyImageID: &legacyID, MediaType: domainmedia.MediaTypeImage,
		StorageConfigID: "storage", StorageDriver: "s3", Bucket: "private", ObjectKey: "legacy/original.png", MIMEType: "image/png",
	}}

	_, object, err := NewService(store, nil, Options{}).assetObject(t.Context(), 7, assetID, AccessPurposeThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if object.ObjectKey != "legacy/original.png" || object.MIMEType != "image/png" {
		t.Fatalf("legacy thumbnail fallback must use the existing private original: %#v", object)
	}
}

func TestAssetObjectKeepsDerivativeRequirementForNewImageThumbnail(t *testing.T) {
	assetID := uuid.New()
	store := &accessStoreProbe{asset: Asset{ID: assetID, UserID: 7, MediaType: domainmedia.MediaTypeImage, ObjectKey: "new/original.png", MIMEType: "image/png"}}

	if _, _, err := NewService(store, nil, Options{}).assetObject(t.Context(), 7, assetID, AccessPurposeThumbnail); err == nil {
		t.Fatal("new image thumbnails must not bypass derivative processing")
	}
}

func TestAssetObjectPrefersLegacyImageThumbnailDerivativeWhenReady(t *testing.T) {
	assetID := uuid.New()
	legacyID := uuid.New()
	store := &accessStoreProbe{
		asset:       Asset{ID: assetID, UserID: 7, LegacyImageID: &legacyID, MediaType: domainmedia.MediaTypeImage, ObjectKey: "legacy/original.png", MIMEType: "image/png"},
		derivatives: []AssetDerivative{{Kind: domainmedia.DerivativeThumbnail640, Status: "ready", ObjectKey: "legacy/thumb.webp", MIMEType: "image/webp"}},
	}

	_, object, err := NewService(store, nil, Options{}).assetObject(t.Context(), 7, assetID, AccessPurposeThumbnail)
	if err != nil {
		t.Fatal(err)
	}
	if object.ObjectKey != "legacy/thumb.webp" {
		t.Fatalf("ready thumbnail must remain preferred, got %#v", object)
	}
}

func TestAssetObjectNeverFallsBackToOriginalVideoForPreview(t *testing.T) {
	assetID := uuid.New()
	store := &accessStoreProbe{asset: Asset{
		ID: assetID, UserID: 7, MediaType: domainmedia.MediaTypeVideo,
		ObjectKey: "media/original/clip.mp4", MIMEType: "video/mp4",
	}}

	if _, _, err := NewService(store, nil, Options{}).assetObject(t.Context(), 7, assetID, AccessPurposePreview); err == nil {
		t.Fatal("video preview must require an MP4 proxy")
	}
}

func TestAssetObjectUsesVideoProxyForPreviewWhenReady(t *testing.T) {
	assetID := uuid.New()
	store := &accessStoreProbe{
		asset:       Asset{ID: assetID, UserID: 7, MediaType: domainmedia.MediaTypeVideo, ObjectKey: "media/original/clip.mp4", MIMEType: "video/mp4"},
		derivatives: []AssetDerivative{{Kind: domainmedia.DerivativeProxy, Status: "ready", ObjectKey: "media/derivatives/proxy.mp4", MIMEType: "video/mp4"}},
	}

	_, object, err := NewService(store, nil, Options{}).assetObject(t.Context(), 7, assetID, AccessPurposePreview)
	if err != nil {
		t.Fatal(err)
	}
	if object.ObjectKey != "media/derivatives/proxy.mp4" {
		t.Fatalf("video preview must use proxy, got %#v", object)
	}
}

func TestAssetObjectDoesNotUseVideoProxyAsPosterFallback(t *testing.T) {
	assetID := uuid.New()
	store := &accessStoreProbe{
		asset:       Asset{ID: assetID, UserID: 7, MediaType: domainmedia.MediaTypeVideo, ObjectKey: "media/original/clip.mp4", MIMEType: "video/mp4"},
		derivatives: []AssetDerivative{{Kind: domainmedia.DerivativeProxy, Status: "ready", ObjectKey: "media/derivatives/proxy.mp4", MIMEType: "video/mp4"}},
	}
	if _, _, err := NewService(store, nil, Options{}).assetObject(t.Context(), 7, assetID, AccessPurposePoster); err == nil {
		t.Fatal("poster access must not fall back to a video proxy")
	}
}
