package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainadminuser "github.com/fatballfish/pic-gallery/internal/domain/adminuser"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	domainredeem "github.com/fatballfish/pic-gallery/internal/domain/redeem"
	"github.com/fatballfish/pic-gallery/internal/provider"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	admincallrecordservice "github.com/fatballfish/pic-gallery/internal/service/admincallrecord"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	adminuserservice "github.com/fatballfish/pic-gallery/internal/service/adminuser"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	capserv "github.com/fatballfish/pic-gallery/internal/service/capabilities"
	compatservice "github.com/fatballfish/pic-gallery/internal/service/compat"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	redeemservice "github.com/fatballfish/pic-gallery/internal/service/redeem"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
	"gopkg.in/yaml.v3"
)

type API struct {
	auth       *authservice.Service
	adminAuth  *adminauthservice.Service
	apiKeys    *apikeyservice.Service
	billing    *billingservice.Service
	assets     *assetservice.Service
	caps       *capserv.Service
	compat     *compatservice.Service
	tasks      *imagetaskservice.Service
	admin      *adminconfigservice.Service
	adminUser  *adminuserservice.Service
	callRecord *admincallrecordservice.Service
	modelAdmin *modeladminservice.Service
	redeem     *redeemservice.Service
	audit      *auditservice.Service
	cfg        config.Config
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
	return NewAPIWithCompletionServices(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, firstAPIKeyService(apiKeySvcs), nil, nil)
}

func NewAPIWithCompletionServices(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service, adminSvc *adminconfigservice.Service, billingSvc *billingservice.Service, apiKeySvc *apikeyservice.Service, adminAuthSvc *adminauthservice.Service, auditSvc *auditservice.Service, adminUserSvcs ...*adminuserservice.Service) *API {
	if authSvc == nil {
		authSvc = authservice.NewService(cfg.Auth, cfg.Billing.UserGroupMultipliers)
	}
	if apiKeySvc == nil {
		apiKeySvc = apikeyservice.NewService(nil)
	}
	if billingSvc == nil && taskSvc != nil {
		if sharedBilling, ok := taskSvc.BillingManager().(*billingservice.Service); ok {
			billingSvc = sharedBilling
		}
	}
	if billingSvc == nil {
		billingSvc = billingservice.NewService(cfg.Billing)
	}
	if assetSvc == nil {
		assetSvc = assetservice.NewService(cfg.Storage, cfg.GenerationLimits)
	}
	if taskSvc == nil {
		taskSvc = imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, nil, nil, billingSvc)
	}
	if adminSvc == nil {
		adminSvc = adminconfigservice.NewService(cfg)
	}
	if adminAuthSvc == nil {
		adminAuthSvc = adminauthservice.NewService(cfg.Auth, nil)
	}
	if auditSvc == nil {
		auditSvc = auditservice.NewService(nil)
	}
	adminUserSvc := firstAdminUserService(adminUserSvcs)
	if adminUserSvc == nil {
		adminUserSvc = adminuserservice.NewServiceWithStore(nil, billingSvc)
	}
	callRecordSvc := admincallrecordservice.NewServiceWithStore(nil)
	return &API{
		auth:       authSvc,
		adminAuth:  adminAuthSvc,
		apiKeys:    apiKeySvc,
		billing:    billingSvc,
		assets:     assetSvc,
		caps:       capserv.NewService(cfg),
		compat:     compatservice.NewServiceWithTaskService(cfg, taskSvc),
		tasks:      taskSvc,
		admin:      adminSvc,
		adminUser:  adminUserSvc,
		callRecord: callRecordSvc,
		redeem:     redeemservice.NewServiceWithStore(nil),
		audit:      auditSvc,
		cfg:        cfg,
	}
}

func NewAPIWithAdminServices(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service, adminSvc *adminconfigservice.Service, billingSvc *billingservice.Service, apiKeySvc *apikeyservice.Service, adminAuthSvc *adminauthservice.Service, auditSvc *auditservice.Service, adminUserSvc *adminuserservice.Service, redeemSvc *redeemservice.Service) *API {
	return NewAPIWithCallRecordService(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, apiKeySvc, adminAuthSvc, auditSvc, adminUserSvc, redeemSvc, nil)
}

func NewAPIWithCallRecordService(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service, adminSvc *adminconfigservice.Service, billingSvc *billingservice.Service, apiKeySvc *apikeyservice.Service, adminAuthSvc *adminauthservice.Service, auditSvc *auditservice.Service, adminUserSvc *adminuserservice.Service, redeemSvc *redeemservice.Service, callRecordSvc *admincallrecordservice.Service) *API {
	return NewAPIWithModelAdminService(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, apiKeySvc, adminAuthSvc, auditSvc, adminUserSvc, redeemSvc, callRecordSvc, nil)
}

func NewAPIWithModelAdminService(cfg config.Config, authSvc *authservice.Service, assetSvc *assetservice.Service, taskSvc *imagetaskservice.Service, adminSvc *adminconfigservice.Service, billingSvc *billingservice.Service, apiKeySvc *apikeyservice.Service, adminAuthSvc *adminauthservice.Service, auditSvc *auditservice.Service, adminUserSvc *adminuserservice.Service, redeemSvc *redeemservice.Service, callRecordSvc *admincallrecordservice.Service, modelAdminSvc *modeladminservice.Service) *API {
	api := NewAPIWithCompletionServices(cfg, authSvc, assetSvc, taskSvc, adminSvc, billingSvc, apiKeySvc, adminAuthSvc, auditSvc, adminUserSvc)
	if redeemSvc == nil {
		redeemSvc = redeemservice.NewServiceWithStore(nil)
	}
	if callRecordSvc == nil {
		callRecordSvc = admincallrecordservice.NewServiceWithStore(nil)
	}
	if modelAdminSvc == nil {
		modelAdminSvc = modeladminservice.NewServiceWithStore(nil)
	}
	api.redeem = redeemSvc
	api.callRecord = callRecordSvc
	api.modelAdmin = modelAdminSvc
	return api
}

func firstAPIKeyService(values []*apikeyservice.Service) *apikeyservice.Service {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func firstAdminUserService(values []*adminuserservice.Service) *adminuserservice.Service {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func (a *API) HandleSendEmailCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
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
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
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

func (a *API) HandlePasswordLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	user, session, err := a.auth.LoginWithPassword(req.Email, req.Password)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
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
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
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
	switch r.Method {
	case http.MethodGet:
		httpx.WriteSuccess(w, r, http.StatusOK, profilePayload(user))
	case http.MethodPut:
		var req struct {
			Nickname        string `json:"nickname"`
			Bio             string `json:"bio"`
			AvatarObjectKey string `json:"avatar_object_key"`
			DefaultLocale   string `json:"default_locale"`
			Theme           string `json:"theme"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.auth.UpdateProfile(domainauth.UpdateProfileRequest{
			UserID:          user.ID,
			Nickname:        req.Nickname,
			Bio:             req.Bio,
			AvatarObjectKey: req.AvatarObjectKey,
			DefaultLocale:   req.DefaultLocale,
			Theme:           req.Theme,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, profilePayload(&updated))
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandlePreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		Theme         string `json:"theme"`
		DefaultLocale string `json:"default_locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if strings.TrimSpace(req.Theme) == "" {
		req.Theme = user.Theme
	}
	if strings.TrimSpace(req.DefaultLocale) == "" {
		req.DefaultLocale = user.DefaultLocale
	}
	updated, err := a.auth.UpdateProfile(domainauth.UpdateProfileRequest{
		UserID:          user.ID,
		Nickname:        user.Nickname,
		Bio:             user.Bio,
		AvatarObjectKey: user.AvatarObjectKey,
		DefaultLocale:   req.DefaultLocale,
		Theme:           req.Theme,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, profilePayload(&updated))
}

func (a *API) HandleAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("file is required"))
		return
	}
	defer file.Close()
	content, err := io.ReadAll(io.LimitReader(file, 2*1024*1024+1))
	if err != nil || len(content) > 2*1024*1024 {
		httpx.WriteError(w, r, errs.BadRequest("avatar must be <= 2MB"))
		return
	}
	asset, err := a.assets.Upload(user.ID, header.Filename, header.Header.Get("Content-Type"), content)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	updated, err := a.auth.UpdateProfile(domainauth.UpdateProfileRequest{
		UserID:          user.ID,
		Nickname:        user.Nickname,
		Bio:             user.Bio,
		AvatarObjectKey: asset.ObjectKey,
		DefaultLocale:   user.DefaultLocale,
		Theme:           user.Theme,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, profilePayload(&updated))
}

func (a *API) HandleCloseAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	closed, err := a.auth.CloseAccount(user.ID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.cfg.Auth.RefreshCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "user.account_close", "user", fmt.Sprintf("%d", user.ID), nil)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"id":        closed.ID,
		"status":    closed.Status,
		"closed_at": closed.ClosedAt,
	})
}

func (a *API) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if _, err := a.auth.ChangePassword(user.ID, req.OldPassword, req.NewPassword); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if cookie, err := r.Cookie(a.cfg.Auth.RefreshCookieName); err == nil {
		_ = a.auth.Logout(cookie.Value)
	}
	a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "auth.password_change", "user", fmt.Sprintf("%d", user.ID), nil)
	http.SetCookie(w, &http.Cookie{Name: a.cfg.Auth.RefreshCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "password_updated"})
}

