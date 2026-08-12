package router

import (
	"net/http"
	stdpprof "net/http/pprof"
	"strings"

	openapispec "github.com/fatballfish/pic-gallery/api/openapi"
	"github.com/fatballfish/pic-gallery/internal/app/observability"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/http/middleware"
)

func New() http.Handler {
	return NewNormal(nil, config.Config{})
}

func NewWithAPI(api *handlers.API) http.Handler {
	return NewNormal(api, config.Config{})
}

func NewWithAPIAndConfig(api *handlers.API, cfg config.Config) http.Handler {
	return NewNormal(api, cfg)
}

func NewNormal(api *handlers.API, cfg config.Config) http.Handler {
	return newNormalMux(api, handlers.NewSystemAPI(handlers.BootstrapStatus{Phase: handlers.BootstrapPhaseReady}), cfg.HTTP.CORSAllowedOrigins)
}

func newNormalMux(api *handlers.API, system *handlers.SystemAPI, corsAllowedOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handlers.Root)
	mux.HandleFunc("/setup", handlers.APINotFound)
	mux.HandleFunc("/setup/", handlers.APINotFound)
	mux.HandleFunc("/api/", handlers.APINotFound)
	registerSystemRoutes(mux, system)
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
		mux.HandleFunc("/api/agent/auth/v1/login/password", api.HandlePasswordLogin)
		mux.HandleFunc("/api/agent/auth/v1/password/setup", api.HandlePasswordSetup)
		mux.HandleFunc("/api/agent/auth/v1/session/refresh", api.HandleRefresh)
		mux.HandleFunc("/api/agent/auth/v1/logout", api.HandleLogout)
		mux.HandleFunc("/api/agent/auth/v1/password/change", api.HandleChangePassword)
		mux.HandleFunc("/api/agent/auth/v1/password/reset", api.HandleResetPassword)
		mux.HandleFunc("/api/agent/auth/v1/password/reset/request", api.HandlePasswordResetRequest)
		mux.HandleFunc("/api/agent/auth/v1/password/reset/confirm", api.HandlePasswordResetConfirm)
		mux.HandleFunc("/api/agent/user/v1/profile", api.HandleProfile)
		mux.HandleFunc("/api/agent/user/v1/preferences", api.HandlePreferences)
		mux.HandleFunc("/api/agent/user/v1/avatar", api.HandleAvatar)
		mux.HandleFunc("/api/agent/user/v1/account/close", api.HandleCloseAccount)
		mux.HandleFunc("/api/agent/features/v1", api.HandleAgentFeatures)
		mux.HandleFunc("/api/agent/project/v1/projects", api.HandleProjects)
		mux.HandleFunc("/api/agent/project/v1/projects/", api.HandleProjectDetail)
		mux.HandleFunc("/api/agent/canvas/v1/canvases", api.HandleCanvases)
		mux.HandleFunc("/api/agent/canvas/v1/canvases/", api.HandleCanvasDetail)
		mux.HandleFunc("/api/agent/gallery/v1/images", api.HandleAgentGalleryImages)
		mux.HandleFunc("/api/agent/gallery/v1/images:batch-publish", api.HandleAgentGalleryBatch)
		mux.HandleFunc("/api/agent/gallery/v1/images:batch-group", api.HandleAgentGalleryBatch)
		mux.HandleFunc("/api/agent/gallery/v1/images:batch-delete", api.HandleAgentGalleryBatch)
		mux.HandleFunc("/api/agent/gallery/v1/images:batch-transfer-project", api.HandleAgentGalleryBatch)
		mux.HandleFunc("/api/agent/gallery/v1/images:batch-download", api.HandleAgentGalleryBatch)
		mux.HandleFunc("/api/agent/gallery/v1/export-jobs/", api.HandleAgentGalleryExportJob)
		mux.HandleFunc("/api/agent/gallery/v1/images/", api.HandleAgentGalleryImageDetail)
		mux.HandleFunc("/api/agent/media/v1/assets", api.HandleMediaAssets)
		mux.HandleFunc("/api/agent/media/v1/assets:batch-group", api.HandleMediaAssetBatch)
		mux.HandleFunc("/api/agent/media/v1/assets:batch-transfer-project", api.HandleMediaAssetBatch)
		mux.HandleFunc("/api/agent/media/v1/assets:batch-delete", api.HandleMediaAssetBatch)
		mux.HandleFunc("/api/agent/media/v1/assets:batch-download", api.HandleMediaAssetBatch)
		mux.HandleFunc("/api/agent/media/v1/export-jobs/", api.HandleAgentGalleryExportJob)
		mux.HandleFunc("/api/agent/media/v1/assets/", api.HandleMediaAssetDetail)
		mux.HandleFunc("/api/agent/media/v1/uploads", api.HandleMediaUploads)
		mux.HandleFunc("/api/agent/media/v1/uploads/", api.HandleMediaUploadDetail)
		mux.HandleFunc("/api/agent/video/v1/capabilities", api.HandleVideoCapabilities)
		mux.HandleFunc("/api/agent/video/v1/estimates", api.HandleVideoEstimates)
		mux.HandleFunc("/api/agent/video/v1/tasks", api.HandleVideoTasks)
		mux.HandleFunc("/api/agent/video/v1/tasks/events", api.HandleVideoTaskEvents)
		mux.HandleFunc("/api/agent/video/v1/tasks/", api.HandleVideoTaskDetail)
		mux.HandleFunc("/api/agent/billing/v1/balance", api.HandleBalance)
		mux.HandleFunc("/api/agent/billing/v1/ledger", api.HandleLedger)
		mux.HandleFunc("/api/agent/billing/v1/plans", api.HandleBillingPlans)
		mux.HandleFunc("/api/agent/billing/v1/subscription", api.HandleBillingSubscription)
		mux.HandleFunc("/api/agent/billing/v1/orders", api.HandleBillingOrders)
		mux.HandleFunc("/api/agent/billing/v1/orders/", api.HandleBillingOrderDetail)
		mux.HandleFunc("/api/agent/cashier/v1/options", api.HandleCashierOptions)
		mux.HandleFunc("/api/agent/cashier/v1/orders", api.HandleCashierOrders)
		mux.HandleFunc("/api/agent/cashier/v1/orders/", api.HandleCashierOrderDetail)
		mux.HandleFunc("/api/agent/image/v1/capabilities", api.HandleCapabilities)
		mux.HandleFunc("/api/agent/billing/v1/estimate", api.HandleEstimate)
		mux.HandleFunc("/api/agent/text/v1/prompt-optimizations/estimate", api.HandlePromptOptimizationEstimate)
		mux.HandleFunc("/api/agent/text/v1/prompt-optimizations", api.HandlePromptOptimization)
		mux.HandleFunc("/api/agent/account/v1/api-keys", api.HandleAgentAPIKeys)
		mux.HandleFunc("/api/agent/account/v1/api-keys/", api.HandleAgentAPIKeyDetail)
		mux.HandleFunc("/api/agent/developer/v1/api-keys", api.HandleDeveloperAPIKeys)
		mux.HandleFunc("/api/agent/developer/v1/api-keys/", api.HandleDeveloperAPIKeyDetail)
		mux.HandleFunc("/api/agent/billing/v1/redeem-codes/redeem", api.HandleRedeemCode)
		mux.HandleFunc("/api/agent/image/v1/reference-assets", api.HandleReferenceAssetUpload)
		mux.HandleFunc("/api/agent/image/v1/reference-assets:import-from-gallery", api.HandleReferenceAssetsImportFromGallery)
		mux.HandleFunc("/api/agent/image/v1/reference-assets/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/access") {
				api.HandleReferenceAssetAccess(w, r)
				return
			}
			api.HandleReferenceAssetGet(w, r)
		})
		mux.HandleFunc("/api/agent/image/v1/images/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/access") {
				api.HandleImageAccess(w, r)
				return
			}
			api.HandleImageDownload(w, r)
		})
		mux.HandleFunc("/api/agent/image/v1/tasks", api.HandleAgentTasks)
		mux.HandleFunc("/api/agent/image/v1/tasks/events", api.HandleAgentTaskEvents)
		mux.HandleFunc("/api/agent/image/v1/tasks/", api.HandleAgentTaskDetail)
		mux.HandleFunc("/api/agent/image/v1/history/tasks", api.HandleAgentHistoryTasks)
		mux.HandleFunc("/api/agent/image/v1/history/tasks/", api.HandleAgentHistoryTaskDetail)
		mux.HandleFunc("/api/open/image/v1/reference-assets/uploads", api.HandleOpenReferenceAssetUploadSession)
		mux.HandleFunc("/api/open/image/v1/reference-assets", api.HandleOpenReferenceAssetMultipartUpload)
		mux.HandleFunc("/api/open/image/v1/reference-assets/", api.HandleOpenReferenceAssetGet)
		mux.HandleFunc("/api/open/image/v1/tasks", api.HandleOpenTasks)
		mux.HandleFunc("/api/open/image/v1/tasks/", api.HandleOpenTaskDetail)
		mux.HandleFunc("/api/open/image/v1/balance", api.HandleOpenBalance)
		mux.HandleFunc("/api/open/image/v1/capabilities", api.HandleOpenCapabilities)
		mux.HandleFunc("/api/open/image/v1/estimate", api.HandleOpenEstimate)
		mux.HandleFunc("/api/open/image/v1/gallery/images", api.HandleOpenGalleryImages)
		mux.HandleFunc("/api/open/image/v1/gallery/images/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/access") {
				api.HandleOpenGalleryImageAccess(w, r)
				return
			}
			api.HandleOpenGalleryImageDetail(w, r)
		})
		mux.HandleFunc("/api/open/video/v1/provider-callbacks/", api.HandleVideoProviderCallback)
		mux.HandleFunc("/api/open/image/v1/payments/webhooks/", api.HandlePaymentWebhook)
		mux.HandleFunc("/api/open/cluster/v1/challenges", api.HandleClusterChallenge)
		mux.HandleFunc("/api/open/cluster/v1/join", api.HandleClusterJoin)
		mux.HandleFunc("/api/ops/admin/v1/auth/login", api.HandleAdminLogin)
		mux.HandleFunc("/api/ops/admin/v1/auth/session/refresh", api.HandleAdminRefresh)
		mux.HandleFunc("/api/ops/admin/v1/auth/logout", api.HandleAdminLogout)
		mux.HandleFunc("/api/ops/admin/v1/audit-logs", api.HandleAdminAuditLogs)
		mux.HandleFunc("/api/ops/admin/v1/cluster/nodes", api.HandleAdminClusterNodes)
		mux.HandleFunc("/api/ops/admin/v1/cluster/tokens", api.HandleAdminClusterTokens)
		mux.HandleFunc("/api/ops/admin/v1/cluster/tokens/", api.HandleAdminClusterTokenDetail)
		mux.HandleFunc("/api/ops/admin/v1/image-reviews", api.HandleAdminImageReviews)
		mux.HandleFunc("/api/ops/admin/v1/image-reviews/", api.HandleAdminImageReviewDetail)
		mux.HandleFunc("/api/ops/admin/v1/admin-users", api.HandleAdminSystemUsers)
		mux.HandleFunc("/api/ops/admin/v1/admin-users/", api.HandleAdminSystemUserDetail)
		mux.HandleFunc("/api/ops/admin/v1/users", api.HandleAdminUsers)
		mux.HandleFunc("/api/ops/admin/v1/users/", api.HandleAdminUserDetail)
		mux.HandleFunc("/api/ops/admin/v1/user-groups", api.HandleAdminUserGroups)
		mux.HandleFunc("/api/ops/admin/v1/user-groups/", api.HandleAdminUserGroupDetail)
		mux.HandleFunc("/api/ops/admin/v1/redeem-codes:batch-create", api.HandleAdminRedeemCodeBatchCreate)
		mux.HandleFunc("/api/ops/admin/v1/redeem-codes:export", api.HandleAdminRedeemCodeExport)
		mux.HandleFunc("/api/ops/admin/v1/redeem-codes", api.HandleAdminRedeemCodes)
		mux.HandleFunc("/api/ops/admin/v1/redeem-codes/", api.HandleAdminRedeemCodeDetail)
		mux.HandleFunc("/api/ops/admin/v1/call-records", api.HandleAdminCallRecords)
		mux.HandleFunc("/api/ops/admin/v1/model-accounts", api.HandleAdminModelAccounts)
		mux.HandleFunc("/api/ops/admin/v1/model-accounts/", api.HandleAdminModelAccountDetail)
		mux.HandleFunc("/api/ops/admin/v1/model-account-models/", api.HandleAdminVideoModelConfiguration)
		mux.HandleFunc("/api/ops/admin/v1/text-model-accounts", api.HandleAdminTextModelAccounts)
		mux.HandleFunc("/api/ops/admin/v1/text-model-accounts/", api.HandleAdminTextModelAccountDetail)
		mux.HandleFunc("/api/ops/admin/v1/text-models/", api.HandleAdminTextModelDetail)
		mux.HandleFunc("/api/ops/admin/v1/route-models", api.HandleAdminRouteModels)
		mux.HandleFunc("/api/ops/admin/v1/route-models/", api.HandleAdminRouteModelDetail)
		mux.HandleFunc("/api/ops/admin/v1/route-model-prices", api.HandleAdminRouteModelPrices)
		mux.HandleFunc("/api/ops/admin/v1/route-model-prices/", api.HandleAdminRouteModelPriceDetail)
		mux.HandleFunc("/api/ops/admin/v1/model-providers", api.HandleAdminModelProviders)
		mux.HandleFunc("/api/ops/admin/v1/model-providers/", api.HandleAdminModelProviderDetail)
		mux.HandleFunc("/api/ops/admin/v1/provider-models", api.HandleAdminProviderModels)
		mux.HandleFunc("/api/ops/admin/v1/provider-models/", api.HandleAdminProviderModelDetail)
		mux.HandleFunc("/api/ops/admin/v1/model-routes", api.HandleAdminModelRoutes)
		mux.HandleFunc("/api/ops/admin/v1/model-routes/", api.HandleAdminModelRouteDetail)
		mux.HandleFunc("/api/ops/admin/v1/metrics/dashboard", api.HandleAdminDashboard)
		mux.HandleFunc("/api/ops/admin/v1/monitoring/snapshot", api.HandleAdminMonitoringSnapshot)
		mux.HandleFunc("/api/ops/admin/v1/readiness", api.HandleAdminReadiness)
		mux.HandleFunc("/api/ops/admin/v1/video/configuration", api.HandleAdminVideoConfiguration)
		mux.HandleFunc("/api/ops/admin/v1/video-pricing-strategies", api.HandleAdminVideoPricingStrategies)
		mux.HandleFunc("/api/ops/admin/v1/video-pricing-strategies/", api.HandleAdminVideoPricingStrategyDetail)
		mux.HandleFunc("/api/ops/admin/v1/video-price-rules", api.HandleAdminVideoPriceRules)
		mux.HandleFunc("/api/ops/admin/v1/video-price-rules/", api.HandleAdminVideoPriceRuleDetail)
		mux.HandleFunc("/api/ops/admin/v1/video-tasks", api.HandleAdminVideoTasks)
		mux.HandleFunc("/api/ops/admin/v1/video-tasks/", api.HandleAdminVideoTaskDetail)
		mux.HandleFunc("/api/ops/admin/v1/media-processing-jobs/", api.HandleAdminMediaProcessingJobDetail)
		mux.HandleFunc("/api/ops/admin/v1/media-policy", api.HandleAdminMediaPolicy)
		mux.HandleFunc("/api/ops/admin/v1/cashier/overview", api.HandleAdminCashierOverview)
		mux.HandleFunc("/api/ops/admin/v1/cashier/plans", api.HandleAdminCashierPlans)
		mux.HandleFunc("/api/ops/admin/v1/cashier/plans/", api.HandleAdminCashierPlanDetail)
		mux.HandleFunc("/api/ops/admin/v1/cashier/custom-amount-config", api.HandleAdminCashierCustomAmountConfig)
		mux.HandleFunc("/api/ops/admin/v1/cashier/visible-methods", api.HandleAdminCashierVisibleMethods)
		mux.HandleFunc("/api/ops/admin/v1/cashier/visible-methods/", api.HandleAdminCashierVisibleMethods)
		mux.HandleFunc("/api/ops/admin/v1/cashier/provider-instances", api.HandleAdminCashierProviderInstances)
		mux.HandleFunc("/api/ops/admin/v1/cashier/provider-instances/", api.HandleAdminCashierProviderInstanceDetail)
		mux.HandleFunc("/api/ops/admin/v1/cashier/orders", api.HandleAdminCashierOrders)
		mux.HandleFunc("/api/ops/admin/v1/cashier/orders/", api.HandleAdminCashierOrderDetail)
		mux.HandleFunc("/api/ops/admin/v1/cashier/webhook-events", api.HandleAdminCashierWebhookEvents)
		mux.HandleFunc("/api/ops/admin/v1/cashier/webhook-events/", api.HandleAdminCashierWebhookEventDetail)
		mux.HandleFunc("/api/ops/admin/v1/config-tabs", api.HandleAdminConfigTabs)
		mux.HandleFunc("/api/ops/admin/v1/config-tabs/", api.HandleAdminConfigTabDetail)
		mux.HandleFunc("/api/ops/admin/v1/storage-configs", api.HandleAdminStorageConfigs)
		mux.HandleFunc("/api/ops/admin/v1/storage-configs:probe", api.HandleAdminStorageConfigProbe)
		mux.HandleFunc("/api/ops/admin/v1/storage-configs/", api.HandleAdminStorageConfigDetail)
		mux.HandleFunc("/api/ops/admin/v1/security/smtp", api.HandleAdminSecuritySMTP)
		mux.HandleFunc("/api/ops/admin/v1/security/smtp/test", api.HandleAdminSecuritySMTPTest)
		mux.HandleFunc("/docs/openapi.yaml", api.HandleDocsOpenAPIYAML)
		mux.HandleFunc("/docs/openapi.json", api.HandleDocsOpenAPIJSON)
		mux.HandleFunc("/docs/examples", api.HandleDocsExamples)
		mux.HandleFunc("/docs/errors", api.HandleDocsErrors)
	}

	var routeContract *openapispec.RouteContract
	if api != nil {
		routeContract, _ = openapispec.LoadRouteContract()
	}
	return wrapHandler(mux, corsAllowedOrigins, normalPreflightPolicy(api != nil, routeContract))
}

