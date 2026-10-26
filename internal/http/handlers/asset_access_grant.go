package handlers

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/service/assetaccess"
	"github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

// NewAssetAccessGrantHandler serves only resource capabilities, without browser,
// API-key, or OAuth bearer authentication. Mount assetaccess.PathPrefix BEFORE
// the general /api/open/v1/ authenticated adapter. Both /{grant} and
// /{grant}/content are supported; purpose/asset cannot be overridden by query.
func NewAssetAccessGrantHandler(service *assetaccess.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			writeMethodNotAllowed(w, r)
			return
		}
		if service == nil {
			httpx.WriteError(w, r, errs.New(503, errs.CodeArtifactStorageUnavailable, "asset access unavailable"))
			return
		}
		token := strings.TrimPrefix(r.URL.Path, assetaccess.PathPrefix)
		token = strings.TrimSuffix(token, "/content")
		if !strings.HasPrefix(r.URL.Path, assetaccess.PathPrefix) || token == "" || strings.Contains(token, "/") || r.URL.RawQuery != "" {
			httpx.WriteError(w, r, errs.New(403, errs.CodeForbidden, "invalid asset access grant"))
			return
		}
		content, err := service.OpenContent(r.Context(), token, r.Header.Get("Range"), r.Method == http.MethodHead)
		if err != nil {
			if content.Reader != nil {
				_ = content.Reader.Close()
			}
			if errors.Is(err, storage.ErrInvalidRange) {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", content.Size))
				httpx.WriteError(w, r, errs.New(416, errs.CodeBadRequest, "requested range is not satisfiable"))
				return
			}
			if errors.Is(err, storage.ErrNotFound) {
				httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "asset content not found"))
				return
			}
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if content.Reader != nil {
			defer content.Reader.Close()
		}
		disposition := "inline"
		if content.Purpose == mediaasset.AccessPurposeDownload {
			disposition = "attachment"
		}
		if content.ContentType == "" {
			content.ContentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", content.ContentType)
		w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": content.Filename}))
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Length", strconv.FormatInt(content.End-content.Start+1, 10))
		status := http.StatusOK
		if content.Partial {
			status = http.StatusPartialContent
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", content.Start, content.End, content.Size))
		}
		w.WriteHeader(status)
		if r.Method == http.MethodGet && content.Reader != nil {
			_, _ = io.CopyN(w, content.Reader, content.End-content.Start+1)
		}
	})
}