func (a *API) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	httpx.WriteError(w, r, errs.BadRequest("use /api/agent/auth/v1/password/reset/request and /confirm"))
}

func (a *API) HandlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if err := a.auth.RequestPasswordReset(req.Email); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusAccepted, map[string]any{
		"email":  strings.TrimSpace(strings.ToLower(req.Email)),
		"status": "queued",
	})
}

func (a *API) HandlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var req struct {
		Email       string `json:"email"`
		Code        string `json:"code"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	user, err := a.auth.ConfirmPasswordReset(req.Email, req.Code, req.NewPassword)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "auth.password_reset", "user", fmt.Sprintf("%d", user.ID), nil)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"status":  "password_reset",
		"user_id": user.ID,
	})
}

func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if cookie, err := r.Cookie(a.cfg.Auth.RefreshCookieName); err == nil {
		_ = a.auth.Logout(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: a.cfg.Auth.RefreshCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "auth.logout", "user", fmt.Sprintf("%d", user.ID), nil)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "logged_out"})
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
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
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

func (a *API) HandleBillingPlans(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireUser(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	result, err := a.billing.ListPlans(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": result})
}

func (a *API) HandleBillingSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	result, err := a.billing.GetSubscription(r.Context(), user.ID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"item": result})
}

func (a *API) HandleBillingOrders(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		page := 1
		pageSize := 20
		if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				httpx.WriteError(w, r, errs.BadRequest("invalid page"))
				return
			}
			page = parsed
		}
		if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				httpx.WriteError(w, r, errs.BadRequest("invalid page_size"))
				return
			}
			pageSize = parsed
		}
		result, err := a.billing.ListOrders(r.Context(), domainbilling.ListOrdersRequest{UserID: user.ID, Page: page, PageSize: pageSize})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case http.MethodPost:
		var req struct {
			PlanCode string `json:"plan_code"`
			Provider string `json:"provider"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		result, err := a.billing.CreateOrder(r.Context(), domainbilling.CreateOrderRequest{
			UserID:   user.ID,
			PlanCode: strings.TrimSpace(req.PlanCode),
			Provider: strings.TrimSpace(req.Provider),
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, result)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleBillingOrderDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	orderID, action, parseErr := parseBillingOrderPath(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		result, err := a.billing.GetOrder(r.Context(), user.ID, orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case r.Method == http.MethodPost && action == "cancel":
		result, err := a.billing.CancelOrder(r.Context(), user.ID, orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
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
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireUser(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, a.caps.List())
}

func (a *API) HandleOpenCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireOpenAPIKey(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, a.caps.List())
}

func (a *API) HandleOpenBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	summary, err := a.billing.GetBalance(r.Context(), identity.UserID, a.userGroupMultiplier(identity.GroupCode))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, summary)
}

func (a *API) HandleAgentAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := a.apiKeys.ListByUser(r.Context(), user.ID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": apiKeyPayloads(keys)})
	case http.MethodPost:
		var req struct {
			Name             string     `json:"name"`
			GroupCode        string     `json:"group_code"`
			TotalQuotaPoints *string    `json:"total_quota_points"`
			DailyQuotaPoints *string    `json:"daily_quota_points"`
			RPMLimit         *int       `json:"rpm_limit"`
			ExpiresAt        *time.Time `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		created, err := a.apiKeys.CreateKey(r.Context(), apikeyservice.CreateRequest{UserID: user.ID, Name: req.Name, GroupCode: user.GroupCode, TotalQuotaPoints: req.TotalQuotaPoints, DailyQuotaPoints: req.DailyQuotaPoints, RPMLimit: req.RPMLimit, ExpiresAt: req.ExpiresAt})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "api_key.create", "api_key", fmt.Sprintf("%d", created.Key.ID), map[string]any{"secret": created.Secret})
		payload := apiKeyPayload(created.Key)
		payload["secret"] = created.Secret
		httpx.WriteSuccess(w, r, http.StatusCreated, payload)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAgentAPIKeyDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	idPart := strings.TrimPrefix(r.URL.Path, "/api/agent/account/v1/api-keys/")
	parts := strings.Split(strings.Trim(idPart, "/"), "/")
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, r, errs.BadRequest("invalid api key id"))
		return
	}
	if len(parts) == 2 && parts[1] == "reset-secret" {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
			return
		}
		reset, err := a.apiKeys.ResetSecret(r.Context(), user.ID, id)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "api_key.reset_secret", "api_key", fmt.Sprintf("%d", id), map[string]any{"secret": reset.Secret})
		payload := apiKeyPayload(reset.Key)
		payload["secret"] = reset.Secret
		httpx.WriteSuccess(w, r, http.StatusOK, payload)
		return
	}
	if len(parts) > 1 {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "api key route not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		key, err := a.apiKeys.GetByID(r.Context(), user.ID, id)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, apiKeyPayload(key))
	case http.MethodPut:
		var req struct {
			Name             *string    `json:"name"`
			GroupCode        *string    `json:"group_code"`
			Status           *string    `json:"status"`
			TotalQuotaPoints *string    `json:"total_quota_points"`
			DailyQuotaPoints *string    `json:"daily_quota_points"`
			RPMLimit         *int       `json:"rpm_limit"`
			ExpiresAt        *time.Time `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		if req.GroupCode != nil {
			httpx.WriteError(w, r, errs.BadRequest("group_code cannot be changed"))
			return
		}
		key, err := a.apiKeys.Update(r.Context(), user.ID, id, apikeyservice.UpdateRequest{Name: req.Name, TotalQuotaPoints: req.TotalQuotaPoints, DailyQuotaPoints: req.DailyQuotaPoints, RPMLimit: req.RPMLimit, ExpiresAt: req.ExpiresAt})
		if err == nil && req.Status != nil {
			err = a.apiKeys.UpdateStatus(r.Context(), user.ID, id, *req.Status)
			if err == nil {
				key, err = a.apiKeys.GetByID(r.Context(), user.ID, id)
			}
		}
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "api_key.update", "api_key", fmt.Sprintf("%d", id), nil)
		httpx.WriteSuccess(w, r, http.StatusOK, apiKeyPayload(key))
	case http.MethodDelete:
		if err := a.apiKeys.Delete(r.Context(), user.ID, id); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "api_key.delete", "api_key", fmt.Sprintf("%d", id), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleDeveloperAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := a.apiKeys.ListByUser(r.Context(), user.ID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": apiKeyPayloads(keys)})
	case http.MethodPost:
		var req struct {
			Name             string     `json:"name"`
			TotalQuotaPoints *string    `json:"total_quota_points"`
			DailyQuotaPoints *string    `json:"daily_quota_points"`
			RPMLimit         *int       `json:"rpm_limit"`
			ExpiresAt        *time.Time `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		created, err := a.apiKeys.CreateKey(r.Context(), apikeyservice.CreateRequest{UserID: user.ID, Name: req.Name, GroupCode: user.GroupCode, TotalQuotaPoints: req.TotalQuotaPoints, DailyQuotaPoints: req.DailyQuotaPoints, RPMLimit: req.RPMLimit, ExpiresAt: req.ExpiresAt})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, map[string]any{"api_key": apiKeyPayload(created.Key), "secret": created.Secret})
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleDeveloperAPIKeyDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/agent/developer/v1/api-keys/")
	if strings.HasSuffix(path, "/reset-secret") {
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
			return
		}
		id, err := strconv.ParseInt(strings.TrimSuffix(path, "/reset-secret"), 10, 64)
		if err != nil || id <= 0 {
			httpx.WriteError(w, r, errs.BadRequest("invalid api key id"))
			return
		}
		reset, err := a.apiKeys.ResetSecret(r.Context(), user.ID, id)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"api_key": apiKeyPayload(reset.Key), "secret": reset.Secret})
		return
	}
	id, err := strconv.ParseInt(strings.Trim(path, "/"), 10, 64)
	if err != nil || id <= 0 {
		httpx.WriteError(w, r, errs.BadRequest("invalid api key id"))
		return
	}
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var req struct {
			Name             *string    `json:"name"`
			GroupCode        *string    `json:"group_code"`
			Status           *string    `json:"status"`
			TotalQuotaPoints *string    `json:"total_quota_points"`
			DailyQuotaPoints *string    `json:"daily_quota_points"`
			RPMLimit         *int       `json:"rpm_limit"`
			ExpiresAt        *time.Time `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		if req.GroupCode != nil {
			httpx.WriteError(w, r, errs.BadRequest("group_code cannot be changed"))
			return
		}
		key, err := a.apiKeys.Update(r.Context(), user.ID, id, apikeyservice.UpdateRequest{Name: req.Name, TotalQuotaPoints: req.TotalQuotaPoints, DailyQuotaPoints: req.DailyQuotaPoints, RPMLimit: req.RPMLimit, ExpiresAt: req.ExpiresAt})
		if err == nil && req.Status != nil {
			err = a.apiKeys.UpdateStatus(r.Context(), user.ID, id, *req.Status)
			if err == nil {
				key, err = a.apiKeys.GetByID(r.Context(), user.ID, id)
			}
		}
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, apiKeyPayload(key))
	case http.MethodDelete:
		if err := a.apiKeys.Delete(r.Context(), user.ID, id); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleRedeemCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if strings.TrimSpace(r.Header.Get("Idempotency-Key")) == "" {
		httpx.WriteError(w, r, errs.BadRequest("Idempotency-Key is required"))
		return
	}
	if strings.TrimSpace(req.Code) == "" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "redeem code not found"))
		return
	}
	summary, err := a.billing.RedeemCode(r.Context(), billingservice.RedeemCodeRequest{UserID: user.ID, Code: req.Code, IdempotencyKey: r.Header.Get("Idempotency-Key")})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, summary)
}