func registerSystemRoutes(mux *http.ServeMux, system *handlers.SystemAPI) {
	mux.HandleFunc("GET /healthz", system.HandleHealthz)
	mux.HandleFunc("GET /readyz", system.HandleReadyz)
	mux.HandleFunc("GET /api/system/v1/bootstrap-status", system.HandleBootstrapStatus)
}

func wrapHandler(mux http.Handler, corsAllowedOrigins []string, preflight middleware.PreflightPolicy) http.Handler {
	handler := middleware.Recovery(mux)
	handler = middleware.Metrics(handler)
	handler = middleware.RequestID(handler)
	handler = middleware.CORSWithPreflightPolicy(corsAllowedOrigins, preflight, handler)
	handler = observability.RequestLogger(handler)
	return handler
}

func normalPreflightPolicy(businessRoutesRegistered bool, routeContract *openapispec.RouteContract) middleware.PreflightPolicy {
	return func(path, method string) bool {
		if path == "/setup" || strings.HasPrefix(path, "/setup/") || strings.HasPrefix(path, "/api/setup/") {
			return false
		}
		if method == http.MethodGet {
			switch path {
			case "/", "/healthz", "/readyz", "/api/system/v1/bootstrap-status", "/metrics":
				return true
			}
			if strings.HasPrefix(path, "/debug/pprof/") {
				return true
			}
		}
		if !businessRoutesRegistered {
			return false
		}
		if routeContract != nil && routeContract.Allows(method, path) {
			return true
		}
		return allowsSupplementalNormalRoute(path, method)
	}
}

