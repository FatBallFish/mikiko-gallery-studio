package middleware

import (
	"net/http"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
)

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observability.DefaultMetrics().IncHTTPRequest()
		next.ServeHTTP(w, r)
	})
}
