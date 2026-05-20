package middleware

import (
	"log/slog"
	"net/http"

	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("request panicked", "request_id", httpx.RequestIDFromContext(r.Context()), "panic", rec)
				httpx.WriteError(w, r, errs.Internal("unexpected server error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
