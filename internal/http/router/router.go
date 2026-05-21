package router

import (
	"net/http"
	stdpprof "net/http/pprof"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/http/middleware"
)

func New() http.Handler {
	return newMux(nil)
}

func NewWithAPI(api *handlers.API) http.Handler {
	return newMux(api)
}

func newMux(api *handlers.API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.Root)
	mux.HandleFunc("/healthz", handlers.Healthz)
	mux.HandleFunc("/readyz", handlers.Readyz)
	mux.Handle("/metrics", handlers.Metrics())
	mux.HandleFunc("/debug/pprof/", stdpprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", stdpprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", stdpprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", stdpprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", stdpprof.Trace)
	if api != nil {
		mux.HandleFunc("/v1/images/generations", api.HandleOpenAIImageGeneration)
		mux.HandleFunc("/v1/images/edits", api.HandleOpenAIImageEdit)
		mux.HandleFunc("/v1/models", api.HandleOpenAIModels)
		mux.HandleFunc("/api/agent/auth/v1/email/send-code", api.HandleSendEmailCode)
		mux.HandleFunc("/api/agent/auth/v1/login/email-code", api.HandleEmailCodeLogin)
		mux.HandleFunc("/api/agent/auth/v1/session/refresh", api.HandleRefresh)
		mux.HandleFunc("/api/agent/auth/v1/logout", api.HandleLogout)
		mux.HandleFunc("/api/agent/auth/v1/password/change", api.HandleChangePassword)
		mux.HandleFunc("/api/agent/auth/v1/password/reset", api.HandleResetPassword)
		mux.HandleFunc("/api/agent/user/v1/profile", api.HandleProfile)
		mux.HandleFunc("/api/agent/user/v1/preferences", api.HandlePreferences)
		mux.HandleFunc("/api/agent/user/v1/avatar", api.HandleAvatar)
		mux.HandleFunc("/api/agent/billing/v1/balance", api.HandleBalance)
		mux.HandleFunc("/api/agent/billing/v1/ledger", api.HandleLedger)
		mux.HandleFunc("/api/agent/billing/v1/redeem-codes/redeem", api.HandleRedeemCode)
		mux.HandleFunc("/api/agent/developer/v1/api-keys", api.HandleAPIKeys)
		mux.HandleFunc("/api/agent/developer/v1/api-keys/", api.HandleAPIKeyDetail)
		mux.HandleFunc("/api/agent/image/v1/capabilities", api.HandleCapabilities)
		mux.HandleFunc("/api/agent/billing/v1/estimate", api.HandleEstimate)
		mux.HandleFunc("/api/agent/image/v1/reference-assets", api.HandleReferenceAssetUpload)
		mux.HandleFunc("/api/agent/image/v1/reference-assets/", api.HandleReferenceAssetGet)
		mux.HandleFunc("/api/agent/image/v1/images/", api.HandleImageDownload)
		mux.HandleFunc("/api/agent/image/v1/tasks", api.HandleAgentTasks)
		mux.HandleFunc("/api/agent/image/v1/tasks/", api.HandleAgentTaskDetail)
		mux.HandleFunc("/api/agent/image/v1/history/tasks", api.HandleAgentHistoryTasks)
		mux.HandleFunc("/api/agent/image/v1/history/tasks/", api.HandleAgentHistoryTaskDetail)
		mux.HandleFunc("/api/open/image/v1/reference-assets/uploads", api.HandleOpenReferenceAssetUploadSession)
		mux.HandleFunc("/api/open/image/v1/reference-assets", api.HandleOpenReferenceAssetMultipart)
		mux.HandleFunc("/api/open/image/v1/reference-assets/", api.HandleOpenReferenceAssetGet)
		mux.HandleFunc("/api/open/image/v1/tasks", api.HandleOpenTasks)
		mux.HandleFunc("/api/open/image/v1/tasks/", api.HandleOpenTaskDetail)
		mux.HandleFunc("/api/open/image/v1/balance", api.HandleOpenBalance)
		mux.HandleFunc("/api/open/image/v1/capabilities", api.HandleOpenCapabilities)
		mux.HandleFunc("/api/open/image/v1/estimate", api.HandleOpenEstimate)
		mux.HandleFunc("/api/ops/admin/v1/auth/login", api.HandleAdminLogin)
		mux.HandleFunc("/api/ops/admin/v1/auth/logout", api.HandleAdminLogout)
		mux.HandleFunc("/api/ops/admin/v1/audit-logs", api.HandleAdminAuditLogs)
		mux.HandleFunc("/api/ops/admin/v1/call-records", api.HandleAdminCallRecords)
		mux.HandleFunc("/api/ops/admin/v1/config-tabs", api.HandleAdminConfigTabs)
		mux.HandleFunc("/api/ops/admin/v1/config-tabs/", api.HandleAdminConfigTabDetail)
		mux.HandleFunc("/docs/openapi.yaml", api.HandleDocsOpenAPIYAML)
		mux.HandleFunc("/docs/openapi.json", api.HandleDocsOpenAPIJSON)
		mux.HandleFunc("/docs/examples", api.HandleDocsExamples)
		mux.HandleFunc("/docs/errors", api.HandleDocsErrors)
	}

	handler := middleware.Recovery(mux)
	handler = middleware.RequestID(handler)
	handler = middleware.Metrics(handler)
	handler = observability.RequestLogger(handler)
	return handler
}
