package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	galleryexportservice "github.com/fatballfish/pic-gallery/internal/service/galleryexport"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const mediaAssetPathPrefix = "/api/agent/media/v1/assets/"

type mediaAssetBatchItemRequest struct {
	ID              string `json:"id"`
	ExpectedVersion int64  `json:"expected_version"`
}

type mediaAssetBatchItemResult struct {
	ID     string                          `json:"id"`
	Status string                          `json:"status"`
	Asset  any                             `json:"asset,omitempty"`
	Access *mediaassetservice.AccessResult `json:"access,omitempty"`
	Error  *errs.Error                     `json:"error,omitempty"`
}

func (a *API) HandleMediaAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.mediaAssets == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeArtifactStorageUnavailable, "media assets are unavailable"))
		return
	}
	request := mediaassetservice.AssetListRequest{
		UserID: user.ID, MediaType: domainmedia.MediaType(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("media_type")))),
		SourceType: r.URL.Query().Get("source_type"), GroupName: r.URL.Query().Get("group_name"), Status: r.URL.Query().Get("status"),
		Keyword: r.URL.Query().Get("keyword"), SortBy: r.URL.Query().Get("sort_by"), SortOrder: r.URL.Query().Get("sort_order"),
		Cursor: r.URL.Query().Get("cursor"), Limit: mediaQueryInt(r, "limit", 40),
	}
	if projectID := strings.TrimSpace(r.URL.Query().Get("project_id")); projectID != "" {
		parsed, err := uuid.Parse(projectID)
		if err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid project_id"))
			return
		}
		request.ProjectID = &parsed
	}
	if request.MediaType != "" && request.MediaType != domainmedia.MediaTypeImage && request.MediaType != domainmedia.MediaTypeVideo && request.MediaType != domainmedia.MediaTypeAudio {
		httpx.WriteError(w, r, errs.BadRequest("invalid media_type"))
		return
	}
	page, err := a.mediaAssets.ListAssets(r.Context(), request)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, page)
}

func (a *API) HandleMediaAssetDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.mediaAssets == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeArtifactStorageUnavailable, "media assets are unavailable"))
		return
	}
	remainder := strings.TrimPrefix(r.URL.Path, mediaAssetPathPrefix)
	action := ""
	for _, suffix := range []string{"/access", "/content", ":retry-processing"} {
		if strings.HasSuffix(remainder, suffix) {
			action, remainder = suffix, strings.TrimSuffix(remainder, suffix)
			break
		}
	}
	if remainder == "" || strings.Contains(remainder, "/") {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "media asset not found"))
		return
	}
	assetID, err := uuid.Parse(remainder)
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "media asset not found"))
		return
	}
	switch action {
	case "/access":
		a.handleMediaAssetAccess(w, r, user.ID, assetID)
		return
	case "/content":
		a.handleMediaAssetContent(w, r, user.ID, assetID)
		return
	case ":retry-processing":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		asset, err := a.mediaAssets.RetryProcessing(r.Context(), user.ID, assetID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, asset)
		return
	}

	switch r.Method {
	case http.MethodGet:
		asset, err := a.mediaAssets.GetAsset(r.Context(), user.ID, assetID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, asset)
	case http.MethodPatch:
		var body struct {
			Name            *string `json:"name"`
			GroupName       *string `json:"group_name"`
			ProjectID       *string `json:"project_id"`
			ExpectedVersion int64   `json:"expected_version"`
		}
		if err := decodeStrictJSON(r, &body); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		request := mediaassetservice.UpdateAssetRequest{UserID: user.ID, AssetID: assetID, Name: body.Name, GroupName: body.GroupName, ExpectedVersion: body.ExpectedVersion}
		if body.ProjectID != nil {
			parsed, err := uuid.Parse(strings.TrimSpace(*body.ProjectID))
			if err != nil {
				httpx.WriteError(w, r, errs.BadRequest("invalid project_id"))
				return
			}
			request.ProjectID = &parsed
		}
		asset, err := a.mediaAssets.UpdateAsset(r.Context(), request)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, asset)
	case http.MethodDelete:
		var body struct {
			ExpectedVersion int64 `json:"expected_version"`
		}
		if err := decodeStrictJSON(r, &body); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		asset, err := a.mediaAssets.DeleteAsset(r.Context(), mediaassetservice.DeleteAssetRequest{UserID: user.ID, AssetID: assetID, ExpectedVersion: body.ExpectedVersion})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, asset)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleMediaAssetBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.mediaAssets == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeArtifactStorageUnavailable, "media assets are unavailable"))
		return
	}
	var body struct {
		Items           []mediaAssetBatchItemRequest `json:"items"`
		ProjectID       string                       `json:"project_id"`
		GroupName       string                       `json:"group_name"`
		TargetProjectID string                       `json:"target_project_id"`
	}
	if err := decodeStrictJSON(r, &body); err != nil || len(body.Items) == 0 || len(body.Items) > 100 {
		httpx.WriteError(w, r, errs.BadRequest("items must contain between 1 and 100 assets"))
		return
	}
	action := strings.TrimPrefix(r.URL.Path, "/api/agent/media/v1/assets:batch-")
	if action != "group" && action != "transfer-project" && action != "delete" && action != "download" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "media batch route not found"))
		return
	}
	if action == "download" {
		projectID, err := uuid.Parse(strings.TrimSpace(body.ProjectID))
		if err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid project_id"))
			return
		}
		ids := make([]string, 0, len(body.Items))
		for _, item := range body.Items {
			ids = append(ids, item.ID)
		}
		a.handleAgentMediaBatchDownload(w, r, user.ID, projectID.String(), ids)
		return
	}
	var targetProjectID *uuid.UUID
	if action == "transfer-project" {
		parsed, err := uuid.Parse(strings.TrimSpace(body.TargetProjectID))
		if err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid target_project_id"))
			return
		}
		targetProjectID = &parsed
	}
	results := make([]mediaAssetBatchItemResult, 0, len(body.Items))
	for _, item := range body.Items {
		result := mediaAssetBatchItemResult{ID: item.ID, Status: "failed"}
		assetID, err := uuid.Parse(strings.TrimSpace(item.ID))
		if err != nil {
			result.Error = errs.BadRequest("invalid media asset id")
			results = append(results, result)
			continue
		}
		switch action {
		case "group":
			group := body.GroupName
			result.Asset, err = a.mediaAssets.UpdateAsset(r.Context(), mediaassetservice.UpdateAssetRequest{
				UserID: user.ID, AssetID: assetID, GroupName: &group, ExpectedVersion: item.ExpectedVersion,
			})
		case "transfer-project":
			result.Asset, err = a.mediaAssets.UpdateAsset(r.Context(), mediaassetservice.UpdateAssetRequest{
				UserID: user.ID, AssetID: assetID, ProjectID: targetProjectID, ExpectedVersion: item.ExpectedVersion,
			})
		case "delete":
			result.Asset, err = a.mediaAssets.DeleteAsset(r.Context(), mediaassetservice.DeleteAssetRequest{
				UserID: user.ID, AssetID: assetID, ExpectedVersion: item.ExpectedVersion,
			})
		case "download":
			var access mediaassetservice.AccessResult
			access, err = a.mediaAssets.Access(r.Context(), user.ID, assetID, storage.TemporaryMediaPurposeDownload)
			result.Access = &access
		}
		if err != nil {
			result.Asset = nil
			result.Access = nil
			result.Error = normalizeAppError(err)
		} else {
			result.Status = "succeeded"
		}
		results = append(results, result)
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": results})
}

