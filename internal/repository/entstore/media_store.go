package entstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/mediauploadsession"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/project"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type MediaStore struct{ client *repoent.Client }

func NewMediaStore(client *repoent.Client) *MediaStore { return &MediaStore{client: client} }

func (s *MediaStore) FindUploadByIdempotency(ctx context.Context, userID int64, key string) (mediaassetservice.UploadSession, bool, error) {
	entity, err := s.client.MediaUploadSession.Query().Where(
		mediauploadsession.UserIDEQ(userID), mediauploadsession.IdempotencyKeyEQ(strings.TrimSpace(key)),
	).Only(ctx)
	if repoent.IsNotFound(err) {
		return mediaassetservice.UploadSession{}, false, nil
	}
	if err != nil {
		return mediaassetservice.UploadSession{}, false, err
	}
	return mapMediaUploadSession(entity), true, nil
}

func (s *MediaStore) CreateUpload(ctx context.Context, req mediaassetservice.CreateUploadRecord) (mediaassetservice.UploadSession, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.UploadSession, error) {
		session := req.Session
		projectExists, err := tx.Project.Query().Where(
			project.IDEQ(session.ProjectID), project.UserIDEQ(session.UserID), project.StatusEQ("active"), project.DeletedAtIsNil(),
		).Exist(ctx)
		if err != nil {
			return mediaassetservice.UploadSession{}, err
		}
		if !projectExists {
			return mediaassetservice.UploadSession{}, errs.New(404, errs.CodeNotFound, "project not found")
		}
		if req.QuotaBytes > 0 {
			usedBytes, err := activeMediaBytes(ctx, tx, session.UserID)
			if err != nil {
				return mediaassetservice.UploadSession{}, err
			}
			if session.ReservedBytes > req.QuotaBytes || usedBytes > req.QuotaBytes-session.ReservedBytes {
				return mediaassetservice.UploadSession{}, errs.New(409, errs.CodeConflict, "media storage quota exceeded")
			}
		}
		storageConfigID := parseOptionalUUID(session.StorageConfigID)
		builder := tx.MediaUploadSession.Create().
			SetID(session.ID).
			SetUserID(session.UserID).
			SetProjectID(session.ProjectID).
			SetGroupName(session.GroupName).
			SetOriginalFilename(session.OriginalFilename).
			SetDeclaredMediaType(string(session.DeclaredMediaType)).
			SetDeclaredMimeType(session.DeclaredMIMEType).
			SetDeclaredSizeBytes(session.DeclaredSizeBytes).
			SetStorageDriver(session.StorageDriver).
			SetBucket(session.Bucket).
			SetObjectKey(session.ObjectKey).
			SetBackendUploadID(session.BackendUploadID).
			SetPartSize(session.PartSize).
			SetPartCount(session.PartCount).
			SetStatus(session.Status).
			SetReservedBytes(session.ReservedBytes).
			SetIdempotencyKey(session.IdempotencyKey).
			SetRequestFingerprint(session.RequestFingerprint).
			SetCompletedParts(completedPartsJSON(session.CompletedParts)).
			SetExpiresAt(session.ExpiresAt)
		if session.DeclaredChecksum != "" {
			builder.SetDeclaredChecksum(session.DeclaredChecksum)
		}
		if storageConfigID != nil {
			builder.SetStorageConfigID(*storageConfigID)
		}
		created, err := builder.Save(ctx)
		if err != nil {
			return mediaassetservice.UploadSession{}, err
		}
		return mapMediaUploadSession(created), nil
	})
}

