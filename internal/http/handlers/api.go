package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	capserv "github.com/fatballfish/pic-gallery/internal/service/capabilities"
	compatservice "github.com/fatballfish/pic-gallery/internal/service/compat"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

type API struct {
	auth    *authservice.Service
	apiKeys *apikeyservice.Service
	billing *billingservice.Service
	assets  *assetservice.Service
	caps    *capserv.Service
	compat  *compatservice.Service
	tasks   *imagetaskservice.Service
	admin   *adminconfigservice.Service
	cfg     config.Config
}

func NewAPI(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service) *API {
	return NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, nil, nil, nil)
}

func NewAPIWithTaskService(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service) *API {
	return NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, nil, nil)
}

func NewAPIWithServices(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service, adminSvc *adminconfigservice.Service) *API {
	return NewAPIWithRuntimeServices(cfg, authSvc, assetSvc, taskSvc, adminSvc, nil)
}

func NewAPIWithRuntimeServices(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service, adminSvc *adminconfigservice.Service, billingSvc *billingservice.Service, apiKeySvcs ...*apikeyservice.Service) *API {
	if authSvc == nil {
		authSvc = authservice.NewService(cfg.Auth, cfg.Billing.UserGroupMultipliers)
	}
	var apiKeySvc *apikeyservice.Service
	if len(apiKeySvcs) > 0 {
		apiKeySvc = apiKeySvcs[0]
	}
	if apiKeySvc == nil {
		apiKeySvc = apikeyservice.NewServiceWithSigningSecretKey(nil, cfg.APIKey.SigningSecretEncryptionKey)
	}
	if billingSvc == nil && taskSvc != nil {
		if sharedBilling, ok := taskSvc.BillingManager().(*billingservice.Service); ok {
			billingSvc = sharedBilling
		}
	}
	if billingSvc == nil {
		billingSvc = billingservice.NewService(cfg.Billing)
	}
	apiKeySvc.SetUsageStore(billingSvc)
	if assetSvc == nil {
		assetSvc = assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	}
	if taskSvc == nil {
		taskSvc = imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, nil, nil, billingSvc)
	}
	taskSvc.SetAPIKeyUsageManager(apiKeySvc)
	if adminSvc == nil {
		adminSvc = adminconfigservice.NewService(cfg)
	}
	return &API{
		auth:    authSvc,
		apiKeys: apiKeySvc,
		billing: billingSvc,
		assets:  assetSvc,
		caps:    capserv.NewService(cfg),
		compat:  compatservice.NewServiceWithTaskService(cfg, taskSvc),
		tasks:   taskSvc,
		admin:   adminSvc,
		cfg:     cfg,
	}
}

func (a *API) HandleSendEmailCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Scene string `json:"scene"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if err := a.auth.SendEmailCode(req.Email, req.Scene); err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, map[string]any{
		"email":  req.Email,
		"scene":  req.Scene,
		"status": "queued",
	})
}

func (a *API) HandleEmailCodeLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	user, session, err := a.auth.LoginWithEmailCode(req.Email, req.Code)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	a.setRefreshCookie(w, session)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"access_token":       session.AccessToken,
		"expires_in_seconds": int(time.Until(session.AccessTokenExpiresAt).Seconds()),
		"user_id":            user.ID,
	})
}

func (a *API) HandleRefresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(a.cfg.Auth.RefreshCookieName)
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeAuthRefreshExpired, "refresh token expired"))
		return
	}
	user, session, appErr := a.auth.Refresh(cookie.Value)
	if appErr != nil {
		httpx.WriteError(w, r, appErr.(*errs.Error))
		return
	}
	a.setRefreshCookie(w, session)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"access_token":       session.AccessToken,
		"expires_in_seconds": int(time.Until(session.AccessTokenExpiresAt).Seconds()),
		"user_id":            user.ID,
	})
}

func (a *API) HandleProfile(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodPut {
		var req struct {
			Nickname      *string `json:"nickname"`
			Bio           *string `json:"bio"`
			Signature     *string `json:"signature"`
			Theme         *string `json:"theme"`
			DefaultLocale *string `json:"default_locale"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		bio := req.Bio
		if bio == nil {
			bio = req.Signature
		}
		updated, err := a.auth.UpdateProfile(user.ID, req.Nickname, bio, req.Theme, req.DefaultLocale, nil)
		if err != nil {
			httpx.WriteError(w, r, err.(*errs.Error))
			return
		}
		writeProfile(w, r, updated)
		return
	}
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	writeProfile(w, r, *user)
}