func allowsSupplementalNormalRoute(path, method string) bool {
	if allowed, ok := supplementalNormalExactRoutes[path]; ok {
		return allowed[method]
	}
	for template, allowed := range supplementalNormalTemplateRoutes {
		if matchesSupplementalPathTemplate(template, path) {
			return allowed[method]
		}
	}
	return false
}

func matchesSupplementalPathTemplate(template, path string) bool {
	templateSegments, templateOK := splitSupplementalAbsolutePath(template)
	pathSegments, pathOK := splitSupplementalAbsolutePath(path)
	if !templateOK || !pathOK {
		return false
	}
	if len(templateSegments) != len(pathSegments) {
		return false
	}
	for i, segment := range templateSegments {
		open := strings.IndexByte(segment, '{')
		close := strings.IndexByte(segment, '}')
		if open < 0 || close < open {
			if segment != pathSegments[i] {
				return false
			}
			continue
		}
		prefix, suffix := segment[:open], segment[close+1:]
		value := pathSegments[i]
		if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) || len(value) <= len(prefix)+len(suffix) {
			return false
		}
	}
	return true
}

func splitSupplementalAbsolutePath(path string) ([]string, bool) {
	if path == "/" {
		return nil, true
	}
	if !strings.HasPrefix(path, "/") || path == "" {
		return nil, false
	}
	return strings.Split(strings.TrimPrefix(path, "/"), "/"), true
}