func (s *MediaStore) GetUpload(ctx context.Context, userID int64, id uuid.UUID) (mediaassetservice.UploadSession, error) {
	entity, err := s.client.MediaUploadSession.Query().Where(mediauploadsession.IDEQ(id), mediauploadsession.UserIDEQ(userID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return mediaassetservice.UploadSession{}, errs.New(404, errs.CodeNotFound, "upload session not found")
	}
	if err != nil {
		return mediaassetservice.UploadSession{}, err
	}
	return mapMediaUploadSession(entity), nil
}

func (s *MediaStore) RecordCompletedParts(ctx context.Context, userID int64, id uuid.UUID, parts []storage.CompletedPart) (mediaassetservice.UploadSession, error) {
	entity, err := s.client.MediaUploadSession.Query().Where(mediauploadsession.IDEQ(id), mediauploadsession.UserIDEQ(userID)).Only(ctx)
	if repoent.IsNotFound(err) {
		return mediaassetservice.UploadSession{}, errs.New(404, errs.CodeNotFound, "upload session not found")
	}
	if err != nil {
		return mediaassetservice.UploadSession{}, err
	}
	if entity.Status == "completed" || entity.Status == "aborted" {
		return mapMediaUploadSession(entity), nil
	}
	updated, err := entity.Update().SetStatus("uploading").SetCompletedParts(completedPartsJSON(parts)).Save(ctx)
	if err != nil {
		return mediaassetservice.UploadSession{}, err
	}
	return mapMediaUploadSession(updated), nil
}

func (s *MediaStore) MarkUploadCompleting(ctx context.Context, userID int64, id uuid.UUID, parts []storage.CompletedPart) (mediaassetservice.UploadSession, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.UploadSession, error) {
		entity, err := tx.MediaUploadSession.Query().Where(mediauploadsession.IDEQ(id), mediauploadsession.UserIDEQ(userID)).Only(ctx)
		if repoent.IsNotFound(err) {
			return mediaassetservice.UploadSession{}, errs.New(404, errs.CodeNotFound, "upload session not found")
		}
		if err != nil {
			return mediaassetservice.UploadSession{}, err
		}
		if entity.Status == "completed" {
			return mapMediaUploadSession(entity), nil
		}
		if entity.Status == "aborted" || entity.Status == "expired" {
			return mediaassetservice.UploadSession{}, errs.New(409, errs.CodeConflict, "upload session is not completable")
		}
		updated, err := tx.MediaUploadSession.UpdateOne(entity).SetStatus("completing").SetCompletedParts(completedPartsJSON(parts)).Save(ctx)
		if err != nil {
			return mediaassetservice.UploadSession{}, err
		}
		return mapMediaUploadSession(updated), nil
	})
}

func (s *MediaStore) CompleteUpload(ctx context.Context, req mediaassetservice.CompleteUploadRecord) (mediaassetservice.Asset, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.Asset, error) {
		session, err := tx.MediaUploadSession.Query().Where(
			mediauploadsession.IDEQ(req.SessionID), mediauploadsession.UserIDEQ(req.UserID),
		).Only(ctx)
		if repoent.IsNotFound(err) {
			return mediaassetservice.Asset{}, errs.New(404, errs.CodeNotFound, "upload session not found")
		}
		if err != nil {
			return mediaassetservice.Asset{}, err
		}
		if session.Status == "completed" && session.AssetID != nil {
			assetEntity, err := tx.MediaAsset.Get(ctx, *session.AssetID)
			if err != nil {
				return mediaassetservice.Asset{}, err
			}
			return mapMediaAsset(assetEntity), nil
		}
		if session.Status != "completing" {
			return mediaassetservice.Asset{}, errs.New(409, errs.CodeConflict, "upload session was not prepared for completion")
		}
		if req.Completed.SizeBytes != session.DeclaredSizeBytes {
			return mediaassetservice.Asset{}, errs.BadRequest("completed object size does not match declaration")
		}
		assetID := req.AssetID
		if assetID == uuid.Nil {
			assetID = uuid.New()
		}
		checksum := strings.ToLower(strings.TrimSpace(req.Completed.SHA256))
		if len(checksum) != 64 {
			checksum = strings.ToLower(strings.TrimSpace(mediaStringValue(session.DeclaredChecksum)))
		}
		if len(checksum) != 64 {
			checksum = ""
		}
		assetBuilder := tx.MediaAsset.Create().
			SetID(assetID).
			SetUserID(session.UserID).
			SetProjectID(session.ProjectID).
			SetName(session.OriginalFilename).
			SetNameKey(strings.ToLower(strings.TrimSpace(session.OriginalFilename))).
			SetGroupName(session.GroupName).
			SetMediaType(session.DeclaredMediaType).
			SetSourceType("local_upload").
			SetStatus("processing").
			SetVisibilityStatus("private").
			SetStorageDriver(session.StorageDriver).
			SetBucket(session.Bucket).
			SetObjectKey(session.ObjectKey).
			SetMimeType(session.DeclaredMimeType).
			SetFileSizeBytes(req.Completed.SizeBytes).
			SetSha256(checksum)
		if session.StorageConfigID != nil {
			assetBuilder.SetStorageConfigID(*session.StorageConfigID)
		}
		assetEntity, err := assetBuilder.Save(ctx)
		if err != nil {
			return mediaassetservice.Asset{}, fmt.Errorf("create media asset: %w", err)
		}
		if _, err := tx.MediaProcessingJob.Create().SetAssetID(assetID).SetJobType("probe").SetTransformVersion(1).SetStatus("pending").Save(ctx); err != nil {
			return mediaassetservice.Asset{}, fmt.Errorf("create media probe job: %w", err)
		}
		if _, err := tx.MediaUploadSession.UpdateOne(session).
			SetStatus("completed").SetReservedBytes(0).SetActualBytes(req.Completed.SizeBytes).
			SetAssetID(assetID).SetCompletedAt(req.CompletedAt).Save(ctx); err != nil {
			return mediaassetservice.Asset{}, fmt.Errorf("complete media upload session: %w", err)
		}
		return mapMediaAsset(assetEntity), nil
	})
}