func writeProfile(w http.ResponseWriter, r *http.Request, user domainauth.User) {
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"id":                user.ID,
		"email":             user.Email,
		"nickname":          user.Nickname,
		"bio":               user.Bio,
		"signature":         user.Bio,
		"avatar_object_key": user.AvatarObjectKey,
		"user_group_code":   user.GroupCode,
		"theme":             defaultString(user.Theme, "system"),
		"default_locale":    defaultString(user.DefaultLocale, "zh-CN"),
	})
}

func (a *API) HandleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	summary, err := a.billing.GetBalance(r.Context(), user.ID, user.GroupMultiplier)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, summary)
}

func (a *API) HandleLedger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	page, queryErr := parsePositiveIntQuery(r, "page", 1)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	pageSize, queryErr := parsePositiveIntQuery(r, "page_size", 20)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	result, err := a.billing.ListLedger(r.Context(), user.ID, page, pageSize)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": result.Items,
		"pagination": map[string]any{
			"page":      result.Page,
			"page_size": result.PageSize,
			"total":     result.Total,
		},
	})
}

func (a *API) HandleEstimate(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	outputCount, queryErr := parsePositiveIntQuery(r, "requested_output_image_count", 1)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	refCount, queryErr := parseNonNegativeIntQuery(r, "reference_image_count", 0)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	result, err := a.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  r.URL.Query().Get("task_type"),
		AbstractModel:             r.URL.Query().Get("abstract_model"),
		RequestedQuality:          r.URL.Query().Get("requested_quality"),
		RequestedSize:             r.URL.Query().Get("requested_size"),
		RequestedOutputImageCount: outputCount,
		ReferenceImageCount:       refCount,
		UserGroupCode:             user.GroupCode,
		UserGroupMultiplier:       user.GroupMultiplier,
	})
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) HandleOpenEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	outputCount, queryErr := parsePositiveIntQuery(r, "requested_output_image_count", 1)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	refCount, queryErr := parseNonNegativeIntQuery(r, "reference_image_count", 0)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	result, err := a.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  r.URL.Query().Get("task_type"),
		AbstractModel:             r.URL.Query().Get("abstract_model"),
		RequestedQuality:          r.URL.Query().Get("requested_quality"),
		RequestedSize:             r.URL.Query().Get("requested_size"),
		RequestedOutputImageCount: outputCount,
		ReferenceImageCount:       refCount,
		UserGroupCode:             identity.GroupCode,
		UserGroupMultiplier:       a.userGroupMultiplier(identity.GroupCode),
	})
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) HandleCapabilities(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireUser(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, a.caps.List())
}

func (a *API) HandleReferenceAssetUpload(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, errs.CodeImageReferenceRequired, "file is required"))
		return
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("failed to read upload"))
		return
	}

	asset, svcErr := a.assets.Upload(user.ID, header.Filename, header.Header.Get("Content-Type"), content)
	if svcErr != nil {
		httpx.WriteError(w, r, svcErr.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, asset)
}

func (a *API) HandleReferenceAssetGet(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	assetID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/reference-assets/")
	if r.Method == http.MethodDelete {
		_ = assetID
		w.WriteHeader(http.StatusNoContent)
		return
	}
	asset, err := a.assets.Get(user.ID, assetID)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, asset)
}

