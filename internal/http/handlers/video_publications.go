package handlers

import (
	"errors"
	"fmt"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	"github.com/fatballfish/pic-gallery/internal/service/videopublication"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"github.com/google/uuid"
)

func (a *API) SetVideoPublications(service *videopublication.Service) { a.videoPublications = service }
func publicationID(path, prefix string) (uuid.UUID, string, error) {
	rest := strings.TrimPrefix(path, prefix)
	action := ""
	for _, suffix := range []string{":withdraw", ":approve", ":reject", ":unpublish", "/video", "/poster", "/download", "/like", "/favorite"} {
		if strings.HasSuffix(rest, suffix) {
			rest = strings.TrimSuffix(rest, suffix)
			action = suffix
			break
		}
	}
	id, e := uuid.Parse(rest)
	return id, action, e
}
func (a *API) videoPublicationAvailable(w http.ResponseWriter, r *http.Request) bool {
	if a.videoPublications == nil {
		httpx.WriteError(w, r, errs.New(503, errs.CodeInternal, "video publication is unavailable"))
		return false
	}
	return true
}
func (a *API) HandleVideoPublication(w http.ResponseWriter, r *http.Request) {
	user, e := a.requireUser(r)
	if e != nil {
		httpx.WriteError(w, r, e)
		return
	}
	if !a.videoPublicationAvailable(w, r) {
		return
	}
	id, action, err := publicationID(r.URL.Path, "/api/agent/media/v1/video-publications/")
	if err != nil || action != "" && action != ":withdraw" {
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "video not found"))
		return
	}
	var result videopublication.Record
	switch {
	case r.Method == http.MethodGet && action == "":
		result, err = a.videoPublications.Owner(r.Context(), user.ID, id)
	case r.Method == http.MethodPost && action == "":
		result, err = a.videoPublications.Submit(r.Context(), user.ID, id)
	case r.Method == http.MethodPost && action == ":withdraw":
		result, err = a.videoPublications.Withdraw(r.Context(), user.ID, id)
	default:
		writeMethodNotAllowed(w, r)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if r.Method == http.MethodPost {
		a.recordAudit(r, "user", strconv.FormatInt(user.ID, 10), "video.publication"+action, "media_asset", id.String(), map[string]any{"status": result.Status})
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}
func (a *API) HandlePublicVideos(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", true) {
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteSuccess(w, r, http.StatusOK, videopublication.Page{Items: []videopublication.Video{}, Page: 1, PageSize: 20})
		return
	}
	if !a.videoPublicationAvailable(w, r) {
		return
	}
	viewer := int64(0)
	if user := a.optionalUserWithQueryToken(r); user != nil {
		viewer = user.ID
	}
	page, err := a.videoPublications.ListPublic(r.Context(), viewer, parseBoolQuery(r, "liked"), parseBoolQuery(r, "favorited"), r.URL.Query().Get("sort"), mediaQueryInt(r, "page", 1), mediaQueryInt(r, "page_size", 20))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	out := videopublication.Page{Items: make([]videopublication.Video, 0, len(page.Items)), Total: page.Total, Page: page.Page, PageSize: page.PageSize}
	for _, v := range page.Items {
		out.Items = append(out.Items, v.Video)
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, r, http.StatusOK, out)
}
func (a *API) HandlePublicVideoDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", true) {
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery is disabled"))
		return
	}
	if !a.videoPublicationAvailable(w, r) {
		return
	}
	id, action, err := publicationID(r.URL.Path, "/api/public/v1/videos/")
	if err != nil {
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "video not found"))
		return
	}

	if action == "/like" || action == "/favorite" {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		user, err := a.requireUser(r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		var body struct {
			Active *bool `json:"active"`
		}
		if err := decodeStrictJSON(r, &body); err != nil || body.Active == nil {
			httpx.WriteError(w, r, errs.BadRequest("active boolean is required"))
			return
		}
		v, interactionErr := a.videoPublications.SetInteraction(r.Context(), id, user.ID, strings.TrimPrefix(action, "/"), *body.Active)
		if interactionErr != nil {
			httpx.WriteError(w, r, normalizeAppError(interactionErr))
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		httpx.WriteSuccess(w, r, http.StatusOK, v.Video)
		return
	}
	if r.Method == http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if action == "/download" {
		if _, err := a.requireUser(r); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		a.servePublicationContent(w, r, id, false, false)
		return
	}
	if action == "/video" || action == "/poster" {
		a.servePublicationContent(w, r, id, action == "/poster", false)
		return
	}
	if action != "" {
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "video not found"))
		return
	}
	viewer := int64(0)
	if user := a.optionalUserWithQueryToken(r); user != nil {
		viewer = user.ID
	}
	v, err := a.videoPublications.GetForViewer(r.Context(), id, viewer)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteSuccess(w, r, http.StatusOK, v.Video)
}
func (a *API) HandleAdminVideoReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, e := a.requireAdminPermission(r, domainadminauth.PermissionManageReviews); e != nil {
		httpx.WriteError(w, r, e)
		return
	}
	if !a.videoPublicationAvailable(w, r) {
		return
	}
	page, err := a.videoPublications.List(r.Context(), r.URL.Query().Get("status"), mediaQueryInt(r, "page", 1), mediaQueryInt(r, "page_size", 20), true)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, page)
}
func (a *API) HandleAdminVideoReviewDetail(w http.ResponseWriter, r *http.Request) {
	admin, e := a.requireAdminPermissionWithQueryToken(r, domainadminauth.PermissionManageReviews)
	if e != nil {
		httpx.WriteError(w, r, e)
		return
	}
	if !a.videoPublicationAvailable(w, r) {
		return
	}
	id, action, err := publicationID(r.URL.Path, "/api/ops/admin/v1/video-reviews/")
	if err != nil {
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "video not found"))
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if action == "/video" || action == "/poster" {
			a.servePublicationContent(w, r, id, action == "/poster", true)
			return
		}
		writeMethodNotAllowed(w, r)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	status := ""
	switch action {
	case ":approve":
		status = videopublication.Approved
	case ":reject":
		status = videopublication.Rejected
	case ":unpublish":
		status = videopublication.Private
	default:
		httpx.WriteError(w, r, errs.BadRequest("invalid review decision"))
		return
	}
	var body struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeStrictJSON(r, &body); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid review body"))
			return
		}
	}
	if status == videopublication.Rejected && strings.TrimSpace(body.Reason) == "" {
		httpx.WriteError(w, r, errs.BadRequest("rejection reason is required"))
		return
	}
	before, err := a.videoPublications.Get(r.Context(), id, true)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	v, err := a.videoPublications.Review(r.Context(), id, status, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	a.recordAudit(r, "admin", strconv.FormatInt(admin.AdminID, 10), "video.review"+action, "media_asset", id.String(), map[string]any{"previous_status": before.Status, "next_status": v.Status, "reason": v.ReviewReason})
	httpx.WriteSuccess(w, r, http.StatusOK, v)
}
func (a *API) servePublicationContent(w http.ResponseWriter, r *http.Request, id uuid.UUID, poster, admin bool) {
	var stream mediaassetservice.RangedContentStream
	var err error
	filename := ""
	download := strings.HasSuffix(r.URL.Path, "/download") && !admin
	if download {
		stream, filename, err = a.videoPublications.OpenDownloadRange(r.Context(), id, r.Header.Get("Range"), r.Method == http.MethodHead)
	} else {
		stream, err = a.videoPublications.OpenRange(r.Context(), id, poster, admin, r.Header.Get("Range"), r.Method == http.MethodHead)
	}
	w.Header().Set("Cache-Control", "no-store")
	if errors.Is(err, storage.ErrInvalidRange) {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", stream.Size))
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if stream.Reader != nil {
		defer stream.Reader.Close()
	}
	if download {
		ext := ".mp4"
		switch stream.ContentType {
		case "video/webm":
			ext = ".webm"
		case "video/quicktime":
			ext = ".mov"
		case "video/x-matroska":
			ext = ".mkv"
		}
		filename = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(filename, "\r", ""), "\n", ""))
		filename = filepath.Base(strings.ReplaceAll(filename, "\\", "/"))
		if filename == "" || filename == "." || filename == "/" {
			filename = "video"
		}
		if old := filepath.Ext(filename); old != "" {
			filename = strings.TrimSuffix(filename, old)
		}
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename + ext}))
	}
	w.Header().Set("Content-Type", stream.ContentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "no-store")
	start, end, partial := stream.Start, stream.End, stream.Partial
	length := end - start + 1
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	status := http.StatusOK
	if partial {
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, stream.Size))
	}
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = io.CopyN(w, stream.Reader, length)
}
func publicationByteRange(value string, size int64) (int64, int64, bool, error) {
	return storage.ParseByteRange(value, size)
}
