package router

import (
	"net/http"

	"github.com/fatballfish/pic-gallery/internal/http/handlers"
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
	return wrapHandler(mux, corsAllowedOrigins)
}

func NewBroken(system *handlers.SystemAPI) http.Handler {
	mux := http.NewServeMux()
	registerSystemRoutes(mux, system)
	mux.HandleFunc("/", handlers.APINotFound)
	return wrapHandler(mux, nil)
}
