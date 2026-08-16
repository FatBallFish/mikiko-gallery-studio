package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domaincluster "github.com/fatballfish/pic-gallery/internal/domain/cluster"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
	adminvideoservice "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

func (a *API) HandleAdminVideoConfiguration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	snapshot, err := a.adminVideo.Snapshot(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, snapshot)
}

func (a *API) HandleAdminVideoModelConfiguration(w http.ResponseWriter, r *http.Request) {
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/model-account-models/")
	modelID, appErr := parseInt64Part(parts, 0, "model_id")
	if appErr != nil || len(parts) < 2 {
		if appErr == nil {
			appErr = errs.BadRequest("invalid video model configuration route")
		}
		httpx.WriteError(w, r, appErr)
		return
	}
	if parts[1] == "video-capability" {
		a.handleAdminVideoCapability(w, r, modelID)
		return
	}
	httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "video model configuration route not found"))
}

func (a *API) HandleAdminVideoRateCards(w http.ResponseWriter, r *http.Request) {
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/video-models/")
	accountModelID, appErr := parseInt64Part(parts, 0, "account_model_id")
	if appErr != nil || len(parts) < 2 || parts[1] != "rate-cards" || len(parts) > 3 {
		if appErr == nil {
			appErr = errs.New(http.StatusNotFound, errs.CodeNotFound, "video rate card route not found")
		}
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodGet && len(parts) == 2 {
		if _, permissionErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); permissionErr != nil {
			httpx.WriteError(w, r, permissionErr)
			return
		}
		items, err := a.adminVideo.ListVideoModelRateCards(r.Context(), accountModelID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
		return
	}
	admin, permissionErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if permissionErr != nil {
		httpx.WriteError(w, r, permissionErr)
		return
	}
	if r.Method == http.MethodPost && len(parts) == 2 {
		var input adminvideoservice.RateCardWrite
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.AccountModelID = accountModelID
		result, err := a.adminVideo.SaveVideoModelRateCard(r.Context(), input)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_rate_card.save", "video_model_rate_card", fmt.Sprintf("%d", result.ID), map[string]any{"rate_version": result.RateVersion})
		httpx.WriteSuccess(w, r, http.StatusCreated, result)
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 3 {
		id, parseErr := strconv.ParseInt(parts[2], 10, 64)
		if parseErr != nil || id <= 0 {
			httpx.WriteError(w, r, errs.BadRequest("invalid rate card id"))
			return
		}
		expected, parseErr := strconv.Atoi(r.URL.Query().Get("expected_version"))
		if parseErr != nil || expected <= 0 {
			httpx.WriteError(w, r, errs.BadRequest("expected_version is required"))
			return
		}
		if err := a.adminVideo.DeleteVideoModelRateCard(r.Context(), id, expected); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_rate_card.delete", "video_model_rate_card", fmt.Sprintf("%d", id), nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeMethodNotAllowed(w, r)
}

func (a *API) HandleAdminVideoRouteQuoteSimulation(w http.ResponseWriter, r *http.Request) {
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	if _, permissionErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels); permissionErr != nil {
		httpx.WriteError(w, r, permissionErr)
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/video-routes/")
	routeModelID, appErr := parseInt64Part(parts, 0, "route_model_id")
	if appErr != nil || len(parts) != 2 || parts[1] != "quote-simulation" {
		if appErr == nil {
			appErr = errs.New(http.StatusNotFound, errs.CodeNotFound, "video route quote simulation route not found")
		}
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var input adminvideoservice.QuoteSimulationRequest
	if !decodeJSONBody(w, r, &input) {
		return
	}
	input.RouteModelID = routeModelID
	result, err := a.adminVideo.SimulateRouteQuote(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) handleAdminVideoCapability(w http.ResponseWriter, r *http.Request, modelID int64) {
	if r.Method == http.MethodGet {
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		snapshot, err := a.adminVideo.Snapshot(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		for _, item := range snapshot.Capabilities {
			if item.AccountModelID == modelID {
				httpx.WriteSuccess(w, r, http.StatusOK, item)
				return
			}
		}
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "video capability not found"))
		return
	}
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var input adminvideoservice.CapabilityWrite
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.AccountModelID = modelID
		result, err := a.adminVideo.SaveCapability(r.Context(), input)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_capability.save", "model_account_model", fmt.Sprintf("%d", modelID), map[string]any{"version": result.Version, "enabled": result.Enabled}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case http.MethodDelete:
		if err := a.adminVideo.DeleteConfig(r.Context(), adminvideoservice.ConfigCapability, modelID, 0); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_capability.delete", "model_account_model", fmt.Sprintf("%d", modelID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) handleAdminVideoCostRules(w http.ResponseWriter, r *http.Request, modelID int64, suffix []string) {
	if r.Method == http.MethodGet && len(suffix) == 0 {
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		snapshot, err := a.adminVideo.Snapshot(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		items := make([]adminvideoservice.CostRuleSummary, 0)
		for _, item := range snapshot.CostRules {
			if item.AccountModelID == modelID {
				items = append(items, item)
			}
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
		return
	}
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodPost && len(suffix) == 0 {
		var input adminvideoservice.CostRuleWrite
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.AccountModelID = modelID
		result, err := a.adminVideo.SaveCostRule(r.Context(), input)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_cost_rule.save", "video_provider_cost_rule", fmt.Sprintf("%d", result.ID), map[string]any{"version": result.RuleVersion})
		httpx.WriteSuccess(w, r, http.StatusCreated, result)
		return
	}
	if r.Method == http.MethodDelete && len(suffix) == 1 {
		id, err := strconv.ParseInt(suffix[0], 10, 64)
		if err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid cost rule id"))
			return
		}
		expected, parseErr := strconv.ParseInt(r.URL.Query().Get("expected_version"), 10, 64)
		if parseErr != nil {
			httpx.WriteError(w, r, errs.BadRequest("expected_version is required"))
			return
		}
		if err := a.adminVideo.DeleteConfig(r.Context(), adminvideoservice.ConfigCostRule, id, expected); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_cost_rule.delete", "video_provider_cost_rule", fmt.Sprintf("%d", id), nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeMethodNotAllowed(w, r)
}

func (a *API) HandleAdminVideoPricingStrategies(w http.ResponseWriter, r *http.Request) {
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	if r.Method == http.MethodGet {
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		snapshot, err := a.adminVideo.Snapshot(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": snapshot.Strategies, "price_rules": snapshot.PriceRules})
		return
	}
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var input adminvideoservice.StrategyWrite
	if !decodeJSONBody(w, r, &input) {
		return
	}
	result, err := a.adminVideo.SaveStrategy(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_pricing_strategy.create", "video_pricing_strategy", fmt.Sprintf("%d", result.ID), map[string]any{"version": result.StrategyVersion})
	httpx.WriteSuccess(w, r, http.StatusCreated, result)
}

func (a *API) HandleAdminVideoPricingStrategyDetail(w http.ResponseWriter, r *http.Request) {
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/video-pricing-strategies/")
	if len(parts) != 1 {
		httpx.WriteError(w, r, errs.BadRequest("invalid strategy route"))
		return
	}
	raw := parts[0]
	action := ""
	for _, candidate := range []string{":simulate", ":recalculate"} {
		if strings.HasSuffix(raw, candidate) {
			action = candidate
			raw = strings.TrimSuffix(raw, candidate)
		}
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, r, errs.BadRequest("invalid strategy id"))
		return
	}
	if r.Method == http.MethodGet && action == "" {
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		snapshot, loadErr := a.adminVideo.Snapshot(r.Context())
		if loadErr != nil {
			httpx.WriteError(w, r, normalizeAppError(loadErr))
			return
		}
		for _, item := range snapshot.Strategies {
			if item.ID == id {
				httpx.WriteSuccess(w, r, http.StatusOK, item)
				return
			}
		}
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "strategy not found"))
		return
	}
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if action == ":simulate" && r.Method == http.MethodPost {
		var input adminvideoservice.SimulationRequest
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.StrategyID = id
		result, serviceErr := a.adminVideo.Simulate(r.Context(), input)
		if serviceErr != nil {
			httpx.WriteError(w, r, normalizeAppError(serviceErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	if action == ":recalculate" && r.Method == http.MethodPost {
		var input adminvideoservice.RecalculateRequest
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.StrategyID = id
		result, serviceErr := a.adminVideo.Recalculate(r.Context(), input)
		if serviceErr != nil {
			httpx.WriteError(w, r, normalizeAppError(serviceErr))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_pricing_strategy.recalculate", "video_pricing_strategy", raw, map[string]any{"rules": len(result)})
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": result})
		return
	}
	if action != "" {
		writeMethodNotAllowed(w, r)
		return
	}
	if r.Method == http.MethodPut {
		var input adminvideoservice.StrategyWrite
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.ID = id
		result, serviceErr := a.adminVideo.SaveStrategy(r.Context(), input)
		if serviceErr != nil {
			httpx.WriteError(w, r, normalizeAppError(serviceErr))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_pricing_strategy.update", "video_pricing_strategy", raw, map[string]any{"version": result.StrategyVersion})
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	if r.Method == http.MethodDelete {
		expected, parseErr := strconv.ParseInt(r.URL.Query().Get("expected_version"), 10, 64)
		if parseErr != nil {
			httpx.WriteError(w, r, errs.BadRequest("expected_version is required"))
			return
		}
		if serviceErr := a.adminVideo.DeleteConfig(r.Context(), adminvideoservice.ConfigStrategy, id, expected); serviceErr != nil {
			httpx.WriteError(w, r, normalizeAppError(serviceErr))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_pricing_strategy.delete", "video_pricing_strategy", raw, nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeMethodNotAllowed(w, r)
}

func (a *API) HandleAdminVideoPriceRules(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var input adminvideoservice.PriceRuleWrite
	if !decodeJSONBody(w, r, &input) {
		return
	}
	result, err := a.adminVideo.SavePriceRule(r.Context(), input)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_price_rule.create", "video_price_rule", fmt.Sprintf("%d", result.ID), map[string]any{"version": result.RuleVersion})
	httpx.WriteSuccess(w, r, http.StatusCreated, result)
}

func (a *API) HandleAdminVideoPriceRuleDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	id, err := strconv.ParseInt(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/ops/admin/v1/video-price-rules/"), "/"), 10, 64)
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid price rule id"))
		return
	}
	if r.Method == http.MethodPut {
		var input adminvideoservice.PriceRuleWrite
		if !decodeJSONBody(w, r, &input) {
			return
		}
		input.ID = id
		result, serviceErr := a.adminVideo.SavePriceRule(r.Context(), input)
		if serviceErr != nil {
			httpx.WriteError(w, r, normalizeAppError(serviceErr))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_price_rule.update", "video_price_rule", fmt.Sprintf("%d", id), map[string]any{"version": result.RuleVersion})
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	if r.Method == http.MethodDelete {
		expected, parseErr := strconv.ParseInt(r.URL.Query().Get("expected_version"), 10, 64)
		if parseErr != nil {
			httpx.WriteError(w, r, errs.BadRequest("expected_version is required"))
			return
		}
		if serviceErr := a.adminVideo.DeleteConfig(r.Context(), adminvideoservice.ConfigPriceRule, id, expected); serviceErr != nil {
			httpx.WriteError(w, r, normalizeAppError(serviceErr))
			return
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_price_rule.delete", "video_price_rule", fmt.Sprintf("%d", id), nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeMethodNotAllowed(w, r)
}

func (a *API) HandleAdminRouteVideoConfiguration(w http.ResponseWriter, r *http.Request, adminID, routeID int64, action string) bool {
	if action != "video-config" && action != "video-impact" {
		return false
	}
	if r.Method == http.MethodGet {
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return true
		}
		snapshot, err := a.adminVideo.Snapshot(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return true
		}
		if action == "video-impact" {
			items := make([]adminvideoservice.Impact, 0)
			for _, item := range snapshot.Impacts {
				if item.RouteModelID == routeID {
					items = append(items, item)
				}
			}
			httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
			return true
		}
		for _, item := range snapshot.Routes {
			if item.RouteModelID == routeID {
				httpx.WriteSuccess(w, r, http.StatusOK, item)
				return true
			}
		}
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "video route config not found"))
		return true
	}
	if action == "video-impact" {
		writeMethodNotAllowed(w, r)
		return true
	}
	if r.Method == http.MethodPut {
		var input adminvideoservice.RouteConfigWrite
		if !decodeJSONBody(w, r, &input) {
			return true
		}
		input.RouteModelID = routeID
		result, err := a.adminVideo.SaveRouteConfig(r.Context(), input)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return true
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "video_route_config.save", "route_model", fmt.Sprintf("%d", routeID), map[string]any{"version": result.ConfigVersion})
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return true
	}
	if r.Method == http.MethodDelete {
		if err := a.adminVideo.DeleteConfig(r.Context(), adminvideoservice.ConfigRoute, routeID, 0); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return true
		}
		_ = a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "video_route_config.delete", "route_model", fmt.Sprintf("%d", routeID), nil)
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	writeMethodNotAllowed(w, r)
	return true
}

func (a *API) HandleAdminVideoTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	filter, appErr := parseAdminVideoTaskFilter(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	page, err := a.adminVideo.ListTasks(r.Context(), filter)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, page)
}

func (a *API) HandleAdminVideoTaskDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/video-tasks/")
	if len(parts) != 1 {
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "video task route not found"))
		return
	}
	rawID := parts[0]
	retryArtifact := strings.HasSuffix(rawID, ":retry-artifact")
	retrySettlement := strings.HasSuffix(rawID, ":retry-settlement")
	rawID = strings.TrimSuffix(rawID, ":retry-artifact")
	rawID = strings.TrimSuffix(rawID, ":retry-settlement")
	taskID, err := uuid.Parse(rawID)
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid video task id"))
		return
	}
	if retryArtifact {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		var body struct {
			ItemID *uuid.UUID `json:"item_id"`
		}
		if r.ContentLength != 0 && !decodeJSONBody(w, r, &body) {
			return
		}
		request := adminvideoservice.RetryRequest{Kind: adminvideoservice.RetryArtifact, TaskID: taskID}
		if body.ItemID != nil {
			request.ItemID = *body.ItemID
		}
		if err := a.adminVideo.Retry(r.Context(), request); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_task.retry_artifact", "video_task", taskID.String(), map[string]any{"item_id": request.ItemID}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusAccepted, map[string]any{"task_id": taskID, "recovery": "artifact", "provider_generation_requested": false})
		return
	}
	if retrySettlement {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		if err := a.adminVideo.Retry(r.Context(), adminvideoservice.RetryRequest{Kind: adminvideoservice.RetrySettlement, TaskID: taskID}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "video_task.retry_settlement", "video_task", taskID.String(), map[string]any{"provider_generation_requested": false}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusAccepted, map[string]any{"task_id": taskID, "recovery": "settlement", "provider_generation_requested": false})
		return
	}
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	detail, serviceErr := a.adminVideo.GetTask(r.Context(), taskID)
	if serviceErr != nil {
		httpx.WriteError(w, r, normalizeAppError(serviceErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, detail)
}

func (a *API) HandleAdminMediaProcessingJobDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/media-processing-jobs/")
	if len(parts) != 1 || !strings.HasSuffix(parts[0], ":retry") {
		httpx.WriteError(w, r, errs.New(404, errs.CodeNotFound, "media processing job route not found"))
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	jobID, err := uuid.Parse(strings.TrimSuffix(parts[0], ":retry"))
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid media processing job id"))
		return
	}
	if err := a.adminVideo.Retry(r.Context(), adminvideoservice.RetryRequest{Kind: adminvideoservice.RetryDerivative, JobID: jobID}); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "media_processing.retry", "media_processing_job", jobID.String(), map[string]any{"provider_generation_requested": false}); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, map[string]any{"job_id": jobID, "recovery": "derivative", "provider_generation_requested": false})
}

func (a *API) HandleAdminMediaPolicy(w http.ResponseWriter, r *http.Request) {
	if a.adminVideo == nil {
		httpx.WriteError(w, r, errs.Internal("video administration is unavailable"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		policy, err := a.adminVideo.GetMediaPolicy(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, policy)
	case http.MethodPut:
		admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageConfig)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		var request struct {
			Version int64                         `json:"version"`
			Policy  adminvideoservice.MediaPolicy `json:"policy"`
		}
		if !decodeJSONBody(w, r, &request) {
			return
		}
		if request.Policy.Version == 0 {
			request.Policy.Version = request.Version
		}
		policy, err := a.adminVideo.UpdateMediaPolicy(r.Context(), request.Version, request.Policy, admin.AdminID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "media_policy.update", "media_policy", "global", map[string]any{"version": policy.Version, "applies_to": policy.AppliesTo}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, policy)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func parseAdminVideoTaskFilter(r *http.Request) (adminvideoservice.TaskFilter, *errs.Error) {
	query := r.URL.Query()
	filter := adminvideoservice.TaskFilter{ProviderTaskID: query.Get("provider_task_id"), Status: query.Get("status"), Cursor: query.Get("cursor"), Limit: 20}
	for key, target := range map[string]*int64{"user_id": &filter.UserID, "route_model_id": &filter.RouteModelID, "account_model_id": &filter.AccountModelID} {
		if raw := strings.TrimSpace(query.Get(key)); raw != "" {
			value, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || value <= 0 {
				return filter, errs.BadRequest("invalid " + key)
			}
			*target = value
		}
	}
	if raw := query.Get("task_id"); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil {
			return filter, errs.BadRequest("invalid task_id")
		}
		filter.TaskID = &value
	}
	if raw := query.Get("project_id"); raw != "" {
		value, err := uuid.Parse(raw)
		if err != nil {
			return filter, errs.BadRequest("invalid project_id")
		}
		filter.ProjectID = &value
	}
	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			return filter, errs.BadRequest("limit must be between 1 and 100")
		}
		filter.Limit = value
	}
	for key, target := range map[string]**time.Time{"created_from": &filter.From, "created_to": &filter.To} {
		if raw := query.Get(key); raw != "" {
			value, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				return filter, errs.BadRequest("invalid " + key)
			}
			*target = &value
		}
	}
	return filter, nil
}

func nonBlockingMediaReadiness(check adminReadinessCheck) adminReadinessCheck {
	check.Blocking = false
	return check
}

func (a *API) videoRoutesReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	if a.adminVideo == nil {
		return nonBlockingMediaReadiness(readinessCheck("video_routes", "视频路由与候选", "fail", "视频管理服务未初始化", "routing", "去配置", checkedAt)), nil
	}
	snapshot, err := a.adminVideo.Readiness(ctx, checkedAt)
	if err != nil {
		return adminReadinessCheck{}, err
	}
	status, detail := "pass", fmt.Sprintf("%d 个视频路由可用", snapshot.EnabledVideoRoutes)
	if snapshot.EnabledVideoRoutes == 0 {
		status, detail = "fail", "暂无启用的视频路由"
	} else if snapshot.RoutesMissingCandidate > 0 {
		status, detail = "fail", fmt.Sprintf("%d 个视频路由缺少可用候选", snapshot.RoutesMissingCandidate)
	}
	return nonBlockingMediaReadiness(readinessCheck("video_routes", "视频路由与候选", status, detail, "routing", "去配置", checkedAt)), nil
}

func (a *API) videoPricesReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	if a.adminVideo == nil {
		return nonBlockingMediaReadiness(readinessCheck("video_prices", "视频可见组合价格", "fail", "视频管理服务未初始化", "pricing", "去配置", checkedAt)), nil
	}
	snapshot, err := a.adminVideo.Readiness(ctx, checkedAt)
	if err != nil {
		return adminReadinessCheck{}, err
	}
	status, detail := "pass", "启用视频路由均有价格规则"
	if snapshot.VisibleCombosMissingPrice > 0 {
		status, detail = "fail", fmt.Sprintf("%d 个视频路由存在缺价组合", snapshot.VisibleCombosMissingPrice)
	}
	return nonBlockingMediaReadiness(readinessCheck("video_prices", "视频可见组合价格", status, detail, "pricing", "去配置", checkedAt)), nil
}

func (a *API) mediaStorageReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	if a.storageCfg == nil || a.storageReg == nil {
		return nonBlockingMediaReadiness(readinessCheck("media_storage", "媒体存储读写", "fail", "媒体存储服务未初始化", "system-settings", "去检查", checkedAt)), nil
	}
	resolved, err := a.storageCfg.ResolveDefaultWritable(ctx)
	if err != nil {
		return nonBlockingMediaReadiness(readinessCheck("media_storage", "媒体存储读写", "fail", "没有可写的默认对象存储", "system-settings", "去检查", checkedAt)), nil
	}
	result := a.storageReg.Probe(ctx, resolved)
	if result.Status != domainstorageconfig.ProbeStatusSuccess {
		return nonBlockingMediaReadiness(readinessCheck("media_storage", "媒体存储读写", "fail", "对象存储读写探针失败", "system-settings", "去检查", checkedAt)), nil
	}
	return nonBlockingMediaReadiness(readinessCheck("media_storage", "媒体存储读写", "pass", "对象存储写入、读取和清理正常", "system-settings", "去检查", checkedAt)), nil
}

func (a *API) mediaWorkerReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	if a.cluster == nil {
		return nonBlockingMediaReadiness(readinessCheck("media_worker", "媒体 Worker 与 FFmpeg", "fail", "集群心跳服务未初始化，无法证明媒体处理能力", "cluster", "去检查", checkedAt)), nil
	}
	page, err := a.cluster.ListNodes(ctx, domaincluster.ListNodesRequest{Page: 1, PageSize: 100})
	if err != nil {
		return adminReadinessCheck{}, err
	}
	for _, node := range page.Items {
		if (node.Role == domaincluster.NodeRoleWorker || node.Role == domaincluster.NodeRoleSingle) && (node.EffectiveHealth == domaincluster.NodeHealthHealthy || node.EffectiveHealth == domaincluster.NodeHealthDegraded) {
			return nonBlockingMediaReadiness(readinessCheck("media_worker", "媒体 Worker 与 FFmpeg", "pass", "媒体 Worker 心跳在线；FFmpeg 二进制由 Worker 启动自检约束", "cluster", "去检查", checkedAt)), nil
		}
	}
	return nonBlockingMediaReadiness(readinessCheck("media_worker", "媒体 Worker 与 FFmpeg", "fail", "没有在线的媒体 Worker，无法证明 FFmpeg 可用", "cluster", "去检查", checkedAt)), nil
}

func (a *API) videoArtifactBacklogReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	return a.videoBacklogReadiness(ctx, checkedAt, "video_artifact_backlog", "视频转存积压", "artifact")
}

func (a *API) mediaDerivativeBacklogReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	return a.videoBacklogReadiness(ctx, checkedAt, "media_derivative_backlog", "媒体派生积压", "derivative")
}

func (a *API) videoSettlementBacklogReadinessProbe(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, error) {
	return a.videoBacklogReadiness(ctx, checkedAt, "video_settlement_backlog", "视频结算异常", "settlement")
}

func (a *API) videoBacklogReadiness(ctx context.Context, checkedAt time.Time, key, label, kind string) (adminReadinessCheck, error) {
	if a.adminVideo == nil {
		return nonBlockingMediaReadiness(readinessCheck(key, label, "fail", "视频管理服务未初始化", "video-tasks", "去处理", checkedAt)), nil
	}
	snapshot, err := a.adminVideo.Readiness(ctx, checkedAt)
	if err != nil {
		return adminReadinessCheck{}, err
	}
	count := snapshot.ArtifactBacklog
	if kind == "derivative" {
		count = snapshot.DerivativeBacklog
	}
	if kind == "settlement" {
		count = snapshot.SettlementBacklog
	}
	status, detail := "pass", "当前无积压"
	if count > 0 {
		status, detail = "warn", fmt.Sprintf("当前有 %d 项待处理", count)
	}
	return nonBlockingMediaReadiness(readinessCheck(key, label, status, detail, "video-tasks", "去处理", checkedAt)), nil
}