func (a *API) HandlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	providerCode := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/open/image/v1/payments/webhooks/"), "/")
	if providerCode == "" {
		httpx.WriteError(w, r, errs.BadRequest("provider is required"))
		return
	}
	var req struct {
		OrderNo string `json:"order_no"`
		TradeNo string `json:"trade_no"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	result, err := a.billing.MarkOrderPaid(r.Context(), domainbilling.MarkOrderPaidRequest{
		Provider: providerCode,
		OrderNo:  strings.TrimSpace(req.OrderNo),
		TradeNo:  strings.TrimSpace(req.TradeNo),
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) HandleReferenceAssetUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
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
	assetID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/reference-assets/")
	if strings.HasSuffix(assetID, "/download") {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}
		user, appErr := a.requireUser(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		assetID = strings.TrimSuffix(assetID, "/download")
		asset, content, err := a.assets.Download(user.ID, assetID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.Header().Set("Content-Type", defaultString(asset.MimeType, "application/octet-stream"))
		w.Header().Set("Content-Disposition", `attachment; filename="`+asset.ID+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, appErr := a.requireUser(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		asset, err := a.assets.Get(user.ID, assetID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, asset)
	case http.MethodDelete:
		user, appErr := a.requireUser(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		if err := a.assets.Delete(user.ID, assetID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleImageDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	imageID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/images/")
	result, content, err := a.tasks.DownloadImageResult(r.Context(), user.ID, imageID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	w.Header().Set("Content-Type", defaultString(result.MimeType, "application/octet-stream"))
	w.Header().Set("Content-Disposition", `attachment; filename="`+result.ID+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
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

func (a *API) HandleOpenReferenceAssetMultipartUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
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
	apiKeyID := identity.APIKeyID
	asset, svcErr := a.assets.UploadWithMetadata(identity.UserID, header.Filename, header.Header.Get("Content-Type"), content, domainassets.UploadMetadata{
		APIKeyID:     &apiKeyID,
		UploadSource: "openapi",
	})
	if svcErr != nil {
		httpx.WriteError(w, r, normalizeAppError(svcErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, asset)
}

func (a *API) HandleOpenReferenceAssetGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	assetID := strings.TrimPrefix(r.URL.Path, "/api/open/image/v1/reference-assets/")
	asset, err := a.assets.Get(identity.UserID, assetID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, asset)
}

func (a *API) HandleAgentTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		a.handleAgentTaskCreate(w, r)
	case http.MethodGet:
		a.handleAgentTaskList(w, r)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAgentTaskDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
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

func (a *API) HandleAgentGalleryImageDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	imageID, action, parseErr := parseAgentGalleryImageAction(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	if action != "publish" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery image route not found"))
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "publish_request_enabled", false) {
		httpx.WriteError(w, r, errs.New(http.StatusForbidden, errs.CodeForbidden, "publish request is disabled"))
		return
	}

	ownedImage, err := a.findOwnedGalleryImage(r.Context(), user.ID, imageID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	allowed, reason, err := a.moderatePublishRequest(r.Context(), ownedImage.Prompt)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if !allowed {
		rejected, reviewErr := a.tasks.ReviewImage(r.Context(), imageID, domainimagetask.VisibilityRejected, defaultString(reason, "auto_moderation_blocked"), nil)
		if reviewErr != nil {
			httpx.WriteError(w, r, normalizeAppError(reviewErr))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "gallery.publish_rejected", "image_result", imageID, map[string]any{"reason": rejected.ReviewReason})
		httpx.WriteSuccess(w, r, http.StatusOK, rejected)
		return
	}

	image, err := a.tasks.RequestPublish(r.Context(), user.ID, imageID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "gallery.publish_request", "image_result", imageID, map[string]any{"status": image.VisibilityStatus})
	httpx.WriteSuccess(w, r, http.StatusAccepted, image)
}

func (a *API) HandleAgentHistoryTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.handleAgentTaskList(w, r)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAgentHistoryTaskDetail(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodDelete:
	default:
		writeMethodNotAllowed(w, r)
		return
	}
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
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	var req domainadminauth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	session, err := a.adminAuth.Login(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	a.recordAudit(r, "admin", fmt.Sprintf("%d", session.AdminID), "admin.login", "admin_user", fmt.Sprintf("%d", session.AdminID), nil)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"access_token":       session.AccessToken,
		"expires_in_seconds": int(time.Until(session.AccessTokenExpiresAt).Seconds()),
		"admin_id":           session.AdminID,
		"email":              session.Email,
		"role":               session.Role,
	})
}

func (a *API) HandleOpenGalleryImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", false) {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery is disabled"))
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
	result, err := a.tasks.ListPublicGallery(r.Context(), domainimagetask.GalleryListRequest{Page: page, PageSize: pageSize})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
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

func (a *API) HandleOpenGalleryImageDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", false) {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery is disabled"))
		return
	}
	imageID := strings.TrimPrefix(r.URL.Path, "/api/open/image/v1/gallery/images/")
	image, err := a.tasks.GetPublicImage(r.Context(), imageID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, image)
}

func (a *API) HandleAdminImageReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdmin(r); appErr != nil {
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
	result, err := a.tasks.ListGallery(r.Context(), domainimagetask.GalleryListRequest{
		Page:     page,
		PageSize: pageSize,
		Status:   r.URL.Query().Get("status"),
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
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

func (a *API) HandleAdminImageReviewDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	imageID, action, parseErr := parseAdminImageReviewAction(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
	}
	var (
		image   domainimagetask.GalleryImage
		err     error
		auditOp string
	)
	switch action {
	case "approve":
		now := time.Now().UTC()
		image, err = a.tasks.ReviewImage(r.Context(), imageID, domainimagetask.VisibilityApproved, "", &now)
		auditOp = "image_review.approve"
	case "reject":
		image, err = a.tasks.ReviewImage(r.Context(), imageID, domainimagetask.VisibilityRejected, defaultString(strings.TrimSpace(req.Reason), "rejected by admin"), nil)
		auditOp = "image_review.reject"
	case "unpublish":
		image, err = a.tasks.ReviewImage(r.Context(), imageID, domainimagetask.VisibilityUnpublished, defaultString(strings.TrimSpace(req.Reason), "unpublished by admin"), nil)
		auditOp = "image_review.unpublish"
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "image review route not found"))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), auditOp, "image_result", imageID, map[string]any{"status": image.VisibilityStatus, "reason": image.ReviewReason})
	httpx.WriteSuccess(w, r, http.StatusOK, image)
}

func (a *API) HandleAdminDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdmin(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}

	callRecords, err := a.callRecord.ListCallRecords(r.Context(), domainadmincallrecord.ListRequest{Page: 1, PageSize: 200})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	users, err := a.adminUser.ListUsers(r.Context(), domainadminuser.ListRequest{Page: 1, PageSize: 100})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	providers, err := a.modelAdmin.ListProviders(r.Context(), domainmodeladmin.ProviderListRequest{Page: 1, PageSize: 100})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	reviews, err := a.tasks.ListGallery(r.Context(), domainimagetask.GalleryListRequest{Page: 1, PageSize: 100, Status: domainimagetask.VisibilityPendingReview})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	audits, err := a.audit.List(r.Context(), domainaudit.ListRequest{Page: 1, PageSize: 6})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}

	var (
		successCount      int
		failedCount       int
		totalDurationSec  float64
		durationCount     int
		totalActualPoints float64
	)
	for _, item := range callRecords.Items {
		switch item.Status {
		case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed:
			successCount++
		case domainimagetask.StatusFailed, domainimagetask.StatusRejected:
			failedCount++
		}
		if item.StartedAt != nil && item.FinishedAt != nil && item.FinishedAt.After(*item.StartedAt) {
			totalDurationSec += item.FinishedAt.Sub(*item.StartedAt).Seconds()
			durationCount++
		}
		if value, parseErr := strconv.ParseFloat(strings.TrimSpace(item.ActualPoints), 64); parseErr == nil {
			totalActualPoints += value
		}
	}
	totalRecords := len(callRecords.Items)
	successRate := 0.0
	if totalRecords > 0 {
		successRate = float64(successCount) / float64(totalRecords) * 100
	}
	avgDuration := 0.0
	if durationCount > 0 {
		avgDuration = totalDurationSec / float64(durationCount)
	}
	activeUsers := 0
	closedUsers := 0
	disabledUsers := 0
	for _, user := range users.Items {
		switch user.Status {
		case "active":
			activeUsers++
		case "closed":
			closedUsers++
		case "disabled":
			disabledUsers++
		}
	}
	providerItems := make([]map[string]any, 0, len(providers.Items))
	for _, item := range providers.Items {
		providerItems = append(providerItems, map[string]any{
			"provider_code": item.ProviderCode,
			"provider_type": item.ProviderType,
			"health_status": item.HealthStatus,
			"enabled":       item.Enabled,
		})
	}

	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"metrics": []map[string]any{
			{"key": "success_rate", "label": "成功率", "value": fmt.Sprintf("%.2f%%", successRate), "detail": fmt.Sprintf("最近 %d 条调用", totalRecords)},
			{"key": "avg_duration_sec", "label": "平均耗时", "value": fmt.Sprintf("%.2fs", avgDuration), "detail": "已完成任务平均执行时长"},
			{"key": "actual_points", "label": "积分消耗", "value": fmt.Sprintf("%.5f", totalActualPoints), "detail": "最近调用累计 actual_points"},
			{"key": "active_users", "label": "活跃用户", "value": strconv.Itoa(activeUsers), "detail": fmt.Sprintf("禁用 %d / 注销 %d", disabledUsers, closedUsers)},
		},
		"providers": providerItems,
		"queue": []map[string]any{
			{"item": "公开图审核", "count": strconv.Itoa(reviews.Total), "detail": "待人工审核图片"},
			{"item": "失败任务", "count": strconv.Itoa(failedCount), "detail": "最近调用中失败或被拒绝的任务"},
			{"item": "用户总量", "count": strconv.Itoa(users.Total), "detail": "当前用户账户总数"},
		},
		"audit": audits.Items,
	})
}

