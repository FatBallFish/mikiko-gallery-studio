package router

import (
	"net/http"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/http/middleware"
)

func NewSetup(api *handlers.SetupAPI, corsAllowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	registerSystemRoutes(mux, api.System())
	mux.HandleFunc("GET /setup", api.HandleSetupPage)
	mux.HandleFunc("POST /api/setup/v1/session", api.HandleSession)
	mux.HandleFunc("POST /api/setup/v1/probes/database", api.HandleDatabaseProbe)
	mux.HandleFunc("POST /api/setup/v1/probes/redis", api.HandleRedisProbe)
	mux.HandleFunc("POST /api/setup/v1/probes/storage", api.HandleStorageProbe)
	mux.HandleFunc("POST /api/setup/v1/apply", api.HandleApply)
	mux.HandleFunc("GET /api/setup/v1/progress/{operation_id}", api.HandleProgress)
	mux.HandleFunc("/", handlers.APINotFound)
	return wrapHandler(mux, corsAllowedOrigins, setupPreflightPolicy)
}

func NewBroken(system *handlers.SystemAPI) http.Handler {
	mux := http.NewServeMux()
	registerSystemRoutes(mux, system)
	mux.HandleFunc("/", handlers.APINotFound)
	return wrapHandler(mux, nil, brokenPreflightPolicy)
}

func setupPreflightPolicy(path, method string) bool {
	if brokenPreflightPolicy(path, method) {
		return true
	}
	switch path {
	case "/setup":
		return method == http.MethodGet
	case "/api/setup/v1/session", "/api/setup/v1/probes/database", "/api/setup/v1/probes/redis", "/api/setup/v1/probes/storage", "/api/setup/v1/apply":
		return method == http.MethodPost
	}
	const progressPrefix = "/api/setup/v1/progress/"
	operationID := strings.TrimPrefix(path, progressPrefix)
	return method == http.MethodGet && strings.HasPrefix(path, progressPrefix) && operationID != "" && !strings.Contains(operationID, "/")
}

func brokenPreflightPolicy(path, method string) bool {
	if method != http.MethodGet {
		return false
	}
	switch path {
	case "/healthz", "/readyz", "/api/system/v1/bootstrap-status":
		return true
	default:
		return false
	}
}

var _ middleware.PreflightPolicy = setupPreflightPolicy