func (s *MediaStore) AbortUpload(ctx context.Context, userID int64, id uuid.UUID) (mediaassetservice.UploadSession, error) {
	return withSerializableTx(ctx, s.client, func(tx *repoent.Tx) (mediaassetservice.UploadSession, error) {
		entity, err := tx.MediaUploadSession.Query().Where(mediauploadsession.IDEQ(id), mediauploadsession.UserIDEQ(userID)).Only(ctx)
		if repoent.IsNotFound(err) {
			return mediaassetservice.UploadSession{}, errs.New(404, errs.CodeNotFound, "upload session not found")
		}
		if err != nil {
			return mediaassetservice.UploadSession{}, err
		}
		if entity.Status == "aborted" {
			return mapMediaUploadSession(entity), nil
		}
		if entity.Status == "completed" {
			return mediaassetservice.UploadSession{}, errs.New(409, errs.CodeConflict, "completed upload cannot be aborted")
		}
		updated, err := tx.MediaUploadSession.UpdateOne(entity).SetStatus("aborted").SetReservedBytes(0).Save(ctx)
		if err != nil {
			return mediaassetservice.UploadSession{}, err
		}
		return mapMediaUploadSession(updated), nil
	})
}

func activeMediaBytes(ctx context.Context, tx *repoent.Tx, userID int64) (int64, error) {
	assets, err := tx.MediaAsset.Query().Where(mediaasset.UserIDEQ(userID), mediaasset.DeletedAtIsNil(), mediaasset.StatusNEQ("deleted")).All(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, assetEntity := range assets {
		total += assetEntity.FileSizeBytes
	}
	sessions, err := tx.MediaUploadSession.Query().Where(
		mediauploadsession.UserIDEQ(userID), mediauploadsession.StatusIn("initialized", "uploading", "completing"),
	).All(ctx)
	if err != nil {
		return 0, err
	}
	for _, session := range sessions {
		total += session.ReservedBytes
	}
	return total, nil
}

func mapMediaUploadSession(entity *repoent.MediaUploadSession) mediaassetservice.UploadSession {
	parts := make([]storage.CompletedPart, 0, len(entity.CompletedParts))
	payload, _ := json.Marshal(entity.CompletedParts)
	_ = json.Unmarshal(payload, &parts)
	storageConfigID := ""
	if entity.StorageConfigID != nil {
		storageConfigID = entity.StorageConfigID.String()
	}
	return mediaassetservice.UploadSession{
		ID: entity.ID, UserID: entity.UserID, ProjectID: entity.ProjectID, GroupName: entity.GroupName,
		OriginalFilename: entity.OriginalFilename, DeclaredMediaType: domainmedia.MediaType(entity.DeclaredMediaType),
		DeclaredMIMEType: entity.DeclaredMimeType, DeclaredSizeBytes: entity.DeclaredSizeBytes,
		DeclaredChecksum: mediaStringValue(entity.DeclaredChecksum), StorageConfigID: storageConfigID, StorageDriver: entity.StorageDriver,
		Bucket: entity.Bucket, ObjectKey: entity.ObjectKey, BackendUploadID: mediaStringValue(entity.BackendUploadID),
		PartSize: entity.PartSize, PartCount: entity.PartCount, Status: entity.Status, ReservedBytes: entity.ReservedBytes,
		ActualBytes: entity.ActualBytes, IdempotencyKey: entity.IdempotencyKey, RequestFingerprint: entity.RequestFingerprint,
		CompletedParts: parts, AssetID: entity.AssetID, ExpiresAt: entity.ExpiresAt, CompletedAt: entity.CompletedAt,
	}
}

func mapMediaAsset(entity *repoent.MediaAsset) mediaassetservice.Asset {
	storageConfigID := ""
	if entity.StorageConfigID != nil {
		storageConfigID = entity.StorageConfigID.String()
	}
	return mediaassetservice.Asset{
		ID: entity.ID, UserID: entity.UserID, ProjectID: entity.ProjectID, Name: entity.Name, GroupName: entity.GroupName,
		MediaType: domainmedia.MediaType(entity.MediaType), SourceType: entity.SourceType, Status: entity.Status,
		VisibilityStatus: entity.VisibilityStatus, StorageConfigID: storageConfigID, StorageDriver: entity.StorageDriver,
		Bucket: entity.Bucket, ObjectKey: entity.ObjectKey, MIMEType: entity.MimeType, FileSizeBytes: entity.FileSizeBytes, SHA256: entity.Sha256,
	}
}

func completedPartsJSON(parts []storage.CompletedPart) []map[string]any {
	payload, _ := json.Marshal(parts)
	var result []map[string]any
	_ = json.Unmarshal(payload, &result)
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func parseOptionalUUID(value string) *uuid.UUID {
	parsed, err := uuid.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	return &parsed
}

func mediaStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
