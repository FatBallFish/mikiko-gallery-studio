package handlers

import (
	"net/http"

	"github.com/fatballfish/pic-gallery/internal/service/uploadaccess"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

func NewUploadAccessHandler(service *uploadaccess.Service) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPut {
			w.Header().Set("Allow", http.MethodPut)
			writeOpenError(w, r, errs.New(405, "METHOD_NOT_ALLOWED", "PUT required"))
			return
		}
		if r.URL.RawQuery != "" {
			writeOpenError(w, r, errs.BadRequest("upload capability accepts no query parameters"))
			return
		}
		if r.ContentLength <= 0 || r.ContentLength > uploadaccess.MaximumPartSize {
			writeOpenError(w, r, errs.BadRequest("Content-Length up to 32 MiB required"))
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, r.ContentLength)
		part, err := service.Put(r.Context(), r.Header.Get(uploadaccess.Header), r.Method, r.URL.Path, r.ContentLength, r.Header.Get("X-Checksum-SHA256"), r.Body)
		if err != nil {
			writeOpenError(w, r, err)
			return
		}
		w.Header().Set("ETag", part.ETag)
		httpx.WriteSuccess(w, r, http.StatusOK, part)
	})
}
