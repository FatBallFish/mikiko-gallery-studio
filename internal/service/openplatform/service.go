package openplatform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"

	groupdomain "github.com/fatballfish/pic-gallery/internal/domain/assetgroup"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

func decode(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return errs.BadRequest("invalid operation input: " + err.Error())
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errs.BadRequest("operation input must contain one JSON object")
	}
	return nil
}
func (s *Service) authorize(ctx context.Context, p Principal, operation string) (domainauth.User, error) {
	if s == nil || s.deps.Auth == nil || p.UserID <= 0 {
		return domainauth.User{}, errs.New(401, "UNAUTHORIZED", "authentication required")
	}
	var spec *OperationSpec
	for _, entry := range Specs() {
		if entry.Name == operation {
			copy := entry
			spec = &copy
			break
		}
	}
	if spec == nil {
		return domainauth.User{}, errs.New(404, "NOT_FOUND", "unknown operation")
	}
	allowed := false
	for _, scope := range p.Scopes {
		if scope == spec.Scope {
			allowed = true
		}
	}
	if !allowed {
		return domainauth.User{}, errs.New(403, "FORBIDDEN", "required scope: "+spec.Scope)
	}
	if s.deps.ValidatePrincipal != nil {
		if err := s.deps.ValidatePrincipal(ctx, p); err != nil {
			return domainauth.User{}, err
		}
	}
	user, ok := s.deps.Auth.GetUserByID(p.UserID)
	if !ok || user.Status != "active" {
		return domainauth.User{}, errs.New(401, "UNAUTHORIZED", "account is unavailable")
	}
	return user, nil
}
func keyID(p Principal) (int64, error) {
	if p.APIKeyID == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(p.APIKeyID, 10, 64)
	if err != nil || id <= 0 {
		return 0, errs.New(401, "UNAUTHORIZED", "invalid API key identity")
	}
	return id, nil
}

func (s *Service) Execute(ctx context.Context, p Principal, operation string, input json.RawMessage) (result any, resultErr error) {
	// REST and MCP share this boundary, including project ownership checks in media operations.
	defer func() { resultErr = projectOperationError(resultErr) }()
	user, err := s.authorize(ctx, p, operation)
	if err != nil {
		return nil, err
	}
	if operation == "get_image_result_access" || operation == "get_video_result_access" {
		return s.resultAccess(ctx, p, operation, input)
	}
	switch operation {
	case "get_current_user":
		return map[string]any{"id": user.ID, "email": user.Email, "nickname": user.Nickname, "status": user.Status, "group_code": user.GroupCode}, nil
	case "get_balance":
		if s.deps.Billing == nil {
			return nil, unavailable()
		}
		return s.deps.Billing.GetBalance(ctx, user.ID, user.GroupMultiplier)
	case "list_image_models":
		if s.deps.ImageModels == nil {
			return nil, unavailable()
		}
		return s.deps.ImageModels.ListForGroups(ctx, user.GroupCodes)
	case "list_video_models":
		if s.deps.VideoModels == nil {
			return nil, unavailable()
		}
		return s.deps.VideoModels.ListCapabilities(ctx, user.GroupCodes)
	}
	if strings.Contains(operation, "image_task") {
		return s.imageOperation(ctx, p, user, operation, input)
	}
	if strings.Contains(operation, "video_task") {
		return s.videoOperation(ctx, p, user, operation, input)
	}
	if strings.HasSuffix(operation, "_group") || operation == "list_groups" {
		return s.groupOperation(ctx, user.ID, operation, input)
	}
	if strings.HasSuffix(operation, "_project") || operation == "list_projects" {
		return s.projectOperation(ctx, user.ID, operation, input)
	}
	if operation == "get_asset_access" && s.deps.AssetAccess != nil {
		var in resourceInput
		if err := decode(input, &in); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, errs.BadRequest("invalid resource id")
		}
		return s.deps.AssetAccess(ctx, p, id, in.Purpose)
	}
	if operation == "get_upload_part_url" && s.deps.UploadTarget != nil {
		var in resourceInput
		if err := decode(input, &in); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(in.ID)
		if err != nil {
			return nil, errs.BadRequest("invalid upload id")
		}
		return s.deps.UploadTarget(ctx, p, id, in.PartNumber, in.Checksum)
	}
	return s.mediaOperation(ctx, user.ID, operation, input)
}
func projectOperationError(err error) error {
	var app *errs.Error
	var nonEmpty *projectservice.NonEmptyError
	switch {
	case err == nil, errors.As(err, &app):
		return err
	case errors.As(err, &nonEmpty):
		return errs.WithDetails(errs.New(409, errs.CodeProjectNotEmpty, "project contains assets; choose a transfer target"), map[string]any{"counts": nonEmpty.Counts})
	case errors.Is(err, projectservice.ErrDefaultImmutable):
		return errs.New(403, errs.CodeDefaultProjectImmutable, "default project cannot be changed")
	case errors.Is(err, projectservice.ErrProjectChanged):
		return errs.New(409, errs.CodeProjectChanged, "project changed; refresh and retry")
	case errors.Is(err, projectservice.ErrCanvasBusy):
		return errs.New(409, "CANVAS_BUSY", "project contains canvas with active generation runs")
	case errors.Is(err, projectservice.ErrNameConflict):
		return errs.New(409, errs.CodeConflict, "project name already exists")
	case errors.Is(err, projectservice.ErrIdempotencyConflict):
		return errs.New(409, errs.CodeConflict, "idempotency key was used for another project")
	case errors.Is(err, projectservice.ErrNotFound):
		return errs.New(404, errs.CodeNotFound, "project not found")
	case errors.Is(err, projectservice.ErrInvalid):
		return errs.BadRequest("invalid project request")
	default:
		return err
	}
}