func (a *API) HandleAdminConfigTabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireAdmin(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
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
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
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
			UpdatedBy: admin.AdminID,
		})
		if err != nil {
			httpx.WriteError(w, r, err.(*errs.Error))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "config.update", "config_tab", tabKey, map[string]any{"version": req.Version}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, tab)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "admin.logout", "admin_user", fmt.Sprintf("%d", admin.AdminID), nil)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) HandleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireAdmin(r); appErr != nil {
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
	if pageSize > 100 {
		httpx.WriteError(w, r, errs.BadRequest("page_size must be at most 100"))
		return
	}
	createdFrom, queryErr := parseOptionalTime(r.URL.Query().Get("created_from"), "created_from")
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	createdTo, queryErr := parseOptionalTime(r.URL.Query().Get("created_to"), "created_to")
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	logs, err := a.audit.List(r.Context(), domainaudit.ListRequest{
		Page:        page,
		PageSize:    pageSize,
		ActorType:   r.URL.Query().Get("actor_type"),
		ActorID:     r.URL.Query().Get("actor_id"),
		Action:      r.URL.Query().Get("action"),
		TargetType:  r.URL.Query().Get("target_type"),
		TargetID:    r.URL.Query().Get("target_id"),
		Result:      r.URL.Query().Get("result"),
		CreatedFrom: createdFrom,
		CreatedTo:   createdTo,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": logs.Items,
		"pagination": map[string]any{
			"page":      logs.Page,
			"page_size": logs.PageSize,
			"total":     logs.Total,
		},
	})
}

