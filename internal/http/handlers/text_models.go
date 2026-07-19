package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	promptoptimizer "github.com/fatballfish/pic-gallery/internal/service/promptoptimizer"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

func (a *API) HandleAdminTextModelAccounts(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.textModels == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "text model configuration is unavailable"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := a.textModels.ListAccounts(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req domaintextmodel.AccountWriteRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		created, err := a.textModels.CreateAccount(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model_account.create", "text_model_account", fmt.Sprintf("%d", created.ID), map[string]any{"platform_type": created.PlatformType, "api_style": created.APIStyle, "enabled": created.Enabled}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminTextModelAccountDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.textModels == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "text model configuration is unavailable"))
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/text-model-accounts/")
	accountID, parseErr := parseInt64Part(parts, 0, "account_id")
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	if len(parts) == 2 && parts[1] == "models" {
		a.handleAdminTextModels(w, r, admin.AdminID, accountID)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req domaintextmodel.AccountWriteRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		updated, err := a.textModels.UpdateAccount(r.Context(), accountID, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model_account.update", "text_model_account", fmt.Sprintf("%d", accountID), map[string]any{"api_style": updated.APIStyle, "enabled": updated.Enabled})
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.textModels.DeleteAccount(r.Context(), accountID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model_account.delete", "text_model_account", fmt.Sprintf("%d", accountID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) handleAdminTextModels(w http.ResponseWriter, r *http.Request, adminID, accountID int64) {
	switch r.Method {
	case http.MethodGet:
		items, err := a.textModels.ListModels(r.Context(), accountID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
	case http.MethodPost:
		var req domaintextmodel.ModelWriteRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		req.AccountID = accountID
		created, err := a.textModels.CreateModel(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "text_model.create", "text_model", fmt.Sprintf("%d", created.ID), map[string]any{"account_id": accountID, "model_code": created.ModelCode})
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminTextModelDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.textModels == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "text model configuration is unavailable"))
		return
	}
	rawID := strings.TrimPrefix(r.URL.Path, "/api/ops/admin/v1/text-models/")
	isDefault := strings.HasSuffix(rawID, ":default")
	isTest := strings.HasSuffix(rawID, ":test")
	rawID = strings.TrimSuffix(rawID, ":default")
	rawID = strings.TrimSuffix(rawID, ":test")
	modelID, err := strconv.ParseInt(strings.Trim(rawID, "/"), 10, 64)
	if err != nil || modelID <= 0 {
		httpx.WriteError(w, r, errs.BadRequest("invalid model_id"))
		return
	}
	if isDefault {
		if r.Method != http.MethodPut {
			writeMethodNotAllowed(w, r)
			return
		}
		selected, selectErr := a.textModels.SetDefaultModel(r.Context(), modelID)
		if selectErr != nil {
			httpx.WriteError(w, r, normalizeAppError(selectErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model.set_default", "text_model", fmt.Sprintf("%d", modelID), nil)
		httpx.WriteSuccess(w, r, http.StatusOK, selected)
		return
	}
	if isTest {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		result, testErr := a.textModels.TestModelConnection(r.Context(), modelID)
		if testErr != nil {
			httpx.WriteError(w, r, normalizeAppError(testErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model.test", "text_model", fmt.Sprintf("%d", modelID), map[string]any{"status": result.Status, "latency_ms": result.LatencyMS})
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	switch r.Method {
	case http.MethodPut:
		var req domaintextmodel.ModelWriteRequest
		if !decodeJSONBody(w, r, &req) {
			return
		}
		updated, updateErr := a.textModels.UpdateModel(r.Context(), modelID, req)
		if updateErr != nil {
			httpx.WriteError(w, r, normalizeAppError(updateErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model.update", "text_model", fmt.Sprintf("%d", modelID), map[string]any{"enabled": updated.Enabled})
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if deleteErr := a.textModels.DeleteModel(r.Context(), modelID); deleteErr != nil {
			httpx.WriteError(w, r, normalizeAppError(deleteErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "text_model.delete", "text_model", fmt.Sprintf("%d", modelID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandlePromptOptimizationEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.promptOpt == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "prompt optimization is unavailable"))
		return
	}
	var req promptoptimizer.EstimateRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.UserID = user.ID
	result, err := a.promptOpt.Estimate(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) HandlePromptOptimization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if a.promptOpt == nil {
		httpx.WriteError(w, r, errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "prompt optimization is unavailable"))
		return
	}
	var req promptoptimizer.OptimizeRequest
	if !decodeJSONBody(w, r, &req) {
		return
	}
	req.UserID = user.ID
	result, err := a.promptOpt.Optimize(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return false
	}
	return true
}