func unavailable() error { return errs.Internal("operation service is unavailable") }

type resourceInput struct {
	GroupID         *uuid.UUID              `json:"group_id"`
	ID              string                  `json:"id"`
	ProjectID       *uuid.UUID              `json:"project_id"`
	Name            *string                 `json:"name"`
	GroupName       *string                 `json:"group_name"`
	ExpectedVersion int64                   `json:"expected_version"`
	TargetProjectID string                  `json:"target_project_id"`
	IdempotencyKey  string                  `json:"idempotency_key"`
	Purpose         string                  `json:"purpose"`
	MediaType       domainmedia.MediaType   `json:"media_type"`
	SourceType      string                  `json:"source_type"`
	Status          string                  `json:"status"`
	Keyword         string                  `json:"keyword"`
	SortBy          string                  `json:"sort_by"`
	SortOrder       string                  `json:"sort_order"`
	Cursor          string                  `json:"cursor"`
	Limit           int                     `json:"limit"`
	Filename        string                  `json:"filename"`
	MIMEType        string                  `json:"mime_type"`
	SizeBytes       int64                   `json:"size_bytes"`
	Checksum        string                  `json:"checksum"`
	PartNumber      int                     `json:"part_number"`
	Parts           []storage.CompletedPart `json:"parts"`
}