func (a *API) HandleOpenReferenceAssetUploadSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		Filename      string `json:"filename"`
		MimeType      string `json:"mime_type"`
		ContentBase64 string `json:"content_base64"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if strings.TrimSpace(req.Filename) == "" || strings.TrimSpace(req.ContentBase64) == "" {
		httpx.WriteError(w, r, errs.BadRequest("filename and content_base64 are required"))
		return
	}
	content, err := decodeBase64Payload(req.ContentBase64)
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid content_base64"))
		return
	}
	apiKeyID := identity.APIKeyID
	asset, svcErr := a.assets.UploadWithMetadata(identity.UserID, req.Filename, req.MimeType, content, domainassets.UploadMetadata{
		APIKeyID:     &apiKeyID,
		UploadSource: "openapi",
	})
	if svcErr != nil {
		httpx.WriteError(w, r, svcErr.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, map[string]any{
		"asset_id":    asset.ID,
		"status":      asset.Status,
		"upload_mode": "inline_base64",
		"asset":       asset,
	})
}

func (a *API) HandleAgentTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleAgentTaskCreate(w, r)
	case http.MethodGet:
		a.handleAgentTaskList(w, r)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAgentTaskDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	taskID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/tasks/")
	task, err := a.tasks.GetByID(r.Context(), user.ID, taskID)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, task)
}

func (a *API) HandleAgentHistoryTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleAgentTaskList(w, r)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAgentHistoryTaskDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	taskID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/history/tasks/")
	switch r.Method {
	case http.MethodGet:
		task, err := a.tasks.GetByID(r.Context(), user.ID, taskID)
		if err != nil {
			httpx.WriteError(w, r, err.(*errs.Error))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, task)
	case http.MethodDelete:
		if err := a.tasks.DeleteByID(r.Context(), user.ID, taskID); err != nil {
			httpx.WriteError(w, r, err.(*errs.Error))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAdminConfigTabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	tabs, err := a.admin.ListTabs(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": tabs})
}

func (a *API) HandleAdminConfigTabDetail(w http.ResponseWriter, r *http.Request) {
	adminID, ok := a.requireAdmin(w, r)
	if !ok {
		return
	}
	tabKey := strings.TrimPrefix(r.URL.Path, "/api/ops/admin/v1/config-tabs/")
	switch r.Method {
	case http.MethodPut:
		var req struct {
			Version int64 `json:"version"`
			Items   []struct {
				ConfigCategory string         `json:"config_category"`
				ConfigKey      string         `json:"config_key"`
				ConfigValue    map[string]any `json:"config_value"`
				Scope          string         `json:"scope"`
			} `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		items := make([]domainadminconfig.Item, 0, len(req.Items))
		for _, item := range req.Items {
			items = append(items, domainadminconfig.Item{
				ConfigCategory: item.ConfigCategory,
				ConfigKey:      item.ConfigKey,
				ConfigValue:    item.ConfigValue,
				Scope:          item.Scope,
			})
		}
		tab, err := a.admin.UpdateTab(r.Context(), domainadminconfig.UpdateTabRequest{
			TabKey:    tabKey,
			Version:   req.Version,
			Items:     items,
			UpdatedBy: adminID,
		})
		if err != nil {
			httpx.WriteError(w, r, err.(*errs.Error))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, tab)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleOpenAIImageGeneration(w http.ResponseWriter, r *http.Request) {
	identity, appErr := a.requireCompatAPIKey(r)
	if appErr != nil {
		a.writeCompatError(w, appErr)
		return
	}

	var req struct {
		Model          string `json:"model"`
		Prompt         string `json:"prompt"`
		Size           string `json:"size"`
		N              int    `json:"n"`
		Quality        string `json:"quality"`
		ResponseFormat string `json:"response_format"`
		User           string `json:"user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid json body"))
		return
	}

	resp, err := a.compat.Generate(r.Context(), compatservice.GenerateRequest{
		UserID:              identity.UserID,
		APIKeyID:            identity.APIKeyID,
		SourceChannel:       "openai_compat",
		UserGroupCode:       identity.GroupCode,
		UserGroupMultiplier: a.userGroupMultiplier(identity.GroupCode),
		Model:               req.Model,
		Prompt:              req.Prompt,
		Size:                req.Size,
		N:                   req.N,
		Quality:             req.Quality,
		ResponseFormat:      req.ResponseFormat,
		User:                req.User,
	})
	if err != nil {
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *API) HandleOpenAIImageEdit(w http.ResponseWriter, r *http.Request) {
	identity, appErr := a.requireCompatAPIKey(r)
	if appErr != nil {
		a.writeCompatError(w, appErr)
		return
	}
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		a.writeCompatError(w, errs.BadRequest("invalid multipart form"))
		return
	}

	images, err := readCompatImages(r.MultipartForm.File["image"])
	if err != nil {
		a.writeCompatError(w, errs.BadRequest("failed to read image input"))
		return
	}

	var mask *provider.ImageInput
	if len(r.MultipartForm.File["mask"]) > 0 {
		loadedMask, maskErr := readCompatImage(r.MultipartForm.File["mask"][0])
		if maskErr != nil {
			a.writeCompatError(w, errs.BadRequest("failed to read mask input"))
			return
		}
		mask = &loadedMask
	}

	count, _ := strconv.Atoi(defaultString(r.FormValue("n"), "1"))
	resp, compatErr := a.compat.Edit(r.Context(), compatservice.EditRequest{
		UserID:              identity.UserID,
		APIKeyID:            identity.APIKeyID,
		SourceChannel:       "openai_compat",
		UserGroupCode:       identity.GroupCode,
		UserGroupMultiplier: a.userGroupMultiplier(identity.GroupCode),
		Model:               r.FormValue("model"),
		Prompt:              r.FormValue("prompt"),
		Size:                r.FormValue("size"),
		N:                   count,
		Quality:             r.FormValue("quality"),
		ResponseFormat:      r.FormValue("response_format"),
		User:                r.FormValue("user"),
		Images:              images,
		Mask:                mask,
	})
	if compatErr != nil {
		a.writeCompatError(w, compatservice.MapError(compatErr))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *API) HandleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireCompatAPIKey(r); appErr != nil {
		a.writeCompatError(w, appErr)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, a.compat.Models())
}

func (a *API) HandleOpenTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleOpenTaskCreate(w, r)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) handleAgentTaskCreate(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}

	var req struct {
		TaskType                  string   `json:"task_type"`
		Prompt                    string   `json:"prompt"`
		NegativePrompt            string   `json:"negative_prompt"`
		AbstractModel             string   `json:"abstract_model"`
		RequestedQuality          string   `json:"requested_quality"`
		Quality                   string   `json:"quality"`
		RequestedSize             string   `json:"requested_size"`
		AspectRatio               string   `json:"aspect_ratio"`
		RequestedOutputImageCount int      `json:"requested_output_image_count"`
		ReferenceAssetIDs         []string `json:"reference_asset_ids"`
		ReferenceStrength         int      `json:"reference_strength"`
		Seed                      *int64   `json:"seed"`
		ResponseMode              string   `json:"response_mode"`
		SavePolicy                string   `json:"save_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}

	result, err := a.tasks.CreateTask(r.Context(), domainimagetask.CreateRequest{
		TaskID:              idempotentTaskID(user.ID, r.Header.Get("Idempotency-Key"), req),
		UserID:              user.ID,
		AbstractModel:       req.AbstractModel,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		NegativePrompt:      req.NegativePrompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    defaultString(req.RequestedQuality, req.Quality),
		AspectRatio:         req.AspectRatio,
		OutputImageCount:    req.RequestedOutputImageCount,
		ReferenceImageCount: len(req.ReferenceAssetIDs),
		ReferenceAssetIDs:   append([]string(nil), req.ReferenceAssetIDs...),
		UserGroupCode:       user.GroupCode,
		UserGroupMultiplier: user.GroupMultiplier,
		ReferenceStrength:   req.ReferenceStrength,
		Seed:                req.Seed,
		ResponseMode:        req.ResponseMode,
		SavePolicy:          defaultString(req.SavePolicy, "private"),
	})
	if err != nil {
		httpx.WriteError(w, r, compatservice.MapError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, result)
}

func (a *API) handleOpenTaskCreate(w http.ResponseWriter, r *http.Request) {
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}

	var req struct {
		TaskType                  string   `json:"task_type"`
		Prompt                    string   `json:"prompt"`
		NegativePrompt            string   `json:"negative_prompt"`
		AbstractModel             string   `json:"abstract_model"`
		RequestedQuality          string   `json:"requested_quality"`
		Quality                   string   `json:"quality"`
		RequestedSize             string   `json:"requested_size"`
		AspectRatio               string   `json:"aspect_ratio"`
		RequestedOutputImageCount int      `json:"requested_output_image_count"`
		ReferenceAssetIDs         []string `json:"reference_asset_ids"`
		ReferenceStrength         int      `json:"reference_strength"`
		Seed                      *int64   `json:"seed"`
		ResponseMode              string   `json:"response_mode"`
		SavePolicy                string   `json:"save_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}

	result, err := a.tasks.CreateTask(r.Context(), domainimagetask.CreateRequest{
		TaskID:              idempotentTaskID(identity.UserID, r.Header.Get("Idempotency-Key"), req),
		UserID:              identity.UserID,
		APIKeyID:            identity.APIKeyID,
		SourceChannel:       "openapi",
		AbstractModel:       req.AbstractModel,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		NegativePrompt:      req.NegativePrompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    defaultString(req.RequestedQuality, req.Quality),
		AspectRatio:         req.AspectRatio,
		OutputImageCount:    req.RequestedOutputImageCount,
		ReferenceImageCount: len(req.ReferenceAssetIDs),
		ReferenceAssetIDs:   append([]string(nil), req.ReferenceAssetIDs...),
		UserGroupCode:       identity.GroupCode,
		UserGroupMultiplier: a.userGroupMultiplier(identity.GroupCode),
		ReferenceStrength:   req.ReferenceStrength,
		Seed:                req.Seed,
		ResponseMode:        req.ResponseMode,
		SavePolicy:          defaultString(req.SavePolicy, "private"),
	})
	if err != nil {
		httpx.WriteError(w, r, compatservice.MapError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, result)
}

func (a *API) handleAgentTaskList(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	tasks, err := a.tasks.ListByUser(r.Context(), user.ID)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, tasks)
}

func (a *API) requireUser(r *http.Request) (*domainauth.User, *errs.Error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token")
	}
	user, _, err := a.auth.ValidateAccessToken(strings.TrimPrefix(authHeader, "Bearer "))
	if err != nil {
		return nil, err.(*errs.Error)
	}
	return &user, nil
}

func (a *API) requireOpenAPIKey(r *http.Request) (domainapikey.Identity, *errs.Error) {
	timestamp := strings.TrimSpace(r.Header.Get("X-Timestamp"))
	if timestamp == "" {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing api key timestamp")
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return domainapikey.Identity{}, errs.BadRequest("failed to read request body")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	sum := sha256.Sum256(body)
	identity, err := a.apiKeys.AuthenticateCanonical(r.Context(), r.Method, r.URL.Path, timestamp, hex.EncodeToString(sum[:]), r.Header.Get("X-Access-Key"), r.Header.Get("X-Signature"), 5*time.Minute)
	if err != nil {
		return domainapikey.Identity{}, normalizeAppError(err)
	}
	return identity, nil
}

func (a *API) requireCompatAPIKey(r *http.Request) (domainapikey.Identity, *errs.Error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	identity, err := a.apiKeys.AuthenticateBearer(r.Context(), token)
	if err != nil {
		return domainapikey.Identity{}, normalizeAppError(err)
	}
	return identity, nil
}

func normalizeAppError(err error) *errs.Error {
	if appErr, ok := err.(*errs.Error); ok {
		return appErr
	}
	return errs.Internal("internal server error")
}

func (a *API) setRefreshCookie(w http.ResponseWriter, session domainauth.Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     a.cfg.Auth.RefreshCookieName,
		Value:    session.RefreshToken,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  session.RefreshTokenExpiresAt,
	})
}

func (a *API) requireCompatBearer(r *http.Request) *errs.Error {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token")
	}
	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	if token == "" {
		return errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token")
	}
	return nil
}

func (a *API) userGroupMultiplier(groupCode string) string {
	if value := strings.TrimSpace(a.cfg.Billing.UserGroupMultipliers[groupCode]); value != "" {
		return value
	}
	return "1.00000"
}

func (a *API) writeCompatError(w http.ResponseWriter, err *errs.Error) {
	if err == nil {
		err = errs.Internal("")
	}
	errType := "server_error"
	if err.StatusCode >= 400 && err.StatusCode < 500 {
		errType = "invalid_request_error"
	}
	httpx.WriteJSON(w, err.StatusCode, map[string]any{
		"error": map[string]any{
			"message": err.Message,
			"type":    errType,
			"code":    err.Code,
		},
	})
}

func readCompatImages(files []*multipart.FileHeader) ([]provider.ImageInput, error) {
	images := make([]provider.ImageInput, 0, len(files))
	for _, file := range files {
		image, err := readCompatImage(file)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

func readCompatImage(file *multipart.FileHeader) (provider.ImageInput, error) {
	opened, err := file.Open()
	if err != nil {
		return provider.ImageInput{}, err
	}
	defer opened.Close()

	content, err := io.ReadAll(opened)
	if err != nil {
		return provider.ImageInput{}, err
	}
	return provider.ImageInput{
		Filename: file.Filename,
		MIMEType: file.Header.Get("Content-Type"),
		Data:     content,
	}, nil
}

func decodeBase64Payload(value string) ([]byte, error) {
	trimmed := strings.TrimSpace(value)
	if comma := strings.Index(trimmed, ","); comma >= 0 && strings.Contains(trimmed[:comma], ";base64") {
		trimmed = trimmed[comma+1:]
	}
	return base64.StdEncoding.DecodeString(trimmed)
}

func parsePositiveIntQuery(r *http.Request, key string, fallback int) (int, *errs.Error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errs.BadRequest("invalid " + key)
	}
	return value, nil
}

func parseNonNegativeIntQuery(r *http.Request, key string, fallback int) (int, *errs.Error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, errs.BadRequest("invalid " + key)
	}
	return value, nil
}

func idempotentTaskID(userID int64, key string, payload any) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return ""
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("{}")
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(strconv.FormatInt(userID, 10)+":"+trimmed+":"+string(body))).String()
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func taskProviderPreference(cfg config.Config) []string {
	preferred := []string{}
	if cfg.Routing.DefaultProvider != "" {
		preferred = append(preferred, cfg.Routing.DefaultProvider)
	}
	preferred = append(preferred, cfg.Routing.FallbackProviders...)
	return preferred
}