var supplementalNormalExactRoutes = map[string]map[string]bool{
	"/api/agent/auth/v1/password/reset":                 {http.MethodPost: true},
	"/api/agent/user/v1/preferences":                    {http.MethodPut: true},
	"/api/agent/user/v1/avatar":                         {http.MethodPost: true},
	"/api/agent/developer/v1/api-keys":                  {http.MethodGet: true, http.MethodPost: true},
	"/api/agent/billing/v1/redeem-codes/redeem":         {http.MethodPost: true},
	"/api/agent/image/v1/tasks/events":                  {http.MethodGet: true},
	"/api/ops/admin/v1/auth/logout":                     {http.MethodPost: true},
	"/api/ops/admin/v1/redeem-codes:batch-create":       {http.MethodPost: true},
	"/api/ops/admin/v1/redeem-codes:export":             {http.MethodPost: true},
	"/api/ops/admin/v1/monitoring/snapshot":             {http.MethodGet: true},
	"/api/ops/admin/v1/storage-configs:probe":           {http.MethodPost: true},
	"/api/ops/admin/v1/video/configuration":             {http.MethodGet: true},
	"/api/ops/admin/v1/video-pricing-strategies":        {http.MethodGet: true, http.MethodPost: true},
	"/api/ops/admin/v1/video-price-rules":               {http.MethodPost: true},
	"/api/ops/admin/v1/video-tasks":                     {http.MethodGet: true},
	"/api/ops/admin/v1/media-policy":                    {http.MethodGet: true, http.MethodPut: true},
	"/api/agent/media/v1/export-jobs/{job_id}":          {http.MethodGet: true},
	"/api/agent/media/v1/export-jobs/{job_id}/download": {http.MethodGet: true},
	"/api/ops/admin/v1/cashier/visible-methods/":        {http.MethodGet: true, http.MethodPut: true},
	"/docs/openapi.yaml":                                {http.MethodGet: true},
	"/docs/openapi.json":                                {http.MethodGet: true},
	"/docs/examples":                                    {http.MethodGet: true},
	"/docs/errors":                                      {http.MethodGet: true},
}