func (s *Service) projectOperation(ctx context.Context, userID int64, op string, raw json.RawMessage) (any, error) {
	if s.deps.Projects == nil {
		return nil, unavailable()
	}
	var in resourceInput
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	switch op {
	case "list_projects":
		items, err := s.deps.Projects.List(ctx, userID)
		return map[string]any{"items": items}, err
	case "get_project":
		return s.deps.Projects.ResolveOwned(ctx, userID, in.ID)
	case "create_project":
		return s.deps.Projects.Create(ctx, userID, domainproject.CreateRequest{Name: name, IdempotencyKey: in.IdempotencyKey})
	case "update_project":
		return s.deps.Projects.Rename(ctx, userID, in.ID, domainproject.RenameRequest{Name: name, ExpectedVersion: in.ExpectedVersion})
	case "delete_project":
		return s.deps.Projects.Delete(ctx, userID, in.ID, domainproject.DeleteRequest{TargetProjectID: in.TargetProjectID, ExpectedVersion: in.ExpectedVersion, IdempotencyKey: in.IdempotencyKey})
	}
	return nil, errs.BadRequest("unknown project operation")
}
func (s *Service) mediaOperation(ctx context.Context, userID int64, op string, raw json.RawMessage) (any, error) {
	if s.deps.Assets == nil {
		return nil, unavailable()
	}
	var in resourceInput
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	if in.ProjectID != nil {
		if s.deps.Projects == nil {
			return nil, unavailable()
		}
		if _, err := s.deps.Projects.ResolveOwned(ctx, userID, in.ProjectID.String()); err != nil {
			return nil, err
		}
	}
	id := uuid.Nil
	if in.ID != "" {
		var err error
		id, err = uuid.Parse(in.ID)
		if err != nil {
			return nil, errs.BadRequest("invalid resource id")
		}
	}
	group := ""
	if in.GroupName != nil {
		group = *in.GroupName
	}
	switch op {
	case "list_assets":
		return s.deps.Assets.ListAssets(ctx, mediaasset.AssetListRequest{UserID: userID, ProjectID: in.ProjectID, MediaType: in.MediaType, SourceType: in.SourceType, GroupName: group, Status: in.Status, Keyword: in.Keyword, SortBy: in.SortBy, SortOrder: in.SortOrder, Cursor: in.Cursor, Limit: in.Limit})
	case "get_asset":
		return s.deps.Assets.GetAsset(ctx, userID, id)
	case "update_asset":
		return s.deps.Assets.UpdateAsset(ctx, mediaasset.UpdateAssetRequest{UserID: userID, AssetID: id, Name: in.Name, GroupName: in.GroupName, GroupID: in.GroupID, ProjectID: in.ProjectID, ExpectedVersion: in.ExpectedVersion})
	case "delete_asset":
		return s.deps.Assets.DeleteAsset(ctx, mediaasset.DeleteAssetRequest{UserID: userID, AssetID: id, ExpectedVersion: in.ExpectedVersion})
	case "get_asset_access":
		return s.deps.Assets.Access(ctx, userID, id, in.Purpose)
	case "create_upload":
		if in.GroupID != nil && *in.GroupID != uuid.Nil {
			if in.GroupName != nil {
				return nil, errs.BadRequest("group_id and group_name cannot both be set")
			}
			if s.deps.Groups == nil {
				return nil, unavailable()
			}
			projectID := ""
			if in.ProjectID != nil {
				projectID = in.ProjectID.String()
			}
			selected, err := s.deps.Groups.Get(ctx, userID, projectID, in.GroupID.String())
			if err != nil {
				return nil, err
			}
			group = selected.Name
		}
		if in.ProjectID == nil {
			return nil, errs.BadRequest("project_id required")
		}
		return s.deps.Assets.InitUpload(ctx, mediaasset.InitUploadRequest{UserID: userID, ProjectID: *in.ProjectID, GroupName: group, Filename: in.Filename, MediaType: in.MediaType, MIMEType: in.MIMEType, SizeBytes: in.SizeBytes, Checksum: in.Checksum, IdempotencyKey: in.IdempotencyKey})
	case "get_upload":
		return s.deps.Assets.Status(ctx, userID, id)
	case "get_upload_part_url":
		return s.deps.Assets.SignPart(ctx, userID, id, in.PartNumber, in.Checksum)
	case "complete_upload":
		return s.deps.Assets.CompleteUpload(ctx, userID, id, in.Parts)
	case "abort_upload":
		err := s.deps.Assets.AbortUpload(ctx, userID, id)
		return map[string]any{"aborted": err == nil}, err
	}
	return nil, errs.BadRequest("unknown operation")
}
func (s *Service) UploadPart(ctx context.Context, p Principal, id uuid.UUID, number int, body io.Reader, size int64, checksum string) (storage.CompletedPart, error) {
	if _, err := s.authorize(ctx, p, "get_upload_part_url"); err != nil {
		return storage.CompletedPart{}, err
	}
	if number < 1 || size <= 0 || size > 32<<20 {
		return storage.CompletedPart{}, errs.BadRequest("positive part number and Content-Length up to 32 MiB required")
	}
	checksum = strings.TrimSpace(checksum)
	decoded, decodeErr := hex.DecodeString(checksum)
	if decodeErr != nil || len(decoded) != 32 {
		decoded, decodeErr = base64.StdEncoding.Strict().DecodeString(checksum)
		if decodeErr != nil || len(decoded) != 32 {
			return storage.CompletedPart{}, errs.BadRequest("X-Checksum-SHA256 must be 64 hex characters or base64 encoded 32 bytes")
		}
	}
	if s.deps.Assets == nil {
		return storage.CompletedPart{}, unavailable()
	}
	part, err := s.deps.Assets.UploadPart(ctx, p.UserID, id, number, body, size, checksum)
	if errors.Is(err, storage.ErrMultipartChecksum) || errors.Is(err, storage.ErrSizeMismatch) {
		return storage.CompletedPart{}, errs.BadRequest("upload part length or checksum does not match")
	}
	return part, err
}

func (s *Service) groupOperation(ctx context.Context, userID int64, op string, raw json.RawMessage) (any, error) {
	if s.deps.Groups == nil {
		return nil, unavailable()
	}
	var in resourceInput
	if err := decode(raw, &in); err != nil {
		return nil, err
	}
	projectID := ""
	if in.ProjectID != nil {
		projectID = in.ProjectID.String()
	}
	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	switch op {
	case "list_groups":
		items, err := s.deps.Groups.List(ctx, userID, projectID)
		return map[string]any{"items": items}, err
	case "get_group":
		return s.deps.Groups.Get(ctx, userID, projectID, in.ID)
	case "create_group":
		return s.deps.Groups.Create(ctx, userID, projectID, groupdomain.CreateRequest{Name: name})
	case "update_group":
		return s.deps.Groups.Update(ctx, userID, projectID, in.ID, groupdomain.UpdateRequest{Name: name, ExpectedVersion: in.ExpectedVersion})
	case "delete_group":
		err := s.deps.Groups.Delete(ctx, userID, projectID, in.ID, groupdomain.DeleteRequest{ExpectedVersion: in.ExpectedVersion})
		return map[string]any{"deleted": err == nil}, err
	}
	return nil, errs.BadRequest("unknown group operation")
}