func (a *API) handleAgentMediaBatchDownload(w http.ResponseWriter, r *http.Request, userID int64, projectID string, assetIDs []string) {
	if a.galleryExport == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeInternal, "media export is unavailable"))
		return
	}
	result, err := a.galleryExport.CreateDownload(r.Context(), galleryexportservice.CreateDownloadRequest{
		UserID: userID, ProjectID: projectID, ImageIDs: assetIDs, ForceAsync: true,
	})
	if err != nil {
		switch {
		case errors.Is(err, galleryexportservice.ErrBatchEmpty):
			httpx.WriteError(w, r, errs.BadRequest("items is required"))
		case errors.Is(err, galleryexportservice.ErrBatchTooLarge), errors.Is(err, galleryexportservice.ErrSourceLimitExceeded), errors.Is(err, galleryexportservice.ErrArchiveLimitExceeded):
			httpx.WriteError(w, r, errs.New(http.StatusRequestEntityTooLarge, errs.CodeExportTooLarge, "media export exceeds the configured size limit"))
		case errors.Is(err, repoerr.ErrNotFound):
			httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "one or more media assets were not found"))
		default:
			httpx.WriteError(w, r, normalizeAppError(err))
		}
		return
	}
	if result.Job == nil {
		httpx.WriteError(w, r, errs.Internal("media export did not create an asynchronous job"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, map[string]any{
		"job": result.Job, "status_url": "/api/agent/media/v1/export-jobs/" + result.Job.ID,
	})
}

func (a *API) handleMediaAssetAccess(w http.ResponseWriter, r *http.Request, userID int64, assetID uuid.UUID) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	purpose, appErr := validatedMediaAccessPurpose(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	access, err := a.mediaAssets.Access(r.Context(), userID, assetID, purpose)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteSuccess(w, r, http.StatusOK, access)
}

func (a *API) handleMediaAssetContent(w http.ResponseWriter, r *http.Request, userID int64, assetID uuid.UUID) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	purpose, appErr := validatedMediaAccessPurpose(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	stream, err := a.mediaAssets.OpenContent(r.Context(), userID, assetID, purpose)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	defer stream.Reader.Close()
	w.Header().Set("Content-Type", stream.ContentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, no-store")
	if purpose == mediaassetservice.AccessPurposeDownload {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": stream.Filename}))
	}
	start, end, partial, rangeErr := parseSingleByteRange(r.Header.Get("Range"), stream.SizeBytes)
	if rangeErr != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", stream.SizeBytes))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if start > 0 {
		if _, err := io.CopyN(io.Discard, stream.Reader, start); err != nil {
			return
		}
	}
	length := end - start + 1
	if partial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, stream.SizeBytes))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(stream.SizeBytes, 10))
		w.WriteHeader(http.StatusOK)
	}
	_, _ = io.CopyN(w, stream.Reader, length)
}

func parseSingleByteRange(value string, size int64) (start, end int64, partial bool, err error) {
	if size <= 0 {
		return 0, 0, false, errors.New("empty object")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, size - 1, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, false, errors.New("unsupported range")
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return 0, 0, false, errors.New("invalid range")
	}
	start, err = strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, errors.New("invalid range start")
	}
	end = size - 1
	if strings.TrimSpace(parts[1]) != "" {
		end, err = strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
		if err != nil || end < start {
			return 0, 0, false, errors.New("invalid range end")
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true, nil
}

func decodeStrictJSON(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func mediaQueryInt(r *http.Request, key string, fallback int) int {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
