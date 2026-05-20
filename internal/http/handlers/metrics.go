package handlers

import (
	"net/http"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
)

func Metrics() http.Handler {
	return observability.DefaultMetrics().Handler()
}
