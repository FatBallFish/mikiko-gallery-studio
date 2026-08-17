package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

const (
	mediaUploadPathPrefix = "/api/agent/media/v1/uploads/"
	mediaUploadPartMax    = int64(32 << 20)
)

func (a *API) HandleMediaUploads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireMediaAssetUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if appErr := a.requireFeature(r.Context(), "media_upload"); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var body struct {
		ProjectID string `json:"project_id"`
		GroupName string `json:"group_name"`
		Filename  string `json:"filename"`
		MediaType string `json:"media_type"`
		MIMEType  string `json:"mime_type"`
		SizeBytes int64  `json:"size_bytes"`
		Checksum  string `json:"checksum"`
	}
	if err := decodeStrictJSON(r, &body); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	projectID, err := uuid.Parse(strings.TrimSpace(body.ProjectID))
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid project_id"))
		return
	}
	mediaType := domainmedia.MediaType(strings.ToLower(strings.TrimSpace(body.MediaType)))
	session, err := a.mediaAssets.InitUpload(r.Context(), mediaassetservice.InitUploadRequest{
		UserID: user.ID, ProjectID: projectID, GroupName: body.GroupName, Filename: body.Filename,
		MediaType: mediaType, MIMEType: body.MIMEType, SizeBytes: body.SizeBytes, Checksum: body.Checksum,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, session)
}

func (a *API) HandleMediaUploadDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireMediaAssetUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	remainder := strings.TrimPrefix(r.URL.Path, mediaUploadPathPrefix)
	if strings.HasSuffix(remainder, ":complete") {
		a.handleMediaUploadComplete(w, r, user.ID, strings.TrimSuffix(remainder, ":complete"))
		return
	}
	if before, after, found := strings.Cut(remainder, "/parts/"); found {
		a.handleMediaUploadPart(w, r, user.ID, before, after)
		return
	}
	sessionID, ok := parseMediaUploadID(remainder)
	if !ok {
		httpx.WriteError(w, r, mediaUploadNotFound())
		return
	}
	switch r.Method {
	case http.MethodGet:
		session, err := a.mediaAssets.Status(r.Context(), user.ID, sessionID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, session)
	case http.MethodDelete:
		if err := a.mediaAssets.AbortUpload(r.Context(), user.ID, sessionID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]string{"status": "aborted"})
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) handleMediaUploadPart(w http.ResponseWriter, r *http.Request, userID int64, sessionValue, partValue string) {
	sessionID, ok := parseMediaUploadID(sessionValue)
	if !ok {
		httpx.WriteError(w, r, mediaUploadNotFound())
		return
	}
	sign := strings.HasSuffix(partValue, ":sign")
	if sign {
		partValue = strings.TrimSuffix(partValue, ":sign")
	}
	partNumber, err := strconv.Atoi(partValue)
	if err != nil || partNumber < 1 || partNumber > 10_000 {
		httpx.WriteError(w, r, errs.BadRequest("invalid part number"))
		return
	}
	if sign {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		var body struct {
			Checksum string `json:"checksum"`
		}
		if err := decodeStrictJSON(r, &body); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		target, err := a.mediaAssets.SignPart(r.Context(), userID, sessionID, partNumber, body.Checksum)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, target)
		return
	}
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w, r)
		return
	}
	if r.ContentLength <= 0 || r.ContentLength > mediaUploadPartMax {
		httpx.WriteError(w, r, errs.BadRequest("invalid upload part size"))
		return
	}
	part, err := a.mediaAssets.UploadPart(r.Context(), userID, sessionID, partNumber, r.Body, r.ContentLength, r.Header.Get("X-Content-SHA256"))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, part)
}

func (a *API) handleMediaUploadComplete(w http.ResponseWriter, r *http.Request, userID int64, sessionValue string) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	sessionID, ok := parseMediaUploadID(sessionValue)
	if !ok {
		httpx.WriteError(w, r, mediaUploadNotFound())
		return
	}
	var body struct {
		Parts []storage.CompletedPart `json:"parts"`
	}
	if err := decodeStrictJSON(r, &body); err != nil || len(body.Parts) == 0 || len(body.Parts) > 10_000 {
		httpx.WriteError(w, r, errs.BadRequest("invalid multipart completion body"))
		return
	}
	asset, err := a.mediaAssets.CompleteUpload(r.Context(), userID, sessionID, body.Parts)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, asset)
}

func (a *API) requireMediaAssetUser(r *http.Request) (*domainauth.User, *errs.Error) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		return nil, appErr
	}
	if a.mediaAssets == nil {
		return nil, errs.New(http.StatusServiceUnavailable, errs.CodeArtifactStorageUnavailable, "media uploads are unavailable")
	}
	return user, nil
}

func parseMediaUploadID(value string) (uuid.UUID, bool) {
	if value == "" || strings.Contains(value, "/") {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(value)
	return parsed, err == nil
}

func mediaUploadNotFound() *errs.Error {
	return errs.New(http.StatusNotFound, errs.CodeNotFound, "media upload not found")
}