func (a *API) HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireAdmin(r); appErr != nil {
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
	result, err := a.adminUser.ListUsers(r.Context(), domainadminuser.ListRequest{
		Page:     page,
		PageSize: pageSize,
		Query:    r.URL.Query().Get("query"),
		Status:   r.URL.Query().Get("status"),
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
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

func (a *API) HandleAdminUserDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	userID, action, parseErr := parseAdminUserAction(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch action {
	case "":
		if r.Method != http.MethodGet {
			httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
			return
		}
		detail, err := a.adminUser.GetUserDetail(r.Context(), userID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, detail)
	case "status":
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.adminUser.UpdateUserStatus(r.Context(), domainadminuser.StatusRequest{UserID: userID, Status: req.Status, OperatorAdmin: admin.AdminID})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if updated.Status == "disabled" || updated.Status == "closed" {
			_ = a.auth.RevokeUserSessions(userID)
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.status_update", "user", fmt.Sprintf("%d", userID), map[string]any{"status": updated.Status, "token_version": updated.TokenVersion}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case "points-adjustments":
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
			return
		}
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			httpx.WriteError(w, r, errs.BadRequest("Idempotency-Key is required"))
			return
		}
		var req struct {
			ChangePoints string `json:"change_points"`
			Reason       string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		balance, err := a.adminUser.AdjustPoints(r.Context(), domainadminuser.PointAdjustmentRequest{
			UserID:         userID,
			ChangePoints:   req.ChangePoints,
			Reason:         req.Reason,
			IdempotencyKey: idempotencyKey,
			OperatorAdmin:  admin.AdminID,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.points_adjust", "user", fmt.Sprintf("%d", userID), map[string]any{"change_points": req.ChangePoints, "reason": req.Reason, "idempotency_key": idempotencyKey}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, balance)
	case "reset-password":
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, methodNotAllowedError())
			return
		}
		var req struct {
			NewPassword string `json:"new_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		if _, err := a.auth.SetPassword(userID, req.NewPassword); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.password_reset", "user", fmt.Sprintf("%d", userID), nil); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "password_reset"})
	case "limits":
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, methodNotAllowedError())
			return
		}
		var req struct {
			RPMLimit         int `json:"rpm_limit"`
			ConcurrencyLimit int `json:"concurrency_limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.adminUser.UpdateUserLimits(r.Context(), domainadminuser.LimitsRequest{
			UserID:           userID,
			RPMLimit:         req.RPMLimit,
			ConcurrencyLimit: req.ConcurrencyLimit,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.limits_update", "user", fmt.Sprintf("%d", userID), map[string]any{"rpm_limit": req.RPMLimit, "concurrency_limit": req.ConcurrencyLimit}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case "group":
		if r.Method != http.MethodPost {
			httpx.WriteError(w, r, methodNotAllowedError())
			return
		}
		var req struct {
			UserGroupCode string `json:"user_group_code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.adminUser.AssignUserGroup(r.Context(), domainadminuser.GroupAssignmentRequest{
			UserID:        userID,
			UserGroupCode: req.UserGroupCode,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.group_update", "user", fmt.Sprintf("%d", userID), map[string]any{"user_group_code": updated.UserGroupCode}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "admin user route not found"))
	}
}

func (a *API) HandleAdminUserGroups(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireAdmin(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
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
		result, err := a.adminUser.ListUserGroups(r.Context(), domainadminuser.UserGroupListRequest{
			Page:     page,
			PageSize: pageSize,
			Query:    r.URL.Query().Get("query"),
			Status:   r.URL.Query().Get("status"),
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
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
	case http.MethodPost:
		group, ok := a.decodeUserGroupWriteRequest(w, r)
		if !ok {
			return
		}
		created, err := a.adminUser.CreateUserGroup(r.Context(), group)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		httpx.WriteError(w, r, methodNotAllowedError())
	}
}

func (a *API) HandleAdminUserGroupDetail(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireAdmin(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	groupCode, parseErr := parseUserGroupCode(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		group, err := a.adminUser.GetUserGroup(r.Context(), groupCode)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, group)
	case http.MethodPut:
		req, ok := a.decodeUserGroupWriteRequest(w, r)
		if !ok {
			return
		}
		updated, err := a.adminUser.UpdateUserGroup(r.Context(), groupCode, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.adminUser.DeleteUserGroup(r.Context(), groupCode); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteError(w, r, methodNotAllowedError())
	}
}

func (a *API) HandleAdminRedeemCodes(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
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
		batchID, queryErr := parseNonNegativeInt64Query(r, "batch_id", 0)
		if queryErr != nil {
			httpx.WriteError(w, r, queryErr)
			return
		}
		result, err := a.redeem.ListCodes(r.Context(), domainredeem.ListRequest{
			Page:     page,
			PageSize: pageSize,
			Status:   r.URL.Query().Get("status"),
			Code:     r.URL.Query().Get("code"),
			BatchID:  batchID,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedRedeemCodesPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := a.decodeAdminRedeemCreateRequest(w, r)
		if !ok {
			return
		}
		req.OperatorAdmin = admin.AdminID
		created, err := a.redeem.CreateCode(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "redeem_code.create", "redeem_code", fmt.Sprintf("%d", created.ID), map[string]any{"code": created.Code, "batch_id": created.BatchID}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminRedeemCodeBatchCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		Count          int    `json:"count"`
		BatchID        int64  `json:"batch_id"`
		Status         string `json:"status"`
		RewardType     string `json:"reward_type"`
		RewardValue    string `json:"reward_value"`
		ValidFrom      string `json:"valid_from"`
		ValidUntil     string `json:"valid_until"`
		MaxRedemptions int    `json:"max_redemptions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	validFrom, parseErr := parseOptionalTime(req.ValidFrom, "valid_from")
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	validUntil, parseErr := parseRequiredTime(req.ValidUntil, "valid_until")
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	result, err := a.redeem.BatchCreate(r.Context(), domainredeem.BatchCreateRequest{
		Count:          req.Count,
		BatchID:        req.BatchID,
		Status:         req.Status,
		RewardType:     req.RewardType,
		RewardValue:    req.RewardValue,
		ValidFrom:      validFrom,
		ValidUntil:     validUntil,
		MaxRedemptions: req.MaxRedemptions,
		OperatorAdmin:  admin.AdminID,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "redeem_code.batch_create", "redeem_code_batch", fmt.Sprintf("%d", result.BatchID), map[string]any{"count": result.Count, "batch_id": result.BatchID}); auditErr != nil {
		httpx.WriteError(w, r, normalizeAppError(auditErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, result)
}

func (a *API) HandleAdminRedeemCodeDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	codeID, action, parseErr := parseAdminRedeemCodeAction(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch action {
	case "status":
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w, r)
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.redeem.UpdateStatus(r.Context(), domainredeem.StatusRequest{ID: codeID, Status: req.Status, OperatorAdmin: admin.AdminID})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "redeem_code.status_update", "redeem_code", fmt.Sprintf("%d", codeID), map[string]any{"status": updated.Status}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case "redemptions":
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r)
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
		result, err := a.redeem.ListRedemptions(r.Context(), codeID, page, pageSize)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
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
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "admin redeem code route not found"))
	}
}

func (a *API) HandleAdminCallRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireAdmin(r); appErr != nil {
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
	if pageSize > 100 {
		httpx.WriteError(w, r, errs.BadRequest("page_size must be at most 100"))
		return
	}
	userID, queryErr := parseNonNegativeInt64Query(r, "user_id", 0)
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	createdFrom, queryErr := parseOptionalTime(r.URL.Query().Get("created_from"), "created_from")
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	createdTo, queryErr := parseOptionalTime(r.URL.Query().Get("created_to"), "created_to")
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	result, err := a.callRecord.ListCallRecords(r.Context(), domainadmincallrecord.ListRequest{
		Page:          page,
		PageSize:      pageSize,
		Status:        r.URL.Query().Get("status"),
		Provider:      r.URL.Query().Get("provider"),
		SourceChannel: r.URL.Query().Get("source_channel"),
		UserID:        userID,
		TaskID:        r.URL.Query().Get("task_id"),
		CreatedFrom:   createdFrom,
		CreatedTo:     createdTo,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
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

func (a *API) HandleAdminModelProviders(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
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
		enabled, parseErr := parseOptionalBoolQuery(r, "enabled")
		if parseErr != nil {
			httpx.WriteError(w, r, parseErr)
			return
		}
		result, err := a.modelAdmin.ListProviders(r.Context(), domainmodeladmin.ProviderListRequest{
			Page:         page,
			PageSize:     pageSize,
			ProviderType: r.URL.Query().Get("provider_type"),
			Enabled:      enabled,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedModelProvidersPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := decodeAdminModelProviderRequest(w, r)
		if !ok {
			return
		}
		created, err := a.modelAdmin.CreateProvider(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_provider.create", "model_provider", created.ProviderCode, map[string]any{"provider_type": created.ProviderType, "enabled": created.Enabled}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminModelProviderDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	providerCode := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/ops/admin/v1/model-providers/"), "/")
	if providerCode == "" || strings.Contains(providerCode, "/") {
		httpx.WriteError(w, r, errs.BadRequest("invalid provider_code"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := a.modelAdmin.GetProvider(r.Context(), providerCode)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, item)
	case http.MethodPut:
		req, ok := decodeAdminModelProviderRequest(w, r)
		if !ok {
			return
		}
		updated, err := a.modelAdmin.UpdateProvider(r.Context(), providerCode, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_provider.update", "model_provider", updated.ProviderCode, map[string]any{"provider_type": updated.ProviderType, "enabled": updated.Enabled}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.modelAdmin.DeleteProvider(r.Context(), providerCode); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_provider.delete", "model_provider", providerCode, nil); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminProviderModels(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
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
		enabled, parseErr := parseOptionalBoolQuery(r, "enabled")
		if parseErr != nil {
			httpx.WriteError(w, r, parseErr)
			return
		}
		result, err := a.modelAdmin.ListProviderModels(r.Context(), domainmodeladmin.ProviderModelListRequest{
			Page:         page,
			PageSize:     pageSize,
			ProviderCode: r.URL.Query().Get("provider_code"),
			ModelCode:    r.URL.Query().Get("model_code"),
			Enabled:      enabled,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedProviderModelsPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := decodeAdminProviderModelRequest(w, r)
		if !ok {
			return
		}
		created, err := a.modelAdmin.CreateProviderModel(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "provider_model.create", "provider_model", fmt.Sprintf("%d", created.ID), map[string]any{"provider_code": created.ProviderCode, "model_code": created.ModelCode}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminProviderModelDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	providerModelID, parseErr := parseAdminProviderModelID(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := a.modelAdmin.GetProviderModel(r.Context(), providerModelID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, item)
	case http.MethodPut:
		req, ok := decodeAdminProviderModelRequest(w, r)
		if !ok {
			return
		}
		updated, err := a.modelAdmin.UpdateProviderModel(r.Context(), providerModelID, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "provider_model.update", "provider_model", fmt.Sprintf("%d", updated.ID), map[string]any{"provider_code": updated.ProviderCode, "model_code": updated.ModelCode}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.modelAdmin.DeleteProviderModel(r.Context(), providerModelID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "provider_model.delete", "provider_model", fmt.Sprintf("%d", providerModelID), nil); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminModelRoutes(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
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
		enabled, parseErr := parseOptionalBoolQuery(r, "enabled")
		if parseErr != nil {
			httpx.WriteError(w, r, parseErr)
			return
		}
		result, err := a.modelAdmin.ListRoutes(r.Context(), domainmodeladmin.RouteListRequest{
			Page:         page,
			PageSize:     pageSize,
			GroupCode:    r.URL.Query().Get("group_code"),
			TaskType:     r.URL.Query().Get("task_type"),
			ProviderCode: r.URL.Query().Get("provider_code"),
			Enabled:      enabled,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedModelRoutesPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := decodeAdminModelRouteRequest(w, r)
		if !ok {
			return
		}
		created, err := a.modelAdmin.CreateRoute(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_route.create", "model_route", fmt.Sprintf("%d", created.ID), map[string]any{"provider_code": created.ProviderCode, "group_code": created.GroupCode, "task_type": created.TaskType}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminModelRouteDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdmin(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	routeID, parseErr := parseAdminModelRouteID(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		item, err := a.modelAdmin.GetRoute(r.Context(), routeID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, item)
	case http.MethodPut:
		req, ok := decodeAdminModelRouteRequest(w, r)
		if !ok {
			return
		}
		updated, err := a.modelAdmin.UpdateRoute(r.Context(), routeID, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_route.update", "model_route", fmt.Sprintf("%d", updated.ID), map[string]any{"provider_code": updated.ProviderCode, "group_code": updated.GroupCode, "task_type": updated.TaskType}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.modelAdmin.DeleteRoute(r.Context(), routeID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_route.delete", "model_route", fmt.Sprintf("%d", routeID), nil); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleDocsOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	content, err := os.ReadFile(openAPIDocumentPath())
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "openapi document not found"))
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func (a *API) HandleDocsOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	content, err := os.ReadFile(openAPIDocumentPath())
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "openapi document not found"))
		return
	}
	var document any
	if err := yaml.Unmarshal(content, &document); err != nil {
		httpx.WriteError(w, r, errs.Internal("failed to parse openapi document"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, normalizeYAMLForJSON(document))
}

func (a *API) HandleDocsExamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"openapi": "/docs/openapi.yaml",
		"examples": []map[string]any{
			{"name": "Open API estimate", "method": "GET", "path": "/api/open/image/v1/estimate"},
			{"name": "OpenAI compatible generation", "method": "POST", "path": "/v1/images/generations"},
		},
	})
}

func (a *API) HandleDocsErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"codes": []string{
			errs.CodeBadRequest,
			errs.CodeUnauthorized,
			errs.CodeForbidden,
			errs.CodeNotFound,
			errs.CodeRateLimited,
			errs.CodeInsufficientPoints,
		},
	})
}

func (a *API) HandleOpenAIImageGeneration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeCompatError(w, methodNotAllowedError())
		return
	}
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

	taskID := idempotentTaskID(identity.UserID, r.Header.Get("Idempotency-Key"), req)
	if taskID == "" {
		taskID = uuid.NewString()
	}
	estimate, err := a.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  string(provider.TaskTypeTextToImage),
		AbstractModel:             a.compatAbstractModel(req.Model),
		RequestedQuality:          compatQuality(req.Quality),
		RequestedSize:             req.Size,
		RequestedOutputImageCount: req.N,
		UserGroupCode:             identity.GroupCode,
		UserGroupMultiplier:       a.userGroupMultiplier(identity.GroupCode),
	})
	if err != nil {
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	if err := a.apiKeys.ReserveQuota(r.Context(), identity, taskID, estimate.EstimatedPoints); err != nil {
		a.writeCompatError(w, normalizeAppError(err))
		return
	}
	resp, err := a.compat.Generate(r.Context(), compatservice.GenerateRequest{
		TaskID:              taskID,
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
		a.apiKeys.ReleaseQuota(r.Context(), identity, taskID)
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *API) HandleOpenAIImageEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeCompatError(w, methodNotAllowedError())
		return
	}
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
	quotaPayload := map[string]any{
		"model":           r.FormValue("model"),
		"prompt":          r.FormValue("prompt"),
		"size":            r.FormValue("size"),
		"n":               count,
		"quality":         r.FormValue("quality"),
		"response_format": r.FormValue("response_format"),
		"user":            r.FormValue("user"),
		"image_count":     len(images),
		"mask_present":    mask != nil,
	}
	taskID := idempotentTaskID(identity.UserID, r.Header.Get("Idempotency-Key"), quotaPayload)
	if taskID == "" {
		taskID = uuid.NewString()
	}
	estimate, err := a.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  string(provider.TaskTypeImageEdit),
		AbstractModel:             a.compatAbstractModel(r.FormValue("model")),
		RequestedQuality:          compatQuality(r.FormValue("quality")),
		RequestedSize:             r.FormValue("size"),
		RequestedOutputImageCount: count,
		ReferenceImageCount:       len(images),
		UserGroupCode:             identity.GroupCode,
		UserGroupMultiplier:       a.userGroupMultiplier(identity.GroupCode),
	})
	if err != nil {
		a.writeCompatError(w, compatservice.MapError(err))
		return
	}
	if err := a.apiKeys.ReserveQuota(r.Context(), identity, taskID, estimate.EstimatedPoints); err != nil {
		a.writeCompatError(w, normalizeAppError(err))
		return
	}
	resp, compatErr := a.compat.Edit(r.Context(), compatservice.EditRequest{
		TaskID:              taskID,
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
		a.apiKeys.ReleaseQuota(r.Context(), identity, taskID)
		a.writeCompatError(w, compatservice.MapError(compatErr))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *API) HandleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeCompatError(w, methodNotAllowedError())
		return
	}
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

func (a *API) HandleOpenTaskDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	taskID := strings.TrimPrefix(r.URL.Path, "/api/open/image/v1/tasks/")
	task, err := a.tasks.GetByID(r.Context(), identity.UserID, taskID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, task)
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
		AbstractModel             string   `json:"abstract_model"`
		RequestedQuality          string   `json:"requested_quality"`
		RequestedSize             string   `json:"requested_size"`
		RequestedOutputImageCount int      `json:"requested_output_image_count"`
		ReferenceAssetIDs         []string `json:"reference_asset_ids"`
		ResponseMode              string   `json:"response_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if appErr := validateNativeTaskResponseMode(req.ResponseMode); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}

	result, err := a.tasks.CreateTask(r.Context(), domainimagetask.CreateRequest{
		TaskID:              idempotentTaskID(user.ID, r.Header.Get("Idempotency-Key"), req),
		UserID:              user.ID,
		AbstractModel:       req.AbstractModel,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    req.RequestedQuality,
		OutputImageCount:    req.RequestedOutputImageCount,
		ReferenceImageCount: len(req.ReferenceAssetIDs),
		ReferenceAssetIDs:   append([]string(nil), req.ReferenceAssetIDs...),
		UserGroupCode:       user.GroupCode,
		UserGroupMultiplier: user.GroupMultiplier,
		ResponseMode:        req.ResponseMode,
		SavePolicy:          "private",
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
		AbstractModel             string   `json:"abstract_model"`
		RequestedQuality          string   `json:"requested_quality"`
		RequestedSize             string   `json:"requested_size"`
		RequestedOutputImageCount int      `json:"requested_output_image_count"`
		ReferenceAssetIDs         []string `json:"reference_asset_ids"`
		ResponseMode              string   `json:"response_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if appErr := validateNativeTaskResponseMode(req.ResponseMode); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}

	taskID := idempotentTaskID(identity.UserID, r.Header.Get("Idempotency-Key"), req)
	if taskID == "" {
		taskID = uuid.NewString()
	}
	estimate, err := a.billing.Estimate(domainbilling.EstimateRequest{
		TaskType:                  req.TaskType,
		AbstractModel:             req.AbstractModel,
		RequestedQuality:          req.RequestedQuality,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: req.RequestedOutputImageCount,
		ReferenceImageCount:       len(req.ReferenceAssetIDs),
		UserGroupCode:             identity.GroupCode,
		UserGroupMultiplier:       a.userGroupMultiplier(identity.GroupCode),
	})
	if err != nil {
		httpx.WriteError(w, r, compatservice.MapError(err))
		return
	}
	if err := a.apiKeys.ReserveQuota(r.Context(), identity, taskID, estimate.EstimatedPoints); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	result, err := a.tasks.CreateTask(r.Context(), domainimagetask.CreateRequest{
		TaskID:              taskID,
		UserID:              identity.UserID,
		APIKeyID:            identity.APIKeyID,
		SourceChannel:       "openapi",
		AbstractModel:       req.AbstractModel,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    req.RequestedQuality,
		OutputImageCount:    req.RequestedOutputImageCount,
		ReferenceImageCount: len(req.ReferenceAssetIDs),
		ReferenceAssetIDs:   append([]string(nil), req.ReferenceAssetIDs...),
		UserGroupCode:       identity.GroupCode,
		UserGroupMultiplier: a.userGroupMultiplier(identity.GroupCode),
		ResponseMode:        req.ResponseMode,
		SavePolicy:          "private",
	})
	if err != nil {
		a.apiKeys.ReleaseQuota(r.Context(), identity, taskID)
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

func validateNativeTaskResponseMode(value string) *errs.Error {
	mode := strings.ToLower(strings.TrimSpace(value))
	if mode == "" || mode == "async" {
		return nil
	}
	return errs.BadRequest("native task creation only supports async response_mode")
}

func (a *API) requireUser(r *http.Request) (*domainauth.User, *errs.Error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token")
	}
	claims, err := a.auth.ParseAccessToken(strings.TrimPrefix(authHeader, "Bearer "))
	if err != nil {
		return nil, err.(*errs.Error)
	}
	user, ok := a.auth.GetUserByID(claims.UserID)
	if !ok {
		return nil, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "user not found")
	}
	if user.Status == "disabled" {
		return nil, errs.New(http.StatusForbidden, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return nil, errs.New(http.StatusForbidden, errs.CodeForbidden, "user account has been closed")
	}
	if user.TokenVersion != claims.TokenVersion {
		return nil, errs.New(http.StatusUnauthorized, errs.CodeAuthAccessExpired, "access token has been revoked")
	}
	return &user, nil
}

func parseHMACTimestamp(value string) (time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if unix, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return time.Unix(unix, 0).UTC(), nil
	}
	return time.Parse(time.RFC3339, trimmed)
}

func profilePayload(user *domainauth.User) map[string]any {
	return map[string]any{
		"id":                user.ID,
		"email":             user.Email,
		"nickname":          user.Nickname,
		"bio":               user.Bio,
		"avatar_object_key": user.AvatarObjectKey,
		"user_group_code":   user.GroupCode,
		"theme":             defaultString(user.Theme, "system"),
		"default_locale":    defaultString(user.DefaultLocale, "zh-CN"),
	}
}

func (a *API) requireOpenAPIKey(r *http.Request) (domainapikey.Identity, *errs.Error) {
	if strings.TrimSpace(r.Header.Get("X-Access-Key")) == "" ||
		strings.TrimSpace(r.Header.Get("X-Signature")) == "" ||
		strings.TrimSpace(r.Header.Get("X-Timestamp")) == "" ||
		strings.TrimSpace(r.Header.Get("X-Body-SHA256")) == "" {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing api key credentials")
	}
	timestamp, parseErr := parseHMACTimestamp(r.Header.Get("X-Timestamp"))
	if parseErr != nil {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key timestamp")
	}
	body, err := readBoundedBody(r.Body, a.cfg.GenerationLimits.ReferenceImageMaxMB)
	if err != nil {
		return domainapikey.Identity{}, err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	identity, verifyErr := a.apiKeys.VerifyCanonicalHMAC(r.Context(), apikeyservice.HMACRequest{
		AccessKey:  r.Header.Get("X-Access-Key"),
		Method:     r.Method,
		Path:       r.URL.RequestURI(),
		Timestamp:  timestamp,
		Body:       body,
		BodySHA256: r.Header.Get("X-Body-SHA256"),
		Signature:  r.Header.Get("X-Signature"),
	})
	if verifyErr != nil {
		return domainapikey.Identity{}, normalizeAppError(verifyErr)
	}
	if appErr := a.requireAPIKeyUserActive(identity); appErr != nil {
		return domainapikey.Identity{}, appErr
	}
	return identity, nil
}

func readBoundedBody(body io.Reader, referenceImageMaxMB int) ([]byte, *errs.Error) {
	limit := int64(referenceImageMaxMB)
	if limit <= 0 {
		limit = 16
	}
	limit = (limit + 1) * 1024 * 1024
	limited := io.LimitReader(body, limit+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, errs.BadRequest("failed to read request body")
	}
	if int64(len(content)) > limit {
		return nil, errs.New(http.StatusRequestEntityTooLarge, errs.CodeValidationFailed, "request body too large")
	}
	return content, nil
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
	if appErr := a.requireAPIKeyUserActive(identity); appErr != nil {
		return domainapikey.Identity{}, appErr
	}
	return identity, nil
}

func (a *API) requireAPIKeyUserActive(identity domainapikey.Identity) *errs.Error {
	if a.auth == nil {
		return nil
	}
	user, ok := a.auth.GetUserByID(identity.UserID)
	if !ok {
		return errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "api key user not found")
	}
	if user.Status == "disabled" {
		return errs.New(http.StatusForbidden, errs.CodeUserDisabled, "user has been disabled")
	}
	if user.Status == "closed" {
		return errs.New(http.StatusForbidden, errs.CodeForbidden, "user account has been closed")
	}
	return nil
}

func (a *API) requireAdmin(r *http.Request) (*adminauthservice.Claims, *errs.Error) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing admin bearer token")
	}
	claims, err := a.adminAuth.ParseAccessToken(r.Context(), strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")))
	if err != nil {
		return nil, normalizeAppError(err)
	}
	return claims, nil
}

func (a *API) recordAudit(r *http.Request, actorType, actorID, action, targetType, targetID string, metadata map[string]any) error {
	if a.audit == nil {
		return nil
	}
	_, err := a.audit.Record(r.Context(), domainaudit.RecordRequest{
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Result:     "success",
		Metadata:   metadata,
		IPAddr:     r.RemoteAddr,
		UserAgent:  r.UserAgent(),
	})
	return err
}

func parseAdminUserAction(path string) (int64, string, *errs.Error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/ops/admin/v1/users/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", errs.BadRequest("invalid user_id")
	}
	userID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || userID <= 0 {
		return 0, "", errs.BadRequest("invalid user_id")
	}
	if len(parts) == 1 {
		return userID, "", nil
	}
	if len(parts) == 2 {
		return userID, parts[1], nil
	}
	return 0, "", errs.New(http.StatusNotFound, errs.CodeNotFound, "admin user route not found")
}

func parseUserGroupCode(path string) (string, *errs.Error) {
	groupCode := strings.ToLower(strings.Trim(strings.TrimPrefix(path, "/api/ops/admin/v1/user-groups/"), "/"))
	if groupCode == "" {
		return "", errs.BadRequest("group_code is required")
	}
	return groupCode, nil
}

func (a *API) decodeUserGroupWriteRequest(w http.ResponseWriter, r *http.Request) (domainadminuser.UserGroupWriteRequest, bool) {
	var req struct {
		GroupCode   string  `json:"group_code"`
		GroupName   string  `json:"group_name"`
		Multiplier  string  `json:"multiplier"`
		Status      string  `json:"status"`
		Description *string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainadminuser.UserGroupWriteRequest{}, false
	}
	return domainadminuser.UserGroupWriteRequest{
		GroupCode:   req.GroupCode,
		GroupName:   req.GroupName,
		Multiplier:  req.Multiplier,
		Status:      req.Status,
		Description: req.Description,
	}, true
}

func (a *API) decodeAdminRedeemCreateRequest(w http.ResponseWriter, r *http.Request) (domainredeem.CreateRequest, bool) {
	var req struct {
		Code           string `json:"code"`
		BatchID        int64  `json:"batch_id"`
		Status         string `json:"status"`
		RewardType     string `json:"reward_type"`
		RewardValue    string `json:"reward_value"`
		ValidFrom      string `json:"valid_from"`
		ValidUntil     string `json:"valid_until"`
		MaxRedemptions int    `json:"max_redemptions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainredeem.CreateRequest{}, false
	}
	validFrom, parseErr := parseOptionalTime(req.ValidFrom, "valid_from")
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return domainredeem.CreateRequest{}, false
	}
	validUntil, parseErr := parseRequiredTime(req.ValidUntil, "valid_until")
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return domainredeem.CreateRequest{}, false
	}
	return domainredeem.CreateRequest{
		Code:           req.Code,
		BatchID:        req.BatchID,
		Status:         req.Status,
		RewardType:     req.RewardType,
		RewardValue:    req.RewardValue,
		ValidFrom:      validFrom,
		ValidUntil:     validUntil,
		MaxRedemptions: req.MaxRedemptions,
	}, true
}

func parseBillingOrderPath(path string) (int64, string, *errs.Error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/agent/billing/v1/orders/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", errs.BadRequest("invalid order_id")
	}
	orderID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || orderID <= 0 {
		return 0, "", errs.BadRequest("invalid order_id")
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	return orderID, action, nil
}

func parseAdminRedeemCodeAction(path string) (int64, string, *errs.Error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/ops/admin/v1/redeem-codes/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", errs.BadRequest("invalid code_id")
	}
	codeID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || codeID <= 0 {
		return 0, "", errs.BadRequest("invalid code_id")
	}
	if len(parts) == 2 {
		return codeID, parts[1], nil
	}
	return 0, "", errs.New(http.StatusNotFound, errs.CodeNotFound, "admin redeem code route not found")
}

func parseAgentGalleryImageAction(path string) (string, string, *errs.Error) {
	trimmed := strings.TrimPrefix(path, "/api/agent/gallery/v1/images/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery image route not found")
}

func parseAdminImageReviewAction(path string) (string, string, *errs.Error) {
	trimmed := strings.TrimPrefix(path, "/api/ops/admin/v1/image-reviews/")
	parts := strings.Split(strings.Trim(trimmed, "/"), ":")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", errs.New(http.StatusNotFound, errs.CodeNotFound, "image review route not found")
}

func (a *API) adminConfigBool(ctx context.Context, tabKey, configKey string, fallback bool) bool {
	tab, err := a.admin.GetTab(ctx, tabKey)
	if err != nil {
		return fallback
	}
	for _, item := range tab.Items {
		if item.ConfigKey != configKey {
			continue
		}
		if value, ok := item.ConfigValue["value"].(bool); ok {
			return value
		}
		if value, ok := item.ConfigValue["value"].(string); ok {
			parsed, parseErr := strconv.ParseBool(strings.TrimSpace(value))
			if parseErr == nil {
				return parsed
			}
		}
	}
	return fallback
}

func (a *API) moderatePublishRequest(ctx context.Context, prompt string) (bool, string, error) {
	if !a.cfg.Providers.OpenAI.Enabled || strings.TrimSpace(a.cfg.Providers.OpenAI.BaseURL) == "" || strings.TrimSpace(a.cfg.Providers.OpenAI.APIKey) == "" {
		return false, "", errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "moderation provider is unavailable")
	}
	body, err := json.Marshal(map[string]any{
		"model": "omni-moderation-latest",
		"input": prompt,
	})
	if err != nil {
		return false, "", errs.Internal("failed to build moderation request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.cfg.Providers.OpenAI.BaseURL, "/")+"/moderations", bytes.NewReader(body))
	if err != nil {
		return false, "", errs.Internal("failed to build moderation request")
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.Providers.OpenAI.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, "", errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "moderation provider request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return false, "", errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "moderation provider rejected request")
	}
	var payload struct {
		Results []struct {
			Flagged    bool            `json:"flagged"`
			Categories map[string]bool `json:"categories"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, "", errs.New(http.StatusServiceUnavailable, errs.CodeUpstreamUnavailable, "moderation provider returned invalid response")
	}
	if len(payload.Results) == 0 {
		return true, "", nil
	}
	if !payload.Results[0].Flagged {
		return true, "", nil
	}
	reasons := make([]string, 0, len(payload.Results[0].Categories))
	for key, flagged := range payload.Results[0].Categories {
		if flagged {
			reasons = append(reasons, key)
		}
	}
	sort.Strings(reasons)
	if len(reasons) == 0 {
		return false, "auto_moderation_blocked", nil
	}
	return false, "auto_moderation_blocked:" + strings.Join(reasons, ","), nil
}

func (a *API) findOwnedGalleryImage(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	tasks, err := a.tasks.ListByUser(ctx, userID)
	if err != nil {
		return domainimagetask.GalleryImage{}, err
	}
	for _, task := range tasks {
		for _, result := range task.Results {
			if result.ID != imageID {
				continue
			}
			return domainimagetask.GalleryImage{
				ID:               result.ID,
				TaskID:           task.ID,
				UserID:           task.UserID,
				Prompt:           task.Prompt,
				AbstractModel:    task.AbstractModel,
				TaskType:         task.TaskType,
				URL:              result.URL,
				DownloadURL:      result.DownloadURL,
				MimeType:         result.MimeType,
				FileSizeBytes:    result.FileSizeBytes,
				Width:            result.Width,
				Height:           result.Height,
				SHA256:           result.SHA256,
				ObjectKey:        result.ObjectKey,
				StorageDriver:    result.StorageDriver,
				VisibilityStatus: defaultString(result.VisibilityStatus, domainimagetask.VisibilityPrivate),
				ReviewReason:     result.ReviewReason,
				PublishedAt:      result.PublishedAt,
			}, nil
		}
	}
	return domainimagetask.GalleryImage{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "image not found")
}

func parseOptionalTime(raw, field string) (time.Time, *errs.Error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errs.BadRequest("invalid " + field)
	}
	return parsed, nil
}

func parseRequiredTime(raw, field string) (time.Time, *errs.Error) {
	parsed, err := parseOptionalTime(raw, field)
	if err != nil {
		return time.Time{}, err
	}
	if parsed.IsZero() {
		return time.Time{}, errs.BadRequest(field + " is required")
	}
	return parsed, nil
}

func parseNonNegativeInt64Query(r *http.Request, key string, fallback int64) (int64, *errs.Error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, errs.BadRequest("invalid " + key)
	}
	return value, nil
}

func parseOptionalBoolQuery(r *http.Request, key string) (*bool, *errs.Error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, errs.BadRequest("invalid " + key)
	}
	return &value, nil
}

func decodeAdminModelProviderRequest(w http.ResponseWriter, r *http.Request) (domainmodeladmin.ProviderWriteRequest, bool) {
	var req struct {
		ProviderCode        string `json:"provider_code"`
		ProviderType        string `json:"provider_type"`
		AuthConfigEncrypted string `json:"auth_config_encrypted"`
		HealthStatus        string `json:"health_status"`
		Enabled             bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.ProviderWriteRequest{}, false
	}
	return domainmodeladmin.ProviderWriteRequest{
		ProviderCode:        req.ProviderCode,
		ProviderType:        req.ProviderType,
		AuthConfigEncrypted: req.AuthConfigEncrypted,
		HealthStatus:        req.HealthStatus,
		Enabled:             req.Enabled,
	}, true
}

func decodeAdminProviderModelRequest(w http.ResponseWriter, r *http.Request) (domainmodeladmin.ProviderModelWriteRequest, bool) {
	var req struct {
		ProviderCode           string   `json:"provider_code"`
		ModelCode              string   `json:"model_code"`
		CompatMode             string   `json:"compat_mode"`
		SupportsImageInput     bool     `json:"supports_image_input"`
		SupportsMask           bool     `json:"supports_mask"`
		SupportedQualities     []string `json:"supported_qualities"`
		SupportedRatios        []string `json:"supported_ratios"`
		MaxImageCount          int      `json:"max_image_count"`
		MaxReferenceImageCount int      `json:"max_reference_image_count"`
		TimeoutMS              int      `json:"timeout_ms"`
		InputCost              string   `json:"input_cost"`
		OutputCost             string   `json:"output_cost"`
		Currency               string   `json:"currency"`
		HealthStatus           string   `json:"health_status"`
		LastHealthCheckedAt    string   `json:"last_health_checked_at"`
		Enabled                bool     `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.ProviderModelWriteRequest{}, false
	}
	lastHealthCheckedAt, parseErr := parseOptionalTime(req.LastHealthCheckedAt, "last_health_checked_at")
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return domainmodeladmin.ProviderModelWriteRequest{}, false
	}
	var checkedAt *time.Time
	if !lastHealthCheckedAt.IsZero() {
		checkedAt = &lastHealthCheckedAt
	}
	return domainmodeladmin.ProviderModelWriteRequest{
		ProviderCode:           req.ProviderCode,
		ModelCode:              req.ModelCode,
		CompatMode:             req.CompatMode,
		SupportsImageInput:     req.SupportsImageInput,
		SupportsMask:           req.SupportsMask,
		SupportedQualities:     append([]string(nil), req.SupportedQualities...),
		SupportedRatios:        append([]string(nil), req.SupportedRatios...),
		MaxImageCount:          req.MaxImageCount,
		MaxReferenceImageCount: req.MaxReferenceImageCount,
		TimeoutMS:              req.TimeoutMS,
		InputCost:              req.InputCost,
		OutputCost:             req.OutputCost,
		Currency:               req.Currency,
		HealthStatus:           req.HealthStatus,
		LastHealthCheckedAt:    checkedAt,
		Enabled:                req.Enabled,
	}, true
}

func decodeAdminModelRouteRequest(w http.ResponseWriter, r *http.Request) (domainmodeladmin.RouteWriteRequest, bool) {
	var req struct {
		GroupCode       string `json:"group_code"`
		TaskType        string `json:"task_type"`
		ProviderModelID int64  `json:"provider_model_id"`
		ProviderCode    string `json:"provider_code"`
		Priority        int    `json:"priority"`
		WeightPercent   int    `json:"weight_percent"`
		FallbackOrder   int    `json:"fallback_order"`
		Enabled         bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.RouteWriteRequest{}, false
	}
	return domainmodeladmin.RouteWriteRequest{
		GroupCode:       req.GroupCode,
		TaskType:        req.TaskType,
		ProviderModelID: req.ProviderModelID,
		ProviderCode:    req.ProviderCode,
		Priority:        req.Priority,
		WeightPercent:   req.WeightPercent,
		FallbackOrder:   req.FallbackOrder,
		Enabled:         req.Enabled,
	}, true
}

func parseAdminModelRouteID(path string) (int64, *errs.Error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/ops/admin/v1/model-routes/"), "/")
	if trimmed == "" || strings.Contains(trimmed, "/") {
		return 0, errs.BadRequest("invalid route_id")
	}
	routeID, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || routeID <= 0 {
		return 0, errs.BadRequest("invalid route_id")
	}
	return routeID, nil
}

func parseAdminProviderModelID(path string) (int64, *errs.Error) {
	raw := strings.Trim(strings.TrimPrefix(path, "/api/ops/admin/v1/provider-models/"), "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, errs.BadRequest("invalid provider_model_id")
	}
	providerModelID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || providerModelID <= 0 {
		return 0, errs.BadRequest("invalid provider_model_id")
	}
	return providerModelID, nil
}

func pagedRedeemCodesPayload(items []domainredeem.Code, page, pageSize, total int) map[string]any {
	return map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	}
}

func pagedModelProvidersPayload(items []domainmodeladmin.Provider, page, pageSize, total int) map[string]any {
	return map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	}
}

func pagedProviderModelsPayload(items []domainmodeladmin.ProviderModel, page, pageSize, total int) map[string]any {
	return map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	}
}

func pagedModelRoutesPayload(items []domainmodeladmin.Route, page, pageSize, total int) map[string]any {
	return map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	}
}

func apiKeyPayloads(keys []domainapikey.APIKey) []map[string]any {
	items := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		items = append(items, apiKeyPayload(key))
	}
	return items
}

func apiKeyPayload(key domainapikey.APIKey) map[string]any {
	return map[string]any{
		"id":                      key.ID,
		"access_key":              key.AccessKey,
		"name":                    key.Name,
		"status":                  key.Status,
		"group_code":              key.GroupCode,
		"total_quota_points":      key.TotalQuotaPoints,
		"daily_quota_points":      key.DailyQuotaPoints,
		"total_quota_used_points": defaultString(key.TotalQuotaUsedPoints, "0.00000"),
		"daily_quota_used_points": defaultString(key.DailyQuotaUsedPoints, "0.00000"),
		"quota_usage_day":         key.QuotaUsageDay,
		"rpm_limit":               key.RPMLimit,
		"rpm_window_started_at":   key.RPMWindowStartedAt,
		"rpm_window_count":        key.RPMWindowCount,
		"expires_at":              key.ExpiresAt,
		"last_used_at":            key.LastUsedAt,
		"created_at":              key.CreatedAt,
		"updated_at":              key.UpdatedAt,
	}
}

func normalizeYAMLForJSON(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[key] = normalizeYAMLForJSON(item)
		}
		return normalized
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, item := range typed {
			normalized[fmt.Sprint(key)] = normalizeYAMLForJSON(item)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i, item := range typed {
			normalized[i] = normalizeYAMLForJSON(item)
		}
		return normalized
	default:
		return typed
	}
}

func openAPIDocumentPath() string {
	relative := filepath.Join("api", "openapi", "openapi.yaml")
	if _, err := os.Stat(relative); err == nil {
		return relative
	}
	dir, err := os.Getwd()
	if err != nil {
		return relative
	}
	for {
		candidate := filepath.Join(dir, relative)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return relative
		}
		dir = parent
	}
}

func normalizeAppError(err error) *errs.Error {
	if appErr, ok := err.(*errs.Error); ok {
		return appErr
	}
	return errs.Internal("internal server error")
}

func methodNotAllowedError() *errs.Error {
	return errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed")
}

func writeMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, methodNotAllowedError())
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
	statusCode := err.StatusCode
	if err.Code == errs.CodeInsufficientPoints {
		statusCode = http.StatusTooManyRequests
	}
	errType := "server_error"
	if statusCode >= 400 && statusCode < 500 {
		errType = "invalid_request_error"
	}
	httpx.WriteJSON(w, statusCode, map[string]any{
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

func (a *API) compatAbstractModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if value := strings.ToLower(strings.TrimSpace(a.cfg.Routing.OpenAICompatModelMap[model])); value != "" {
		return value
	}
	if _, ok := a.cfg.Billing.QualityPointsByModel[model]; ok {
		return model
	}
	return ""
}

func compatQuality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return "1k"
	case "medium":
		return "2k"
	case "high":
		return "4k"
	case "1k", "2k", "4k":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

func taskProviderPreference(cfg config.Config) []string {
	preferred := []string{}
	if cfg.Routing.DefaultProvider != "" {
		preferred = append(preferred, cfg.Routing.DefaultProvider)
	}
	preferred = append(preferred, cfg.Routing.FallbackProviders...)
	return preferred
}