var supplementalNormalTemplateRoutes = map[string]map[string]bool{
	"/api/agent/developer/v1/api-keys/{key_id}":                              {http.MethodPut: true, http.MethodPatch: true, http.MethodDelete: true},
	"/api/agent/developer/v1/api-keys/{key_id}/reset-secret":                 {http.MethodPost: true},
	"/api/agent/gallery/v1/images/{image_id}":                                {http.MethodDelete: true},
	"/api/agent/gallery/v1/images/{image_id}/group":                          {http.MethodPut: true, http.MethodPatch: true},
	"/api/agent/gallery/v1/images/{image_id}/like":                           {http.MethodPost: true},
	"/api/agent/gallery/v1/images/{image_id}/favorite":                       {http.MethodPost: true},
	"/api/agent/gallery/v1/images/{image_id}/publish":                        {http.MethodPost: true, http.MethodDelete: true},
	"/api/agent/gallery/v1/images:batch-publish":                             {http.MethodPost: true},
	"/api/agent/gallery/v1/images:batch-group":                               {http.MethodPost: true},
	"/api/agent/gallery/v1/images:batch-delete":                              {http.MethodPost: true},
	"/api/agent/gallery/v1/images:batch-transfer-project":                    {http.MethodPost: true},
	"/api/agent/gallery/v1/images:batch-download":                            {http.MethodPost: true},
	"/api/agent/gallery/v1/export-jobs/{job_id}":                             {http.MethodGet: true},
	"/api/agent/gallery/v1/export-jobs/{job_id}/download":                    {http.MethodGet: true},
	"/api/ops/admin/v1/image-reviews/{image_id}:approve":                     {http.MethodPost: true},
	"/api/ops/admin/v1/image-reviews/{image_id}:reject":                      {http.MethodPost: true},
	"/api/ops/admin/v1/image-reviews/{image_id}:unpublish":                   {http.MethodPost: true},
	"/api/ops/admin/v1/route-model-prices/{price_id}":                        {http.MethodGet: true, http.MethodPut: true, http.MethodDelete: true},
	"/api/ops/admin/v1/video-tasks/{task_id}":                                {http.MethodGet: true},
	"/api/ops/admin/v1/video-tasks/{task_id}:retry-artifact":                 {http.MethodPost: true},
	"/api/ops/admin/v1/video-tasks/{task_id}:retry-settlement":               {http.MethodPost: true},
	"/api/ops/admin/v1/media-processing-jobs/{job_id}:retry":                 {http.MethodPost: true},
	"/api/ops/admin/v1/model-account-models/{id}/video-capability":           {http.MethodGet: true, http.MethodPut: true, http.MethodDelete: true},
	"/api/ops/admin/v1/model-account-models/{id}/video-cost-rules":           {http.MethodGet: true, http.MethodPost: true},
	"/api/ops/admin/v1/model-account-models/{id}/video-cost-rules/{rule_id}": {http.MethodDelete: true},
	"/api/ops/admin/v1/video-pricing-strategies/{id}":                        {http.MethodGet: true, http.MethodPut: true, http.MethodDelete: true},
	"/api/ops/admin/v1/video-pricing-strategies/{id}:simulate":               {http.MethodPost: true},
	"/api/ops/admin/v1/video-pricing-strategies/{id}:recalculate":            {http.MethodPost: true},
	"/api/ops/admin/v1/video-price-rules/{id}":                               {http.MethodPut: true, http.MethodDelete: true},
	"/api/ops/admin/v1/route-models/{id}/video-config":                       {http.MethodGet: true, http.MethodPut: true, http.MethodDelete: true},
	"/api/ops/admin/v1/route-models/{id}/video-impact":                       {http.MethodGet: true},
}
