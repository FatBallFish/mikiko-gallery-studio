package handlers

import (
	"net/http"
	"strings"
	"time"

	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

type mediaAccessPayload struct {
	URL       string     `json:"url"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (a *API) HandleImageAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	purpose, appErr := validatedMediaAccessPurpose(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	imageID := accessResourceID(r.URL.Path, "/api/agent/image/v1/images/")
	access, err := a.tasks.RefreshImageResultAccess(r.Context(), user.ID, imageID, purpose)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	writeMediaAccessSuccess(w, r, access)
}

func (a *API) HandleReferenceAssetAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	purpose, appErr := validatedMediaAccessPurpose(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	assetID := accessResourceID(r.URL.Path, "/api/agent/image/v1/reference-assets/")
	access, err := a.assets.RefreshAccess(r.Context(), user.ID, assetID, purpose)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	writeMediaAccessSuccess(w, r, access)
}

func (a *API) HandleOpenGalleryImageAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", true) {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery is disabled"))
		return
	}
	purpose, appErr := validatedMediaAccessPurpose(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	imageID := accessResourceID(r.URL.Path, "/api/open/image/v1/gallery/images/")
	access, err := a.tasks.RefreshPublicImageAccess(r.Context(), imageID, purpose)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	writeMediaAccessSuccess(w, r, access)
}

func mediaAccessPurpose(r *http.Request) string {
	purpose := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("purpose")))
	if purpose == "" {
		return storage.TemporaryMediaPurposePreview
	}
	return purpose
}

func validatedMediaAccessPurpose(r *http.Request) (string, *errs.Error) {
	purpose := mediaAccessPurpose(r)
	if !mediaassetservice.ValidAccessPurpose(purpose) {
		return "", errs.BadRequest("purpose must be thumbnail, poster, hover, preview, waveform, content or download")
	}
	return purpose, nil
}

func accessResourceID(path, prefix string) string {
	return strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/access")
}

func newMediaAccessPayload(access storage.TemporaryMediaAccess) mediaAccessPayload {
	payload := mediaAccessPayload{URL: access.URL}
	if !access.ExpiresAt.IsZero() {
		expiresAt := access.ExpiresAt.UTC()
		payload.ExpiresAt = &expiresAt
	}
	return payload
}

func writeMediaAccessSuccess(w http.ResponseWriter, r *http.Request, access storage.TemporaryMediaAccess) {
	w.Header().Set("Cache-Control", "private, no-store")
	httpx.WriteSuccess(w, r, http.StatusOK, newMediaAccessPayload(access))
}
