package handlers

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/fatballfish/pic-gallery/internal/app/observability"
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
	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	domainmodelhub "github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	domainredeem "github.com/fatballfish/pic-gallery/internal/domain/redeem"
	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
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
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
	compatservice "github.com/fatballfish/pic-gallery/internal/service/compat"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	redeemservice "github.com/fatballfish/pic-gallery/internal/service/redeem"
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
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
	cashierCfg *cashierservice.ConfigFacade
	adminUser  *adminuserservice.Service
	callRecord *admincallrecordservice.Service
	modelAdmin *modeladminservice.Service
	secureCfg  *secureconfigservice.Service
	redeem     *redeemservice.Service
	audit      *auditservice.Service
	adminPerms domainadminauth.PermissionResolver
	docsReady  DocsReadinessChecker
	cfg        config.Config
}

type cashierCustomAmountConfig = domaincashier.CustomAmountConfig
type cashierVisibleMethod = domaincashier.VisibleMethod
type cashierProviderInstance = domaincashier.ProviderInstance
type cashierProviderInstanceWriteRequest = domaincashier.ProviderInstanceWriteRequest

type adminReadinessCheck struct {
	Key         string    `json:"key"`
	Label       string    `json:"label"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail"`
	Summary     string    `json:"summary"`
	FixRoute    string    `json:"fix_route,omitempty"`
	FixAction   string    `json:"fix_action,omitempty"`
	ActionRoute string    `json:"action_route,omitempty"`
	ActionLabel string    `json:"action_label,omitempty"`
	Blocking    bool      `json:"blocking"`
	CheckedAt   time.Time `json:"checked_at"`
}

type adminDashboardOperations struct {
	TodayOrderCount                int            `json:"today_order_count"`
	PaymentSuccessRate             string         `json:"payment_success_rate"`
	FailedWebhookCount             int            `json:"failed_webhook_count"`
	RefundCompensationFailedCount  int            `json:"refund_compensation_failed_count"`
	RefundCompensationOldestFailed *time.Time     `json:"refund_compensation_oldest_failed_at,omitempty"`
	MockEnabled                    bool           `json:"mock_enabled"`
	SignupTrialGrantedUserCount    int            `json:"signup_trial_granted_user_count"`
	TrialExpiringUserCount         int            `json:"trial_expiring_user_count"`
	PreflightFailureCount          int            `json:"preflight_failure_count"`
	PreflightFailuresByErrorCode   map[string]int `json:"preflight_failures_by_error_code"`
	PublicGalleryListViews         uint64         `json:"public_gallery_list_views"`
	PublicGalleryDetailLoginBlocks uint64         `json:"public_gallery_detail_login_blocks"`
	EnabledPaymentMethods          []string       `json:"enabled_payment_methods"`
	GeneratedAt                    time.Time      `json:"generated_at"`
}

type adminCashierOrderSyncResult = cashierservice.QueryOrderStatusResult

type adminCashierOrderSyncResponse struct {
	Order domainbilling.PaymentOrder  `json:"order"`
	Sync  adminCashierOrderSyncResult `json:"sync"`
}

type cashierProviderRefundResult = cashierservice.RefundPaymentResult

type DocsReadinessResult struct {
	Status string
	Detail string
}

type DocsReadinessChecker func(ctx context.Context) DocsReadinessResult

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
		cashierCfg: cashierservice.NewConfigFacade(cashierservice.NewAdminConfigStoreWithDefaultCNYPerPoint(adminSvc, isProductionAppEnv(cfg.App.Env), cfg.Billing.CNYPerPoint)),
		adminUser:  adminUserSvc,
		callRecord: callRecordSvc,
		redeem:     redeemservice.NewServiceWithStore(nil),
		audit:      auditSvc,
		adminPerms: domainadminauth.RolePermissionResolver{},
		docsReady:  defaultDocsReadinessChecker,
		cfg:        cfg,
	}
}

func (a *API) SetAdminPermissionResolver(resolver domainadminauth.PermissionResolver) {
	if resolver == nil {
		a.adminPerms = domainadminauth.RolePermissionResolver{}
		return
	}
	a.adminPerms = resolver
}

func (a *API) SetDocsReadinessChecker(checker DocsReadinessChecker) {
	if checker == nil {
		a.docsReady = defaultDocsReadinessChecker
		return
	}
	a.docsReady = checker
}

func (a *API) SetCashierProviderInstanceStore(store cashierservice.ProviderInstanceStore) {
	a.cashierConfigFacade().WithProviderInstanceStore(store)
}

func (a *API) SetSecureConfigService(service *secureconfigservice.Service) {
	a.secureCfg = service
}

func (a *API) cashierConfigFacade() *cashierservice.ConfigFacade {
	if a.cashierCfg == nil {
		a.cashierCfg = cashierservice.NewConfigFacade(cashierservice.NewAdminConfigStoreWithDefaultCNYPerPoint(a.admin, isProductionAppEnv(a.cfg.App.Env), a.cfg.Billing.CNYPerPoint))
	}
	return a.cashierCfg
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
	api.caps.SetModelRoutingSource(modelAdminSvc)
	api.tasks.SetModelRoutingSource(modelAdminSvc)
	api.billing.SetModelRoutingSource(modelAdminSvc)
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
	login, err := a.auth.LoginWithEmailCodeResult(req.Email, req.Code)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	signupGrant, err := a.signupTrialGrantResult(r.Context(), login.User.ID, login.Created)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	a.setRefreshCookie(w, login.Session)
	payload := map[string]any{
		"access_token":       login.Session.AccessToken,
		"expires_in_seconds": int(time.Until(login.Session.AccessTokenExpiresAt).Seconds()),
		"user_id":            login.User.ID,
	}
	if signupGrant != nil {
		payload["signup_grant"] = signupGrant
	}
	httpx.WriteSuccess(w, r, http.StatusOK, payload)
}

func (a *API) signupTrialGrantResult(ctx context.Context, userID int64, newlyCreated bool) (*billingservice.SignupTrialGrantResult, error) {
	if a.billing == nil {
		return nil, nil
	}
	if newlyCreated {
		trial := a.signupTrialConfig(ctx)
		result, err := a.billing.EnsureSignupTrialGrant(ctx, billingservice.SignupTrialGrantRequest{UserID: userID, SignupTrial: &trial})
		if err != nil {
			return nil, err
		}
		return &result, nil
	}
	balance, err := a.billing.GetBalance(ctx, userID, "")
	if err != nil {
		return nil, err
	}
	return &billingservice.SignupTrialGrantResult{Balance: balance}, nil
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
		ThemeMode     string `json:"theme_mode"`
		AccentTheme   string `json:"accent_theme"`
		DefaultLocale string `json:"default_locale"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if strings.TrimSpace(req.Theme) == "" && strings.TrimSpace(req.ThemeMode) != "" && strings.TrimSpace(req.AccentTheme) != "" {
		req.Theme = strings.TrimSpace(req.ThemeMode) + ":" + strings.TrimSpace(req.AccentTheme)
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
		RouteModelCode:            r.URL.Query().Get("route_model_code"),
		RequestedQuality:          r.URL.Query().Get("requested_quality"),
		RequestedSize:             r.URL.Query().Get("requested_size"),
		RequestedOutputImageCount: outputCount,
		ReferenceImageCount:       refCount,
		UserGroupCode:             user.GroupCode,
		UserGroupCodes:            userGroupCodes(user),
		UserGroupMultiplier:       user.GroupMultiplier,
	})
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	if balance, err := a.billing.GetBalance(r.Context(), user.ID, user.GroupMultiplier); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	} else {
		result.Balance = &balance
		result.Sufficient = true
		result.InsufficientPoints = decimal.Zero.StringFixed(5)
		requiredPoints := strings.TrimSpace(result.ChargedPoints)
		if requiredPoints == "" {
			requiredPoints = strings.TrimSpace(result.EstimatedPoints)
		}
		required, parseRequiredErr := decimal.NewFromString(requiredPoints)
		available, parseAvailableErr := decimal.NewFromString(balance.AvailablePoints)
		if parseRequiredErr != nil || parseAvailableErr != nil {
			httpx.WriteError(w, r, errs.Internal("invalid billing estimate or balance"))
			return
		}
		if available.LessThan(required) {
			result.Sufficient = false
			result.InsufficientPoints = required.Sub(available).Round(5).StringFixed(5)
		}
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
	items := make([]domainbilling.SubscriptionPlan, 0, len(result))
	for _, plan := range result {
		if isPurchasableCashierPlan(plan) {
			items = append(items, plan)
		}
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": items})
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
		result, appErr := a.createCashierOrder(r.Context(), user.ID, cashierOrderCreateInput{
			PurchaseType:   "plan",
			PlanCode:       strings.TrimSpace(req.PlanCode),
			VisibleMethod:  legacyBillingProviderToVisibleMethod(req.Provider),
			IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
		})
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
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

func (a *API) HandleCashierOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireUser(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	plans, err := a.billing.ListPlans(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	cashierPlans := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		if !isPurchasableCashierPlan(plan) {
			continue
		}
		cashierPlans = append(cashierPlans, cashierPlanPayload(plan))
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"plans":                 cashierPlans,
		"custom_amount":         a.cashierCustomAmountConfig(r.Context()),
		"visible_methods":       a.cashierVisibleMethods(r.Context(), false),
		"order_timeout_seconds": a.cashierOrderTimeoutSeconds(),
	})
}

func (a *API) HandleCashierOrders(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodGet {
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
		result, err := a.billing.ListOrders(r.Context(), domainbilling.ListOrdersRequest{UserID: user.ID, Page: page, PageSize: pageSize})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
		return
	}
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var req struct {
		PurchaseType    string `json:"purchase_type"`
		PlanCode        string `json:"plan_code"`
		AmountCNY       string `json:"amount_cny"`
		VisibleMethod   string `json:"visible_method"`
		ClientReturnURL string `json:"client_return_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	result, createErr := a.createCashierOrder(r.Context(), user.ID, cashierOrderCreateInput{
		PurchaseType:    req.PurchaseType,
		PlanCode:        req.PlanCode,
		AmountCNY:       req.AmountCNY,
		VisibleMethod:   req.VisibleMethod,
		ClientReturnURL: req.ClientReturnURL,
		IdempotencyKey:  strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if createErr != nil {
		httpx.WriteError(w, r, createErr)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, result)
}

type cashierOrderCreateInput struct {
	PurchaseType    string
	PlanCode        string
	AmountCNY       string
	VisibleMethod   string
	ClientReturnURL string
	IdempotencyKey  string
}

func (a *API) createCashierOrder(ctx context.Context, userID int64, req cashierOrderCreateInput) (domainbilling.PaymentOrder, *errs.Error) {
	idempotencyKey := strings.TrimSpace(req.IdempotencyKey)
	if idempotencyKey != "" {
		existing, err := a.billing.GetOrderByIdempotencyKey(ctx, userID, idempotencyKey)
		if err == nil {
			return existing, nil
		}
		appErr := normalizeAppError(err)
		if appErr.Code != errs.CodeNotFound {
			return domainbilling.PaymentOrder{}, appErr
		}
	}
	purchaseType := strings.ToLower(strings.TrimSpace(req.PurchaseType))
	if purchaseType == "" {
		purchaseType = "plan"
	}
	if limitErr := a.ensureCashierPendingOrderLimit(ctx, userID); limitErr != nil {
		return domainbilling.PaymentOrder{}, limitErr
	}
	visibleMethod := strings.ToLower(strings.TrimSpace(req.VisibleMethod))
	if visibleMethod == "mock" && isProductionAppEnv(a.cfg.App.Env) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusForbidden, errs.CodeForbidden, "mock payment is disabled in production")
	}
	method, ok := a.cashierVisibleMethod(ctx, visibleMethod)
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusBadRequest, errs.CodePaymentMethodUnavailable, "payment method is unavailable")
	}
	if purchaseType == "custom_amount" {
		customAmountConfig := a.cashierCustomAmountConfig(ctx)
		if !customAmountConfig.Enabled {
			return domainbilling.PaymentOrder{}, errs.BadRequest("custom amount recharge is disabled")
		}
		if amountErr := validateCashierCustomAmount(strings.TrimSpace(req.AmountCNY), customAmountConfig); amountErr != nil {
			return domainbilling.PaymentOrder{}, amountErr
		}
		instance, scheduleErr := a.scheduleCashierProviderInstance(ctx, method, strings.TrimSpace(req.AmountCNY))
		if scheduleErr != nil {
			return domainbilling.PaymentOrder{}, scheduleErr
		}
		orderNo := newCashierOrderNo()
		payment, paymentErr := a.cashierPaymentDisplay(ctx, method, instance, cashierPaymentDisplayRequest{
			OrderNo:         orderNo,
			AmountCNY:       strings.TrimSpace(req.AmountCNY),
			Subject:         "自定义充值",
			ClientReturnURL: strings.TrimSpace(req.ClientReturnURL),
		})
		if paymentErr != nil {
			return domainbilling.PaymentOrder{}, paymentErr
		}
		result, err := a.billing.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
			UserID:             userID,
			OrderNo:            orderNo,
			AmountCNY:          strings.TrimSpace(req.AmountCNY),
			Provider:           method.SourceProviderType,
			CNYPerPoint:        customAmountConfig.CNYPerPoint,
			PurchaseType:       "custom_amount",
			VisibleMethod:      method.Method,
			ProviderType:       instance.ProviderType,
			ProviderInstanceID: instance.ID,
			PaymentDisplay:     payment.Display,
			PaymentURL:         payment.PaymentURL,
			QRCode:             payment.QRCode,
			ClientToken:        payment.ClientToken,
			IdempotencyKey:     idempotencyKey,
		})
		if err != nil {
			return domainbilling.PaymentOrder{}, normalizeAppError(err)
		}
		return result, nil
	}
	if purchaseType != "plan" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("purchase_type must be plan or custom_amount")
	}
	plan, planErr := a.cashierPurchasablePlan(ctx, strings.TrimSpace(req.PlanCode))
	if planErr != nil {
		return domainbilling.PaymentOrder{}, planErr
	}
	instance, scheduleErr := a.scheduleCashierProviderInstance(ctx, method, plan.PriceCNY)
	if scheduleErr != nil {
		return domainbilling.PaymentOrder{}, scheduleErr
	}
	orderNo := newCashierOrderNo()
	payment, paymentErr := a.cashierPaymentDisplay(ctx, method, instance, cashierPaymentDisplayRequest{
		OrderNo:         orderNo,
		AmountCNY:       plan.PriceCNY,
		Subject:         plan.PlanName,
		ClientReturnURL: strings.TrimSpace(req.ClientReturnURL),
	})
	if paymentErr != nil {
		return domainbilling.PaymentOrder{}, paymentErr
	}
	result, err := a.billing.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID:             userID,
		OrderNo:            orderNo,
		PlanCode:           plan.PlanCode,
		Provider:           method.SourceProviderType,
		PurchaseType:       "plan",
		VisibleMethod:      method.Method,
		ProviderType:       instance.ProviderType,
		ProviderInstanceID: instance.ID,
		PaymentDisplay:     payment.Display,
		PaymentURL:         payment.PaymentURL,
		QRCode:             payment.QRCode,
		ClientToken:        payment.ClientToken,
		IdempotencyKey:     idempotencyKey,
	})
	if err != nil {
		return domainbilling.PaymentOrder{}, normalizeAppError(err)
	}
	return result, nil
}

func legacyBillingProviderToVisibleMethod(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "mock":
		return "mock"
	case "alipay", "alipay_direct", "easypay_alipay", "jeepay_alipay":
		return "alipay"
	case "wxpay", "wechat", "wechatpay", "wxpay_direct", "easypay_wxpay", "jeepay_wxpay":
		return "wxpay"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

func (a *API) ensureCashierPendingOrderLimit(ctx context.Context, userID int64) *errs.Error {
	limit := a.cfg.Cashier.MaxPendingOrdersPerUser
	if limit <= 0 {
		limit = 3
	}
	page, err := a.billing.ListOrders(ctx, domainbilling.ListOrdersRequest{UserID: userID, Status: "pending", Page: 1, PageSize: limit})
	if err != nil {
		return normalizeAppError(err)
	}
	if page.Total >= limit {
		return errs.New(http.StatusConflict, errs.CodePaymentTooManyPending, "too many pending payment orders")
	}
	return nil
}

func (a *API) cashierOrderTimeoutSeconds() int {
	if a.cfg.Cashier.OrderTimeoutSeconds > 0 {
		return a.cfg.Cashier.OrderTimeoutSeconds
	}
	return 1800
}

func (a *API) HandleCashierOrderDetail(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	orderID, action, parseErr := parseCashierOrderPath(r.URL.Path)
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
	case r.Method == http.MethodPost && action == "mock-pay":
		if isProductionAppEnv(a.cfg.App.Env) {
			httpx.WriteError(w, r, errs.New(http.StatusForbidden, errs.CodeForbidden, "mock payment is disabled in production"))
			return
		}
		order, err := a.billing.GetOrder(r.Context(), user.ID, orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if cashierOrderProviderType(order, cashierProviderInstance{}) != "mock" {
			httpx.WriteError(w, r, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "mock payment is only available for mock provider orders"))
			return
		}
		result, err := a.billing.CompleteRechargeOrder(r.Context(), domainbilling.CompleteRechargeOrderRequest{
			UserID:   user.ID,
			OrderID:  orderID,
			Provider: "mock",
			TradeNo:  fmt.Sprintf("MOCK-%d-%d", orderID, time.Now().UTC().UnixNano()),
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	default:
		writeMethodNotAllowed(w, r)
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
		RouteModelCode:            r.URL.Query().Get("route_model_code"),
		RequestedQuality:          r.URL.Query().Get("requested_quality"),
		RequestedSize:             r.URL.Query().Get("requested_size"),
		RequestedOutputImageCount: outputCount,
		ReferenceImageCount:       refCount,
		UserGroupCode:             identity.GroupCode,
		UserGroupCodes:            []string{identity.GroupCode},
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
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	result, err := a.caps.ListForGroups(r.Context(), userGroupCodes(user), a.cfg.Billing.TaskMultipliers)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) HandleOpenCapabilities(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	identity, appErr := a.requireOpenAPIKey(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	result, err := a.caps.ListForGroups(r.Context(), []string{identity.GroupCode}, a.cfg.Billing.TaskMultipliers)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
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
	if strings.EqualFold(providerCode, "mock") && isProductionAppEnv(a.cfg.App.Env) {
		httpx.WriteError(w, r, errs.New(http.StatusForbidden, errs.CodeForbidden, "mock payment is disabled in production"))
		return
	}
	if strings.EqualFold(providerCode, "easypay") {
		_, appErr := a.handleEasyPayWebhook(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		writePaymentWebhookSuccess(w, providerCode)
		return
	}
	if strings.EqualFold(providerCode, "alipay_direct") || strings.EqualFold(providerCode, "alipay") {
		_, appErr := a.handleAlipayWebhook(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		writePaymentWebhookSuccess(w, providerCode)
		return
	}
	if strings.EqualFold(providerCode, "wxpay_direct") || strings.EqualFold(providerCode, "wxpay") || strings.EqualFold(providerCode, "wechatpay") {
		_, appErr := a.handleWxPayWebhook(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		writePaymentWebhookSuccess(w, providerCode)
		return
	}
	if strings.EqualFold(providerCode, "jeepay") || strings.HasPrefix(strings.ToLower(providerCode), "jeepay_") {
		_, appErr := a.handleJeePayWebhook(r, providerCode)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		writePaymentWebhookSuccess(w, providerCode)
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

func writePaymentWebhookSuccess(w http.ResponseWriter, providerCode string) {
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	if providerCode == "wxpay_direct" || providerCode == "wxpay" || providerCode == "wechatpay" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("success"))
}

func (a *API) handleEasyPayWebhook(r *http.Request) (domainbilling.PaymentOrder, *errs.Error) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid webhook body")
	}
	values, parseErr := url.ParseQuery(string(body))
	if parseErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid easypay webhook body")
	}
	pid := strings.TrimSpace(values.Get("pid"))
	instance, ok := a.easypayProviderInstanceByPID(r.Context(), pid)
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if key == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	sign := strings.TrimSpace(values.Get("sign"))
	if sign == "" || !hmac.Equal([]byte(easyPaySignFromValues(values, key)), []byte(sign)) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusBadRequest, errs.CodePaymentSignatureInvalid, "payment webhook signature is invalid")
	}
	status := strings.TrimSpace(values.Get("trade_status"))
	if status != "" && !strings.EqualFold(status, "TRADE_SUCCESS") && status != "1" {
		return domainbilling.PaymentOrder{}, errs.BadRequest("payment webhook status is not success")
	}
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	result, err := a.billing.MarkOrderPaid(r.Context(), domainbilling.MarkOrderPaidRequest{
		Provider:  providerType,
		OrderNo:   strings.TrimSpace(values.Get("out_trade_no")),
		TradeNo:   strings.TrimSpace(values.Get("trade_no")),
		AmountCNY: strings.TrimSpace(values.Get("money")),
	})
	if err != nil {
		return domainbilling.PaymentOrder{}, normalizeAppError(err)
	}
	return result, nil
}

func (a *API) handleJeePayWebhook(r *http.Request, providerCode string) (domainbilling.PaymentOrder, *errs.Error) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid webhook body")
	}
	values, parseErr := url.ParseQuery(string(body))
	if parseErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid jeepay webhook body")
	}
	mchNo := strings.TrimSpace(values.Get("mchNo"))
	appID := strings.TrimSpace(values.Get("appId"))
	instance, ok := a.jeepayProviderInstanceByMerchant(r.Context(), providerCode, mchNo, appID)
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if key == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	sign := strings.TrimSpace(values.Get("sign"))
	if sign == "" || !hmac.Equal([]byte(jeepaySignFromValues(values, key)), []byte(sign)) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusBadRequest, errs.CodePaymentSignatureInvalid, "payment webhook signature is invalid")
	}
	state := strings.TrimSpace(values.Get("state"))
	status := strings.TrimSpace(values.Get("status"))
	if state != "" && state != "2" && !strings.EqualFold(state, "success") {
		return domainbilling.PaymentOrder{}, errs.BadRequest("payment webhook status is not success")
	}
	if status != "" && !strings.EqualFold(status, "success") && !strings.EqualFold(status, "paid") {
		return domainbilling.PaymentOrder{}, errs.BadRequest("payment webhook status is not success")
	}
	tradeNo := strings.TrimSpace(values.Get("payOrderId"))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(values.Get("channelOrderNo"))
	}
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(values.Get("trade_no"))
	}
	result, err := a.billing.MarkOrderPaid(r.Context(), domainbilling.MarkOrderPaidRequest{
		Provider:  strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		OrderNo:   strings.TrimSpace(values.Get("mchOrderNo")),
		TradeNo:   tradeNo,
		AmountCNY: jeepayAmountCNYFromFen(values.Get("amount")),
	})
	if err != nil {
		return domainbilling.PaymentOrder{}, normalizeAppError(err)
	}
	return result, nil
}

func (a *API) handleAlipayWebhook(r *http.Request) (domainbilling.PaymentOrder, *errs.Error) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid webhook body")
	}
	values, parseErr := url.ParseQuery(string(body))
	if parseErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid alipay webhook body")
	}
	appID := strings.TrimSpace(values.Get("app_id"))
	instance, ok := a.alipayProviderInstanceByAppID(r.Context(), appID)
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	publicKey := strings.TrimSpace(mapStringValue(instance.Config, "alipay_public_key", "public_key", "publicKey"))
	if publicKey == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	sign := strings.TrimSpace(values.Get("sign"))
	if sign == "" || !alipayRSA2Verify(values, publicKey, sign) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusBadRequest, errs.CodePaymentSignatureInvalid, "payment webhook signature is invalid")
	}
	status := strings.TrimSpace(values.Get("trade_status"))
	if status != "" && !strings.EqualFold(status, "TRADE_SUCCESS") && !strings.EqualFold(status, "TRADE_FINISHED") {
		return domainbilling.PaymentOrder{}, errs.BadRequest("payment webhook status is not success")
	}
	result, err := a.billing.MarkOrderPaid(r.Context(), domainbilling.MarkOrderPaidRequest{
		Provider:  "alipay_direct",
		OrderNo:   strings.TrimSpace(values.Get("out_trade_no")),
		TradeNo:   strings.TrimSpace(values.Get("trade_no")),
		AmountCNY: strings.TrimSpace(values.Get("total_amount")),
	})
	if err != nil {
		return domainbilling.PaymentOrder{}, normalizeAppError(err)
	}
	return result, nil
}

func (a *API) handleWxPayWebhook(r *http.Request) (domainbilling.PaymentOrder, *errs.Error) {
	body, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		return domainbilling.PaymentOrder{}, errs.BadRequest("invalid webhook body")
	}
	serial := strings.TrimSpace(r.Header.Get("Wechatpay-Serial"))
	instance, ok := a.wxpayProviderInstanceBySerial(r.Context(), serial)
	if !ok {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	publicKey := strings.TrimSpace(mapStringValue(instance.Config, "wechat_pay_public_key", "public_key", "publicKey"))
	if publicKey == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	timestamp := strings.TrimSpace(r.Header.Get("Wechatpay-Timestamp"))
	nonce := strings.TrimSpace(r.Header.Get("Wechatpay-Nonce"))
	signature := strings.TrimSpace(r.Header.Get("Wechatpay-Signature"))
	if timestamp == "" || nonce == "" || signature == "" || !wxPayVerifySignature(publicKey, timestamp, nonce, string(body), signature) {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusBadRequest, errs.CodePaymentSignatureInvalid, "payment webhook signature is invalid")
	}
	apiV3Key := strings.TrimSpace(mapStringValue(instance.Config, "api_v3_key", "apiv3_key", "apiV3Key"))
	if apiV3Key == "" {
		return domainbilling.PaymentOrder{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	transaction, decryptErr := decryptWxPayTransaction(body, apiV3Key)
	if decryptErr != nil {
		return domainbilling.PaymentOrder{}, decryptErr
	}
	if transaction.TradeState != "" && !strings.EqualFold(transaction.TradeState, "SUCCESS") {
		return domainbilling.PaymentOrder{}, errs.BadRequest("payment webhook status is not success")
	}
	amountCNY := wxPayAmountCNYFromFen(transaction.Amount.Total)
	result, err := a.billing.MarkOrderPaid(r.Context(), domainbilling.MarkOrderPaidRequest{
		Provider:  "wxpay_direct",
		OrderNo:   strings.TrimSpace(transaction.OutTradeNo),
		TradeNo:   strings.TrimSpace(transaction.TransactionID),
		AmountCNY: amountCNY,
	})
	if err != nil {
		return domainbilling.PaymentOrder{}, normalizeAppError(err)
	}
	return result, nil
}

func (a *API) easypayProviderInstanceByPID(ctx context.Context, pid string) (cashierProviderInstance, bool) {
	pid = strings.TrimSpace(pid)
	if pid == "" {
		return cashierProviderInstance{}, false
	}
	for _, instance := range a.cashierProviderInstances(ctx) {
		providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
		if providerType != "easypay_alipay" && providerType != "easypay_wxpay" {
			continue
		}
		if !instance.Enabled || instance.ConfigStatus != "configured" {
			continue
		}
		if strings.TrimSpace(mapStringValue(instance.Config, "pid", "merchant_id", "merchantId")) == pid {
			return instance, true
		}
	}
	return cashierProviderInstance{}, false
}

func (a *API) alipayProviderInstanceByAppID(ctx context.Context, appID string) (cashierProviderInstance, bool) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return cashierProviderInstance{}, false
	}
	for _, instance := range a.cashierProviderInstances(ctx) {
		if strings.ToLower(strings.TrimSpace(instance.ProviderType)) != "alipay_direct" {
			continue
		}
		if !instance.Enabled || instance.ConfigStatus != "configured" {
			continue
		}
		if strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId")) == appID {
			return instance, true
		}
	}
	return cashierProviderInstance{}, false
}

func (a *API) wxpayProviderInstanceBySerial(ctx context.Context, serial string) (cashierProviderInstance, bool) {
	serial = strings.TrimSpace(serial)
	if serial == "" {
		return cashierProviderInstance{}, false
	}
	for _, instance := range a.cashierProviderInstances(ctx) {
		if strings.ToLower(strings.TrimSpace(instance.ProviderType)) != "wxpay_direct" {
			continue
		}
		if !instance.Enabled || instance.ConfigStatus != "configured" {
			continue
		}
		instanceSerial := strings.TrimSpace(mapStringValue(instance.Config, "wechat_pay_public_key_id", "wechatpay_serial", "wechatpaySerial", "serial"))
		if instanceSerial == "" {
			instanceSerial = strings.TrimSpace(mapStringValue(instance.Config, "merchant_certificate_serial", "merchantCertificateSerial"))
		}
		if instanceSerial == serial {
			return instance, true
		}
	}
	return cashierProviderInstance{}, false
}

func (a *API) jeepayProviderInstanceByMerchant(ctx context.Context, providerCode string, mchNo string, appID string) (cashierProviderInstance, bool) {
	mchNo = strings.TrimSpace(mchNo)
	appID = strings.TrimSpace(appID)
	if mchNo == "" {
		return cashierProviderInstance{}, false
	}
	providerCode = strings.ToLower(strings.TrimSpace(providerCode))
	for _, instance := range a.cashierProviderInstances(ctx) {
		providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
		if providerType != "jeepay_alipay" && providerType != "jeepay_wxpay" {
			continue
		}
		if providerCode != "" && providerCode != "jeepay" && providerCode != providerType {
			continue
		}
		if !instance.Enabled || instance.ConfigStatus != "configured" {
			continue
		}
		instanceMchNo := strings.TrimSpace(mapStringValue(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
		instanceAppID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
		if instanceMchNo == mchNo && (appID == "" || instanceAppID == "" || instanceAppID == appID) {
			return instance, true
		}
	}
	return cashierProviderInstance{}, false
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
		httpx.WriteError(w, r, normalizeAppError(svcErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, referenceAssetPayload(asset, false))
}

func (a *API) HandleReferenceAssetsImportFromGallery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		GalleryImageIDs []string `json:"gallery_image_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	ids := make([]string, 0, len(req.GalleryImageIDs))
	seen := make(map[string]struct{}, len(req.GalleryImageIDs))
	for _, raw := range req.GalleryImageIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		httpx.WriteError(w, r, errs.BadRequest("gallery_image_ids is required"))
		return
	}
	if len(ids) > 20 {
		httpx.WriteError(w, r, errs.BadRequest("gallery_image_ids exceeds limit"))
		return
	}

	items := make([]domainassets.ReferenceAsset, 0, len(ids))
	for _, imageID := range ids {
		result, content, err := a.tasks.DownloadImageResult(r.Context(), user.ID, imageID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		contentType := defaultString(result.MimeType, http.DetectContentType(content))
		asset, svcErr := a.assets.UploadWithMetadata(user.ID, imageDownloadFilename(result), contentType, content, domainassets.UploadMetadata{UploadSource: "gallery_import"})
		if svcErr != nil {
			httpx.WriteError(w, r, normalizeAppError(svcErr))
			return
		}
		items = append(items, referenceAssetPayload(asset, false))
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, map[string]any{"items": items})
}

func (a *API) HandleReferenceAssetGet(w http.ResponseWriter, r *http.Request) {
	assetID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/reference-assets/")
	if strings.HasSuffix(assetID, "/download") {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w, r)
			return
		}
		user, appErr := a.requireUserWithQueryToken(r)
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
		httpx.WriteSuccess(w, r, http.StatusOK, referenceAssetPayload(asset, false))
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

func referenceAssetPayload(asset domainassets.ReferenceAsset, openAPI bool) domainassets.ReferenceAsset {
	if strings.TrimSpace(asset.PreviewURL) != "" && strings.TrimSpace(asset.DownloadURL) != "" {
		return asset
	}
	prefix := "/api/agent/image/v1/reference-assets/"
	if openAPI {
		prefix = "/api/open/image/v1/reference-assets/"
	}
	downloadURL := prefix + asset.ID + "/download"
	asset.PreviewURL = downloadURL
	asset.DownloadURL = downloadURL
	return asset
}

func (a *API) HandleImageDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	user, appErr := a.requireUserWithQueryToken(r)
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
	w.Header().Set("Content-Disposition", `attachment; filename="`+imageDownloadFilename(result)+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}

func imageDownloadFilename(result provider.ImageResult) string {
	name := strings.TrimSpace(result.ID)
	if name == "" {
		name = "image"
	}
	if ext := filepath.Ext(name); ext != "" {
		return name
	}
	ext := filepath.Ext(strings.TrimSpace(result.ObjectKey))
	if ext == "" {
		switch strings.ToLower(strings.TrimSpace(result.MimeType)) {
		case "image/jpeg", "image/jpg":
			ext = ".jpg"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			ext = ".png"
		}
	}
	return name + ext
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
		httpx.WriteError(w, r, normalizeAppError(svcErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, map[string]any{
		"asset_id":    asset.ID,
		"status":      asset.Status,
		"upload_mode": "inline_base64",
		"asset":       referenceAssetPayload(asset, true),
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
	httpx.WriteSuccess(w, r, http.StatusCreated, referenceAssetPayload(asset, true))
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
	httpx.WriteSuccess(w, r, http.StatusOK, referenceAssetPayload(asset, true))
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
	user, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	taskID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/tasks/")
	if strings.HasSuffix(taskID, "/events") {
		taskID = strings.TrimSuffix(taskID, "/events")
		a.handleAgentTaskEvents(w, r, user, taskID)
		return
	}
	task, err := a.tasks.GetByID(r.Context(), user.ID, taskID)
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	task = decorateTaskProgress(task)
	httpx.WriteSuccess(w, r, http.StatusOK, task)
}

func (a *API) HandleAgentTaskEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	a.handleAgentTaskStream(w, r, user)
}

func (a *API) requireUserWithQueryToken(r *http.Request) (*domainauth.User, *errs.Error) {
	if token := strings.TrimSpace(r.URL.Query().Get("access_token")); token != "" && r.Header.Get("Authorization") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return a.requireUser(r)
}

func (a *API) requireAdminWithQueryToken(r *http.Request) (*adminauthservice.Claims, *errs.Error) {
	if token := strings.TrimSpace(r.URL.Query().Get("access_token")); token != "" && r.Header.Get("Authorization") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return a.requireAdmin(r)
}

func (a *API) handleAgentTaskEvents(w http.ResponseWriter, r *http.Request, user *domainauth.User, taskID string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, errs.CodeInternal, "streaming is not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	sendTask := func() bool {
		task, err := a.tasks.GetByID(r.Context(), user.ID, taskID)
		if err != nil {
			writeSSE(w, "error", normalizeAppError(err))
			flusher.Flush()
			return true
		}
		task = decorateTaskProgress(task)
		writeSSE(w, "task", task)
		flusher.Flush()
		return isTerminalTaskStatus(task.Status)
	}
	if sendTask() {
		return
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if sendTask() {
				return
			}
		}
	}
}

func (a *API) handleAgentTaskStream(w http.ResponseWriter, r *http.Request, user *domainauth.User) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.WriteError(w, r, errs.New(http.StatusInternalServerError, errs.CodeInternal, "streaming is not supported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	seen := map[string]string{}
	sendSnapshot := func(initial bool) {
		tasks, err := a.latestUserTasks(r.Context(), user.ID, 20)
		if err != nil {
			writeSSE(w, "error", normalizeAppError(err))
			flusher.Flush()
			return
		}
		if initial {
			writeSSE(w, "history", tasks)
			for _, task := range tasks {
				seen[task.ID] = taskStreamSignature(task)
			}
			flusher.Flush()
			return
		}
		sent := false
		for _, task := range tasks {
			signature := taskStreamSignature(task)
			if seen[task.ID] == signature {
				continue
			}
			seen[task.ID] = signature
			writeSSE(w, "task", task)
			sent = true
		}
		if !sent {
			writeSSE(w, "ping", map[string]string{"time": time.Now().UTC().Format(time.RFC3339)})
		}
		flusher.Flush()
	}

	sendSnapshot(true)
	if strings.EqualFold(r.URL.Query().Get("once"), "true") {
		return
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			sendSnapshot(false)
		}
	}
}

func (a *API) latestUserTasks(ctx context.Context, userID int64, limit int) ([]domainimagetask.Task, error) {
	tasks, err := a.tasks.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return taskSortTime(tasks[i]).After(taskSortTime(tasks[j]))
	})
	if limit > 0 && len(tasks) > limit {
		tasks = tasks[:limit]
	}
	tasks = decorateTaskProgressList(tasks)
	sort.SliceStable(tasks, func(i, j int) bool {
		return taskSortTime(tasks[i]).Before(taskSortTime(tasks[j]))
	})
	return tasks, nil
}

func taskSortTime(task domainimagetask.Task) time.Time {
	if !task.CreatedAt.IsZero() {
		return task.CreatedAt
	}
	if !task.UpdatedAt.IsZero() {
		return task.UpdatedAt
	}
	return time.Time{}
}

func taskStreamSignature(task domainimagetask.Task) string {
	return fmt.Sprintf("%s:%s:%s:%s:%s:%d", task.ID, task.Status, task.ProgressStage, task.ProgressMessage, task.UpdatedAt.Format(time.RFC3339Nano), len(task.Results))
}

func decorateTaskProgressList(tasks []domainimagetask.Task) []domainimagetask.Task {
	for index := range tasks {
		tasks[index] = decorateTaskProgress(tasks[index])
	}
	return tasks
}

func decorateTaskProgress(task domainimagetask.Task) domainimagetask.Task {
	if task.ProgressStage != "" && task.ProgressMessage != "" {
		return task
	}
	stage, message := taskProgressView(task)
	if task.ProgressStage == "" {
		task.ProgressStage = stage
	}
	if task.ProgressMessage == "" {
		task.ProgressMessage = message
	}
	return task
}

func taskProgressView(task domainimagetask.Task) (string, string) {
	switch task.Status {
	case domainimagetask.StatusQueued:
		return "queued", "任务已进入生成队列"
	case domainimagetask.StatusRunning:
		if len(task.Results) > 0 {
			return "persisting", fmt.Sprintf("已生成 %d 张图片，正在同步结果", len(task.Results))
		}
		if attempt := latestTaskAttempt(task.Attempts); attempt != nil {
			if attempt.ErrorMessage != "" {
				return "provider", attempt.ErrorMessage
			}
			if attempt.Error != "" {
				return "provider", attempt.Error
			}
			if attempt.ModelCode != "" {
				return "provider", fmt.Sprintf("正在调用 %s 生成图片", attempt.ModelCode)
			}
			if attempt.Provider != "" {
				return "provider", fmt.Sprintf("正在调用 %s 生成图片", attempt.Provider)
			}
		}
		if task.RouteModelCode != "" {
			return "routing", fmt.Sprintf("已匹配模型 %s，正在生成图片", task.RouteModelCode)
		}
		if task.AbstractModel != "" {
			return "routing", fmt.Sprintf("已匹配模型 %s，正在生成图片", task.AbstractModel)
		}
		return "running", "任务正在生成中"
	case domainimagetask.StatusSucceeded:
		return "completed", "生成完成，结果已同步到资产"
	case domainimagetask.StatusPartialFailed:
		return "completed", "部分图片生成完成，其余图片生成失败"
	case domainimagetask.StatusFailed:
		if task.ErrorMessage != "" {
			return "failed", task.ErrorMessage
		}
		if task.ErrorCode != "" {
			return "failed", task.ErrorCode
		}
		return "failed", "任务生成失败"
	case domainimagetask.StatusRejected:
		if task.ErrorMessage != "" {
			return "failed", task.ErrorMessage
		}
		return "failed", "任务已被拒绝"
	case "cancelled":
		return "cancelled", "任务已取消"
	case domainimagetask.StatusDeleted:
		return "deleted", "任务已删除"
	default:
		if task.Status != "" {
			return task.Status, "任务状态已更新"
		}
		return "unknown", "正在等待任务状态"
	}
}

func latestTaskAttempt(attempts []domainimagetask.Attempt) *domainimagetask.Attempt {
	if len(attempts) == 0 {
		return nil
	}
	latestIndex := len(attempts) - 1
	for index := range attempts {
		if attempts[index].StartedAt == nil {
			continue
		}
		latest := attempts[latestIndex].StartedAt
		if latest == nil || attempts[index].StartedAt.After(*latest) {
			latestIndex = index
		}
	}
	return &attempts[latestIndex]
}

func writeSSE(w io.Writer, event string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		data = []byte(`{"error":{"code":"INTERNAL_ERROR","message":"failed to encode event"}}`)
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}

func isTerminalTaskStatus(status string) bool {
	switch status {
	case "succeeded", "partial_failed", "failed", "cancelled", "rejected", "deleted":
		return true
	default:
		return false
	}
}

func (a *API) HandleAgentGalleryImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
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
	result, err := a.tasks.ListGalleryByUser(r.Context(), user.ID, domainimagetask.GalleryListRequest{
		Page:     page,
		PageSize: pageSize,
		Status:   r.URL.Query().Get("visibility_status"),
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

func (a *API) HandleAgentGalleryImageDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && strings.TrimSuffix(r.URL.Path, "/") == "/api/agent/gallery/v1/images" {
		a.HandleAgentGalleryImages(w, r)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodDelete && r.Method != http.MethodPut && r.Method != http.MethodPatch {
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
	if r.Method == http.MethodDelete {
		if action != "" {
			httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery image route not found"))
			return
		}
		if err := a.tasks.DeleteImageResult(r.Context(), user.ID, imageID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "gallery.image_delete", "image_result", imageID, nil)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodPut || r.Method == http.MethodPatch {
		if action != "group" {
			httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery image route not found"))
			return
		}
		var payload struct {
			ImageGroup string `json:"image_group"`
			Group      string `json:"group"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		imageGroup := payload.ImageGroup
		if imageGroup == "" {
			imageGroup = payload.Group
		}
		image, err := a.tasks.SetImageGroup(r.Context(), user.ID, imageID, imageGroup)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "gallery.image_group_update", "image_result", imageID, map[string]any{"image_group": image.ImageGroup})
		httpx.WriteSuccess(w, r, http.StatusOK, image)
		return
	}
	if action == "like" || action == "favorite" {
		var payload struct {
			Active *bool `json:"active"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&payload)
		}
		active := true
		if payload.Active != nil {
			active = *payload.Active
		}
		image, err := a.tasks.SetPublicImageInteraction(r.Context(), user.ID, imageID, action, active)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "gallery.public_"+action, "image_result", imageID, map[string]any{"active": active})
		httpx.WriteSuccess(w, r, http.StatusOK, image)
		return
	}
	if action != "publish" {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery image route not found"))
		return
	}
	ownedImage, err := a.findOwnedGalleryImage(r.Context(), user.ID, imageID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	allowed, reason, err := a.moderatePublishRequest(r.Context(), ownedImage.Prompt)
	if err != nil {
		a.recordAudit(r, "user", fmt.Sprintf("%d", user.ID), "gallery.publish_moderation_skipped", "image_result", imageID, map[string]any{"reason": err.Error()})
		allowed = true
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
	taskID := strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/history/tasks/")
	retry := strings.HasSuffix(taskID, "/retry")
	if retry {
		taskID = strings.TrimSuffix(taskID, "/retry")
	}
	switch r.Method {
	case http.MethodGet, http.MethodDelete:
	case http.MethodPost:
		if !retry {
			writeMethodNotAllowed(w, r)
			return
		}
	default:
		writeMethodNotAllowed(w, r)
		return
	}
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if retry {
		task, err := a.tasks.RetryTask(r.Context(), user.ID, taskID, domainimagetask.RetryRequest{
			UserGroupCode:       user.GroupCode,
			UserGroupCodes:      userGroupCodes(user),
			UserGroupMultiplier: a.userGroupMultiplier(user.GroupCode),
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		task = decorateTaskProgress(task)
		httpx.WriteSuccess(w, r, http.StatusAccepted, task)
		return
	}
	switch r.Method {
	case http.MethodGet:
		task, err := a.tasks.GetByID(r.Context(), user.ID, taskID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		task = decorateTaskProgress(task)
		httpx.WriteSuccess(w, r, http.StatusOK, task)
	case http.MethodDelete:
		if err := a.tasks.DeleteByID(r.Context(), user.ID, taskID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
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
		"permissions": a.adminPermissionsForPrincipal(domainadminauth.AdminPrincipal{
			ID:     session.AdminID,
			Email:  session.Email,
			Role:   session.Role,
			Status: session.Status,
		}),
	})
}

func (a *API) HandleOpenGalleryImages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", true) {
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
			"items": []domainimagetask.GalleryImage{},
			"pagination": map[string]any{
				"page":      1,
				"page_size": 20,
				"total":     0,
			},
		})
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
	viewer := a.optionalUserWithQueryToken(r)
	var viewerID int64
	if viewer != nil {
		viewerID = viewer.ID
	}
	result, err := a.tasks.ListPublicGallery(r.Context(), domainimagetask.GalleryListRequest{
		Page:           page,
		PageSize:       pageSize,
		Sort:           r.URL.Query().Get("sort"),
		Query:          r.URL.Query().Get("query"),
		RouteModelCode: r.URL.Query().Get("route_model_code"),
		TaskType:       r.URL.Query().Get("task_type"),
		ViewerUserID:   viewerID,
		LikedOnly:      parseBoolQuery(r, "liked"),
		FavoritedOnly:  parseBoolQuery(r, "favorited"),
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	observability.DefaultMetrics().IncPublicGalleryListView()
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": publicGalleryListItems(result.Items),
		"pagination": map[string]any{
			"page":      result.Page,
			"page_size": result.PageSize,
			"total":     result.Total,
		},
	})
}

func (a *API) optionalUserWithQueryToken(r *http.Request) *domainauth.User {
	if r.Header.Get("Authorization") == "" && strings.TrimSpace(r.URL.Query().Get("access_token")) == "" {
		return nil
	}
	user, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		return nil
	}
	return user
}

func hasUserCredential(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("Authorization")) != "" || strings.TrimSpace(r.URL.Query().Get("access_token")) != ""
}

type publicGalleryListImage struct {
	ID                string     `json:"id"`
	TaskID            string     `json:"task_id,omitempty"`
	Prompt            *string    `json:"prompt"`
	PromptExcerpt     string     `json:"prompt_excerpt"`
	AbstractModel     string     `json:"abstract_model,omitempty"`
	RouteModelCode    string     `json:"route_model_code,omitempty"`
	TaskType          string     `json:"task_type,omitempty"`
	TaskStatus        string     `json:"task_status,omitempty"`
	Quality           string     `json:"quality,omitempty"`
	AspectRatio       string     `json:"aspect_ratio,omitempty"`
	URL               string     `json:"url,omitempty"`
	DownloadURL       string     `json:"download_url,omitempty"`
	MimeType          string     `json:"mime_type,omitempty"`
	FileSizeBytes     int64      `json:"file_size_bytes"`
	Width             int        `json:"width"`
	Height            int        `json:"height"`
	VisibilityStatus  string     `json:"visibility_status"`
	PublishedAt       *time.Time `json:"published_at,omitempty"`
	AuthorName        string     `json:"author_name,omitempty"`
	LikeCount         int        `json:"like_count"`
	FavoriteCount     int        `json:"favorite_count"`
	LikedByViewer     bool       `json:"liked_by_viewer,omitempty"`
	FavoritedByViewer bool       `json:"favorited_by_viewer,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

func publicGalleryListItems(items []domainimagetask.GalleryImage) []publicGalleryListImage {
	redacted := make([]publicGalleryListImage, 0, len(items))
	for _, item := range items {
		redacted = append(redacted, publicGalleryListImage{
			ID:                item.ID,
			TaskID:            item.TaskID,
			Prompt:            nil,
			PromptExcerpt:     promptExcerpt(item.Prompt, 24),
			AbstractModel:     item.AbstractModel,
			RouteModelCode:    item.RouteModelCode,
			TaskType:          item.TaskType,
			TaskStatus:        item.TaskStatus,
			Quality:           item.Quality,
			AspectRatio:       item.AspectRatio,
			URL:               item.URL,
			DownloadURL:       item.DownloadURL,
			MimeType:          item.MimeType,
			FileSizeBytes:     item.FileSizeBytes,
			Width:             item.Width,
			Height:            item.Height,
			VisibilityStatus:  item.VisibilityStatus,
			PublishedAt:       item.PublishedAt,
			AuthorName:        item.AuthorName,
			LikeCount:         item.LikeCount,
			FavoriteCount:     item.FavoriteCount,
			LikedByViewer:     item.LikedByViewer,
			FavoritedByViewer: item.FavoritedByViewer,
			CreatedAt:         item.CreatedAt,
		})
	}
	return redacted
}

func publicGalleryDetailItem(item domainimagetask.GalleryImage) publicGalleryListImage {
	prompt := item.Prompt
	return publicGalleryListImage{
		ID:                item.ID,
		TaskID:            item.TaskID,
		Prompt:            &prompt,
		PromptExcerpt:     promptExcerpt(item.Prompt, 24),
		AbstractModel:     item.AbstractModel,
		RouteModelCode:    item.RouteModelCode,
		TaskType:          item.TaskType,
		TaskStatus:        item.TaskStatus,
		Quality:           item.Quality,
		AspectRatio:       item.AspectRatio,
		URL:               item.URL,
		DownloadURL:       item.DownloadURL,
		MimeType:          item.MimeType,
		FileSizeBytes:     item.FileSizeBytes,
		Width:             item.Width,
		Height:            item.Height,
		VisibilityStatus:  item.VisibilityStatus,
		PublishedAt:       item.PublishedAt,
		AuthorName:        item.AuthorName,
		LikeCount:         item.LikeCount,
		FavoriteCount:     item.FavoriteCount,
		LikedByViewer:     item.LikedByViewer,
		FavoritedByViewer: item.FavoritedByViewer,
		CreatedAt:         item.CreatedAt,
	}
}

func promptExcerpt(prompt string, limit int) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if limit <= 0 || prompt == "" {
		return ""
	}
	runes := []rune(prompt)
	if len(runes) <= limit {
		visible := len(runes) / 2
		if visible < 1 {
			return "…"
		}
		if visible > limit-1 {
			visible = limit - 1
		}
		return string(runes[:visible]) + "…"
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func parseBoolQuery(r *http.Request, key string) bool {
	value := strings.TrimSpace(strings.ToLower(r.URL.Query().Get(key)))
	return value == "1" || value == "true" || value == "yes"
}

func (a *API) HandleOpenGalleryImageDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if !a.adminConfigBool(r.Context(), "public_gallery", "gallery_enabled", true) {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery is disabled"))
		return
	}
	imageID := strings.TrimPrefix(r.URL.Path, "/api/open/image/v1/gallery/images/")
	if strings.HasSuffix(imageID, "/image") {
		imageID = strings.TrimSuffix(imageID, "/image")
		result, content, err := a.tasks.DownloadPublicImageResult(r.Context(), imageID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.Header().Set("Content-Type", defaultString(result.MimeType, "application/octet-stream"))
		w.Header().Set("Content-Disposition", `inline; filename="`+imageDownloadFilename(result)+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
		return
	}
	if !hasUserCredential(r) {
		observability.DefaultMetrics().IncPublicGalleryDetailLoginBlock()
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeLoginRequiredGalleryDetail, "login required to view public image detail"))
		return
	}
	user, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	image, err := a.tasks.GetPublicImage(r.Context(), imageID, user.ID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, publicGalleryDetailItem(image))
}

func (a *API) HandleAdminImageReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageReviews); appErr != nil {
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
		Page:       page,
		PageSize:   pageSize,
		Status:     r.URL.Query().Get("status"),
		ReviewOnly: true,
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
	admin, appErr := a.requireAdminPermissionWithQueryToken(r, domainadminauth.PermissionManageReviews)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodGet {
		imageID, action, parseErr := parseAdminImageReviewAction(r.URL.Path)
		if parseErr != nil {
			httpx.WriteError(w, r, parseErr)
			return
		}
		if action != "image" {
			httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "image review route not found"))
			return
		}
		result, content, err := a.tasks.DownloadImageResultForAdmin(r.Context(), imageID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.Header().Set("Content-Type", defaultString(result.MimeType, "application/octet-stream"))
		w.Header().Set("Content-Disposition", `inline; filename="`+imageDownloadFilename(result)+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
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
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
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
	orders, err := a.billing.ListOrders(r.Context(), domainbilling.ListOrdersRequest{Page: 1, PageSize: 1000})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	webhookEvents, err := a.billing.ListWebhookEvents(r.Context(), 1, 1000)
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
	preflightFailuresByCode := map[string]int{}
	preflightFailureCount := 0
	for _, item := range callRecords.Items {
		switch item.Status {
		case domainimagetask.StatusSucceeded, domainimagetask.StatusPartialFailed:
			successCount++
		case domainimagetask.StatusFailed, domainimagetask.StatusRejected:
			failedCount++
		}
		if item.ErrorCode != nil && isPreflightErrorCode(*item.ErrorCode) {
			preflightFailuresByCode[*item.ErrorCode]++
			preflightFailureCount++
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
	now := time.Now().UTC()
	activeUsers := 0
	closedUsers := 0
	disabledUsers := 0
	signupTrialGrantedUserCount := 0
	trialExpiringUserCount := 0
	for _, user := range users.Items {
		switch user.Status {
		case "active":
			activeUsers++
		case "closed":
			closedUsers++
		case "disabled":
			disabledUsers++
		}
		if balance, balanceErr := a.billing.GetBalance(r.Context(), user.ID, ""); balanceErr == nil {
			if hasTrialBalance(balance) {
				signupTrialGrantedUserCount++
			}
			if hasTrialExpiringBalance(balance) {
				trialExpiringUserCount++
			}
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
	todayOrderCount := 0
	todayCompletedCount := 0
	for _, order := range orders.Items {
		if !sameUTCDate(order.CreatedAt, now) {
			continue
		}
		todayOrderCount++
		if isPaidCashierOrder(order.Status) {
			todayCompletedCount++
		}
	}
	failedWebhookCount := 0
	for _, event := range webhookEvents.Items {
		if event.Status == "failed" {
			failedWebhookCount++
		}
	}
	refundCompensation := summarizeRefundCompensationFailures(webhookEvents.Items)
	paymentSuccessRate := "0.00%"
	if todayOrderCount > 0 {
		paymentSuccessRate = fmt.Sprintf("%.2f%%", float64(todayCompletedCount)*100/float64(todayOrderCount))
	}
	enabledMethods := make([]string, 0)
	for _, method := range a.cashierVisibleMethods(r.Context(), false) {
		enabledMethods = append(enabledMethods, method.Method)
	}
	mockEnabled := false
	for _, method := range enabledMethods {
		if method == "mock" {
			mockEnabled = true
			break
		}
	}
	ops := adminDashboardOperations{
		TodayOrderCount:                todayOrderCount,
		PaymentSuccessRate:             paymentSuccessRate,
		FailedWebhookCount:             failedWebhookCount,
		RefundCompensationFailedCount:  refundCompensation.Count,
		RefundCompensationOldestFailed: refundCompensation.OldestFailedAt,
		MockEnabled:                    mockEnabled,
		SignupTrialGrantedUserCount:    signupTrialGrantedUserCount,
		TrialExpiringUserCount:         trialExpiringUserCount,
		PreflightFailureCount:          preflightFailureCount,
		PreflightFailuresByErrorCode:   preflightFailuresByCode,
		PublicGalleryListViews:         observability.DefaultMetrics().PublicGalleryListViewsTotal(),
		PublicGalleryDetailLoginBlocks: observability.DefaultMetrics().PublicGalleryDetailLoginBlockTotal(),
		EnabledPaymentMethods:          enabledMethods,
		GeneratedAt:                    now,
	}

	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"operations": ops,
		"metrics": []map[string]any{
			{"key": "payment_success_rate", "label": "支付成功率", "value": ops.PaymentSuccessRate, "trend": fmt.Sprintf("今日 %d 单", ops.TodayOrderCount), "detail": "今日收银台订单支付完成率", "tone": dashboardMetricTone(ops.FailedWebhookCount == 0, ops.FailedWebhookCount > 0)},
			{"key": "failed_webhook_count", "label": "失败回调", "value": strconv.Itoa(ops.FailedWebhookCount), "trend": boolTrend(ops.FailedWebhookCount == 0, "正常", "需处理"), "detail": "当前失败支付回调事件数", "tone": dashboardMetricTone(ops.FailedWebhookCount == 0, ops.FailedWebhookCount > 0)},
			{"key": "refund_compensation_failures", "label": "退款补偿", "value": strconv.Itoa(ops.RefundCompensationFailedCount), "trend": refundCompensationTrend(refundCompensation), "detail": "真实渠道已退款但本地落账失败的待补偿事件", "tone": refundCompensationTone(refundCompensation.Count)},
			{"key": "signup_trial_users", "label": "体验额度用户", "value": strconv.Itoa(ops.SignupTrialGrantedUserCount), "trend": fmt.Sprintf("临期 %d", ops.TrialExpiringUserCount), "detail": "当前仍持有体验额度的用户数"},
			{"key": "preflight_failures", "label": "前置失败", "value": strconv.Itoa(ops.PreflightFailureCount), "trend": fmt.Sprintf("%d 类错误", len(ops.PreflightFailuresByErrorCode)), "detail": "最近调用中可归因的前置失败"},
			{"key": "public_gallery_views", "label": "广场访问", "value": strconv.FormatUint(ops.PublicGalleryListViews, 10), "trend": fmt.Sprintf("登录拦截 %d", ops.PublicGalleryDetailLoginBlocks), "detail": "公开广场列表访问和详情登录拦截"},
			{"key": "mock_payment", "label": "Mock 支付", "value": boolLabel(ops.MockEnabled, "Enabled", "Hidden"), "trend": strings.Join(ops.EnabledPaymentMethods, ","), "detail": "当前用户可见支付方式中的 Mock 状态", "tone": dashboardMetricTone(!ops.MockEnabled || !isProductionAppEnv(a.cfg.App.Env), ops.MockEnabled && isProductionAppEnv(a.cfg.App.Env))},
			{"key": "generation_success_rate", "label": "生图成功率", "value": fmt.Sprintf("%.2f%%", successRate), "trend": fmt.Sprintf("最近 %d 条", totalRecords), "detail": "最近调用记录成功率"},
			{"key": "avg_duration_sec", "label": "平均耗时", "value": fmt.Sprintf("%.2fs", avgDuration), "trend": fmt.Sprintf("%d 条完成记录", durationCount), "detail": "已完成任务平均执行时长"},
			{"key": "actual_points", "label": "积分消耗", "value": fmt.Sprintf("%.5f", totalActualPoints), "trend": fmt.Sprintf("最近 %d 条", totalRecords), "detail": "最近调用累计 actual_points"},
			{"key": "active_users", "label": "活跃用户", "value": strconv.Itoa(activeUsers), "trend": fmt.Sprintf("禁用 %d / 注销 %d", disabledUsers, closedUsers), "detail": "当前可登录并可使用产品的用户数"},
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

func (a *API) HandleAdminReadiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}

	checks, appErr := a.adminReadinessChecks(r.Context())
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	status, summary := summarizeReadinessChecks(checks)
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"status":         status,
		"overall_status": status,
		"generated_at":   time.Now().UTC(),
		"summary":        summary,
		"checks":         checks,
		"items":          checks,
	})
}

func (a *API) HandleAdminCashierOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	orders, err := a.billing.ListOrders(r.Context(), domainbilling.ListOrdersRequest{Page: 1, PageSize: 1000})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	events, err := a.billing.ListWebhookEvents(r.Context(), 1, 1000)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	now := time.Now().UTC()
	todayAmount := decimal.Zero
	todayOrderCount := 0
	todayCompletedCount := 0
	pendingCount := 0
	for _, order := range orders.Items {
		if order.Status == "pending" {
			pendingCount++
		}
		if !sameUTCDate(order.CreatedAt, now) {
			continue
		}
		todayOrderCount++
		if isPaidCashierOrder(order.Status) {
			todayCompletedCount++
			if amount, parseErr := decimal.NewFromString(strings.TrimSpace(order.AmountCNY)); parseErr == nil {
				todayAmount = todayAmount.Add(amount)
			}
		}
	}
	failedWebhookCount := 0
	for _, event := range events.Items {
		if event.Status == "failed" {
			failedWebhookCount++
		}
	}
	successRate := "0.00%"
	if todayOrderCount > 0 {
		successRate = fmt.Sprintf("%.2f%%", float64(todayCompletedCount)*100/float64(todayOrderCount))
	}
	enabledMethods := make([]string, 0)
	for _, method := range a.cashierVisibleMethods(r.Context(), false) {
		enabledMethods = append(enabledMethods, method.Method)
	}
	mockEnabled := false
	for _, method := range enabledMethods {
		if method == "mock" {
			mockEnabled = true
			break
		}
	}
	enabledProviderInstances := 0
	for _, instance := range a.cashierProviderInstances(r.Context()) {
		if instance.Enabled {
			enabledProviderInstances++
		}
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"today_order_count":          todayOrderCount,
		"today_completed_count":      todayCompletedCount,
		"today_amount_cny":           todayAmount.StringFixed(5),
		"success_rate":               successRate,
		"pending_count":              pendingCount,
		"failed_webhook_count":       failedWebhookCount,
		"enabled_methods":            enabledMethods,
		"enabled_provider_instances": enabledProviderInstances,
		"mock_enabled":               mockEnabled,
	})
}

func (a *API) HandleAdminCashierPlans(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodPost {
		var req domainbilling.CreateSubscriptionPlanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		plan, err := a.billing.CreatePlan(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.plan.create", "cashier_plan", fmt.Sprintf("%d", plan.ID), map[string]any{"plan_code": plan.PlanCode}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, cashierPlanPayload(plan))
		return
	}
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
	plans, err := a.billing.ListPlans(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	items := make([]map[string]any, 0, len(plans))
	for _, plan := range plans {
		items = append(items, cashierPlanPayload(plan))
	}
	httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(paginateAny(items, page, pageSize), page, pageSize, len(items)))
}

func (a *API) HandleAdminCashierPlanDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	planID, parseErr := parseAdminCashierPlanID(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	if r.Method == http.MethodDelete {
		plan, err := a.billing.DeletePlan(r.Context(), planID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.plan.delete", "cashier_plan", fmt.Sprintf("%d", plan.ID), map[string]any{"plan_code": plan.PlanCode, "status": plan.Status}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, cashierPlanPayload(plan))
		return
	}
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w, r)
		return
	}
	var req domainbilling.UpdateSubscriptionPlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	req.PlanID = planID
	plan, err := a.billing.UpdatePlan(r.Context(), req)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.plan.update", "cashier_plan", fmt.Sprintf("%d", plan.ID), map[string]any{"plan_code": plan.PlanCode}); auditErr != nil {
		httpx.WriteError(w, r, normalizeAppError(auditErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, cashierPlanPayload(plan))
}

func (a *API) HandleAdminCashierCustomAmountConfig(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		httpx.WriteSuccess(w, r, http.StatusOK, a.cashierCustomAmountConfig(r.Context()))
	case http.MethodPut:
		var req cashierCustomAmountConfig
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.cashierConfigFacade().UpdateCustomAmountConfig(r.Context(), req, admin.AdminID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.custom_amount_config.update", "cashier", "custom_amount_config", map[string]any{"enabled": updated.Enabled}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminCashierVisibleMethods(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": a.cashierVisibleMethods(r.Context(), true)})
	case http.MethodPut:
		var req struct {
			Items []cashierVisibleMethod `json:"items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		normalized, err := a.cashierConfigFacade().UpdateVisibleMethods(r.Context(), req.Items, admin.AdminID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.visible_methods.update", "cashier", "visible_methods", map[string]any{"count": len(normalized)}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": a.cashierVisibleMethods(r.Context(), true)})
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminCashierProviderInstances(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
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
		instances, err := a.cashierConfigFacade().ProviderInstances(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		items := cashierProviderInstancePayloads(instances)
		httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(paginateAny(items, page, pageSize), page, pageSize, len(items)))
	case http.MethodPost:
		req, ok := decodeCashierProviderInstanceRequest(w, r)
		if !ok {
			return
		}
		normalized, err := a.cashierConfigFacade().CreateProviderInstance(r.Context(), req, admin.AdminID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.provider.create", "payment_provider_instance", fmt.Sprintf("%d", normalized.ID), map[string]any{"provider_type": normalized.ProviderType}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, cashierProviderInstancePayload(normalized))
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminCashierProviderInstanceDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	instanceID, parseErr := parseAdminCashierProviderInstanceID(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	instances, err := a.cashierConfigFacade().ProviderInstances(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	index := -1
	for itemIndex, item := range instances {
		if item.ID == instanceID {
			index = itemIndex
			break
		}
	}
	if index < 0 {
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "payment provider instance not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		httpx.WriteSuccess(w, r, http.StatusOK, cashierProviderInstancePayload(instances[index]))
	case http.MethodPut:
		req, ok := decodeCashierProviderInstanceRequest(w, r)
		if !ok {
			return
		}
		normalized, err := a.cashierConfigFacade().UpdateProviderInstance(r.Context(), instanceID, req, admin.AdminID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.provider.update", "payment_provider_instance", fmt.Sprintf("%d", normalized.ID), map[string]any{"provider_type": normalized.ProviderType, "enabled": normalized.Enabled}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, cashierProviderInstancePayload(normalized))
	case http.MethodDelete:
		deleted, err := a.cashierConfigFacade().DeleteProviderInstance(r.Context(), instanceID, admin.AdminID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.provider.delete", "payment_provider_instance", fmt.Sprintf("%d", deleted.ID), map[string]any{"provider_type": deleted.ProviderType, "name": deleted.Name}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, cashierProviderInstancePayload(deleted))
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminCashierOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier); appErr != nil {
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
	userID, queryErr := parseOptionalInt64Query(r, "user_id")
	if queryErr != nil {
		httpx.WriteError(w, r, queryErr)
		return
	}
	result, err := a.billing.ListOrders(r.Context(), domainbilling.ListOrdersRequest{
		UserID:        userID,
		Status:        strings.TrimSpace(r.URL.Query().Get("status")),
		OrderNo:       strings.TrimSpace(r.URL.Query().Get("order_no")),
		VisibleMethod: strings.TrimSpace(r.URL.Query().Get("visible_method")),
		ProviderType:  strings.TrimSpace(r.URL.Query().Get("provider_type")),
		PurchaseType:  strings.TrimSpace(r.URL.Query().Get("purchase_type")),
		Page:          page,
		PageSize:      pageSize,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
}

func (a *API) HandleAdminCashierOrderDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	orderID, action, parseErr := parseAdminCashierOrderPath(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch {
	case r.Method == http.MethodGet && action == "":
		result, err := a.billing.GetOrderForAdmin(r.Context(), orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case r.Method == http.MethodPost && action == "complete":
		var req struct {
			Provider string `json:"provider"`
			TradeNo  string `json:"trade_no"`
			Reason   string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		order, err := a.billing.GetOrderForAdmin(r.Context(), orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		provider := strings.ToLower(strings.TrimSpace(req.Provider))
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(order.ProviderType))
		}
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(order.Provider))
		}
		if provider == "" {
			provider = strings.ToLower(strings.TrimSpace(order.VisibleMethod))
		}
		if provider == "" {
			provider = "manual"
		}
		tradeNo := strings.TrimSpace(req.TradeNo)
		if tradeNo == "" {
			httpx.WriteError(w, r, errs.BadRequest("trade_no is required"))
			return
		}
		result, err := a.billing.CompleteRechargeOrder(r.Context(), domainbilling.CompleteRechargeOrderRequest{
			UserID:   order.UserID,
			OrderID:  orderID,
			Provider: provider,
			TradeNo:  tradeNo,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.order.manual_complete", "payment_order", fmt.Sprintf("%d", result.ID), map[string]any{"order_no": result.OrderNo, "provider": provider, "trade_no": tradeNo, "reason": strings.TrimSpace(req.Reason)}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case r.Method == http.MethodPost && action == "close":
		var req struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		order, err := a.billing.GetOrderForAdmin(r.Context(), orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		result, err := a.billing.CancelOrder(r.Context(), order.UserID, orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.order.close", "payment_order", fmt.Sprintf("%d", result.ID), map[string]any{"order_no": result.OrderNo, "reason": strings.TrimSpace(req.Reason)}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case r.Method == http.MethodPost && action == "refund":
		var req struct {
			RefundTradeNo   string `json:"refund_trade_no"`
			RefundAmountCNY string `json:"refund_amount_cny"`
			Reason          string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		order, err := a.billing.GetOrderForAdmin(r.Context(), orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		refundTradeNo := strings.TrimSpace(req.RefundTradeNo)
		refundAmountCNY := strings.TrimSpace(req.RefundAmountCNY)
		if refundTradeNo == "" {
			httpx.WriteError(w, r, errs.BadRequest("refund_trade_no is required"))
			return
		}
		checkResult, err := a.billing.CheckRefundPaymentOrder(r.Context(), domainbilling.RefundPaymentOrderRequest{
			UserID:          order.UserID,
			OrderID:         orderID,
			RefundTradeNo:   refundTradeNo,
			RefundAmountCNY: refundAmountCNY,
			Reason:          strings.TrimSpace(req.Reason),
			OperatorAdminID: admin.AdminID,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		refundReq := domainbilling.RefundPaymentOrderRequest{
			UserID:          order.UserID,
			OrderID:         orderID,
			RefundTradeNo:   refundTradeNo,
			RefundAmountCNY: refundAmountCNY,
			Reason:          strings.TrimSpace(req.Reason),
			OperatorAdminID: admin.AdminID,
		}
		if strings.TrimSpace(checkResult.RefundTradeNo) == refundTradeNo && (checkResult.Status == "refunded" || checkResult.Status == "partially_refunded") {
			result, err := a.billing.RefundPaymentOrder(r.Context(), refundReq)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			auditMetadata := map[string]any{"order_no": result.OrderNo, "refund_trade_no": refundTradeNo, "refund_amount_cny": refundAmountCNY, "refunded_amount_cny": result.RefundedAmountCNY, "refunded_points": result.RefundedPoints, "reason": strings.TrimSpace(req.Reason), "idempotent": true}
			if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.order.refund", "payment_order", fmt.Sprintf("%d", result.ID), auditMetadata); auditErr != nil {
				httpx.WriteError(w, r, normalizeAppError(auditErr))
				return
			}
			httpx.WriteSuccess(w, r, http.StatusOK, result)
			return
		}
		if _, err := a.billing.FreezeRefundPaymentOrder(r.Context(), refundReq); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		providerRefundAmountCNY, amountErr := cashierRefundAmountCNYForProvider(order, refundAmountCNY)
		if amountErr != nil {
			if _, releaseErr := a.billing.ReleaseRefundPaymentOrder(r.Context(), refundReq); releaseErr != nil {
				httpx.WriteError(w, r, normalizeAppError(releaseErr))
				return
			}
			httpx.WriteError(w, r, amountErr)
			return
		}
		channelRefund, channelErr := a.refundCashierOrderWithProvider(r.Context(), order, refundTradeNo, providerRefundAmountCNY, strings.TrimSpace(req.Reason))
		if channelErr != nil {
			if _, releaseErr := a.billing.ReleaseRefundPaymentOrder(r.Context(), refundReq); releaseErr != nil {
				httpx.WriteError(w, r, normalizeAppError(releaseErr))
				return
			}
			httpx.WriteError(w, r, channelErr)
			return
		}
		result, err := a.billing.RefundPaymentOrder(r.Context(), refundReq)
		if err != nil {
			if channelRefund != nil {
				if _, recordErr := a.billing.RecordRefundFinalizeFailure(r.Context(), billingservice.RefundFinalizeFailureRequest{
					RefundPaymentOrderRequest: refundReq,
					FailureReason:             err.Error(),
				}); recordErr != nil {
					httpx.WriteError(w, r, normalizeAppError(recordErr))
					return
				}
			}
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		auditMetadata := map[string]any{"order_no": result.OrderNo, "refund_trade_no": refundTradeNo, "refund_amount_cny": refundAmountCNY, "refunded_amount_cny": result.RefundedAmountCNY, "refunded_points": result.RefundedPoints, "reason": strings.TrimSpace(req.Reason)}
		if channelRefund != nil {
			auditMetadata["provider_type"] = channelRefund.ProviderType
			auditMetadata["provider_instance_id"] = channelRefund.ProviderInstanceID
			auditMetadata["channel_refund_status"] = channelRefund.RefundStatus
			auditMetadata["channel_refund_no"] = channelRefund.ChannelRefundNo
			auditMetadata["channel_refund_message"] = channelRefund.Message
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.order.refund", "payment_order", fmt.Sprintf("%d", result.ID), auditMetadata); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	case r.Method == http.MethodPost && action == "chargeback":
		idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
		if idempotencyKey == "" {
			httpx.WriteError(w, r, errs.BadRequest("Idempotency-Key is required"))
			return
		}
		var req struct {
			ChargePoints string `json:"charge_points"`
			Reason       string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		order, err := a.billing.GetOrderForAdmin(r.Context(), orderID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		chargePoints, parseErr := decimal.NewFromString(strings.TrimSpace(req.ChargePoints))
		if parseErr != nil || !chargePoints.IsPositive() {
			httpx.WriteError(w, r, errs.BadRequest("charge_points must be positive"))
			return
		}
		reason := strings.TrimSpace(req.Reason)
		if reason == "" {
			httpx.WriteError(w, r, errs.BadRequest("reason is required"))
			return
		}
		if len(reason) > 255 {
			httpx.WriteError(w, r, errs.BadRequest("reason must be at most 255 characters"))
			return
		}
		changePoints := chargePoints.Round(5).Neg().StringFixed(5)
		auditReason := fmt.Sprintf("cashier chargeback order %s: %s", order.OrderNo, reason)
		balance, err := a.billing.AdminAdjust(r.Context(), domainbilling.AdjustRequest{
			UserID:          order.UserID,
			ChangePoints:    changePoints,
			Reason:          auditReason,
			OperatorAdminID: admin.AdminID,
			IdempotencyKey:  idempotencyKey,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		updatedOrder, err := a.billing.RecordChargebackSummary(r.Context(), billingservice.ChargebackSummaryStoreRequest{
			OrderID:        order.ID,
			ChargePoints:   chargePoints.Round(5).StringFixed(5),
			Reason:         reason,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.order.chargeback", "payment_order", fmt.Sprintf("%d", updatedOrder.ID), map[string]any{"order_no": updatedOrder.OrderNo, "charge_points": chargePoints.Round(5).StringFixed(5), "change_points": changePoints, "reason": reason, "idempotency_key": idempotencyKey}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"order": updatedOrder, "balance": balance})
	case r.Method == http.MethodPost && action == "sync":
		result, syncErr := a.syncAdminCashierOrder(r.Context(), orderID)
		if syncErr != nil {
			httpx.WriteError(w, r, syncErr)
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.order.sync", "payment_order", fmt.Sprintf("%d", result.Order.ID), map[string]any{"order_no": result.Order.OrderNo, "provider_type": result.Sync.ProviderType, "query_status": result.Sync.QueryStatus, "completed": result.Sync.Completed, "trade_no": result.Sync.TradeNo}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminCashierWebhookEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier); appErr != nil {
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
	result, err := a.billing.ListWebhookEvents(r.Context(), page, pageSize)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
}

func (a *API) syncAdminCashierOrder(ctx context.Context, orderID int64) (adminCashierOrderSyncResponse, *errs.Error) {
	order, err := a.billing.GetOrderForAdmin(ctx, orderID)
	if err != nil {
		return adminCashierOrderSyncResponse{}, normalizeAppError(err)
	}
	instance, ok := a.cashierProviderInstanceForOrder(ctx, order)
	if !ok {
		return adminCashierOrderSyncResponse{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	syncResult, syncErr := a.queryCashierOrderStatus(ctx, order, instance)
	if syncErr != nil {
		return adminCashierOrderSyncResponse{}, syncErr
	}
	if syncResult.Paid && order.Status == "pending" {
		if !cashierSyncAmountMatches(order.AmountCNY, syncResult.AmountCNY) {
			return adminCashierOrderSyncResponse{}, errs.New(http.StatusConflict, errs.CodePaymentAmountMismatch, "payment amount does not match order")
		}
		tradeNo := strings.TrimSpace(syncResult.TradeNo)
		if tradeNo == "" {
			tradeNo = fmt.Sprintf("SYNC-%s-%d", order.OrderNo, time.Now().UTC().UnixNano())
			syncResult.TradeNo = tradeNo
		}
		completed, completeErr := a.billing.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
			UserID:   order.UserID,
			OrderID:  order.ID,
			Provider: syncResult.ProviderType,
			TradeNo:  tradeNo,
		})
		if completeErr != nil {
			return adminCashierOrderSyncResponse{}, normalizeAppError(completeErr)
		}
		order = completed
		syncResult.Completed = true
	} else if order.Status == "completed" {
		syncResult.Paid = true
		if strings.TrimSpace(syncResult.TradeNo) == "" {
			syncResult.TradeNo = order.TradeNo
		}
	}
	return adminCashierOrderSyncResponse{Order: order, Sync: syncResult}, nil
}

func (a *API) refundCashierOrderWithProvider(ctx context.Context, order domainbilling.PaymentOrder, refundTradeNo string, refundAmountCNY string, reason string) (*cashierProviderRefundResult, *errs.Error) {
	if !cashierservice.RefundRequiresProvider(cashierOrderSnapshot(order), cashierProviderInstance{ProviderType: cashierOrderProviderType(order, cashierProviderInstance{})}) {
		return nil, nil
	}
	instance, ok := a.cashierProviderInstanceForOrder(ctx, order)
	if !ok {
		return nil, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	registry := cashierservice.NewRefundAdapterRegistryWithBuilders(cashierservice.StandardRefundProviderBuilders())
	result, shouldCall, err := registry.RefundPayment(ctx, cashierservice.RefundPaymentRequest{
		Order:           cashierOrderSnapshot(order),
		Instance:        instance,
		RefundTradeNo:   refundTradeNo,
		RefundAmountCNY: refundAmountCNY,
		Reason:          reason,
	})
	if err != nil {
		return nil, normalizeAppError(err)
	}
	if !shouldCall {
		return nil, nil
	}
	return &result, nil
}

func (a *API) cashierProviderInstanceForOrder(ctx context.Context, order domainbilling.PaymentOrder) (cashierProviderInstance, bool) {
	providerInstanceID := order.ProviderInstanceID
	providerType := strings.ToLower(strings.TrimSpace(order.ProviderType))
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(order.Provider))
	}
	for _, instance := range a.cashierProviderInstances(ctx) {
		if providerInstanceID > 0 && instance.ID == providerInstanceID {
			return instance, true
		}
		if providerInstanceID == 0 && providerType != "" && strings.ToLower(strings.TrimSpace(instance.ProviderType)) == providerType {
			return instance, true
		}
	}
	if providerType == "mock" && !isProductionAppEnv(a.cfg.App.Env) {
		return cashierservice.DefaultMockProviderInstance(time.Now().UTC()), true
	}
	return cashierProviderInstance{}, false
}

func cashierOrderProviderType(order domainbilling.PaymentOrder, instance cashierProviderInstance) string {
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(order.ProviderType))
	}
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(order.Provider))
	}
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(order.VisibleMethod))
	}
	return providerType
}

func cashierOrderSnapshot(order domainbilling.PaymentOrder) cashierservice.OrderSnapshot {
	return cashierservice.OrderSnapshot{
		OrderNo:   order.OrderNo,
		AmountCNY: order.AmountCNY,
		TradeNo:   order.TradeNo,
		Status:    order.Status,
	}
}

func (a *API) queryCashierOrderStatus(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	registry := cashierservice.NewQueryAdapterRegistryWithBuilders(cashierservice.StandardQueryProviderBuilders())
	result, err := registry.QueryOrderStatus(ctx, cashierservice.QueryOrderStatusRequest{
		Order:    cashierOrderSnapshot(order),
		Instance: instance,
	})
	if err != nil {
		return adminCashierOrderSyncResult{}, normalizeAppError(err)
	}
	return result, nil
}

func (a *API) queryCashierOrderStatusFromConfig(order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	return cashierservice.ConfigDrivenQueryOrderStatus(cashierservice.QueryOrderStatusRequest{
		Order:    cashierOrderSnapshot(order),
		Instance: instance,
	}), nil
}

func cashierSyncAmountMatches(orderAmountCNY, syncAmountCNY string) bool {
	syncAmountCNY = strings.TrimSpace(syncAmountCNY)
	if syncAmountCNY == "" {
		return true
	}
	orderAmount, orderErr := decimal.NewFromString(strings.TrimSpace(orderAmountCNY))
	syncAmount, syncErr := decimal.NewFromString(syncAmountCNY)
	return orderErr == nil && syncErr == nil && orderAmount.Round(5).Equal(syncAmount.Round(5))
}

func cashierRefundAmountCNYForProvider(order domainbilling.PaymentOrder, requestedAmountCNY string) (string, *errs.Error) {
	totalAmount, totalErr := decimal.NewFromString(strings.TrimSpace(order.AmountCNY))
	if totalErr != nil || !totalAmount.IsPositive() {
		return "", errs.Internal("payment order amount is invalid")
	}
	refundedAmount := decimal.Zero
	if strings.TrimSpace(order.RefundedAmountCNY) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(order.RefundedAmountCNY))
		if parseErr != nil {
			return "", errs.Internal("payment order refunded amount is invalid")
		}
		refundedAmount = parsed.Round(5)
	}
	remainingAmount := totalAmount.Sub(refundedAmount).Round(5)
	if !remainingAmount.IsPositive() {
		return "", errs.New(http.StatusConflict, errs.CodeConflict, "payment order has no refundable amount")
	}
	refundAmount := remainingAmount
	if strings.TrimSpace(requestedAmountCNY) != "" {
		parsed, parseErr := decimal.NewFromString(strings.TrimSpace(requestedAmountCNY))
		if parseErr != nil || !parsed.IsPositive() {
			return "", errs.BadRequest("refund_amount_cny must be positive")
		}
		refundAmount = parsed.Round(5)
	}
	if refundAmount.GreaterThan(remainingAmount) {
		return "", errs.New(http.StatusConflict, errs.CodeConflict, "refund amount exceeds refundable amount")
	}
	return refundAmount.StringFixed(5), nil
}

func (a *API) HandleAdminCashierWebhookEventDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageCashier)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	eventID, action, parseErr := parseAdminCashierWebhookEventPath(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch {
	case r.Method == http.MethodPost && action == "retry":
		result, err := a.billing.RetryWebhookEvent(r.Context(), eventID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.webhook.retry", "payment_webhook_event", fmt.Sprintf("%d", result.ID), map[string]any{"order_id": result.OrderID, "provider_type": result.ProviderType}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminConfigTabs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageConfig); appErr != nil {
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
	tabKey := strings.TrimPrefix(r.URL.Path, "/api/ops/admin/v1/config-tabs/")
	admin, appErr := a.requireAdminPermission(r, configUpdatePermission(tabKey))
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
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

func (a *API) HandleAdminSecuritySMTP(w http.ResponseWriter, r *http.Request) {
	if a.secureCfg == nil {
		httpx.WriteError(w, r, errs.Internal("secure config service is not available"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		cfg, err := a.secureCfg.GetSMTPConfig(r.Context())
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, cfg)
	case http.MethodPut:
		admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		var req domainsecureconfig.UpdateSMTPConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UpdatedBy = admin.AdminID
		updated, err := a.secureCfg.UpdateSMTPConfig(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "security.smtp.update", "secure_config", "smtp/default", map[string]any{"enabled": updated.Enabled, "secret_fields": updated.SecretStatus.SecretFields, "fingerprint": updated.SecretStatus.Fingerprint}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminSecuritySMTPTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if a.secureCfg == nil {
		httpx.WriteError(w, r, errs.Internal("secure config service is not available"))
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req domainsecureconfig.SMTPTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	recipient := strings.TrimSpace(strings.ToLower(req.Email))
	if recipient == "" || !strings.Contains(recipient, "@") {
		httpx.WriteError(w, r, errs.BadRequest("email is required"))
		return
	}
	cfg, ok, err := a.secureCfg.ResolveSMTPConfig(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if !ok {
		httpx.WriteError(w, r, errs.Internal("smtp is not configured or disabled"))
		return
	}
	scene := strings.TrimSpace(req.Scene)
	if scene == "" {
		scene = "smtp_test"
	}
	if err := authservice.NewSMTPEmailSender(cfg).SendVerificationCode(recipient, scene, "000000"); err != nil {
		httpx.WriteError(w, r, errs.Internal("failed to send smtp test email"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "sent", "recipient": recipient})
}

func (a *API) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly)
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
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionViewAudit); appErr != nil {
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

func (a *API) HandleAdminSystemUsers(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageAdmins)
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
		result, err := a.adminAuth.ListAdmins(r.Context(), domainadminauth.AdminListRequest{
			Page:     page,
			PageSize: pageSize,
			Query:    r.URL.Query().Get("query"),
			Role:     r.URL.Query().Get("role"),
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
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Role     string `json:"role"`
			Status   string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		created, err := a.adminAuth.CreateAdmin(r.Context(), domainadminauth.AdminCreateRequest{Email: req.Email, Password: req.Password, Role: req.Role, Status: req.Status})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "admin_user.create", "admin_user", fmt.Sprintf("%d", created.ID), map[string]any{"email": created.Email, "role": created.Role, "status": created.Status}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		httpx.WriteError(w, r, methodNotAllowedError())
	}
}

func (a *API) HandleAdminSystemUserDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageAdmins)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	adminID, action, parseErr := parseAdminSystemUserAction(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
		return
	}
	switch action {
	case "":
		switch r.Method {
		case http.MethodPut:
			var req struct {
				Role   *string `json:"role"`
				Status *string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
				return
			}
			role := ""
			if req.Role != nil {
				role = *req.Role
			}
			status := ""
			if req.Status != nil {
				status = *req.Status
			}
			updated, err := a.adminAuth.UpdateAdmin(r.Context(), domainadminauth.AdminUpdateRequest{
				AdminID:        adminID,
				OperatorID:     admin.AdminID,
				Role:           role,
				Status:         status,
				RoleProvided:   req.Role != nil,
				StatusProvided: req.Status != nil,
			})
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "admin_user.update", "admin_user", fmt.Sprintf("%d", updated.ID), map[string]any{"email": updated.Email, "role": updated.Role, "status": updated.Status}); auditErr != nil {
				httpx.WriteError(w, r, normalizeAppError(auditErr))
				return
			}
			httpx.WriteSuccess(w, r, http.StatusOK, updated)
		case http.MethodDelete:
			deleted, err := a.adminAuth.DeleteAdmin(r.Context(), domainadminauth.AdminDeleteRequest{AdminID: adminID, OperatorID: admin.AdminID})
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "admin_user.delete", "admin_user", fmt.Sprintf("%d", deleted.ID), map[string]any{"email": deleted.Email, "role": deleted.Role, "status": deleted.Status}); auditErr != nil {
				httpx.WriteError(w, r, normalizeAppError(auditErr))
				return
			}
			httpx.WriteSuccess(w, r, http.StatusOK, deleted)
		default:
			httpx.WriteError(w, r, methodNotAllowedError())
		}
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
		if err := a.adminAuth.ResetAdminPassword(r.Context(), domainadminauth.AdminPasswordResetRequest{AdminID: adminID, NewPassword: req.NewPassword}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "admin_user.reset_password", "admin_user", fmt.Sprintf("%d", adminID), nil); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "password_reset"})
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "admin user route not found"))
	}
}

func (a *API) HandleAdminUsers(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageUsers)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method == http.MethodPost {
		var req struct {
			Email            string `json:"email"`
			Nickname         string `json:"nickname"`
			Status           string `json:"status"`
			UserGroupCode    string `json:"user_group_code"`
			Password         string `json:"password"`
			RPMLimit         int    `json:"rpm_limit"`
			ConcurrencyLimit int    `json:"concurrency_limit"`
			DefaultLocale    string `json:"default_locale"`
			Theme            string `json:"theme"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		created, err := a.adminUser.CreateUser(r.Context(), domainadminuser.CreateUserRequest{
			Email:            req.Email,
			Nickname:         req.Nickname,
			Status:           req.Status,
			UserGroupCode:    req.UserGroupCode,
			RPMLimit:         req.RPMLimit,
			ConcurrencyLimit: req.ConcurrencyLimit,
			DefaultLocale:    req.DefaultLocale,
			Theme:            req.Theme,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if strings.TrimSpace(req.Password) != "" {
			if _, err := a.auth.SetPassword(created.ID, req.Password); err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			refreshed, err := a.adminUser.GetUserDetail(r.Context(), created.ID)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			created = refreshed.User
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.create", "user", fmt.Sprintf("%d", created.ID), map[string]any{"email": created.Email, "status": created.Status, "user_group_code": created.UserGroupCode}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
		return
	}
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
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
		Page:      page,
		PageSize:  pageSize,
		Query:     r.URL.Query().Get("query"),
		Status:    r.URL.Query().Get("status"),
		GroupCode: defaultString(r.URL.Query().Get("group"), r.URL.Query().Get("group_code")),
		SortBy:    r.URL.Query().Get("sort_by"),
		SortDir:   r.URL.Query().Get("sort_dir"),
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageUsers)
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
		if r.Method == http.MethodDelete {
			deleted, err := a.adminUser.DeleteUser(r.Context(), userID)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			_ = a.auth.RevokeUserSessions(userID)
			if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.delete", "user", fmt.Sprintf("%d", userID), map[string]any{"email": deleted.Email, "token_version": deleted.TokenVersion}); auditErr != nil {
				httpx.WriteError(w, r, normalizeAppError(auditErr))
				return
			}
			httpx.WriteSuccess(w, r, http.StatusOK, deleted)
			return
		}
		if r.Method != http.MethodGet {
			httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
			return
		}
		detail, err := a.adminUser.GetUserDetail(r.Context(), userID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		payload, enrichErr := a.adminUserDetailPayload(r.Context(), userID, detail)
		if enrichErr != nil {
			httpx.WriteError(w, r, enrichErr)
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, payload)
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
	case "groups":
		if r.Method != http.MethodPut && r.Method != http.MethodPost {
			httpx.WriteError(w, r, methodNotAllowedError())
			return
		}
		var req struct {
			GroupIDs []int64 `json:"group_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		updated, err := a.adminUser.AssignUserGroups(r.Context(), domainadminuser.MultiGroupAssignmentRequest{UserID: userID, GroupIDs: req.GroupIDs})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "user.groups_update", "user", fmt.Sprintf("%d", userID), map[string]any{"group_ids": req.GroupIDs}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "admin user route not found"))
	}
}

func (a *API) HandleAdminUserGroups(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageUsers); appErr != nil {
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
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageUsers); appErr != nil {
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageBilling)
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageBilling)
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

func (a *API) HandleAdminRedeemCodeExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageBilling)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	var req struct {
		Status  string `json:"status"`
		Code    string `json:"code"`
		BatchID int64  `json:"batch_id"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
	}
	if req.BatchID < 0 {
		httpx.WriteError(w, r, errs.BadRequest("invalid batch_id"))
		return
	}
	status := strings.TrimSpace(req.Status)
	code := strings.TrimSpace(req.Code)
	result, err := a.redeem.ListCodes(r.Context(), domainredeem.ListRequest{
		Page:     1,
		PageSize: 1000,
		Status:   status,
		Code:     code,
		BatchID:  req.BatchID,
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	filters := map[string]any{
		"status":   status,
		"code":     code,
		"batch_id": req.BatchID,
	}
	if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "redeem.export", "redeem_code", "export", map[string]any{"count": len(result.Items), "filters": filters, "status": status, "code": code, "batch_id": req.BatchID}); auditErr != nil {
		httpx.WriteError(w, r, normalizeAppError(auditErr))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items":   result.Items,
		"count":   len(result.Items),
		"filters": filters,
	})
}

func (a *API) HandleAdminRedeemCodeDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageBilling)
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
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionReadOnly); appErr != nil {
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
		ErrorCode:     r.URL.Query().Get("error_code"),
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

func (a *API) HandleAdminModelAccounts(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, _ := parsePositiveIntQuery(r, "page", 1)
		pageSize, _ := parsePositiveIntQuery(r, "page_size", 20)
		result, err := a.modelAdmin.ListModelAccounts(r.Context(), domainmodeladmin.ModelAccountListRequest{Page: page, PageSize: pageSize, AdapterType: r.URL.Query().Get("adapter_type"), AuthType: r.URL.Query().Get("auth_type"), Status: r.URL.Query().Get("status"), Keyword: r.URL.Query().Get("keyword")})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := decodeModelAccountWriteRequest(w, r)
		if !ok {
			return
		}
		created, err := a.modelAdmin.CreateModelAccount(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_account.create", "model_account", fmt.Sprintf("%d", created.ID), map[string]any{"adapter_type": created.AdapterType, "status": created.Status}); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminModelAccountDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/model-accounts/")
	accountID, err := parseInt64Part(parts, 0, "account_id")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(parts) >= 2 && parts[1] == "models" {
		a.handleAdminModelAccountModels(w, r, admin.AdminID, accountID, parts)
		return
	}
	if len(parts) == 2 && parts[1] == "test-image" {
		a.handleAdminModelAccountTestImage(w, r, admin.AdminID, accountID)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, ok := decodeModelAccountWriteRequest(w, r)
		if !ok {
			return
		}
		updated, updateErr := a.modelAdmin.UpdateModelAccount(r.Context(), accountID, req)
		if updateErr != nil {
			httpx.WriteError(w, r, normalizeAppError(updateErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_account.update", "model_account", fmt.Sprintf("%d", updated.ID), map[string]any{"adapter_type": updated.AdapterType, "status": updated.Status})
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if deleteErr := a.modelAdmin.DeleteModelAccount(r.Context(), accountID); deleteErr != nil {
			httpx.WriteError(w, r, normalizeAppError(deleteErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "model_account.delete", "model_account", fmt.Sprintf("%d", accountID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) handleAdminModelAccountTestImage(w http.ResponseWriter, r *http.Request, adminID, accountID int64) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	req, ok := decodeModelAccountTestImageRequest(w, r)
	if !ok {
		return
	}
	account, err := a.modelAdmin.GetModelAccount(r.Context(), accountID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	model, err := a.modelAdmin.GetModelAccountModel(r.Context(), req.ModelID)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if model.AccountID != accountID {
		httpx.WriteError(w, r, errs.New(http.StatusBadRequest, errs.CodeBadRequest, "model does not belong to account"))
		return
	}
	if !model.Enabled {
		httpx.WriteError(w, r, errs.New(http.StatusConflict, errs.CodeImageCapabilityMismatch, "model is disabled"))
		return
	}
	candidate := domainmodelhub.ProviderCandidate{
		AccountModelID:     model.ID,
		ModelAccountID:     account.ID,
		Provider:           account.AdapterType,
		AdapterType:        account.AdapterType,
		AuthType:           account.AuthType,
		BaseURL:            account.BaseURL,
		Credentials:        account.CredentialsEncrypted,
		ModelCode:          model.ModelCode,
		SupportedTaskTypes: append([]string(nil), model.TaskTypes...),
		SupportedQualities: append([]string(nil), model.Qualities...),
		HealthStatus:       account.Status,
		InputCost:          model.CostPerImage,
		OutputCost:         model.CostPerImage,
		Currency:           model.Currency,
		AccountExtra:       cloneMapAny(account.Extra),
		ModelExtra:         cloneMapAny(model.Extra),
	}
	result, testErr := a.tasks.TestModelAccount(r.Context(), domainimagetask.TestModelAccountRequest{
		AccountID:  accountID,
		ModelID:    model.ID,
		ModelCode:  model.ModelCode,
		Prompt:     req.Prompt,
		SourceMode: req.SourceMode,
	}, candidate)
	if testErr != nil {
		httpx.WriteError(w, r, normalizeAppError(testErr))
		return
	}
	if result.Image.ID != "" {
		result.ImageURL = "/api/ops/admin/v1/image-reviews/" + result.Image.ID + "/image"
	}
	if err := a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "model_account.test_image", "model_account", fmt.Sprintf("%d", accountID), map[string]any{"model_id": model.ID, "model_code": model.ModelCode, "status": result.Status}); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, result)
}

func (a *API) handleAdminModelAccountModels(w http.ResponseWriter, r *http.Request, adminID, accountID int64, parts []string) {
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			result, err := a.modelAdmin.ListModelAccountModels(r.Context(), domainmodeladmin.ModelAccountModelListRequest{Page: 1, PageSize: 100, AccountID: accountID})
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
		case http.MethodPost:
			req, ok := decodeModelAccountModelWriteRequest(w, r, accountID)
			if !ok {
				return
			}
			created, err := a.modelAdmin.CreateModelAccountModel(r.Context(), req)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "model_account_model.create", "model_account_model", fmt.Sprintf("%d", created.ID), map[string]any{"account_id": accountID, "model_code": created.ModelCode})
			httpx.WriteSuccess(w, r, http.StatusCreated, created)
		default:
			writeMethodNotAllowed(w, r)
		}
		return
	}
	modelID, appErr := parseInt64Part(parts, 2, "model_id")
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, ok := decodeModelAccountModelWriteRequest(w, r, accountID)
		if !ok {
			return
		}
		updated, err := a.modelAdmin.UpdateModelAccountModel(r.Context(), modelID, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "model_account_model.update", "model_account_model", fmt.Sprintf("%d", updated.ID), map[string]any{"account_id": accountID, "model_code": updated.ModelCode})
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.modelAdmin.DeleteModelAccountModel(r.Context(), modelID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "model_account_model.delete", "model_account_model", fmt.Sprintf("%d", modelID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminRouteModels(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, _ := parsePositiveIntQuery(r, "page", 1)
		pageSize, _ := parsePositiveIntQuery(r, "page_size", 20)
		enabled, parseErr := parseOptionalBoolQuery(r, "enabled")
		if parseErr != nil {
			httpx.WriteError(w, r, parseErr)
			return
		}
		result, err := a.modelAdmin.ListRouteModels(r.Context(), domainmodeladmin.RouteModelListRequest{Page: page, PageSize: pageSize, Visibility: r.URL.Query().Get("visibility"), Enabled: enabled, Keyword: r.URL.Query().Get("keyword")})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := decodeRouteModelWriteRequest(w, r)
		if !ok {
			return
		}
		created, err := a.modelAdmin.CreateRouteModel(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "route_model.create", "route_model", fmt.Sprintf("%d", created.ID), map[string]any{"code": created.Code, "visibility": created.Visibility})
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminRouteModelDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/route-models/")
	routeModelID, err := parseInt64Part(parts, 0, "route_model_id")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(parts) >= 2 && parts[1] == "candidates" {
		a.handleAdminRouteModelCandidates(w, r, admin.AdminID, routeModelID, parts)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, ok := decodeRouteModelWriteRequest(w, r)
		if !ok {
			return
		}
		updated, updateErr := a.modelAdmin.UpdateRouteModel(r.Context(), routeModelID, req)
		if updateErr != nil {
			httpx.WriteError(w, r, normalizeAppError(updateErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "route_model.update", "route_model", fmt.Sprintf("%d", updated.ID), map[string]any{"code": updated.Code, "visibility": updated.Visibility})
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if deleteErr := a.modelAdmin.DeleteRouteModel(r.Context(), routeModelID); deleteErr != nil {
			httpx.WriteError(w, r, normalizeAppError(deleteErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "route_model.delete", "route_model", fmt.Sprintf("%d", routeModelID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) handleAdminRouteModelCandidates(w http.ResponseWriter, r *http.Request, adminID, routeModelID int64, parts []string) {
	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			items, err := a.modelAdmin.ListRouteModelCandidates(r.Context(), routeModelID)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(items, 1, len(items), len(items)))
		case http.MethodPost:
			req, ok := decodeRouteModelCandidateWriteRequest(w, r, routeModelID)
			if !ok {
				return
			}
			created, err := a.modelAdmin.CreateRouteModelCandidate(r.Context(), req)
			if err != nil {
				httpx.WriteError(w, r, normalizeAppError(err))
				return
			}
			a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "route_model_candidate.create", "route_model_candidate", fmt.Sprintf("%d", created.ID), map[string]any{"route_model_id": routeModelID, "account_model_id": created.AccountModelID})
			httpx.WriteSuccess(w, r, http.StatusCreated, created)
		default:
			writeMethodNotAllowed(w, r)
		}
		return
	}
	candidateID, appErr := parseInt64Part(parts, 2, "candidate_id")
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, ok := decodeRouteModelCandidateWriteRequest(w, r, routeModelID)
		if !ok {
			return
		}
		updated, err := a.modelAdmin.UpdateRouteModelCandidate(r.Context(), candidateID, req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "route_model_candidate.update", "route_model_candidate", fmt.Sprintf("%d", updated.ID), nil)
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.modelAdmin.DeleteRouteModelCandidate(r.Context(), candidateID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", adminID), "route_model_candidate.delete", "route_model_candidate", fmt.Sprintf("%d", candidateID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminRouteModelPrices(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		page, _ := parsePositiveIntQuery(r, "page", 1)
		pageSize, _ := parsePositiveIntQuery(r, "page_size", 20)
		routeModelID, _ := parseOptionalInt64Query(r, "route_model_id")
		result, err := a.modelAdmin.ListRouteModelPrices(r.Context(), domainmodeladmin.RouteModelPriceListRequest{Page: page, PageSize: pageSize, RouteModelID: routeModelID, TaskType: r.URL.Query().Get("task_type"), Quality: r.URL.Query().Get("quality")})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(result.Items, result.Page, result.PageSize, result.Total))
	case http.MethodPost:
		req, ok := decodeRouteModelPriceWriteRequest(w, r)
		if !ok {
			return
		}
		created, err := a.modelAdmin.CreateRouteModelPrice(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "route_model_price.create", "route_model_price", fmt.Sprintf("%d", created.ID), map[string]any{"route_model_id": created.RouteModelID, "task_type": created.TaskType, "quality": created.Quality})
		httpx.WriteSuccess(w, r, http.StatusCreated, created)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminRouteModelPriceDetail(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	parts := splitAdminSuffix(r.URL.Path, "/api/ops/admin/v1/route-model-prices/")
	priceID, err := parseInt64Part(parts, 0, "price_id")
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	switch r.Method {
	case http.MethodPut:
		req, ok := decodeRouteModelPriceWriteRequest(w, r)
		if !ok {
			return
		}
		updated, updateErr := a.modelAdmin.UpdateRouteModelPrice(r.Context(), priceID, req)
		if updateErr != nil {
			httpx.WriteError(w, r, normalizeAppError(updateErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "route_model_price.update", "route_model_price", fmt.Sprintf("%d", updated.ID), nil)
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if deleteErr := a.modelAdmin.DeleteRouteModelPrice(r.Context(), priceID); deleteErr != nil {
			httpx.WriteError(w, r, normalizeAppError(deleteErr))
			return
		}
		a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "route_model_price.delete", "route_model_price", fmt.Sprintf("%d", priceID), nil)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminModelProviders(w http.ResponseWriter, r *http.Request) {
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
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
	admin, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageModels)
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

func docsExampleItems() []map[string]any {
	return []map[string]any{
		{
			"id":       "openapi-estimate-curl",
			"title":    "Open API 预估生图积分",
			"language": "curl",
			"code": strings.TrimSpace(`
curl -G "https://api.example.com/api/open/image/v1/estimate" \
  -H "X-Access-Key: ${PIC_GALLERY_AK}" \
  -H "X-Timestamp: ${TIMESTAMP}" \
  -H "X-Body-SHA256: ${BODY_SHA256}" \
  -H "X-Signature: ${SIGNATURE}" \
  --data-urlencode "task_type=text_to_image" \
  --data-urlencode "route_model_code=basic" \
  --data-urlencode "requested_quality=1k" \
  --data-urlencode "requested_size=1:1" \
  --data-urlencode "requested_output_image_count=1"`),
		},
		{
			"id":       "openapi-create-task-curl",
			"title":    "Open API 创建生图任务",
			"language": "curl",
			"code": strings.TrimSpace(`
curl -X POST "https://api.example.com/api/open/image/v1/tasks" \
  -H "Content-Type: application/json" \
  -H "X-Access-Key: ${PIC_GALLERY_AK}" \
  -H "X-Timestamp: ${TIMESTAMP}" \
  -H "X-Body-SHA256: ${BODY_SHA256}" \
  -H "X-Signature: ${SIGNATURE}" \
  -d '{
    "task_type": "text_to_image",
    "route_model_code": "basic",
    "prompt": "A clean product poster",
    "requested_quality": "1k",
    "requested_size": "1:1",
    "requested_output_image_count": 1,
    "response_mode": "async"
  }'`),
		},
		{
			"id":       "openai-compatible-generation-curl",
			"title":    "OpenAI 兼容生图",
			"language": "curl",
			"code": strings.TrimSpace(`
curl -X POST "https://api.example.com/v1/images/generations" \
  -H "Authorization: Bearer ${PIC_GALLERY_API_KEY}" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "basic",
    "prompt": "A cinematic product photo",
    "size": "1024x1024",
    "n": 1
  }'`),
		},
	}
}

func (a *API) HandleDocsExamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": docsExampleItems(),
	})
}

func docsErrorItems() []map[string]any {
	return []map[string]any{
		{"code": errs.CodeBadRequest, "http_status": http.StatusBadRequest, "message": "Request parameters are invalid.", "retryable": false},
		{"code": errs.CodeUnauthorized, "http_status": http.StatusUnauthorized, "message": "Authentication is missing or invalid.", "retryable": false},
		{"code": errs.CodeForbidden, "http_status": http.StatusForbidden, "message": "The authenticated caller cannot access this resource.", "retryable": false},
		{"code": errs.CodeNotFound, "http_status": http.StatusNotFound, "message": "The requested resource does not exist.", "retryable": false},
		{"code": errs.CodeRateLimited, "http_status": http.StatusTooManyRequests, "message": "Rate limit exceeded; retry after backing off.", "retryable": true},
		{"code": errs.CodeInsufficientPoints, "http_status": http.StatusConflict, "message": "Available points are insufficient for the requested generation.", "retryable": false},
		{"code": errs.CodeModelRouteNotFound, "http_status": http.StatusNotFound, "message": "The requested route model is not configured.", "retryable": false},
		{"code": errs.CodeModelRouteNotVisible, "http_status": http.StatusForbidden, "message": "The requested route model is not visible to the current user.", "retryable": false},
		{"code": errs.CodeModelRouteNoCandidate, "http_status": http.StatusConflict, "message": "The route model has no available provider candidate.", "retryable": true},
		{"code": errs.CodeRouteModelPriceMissing, "http_status": http.StatusConflict, "message": "The route model has no active price configuration.", "retryable": false},
		{"code": errs.CodePaymentMethodUnavailable, "http_status": http.StatusBadRequest, "message": "The selected payment method is unavailable.", "retryable": false},
		{"code": errs.CodePaymentAmountOutOfRange, "http_status": http.StatusBadRequest, "message": "Recharge amount is outside the configured allowed range.", "retryable": false},
		{"code": errs.CodePaymentProviderUnavailable, "http_status": http.StatusConflict, "message": "No enabled payment provider instance can process this order.", "retryable": true},
		{"code": errs.CodePaymentProviderNotImplemented, "http_status": http.StatusNotImplemented, "message": "The selected payment provider adapter is not implemented.", "retryable": false},
		{"code": errs.CodePaymentTooManyPending, "http_status": http.StatusConflict, "message": "The user has too many pending payment orders.", "retryable": false},
		{"code": errs.CodePaymentSignatureInvalid, "http_status": http.StatusForbidden, "message": "Payment webhook signature verification failed.", "retryable": false},
		{"code": errs.CodePaymentAmountMismatch, "http_status": http.StatusConflict, "message": "Payment webhook amount does not match the order amount.", "retryable": false},
		{"code": errs.CodeLoginRequiredGalleryDetail, "http_status": http.StatusUnauthorized, "message": "Login is required to view the full public gallery prompt.", "retryable": false},
		{"code": errs.CodeSignupTrialConfigInvalid, "http_status": http.StatusInternalServerError, "message": "Signup trial credit configuration is invalid.", "retryable": true},
		{"code": errs.CodeUpstreamUnavailable, "http_status": http.StatusServiceUnavailable, "message": "Upstream model provider is temporarily unavailable.", "retryable": true},
	}
}

func (a *API) HandleDocsErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	items := docsErrorItems()
	codes := make([]string, 0, len(items))
	for _, item := range items {
		codes = append(codes, fmt.Sprint(item["code"]))
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": items,
		"codes": codes,
	})
}

func defaultDocsReadinessChecker(_ context.Context) DocsReadinessResult {
	content, err := os.ReadFile(openAPIDocumentPath())
	if err != nil {
		return DocsReadinessResult{Status: "fail", Detail: "OpenAPI JSON 不可读取"}
	}
	var document any
	if err := yaml.Unmarshal(content, &document); err != nil {
		return DocsReadinessResult{Status: "fail", Detail: "OpenAPI JSON 解析失败"}
	}
	normalized, ok := normalizeYAMLForJSON(document).(map[string]any)
	if !ok {
		return DocsReadinessResult{Status: "fail", Detail: "OpenAPI JSON 结构异常"}
	}
	if strings.TrimSpace(fmt.Sprint(normalized["openapi"])) == "" {
		return DocsReadinessResult{Status: "fail", Detail: "OpenAPI JSON 缺少 openapi 版本"}
	}
	paths, ok := normalized["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		return DocsReadinessResult{Status: "fail", Detail: "OpenAPI JSON 缺少接口路径"}
	}
	examples := docsExampleItems()
	if len(examples) == 0 {
		return DocsReadinessResult{Status: "fail", Detail: "接口示例为空"}
	}
	errors := docsErrorItems()
	if len(errors) == 0 {
		return DocsReadinessResult{Status: "fail", Detail: "错误码示例为空"}
	}
	return DocsReadinessResult{
		Status: "pass",
		Detail: fmt.Sprintf("OpenAPI %d 个路径，示例 %d 条，错误码 %d 条", len(paths), len(examples), len(errors)),
	}
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
	task = decorateTaskProgress(task)
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
		RouteModelCode            string   `json:"route_model_code"`
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
		RouteModelCode:      req.RouteModelCode,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    req.RequestedQuality,
		OutputImageCount:    req.RequestedOutputImageCount,
		ReferenceImageCount: len(req.ReferenceAssetIDs),
		ReferenceAssetIDs:   append([]string(nil), req.ReferenceAssetIDs...),
		UserGroupCode:       user.GroupCode,
		UserGroupCodes:      userGroupCodes(user),
		UserGroupMultiplier: user.GroupMultiplier,
		ResponseMode:        req.ResponseMode,
		SavePolicy:          "private",
	})
	if err != nil {
		httpx.WriteError(w, r, compatservice.MapError(err))
		return
	}
	result = decorateTaskProgress(result)
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
		RouteModelCode            string   `json:"route_model_code"`
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
		RouteModelCode:            req.RouteModelCode,
		RequestedQuality:          req.RequestedQuality,
		RequestedSize:             req.RequestedSize,
		RequestedOutputImageCount: req.RequestedOutputImageCount,
		ReferenceImageCount:       len(req.ReferenceAssetIDs),
		UserGroupCode:             identity.GroupCode,
		UserGroupCodes:            []string{identity.GroupCode},
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
		RouteModelCode:      req.RouteModelCode,
		TaskType:            req.TaskType,
		Prompt:              req.Prompt,
		RequestedSize:       req.RequestedSize,
		RequestedQuality:    req.RequestedQuality,
		OutputImageCount:    req.RequestedOutputImageCount,
		ReferenceImageCount: len(req.ReferenceAssetIDs),
		ReferenceAssetIDs:   append([]string(nil), req.ReferenceAssetIDs...),
		UserGroupCode:       identity.GroupCode,
		UserGroupCodes:      []string{identity.GroupCode},
		UserGroupMultiplier: a.userGroupMultiplier(identity.GroupCode),
		ResponseMode:        req.ResponseMode,
		SavePolicy:          "private",
	})
	if err != nil {
		a.apiKeys.ReleaseQuota(r.Context(), identity, taskID)
		httpx.WriteError(w, r, compatservice.MapError(err))
		return
	}
	result = decorateTaskProgress(result)
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
	tasks = decorateTaskProgressList(tasks)
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
	payload := map[string]any{
		"id":                user.ID,
		"email":             user.Email,
		"nickname":          user.Nickname,
		"bio":               user.Bio,
		"avatar_object_key": user.AvatarObjectKey,
		"user_group_code":   user.GroupCode,
		"theme":             defaultString(user.Theme, "system"),
		"default_locale":    defaultString(user.DefaultLocale, "zh-CN"),
	}
	if mode, accent, ok := profileThemePreference(user.Theme); ok {
		payload["preferences"] = map[string]any{
			"theme_mode":     mode,
			"accent_theme":   accent,
			"default_locale": defaultString(user.DefaultLocale, "zh-CN"),
		}
	}
	return payload
}

func profileThemePreference(theme string) (string, string, bool) {
	parts := strings.Split(strings.TrimSpace(theme), ":")
	if len(parts) != 2 {
		return "", "", false
	}
	mode := strings.TrimSpace(parts[0])
	accent := strings.TrimSpace(parts[1])
	switch mode {
	case "dark", "light":
	default:
		return "", "", false
	}
	switch accent {
	case "amber", "violet", "emerald", "coral":
	default:
		return "", "", false
	}
	return mode, accent, true
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

func parseAdminSystemUserAction(path string) (int64, string, *errs.Error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/ops/admin/v1/admin-users/"), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", errs.BadRequest("invalid admin_id")
	}
	adminID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || adminID <= 0 {
		return 0, "", errs.BadRequest("invalid admin_id")
	}
	if len(parts) == 1 {
		return adminID, "", nil
	}
	if len(parts) == 2 {
		return adminID, parts[1], nil
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
		Code        string  `json:"code"`
		GroupName   string  `json:"group_name"`
		Name        string  `json:"name"`
		Multiplier  string  `json:"multiplier"`
		Status      string  `json:"status"`
		Description *string `json:"description"`
		SortOrder   int     `json:"sort_order"`
		IsDefault   bool    `json:"is_default"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainadminuser.UserGroupWriteRequest{}, false
	}
	return domainadminuser.UserGroupWriteRequest{
		GroupCode:   defaultString(req.GroupCode, req.Code),
		GroupName:   defaultString(req.GroupName, req.Name),
		Multiplier:  req.Multiplier,
		Status:      req.Status,
		Description: req.Description,
		SortOrder:   req.SortOrder,
		IsDefault:   req.IsDefault,
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

func parseCashierOrderPath(path string) (int64, string, *errs.Error) {
	trimmed := strings.Trim(strings.TrimPrefix(path, "/api/agent/cashier/v1/orders/"), "/")
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

func parseAdminCashierPlanID(path string) (int64, *errs.Error) {
	const prefix = "/api/ops/admin/v1/cashier/plans/"
	raw := strings.TrimPrefix(path, prefix)
	raw = strings.Trim(raw, "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, errs.BadRequest("invalid plan_id")
	}
	planID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || planID <= 0 {
		return 0, errs.BadRequest("invalid plan_id")
	}
	return planID, nil
}

func parseAdminCashierProviderInstanceID(path string) (int64, *errs.Error) {
	const prefix = "/api/ops/admin/v1/cashier/provider-instances/"
	raw := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if raw == "" || strings.Contains(raw, "/") {
		return 0, errs.BadRequest("invalid instance_id")
	}
	instanceID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || instanceID <= 0 {
		return 0, errs.BadRequest("invalid instance_id")
	}
	return instanceID, nil
}

func parseAdminCashierOrderPath(path string) (int64, string, *errs.Error) {
	const prefix = "/api/ops/admin/v1/cashier/orders/"
	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
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
	if len(parts) > 2 {
		return 0, "", errs.BadRequest("invalid cashier order action")
	}
	return orderID, action, nil
}

func parseAdminCashierWebhookEventPath(path string) (int64, string, *errs.Error) {
	const prefix = "/api/ops/admin/v1/cashier/webhook-events/"
	trimmed := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 || parts[0] == "" {
		return 0, "", errs.BadRequest("invalid event_id")
	}
	eventID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || eventID <= 0 {
		return 0, "", errs.BadRequest("invalid event_id")
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	if len(parts) > 2 {
		return 0, "", errs.BadRequest("invalid webhook event action")
	}
	return eventID, action, nil
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
	if len(parts) == 1 && parts[0] != "" {
		return parts[0], "", nil
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
	return "", "", errs.New(http.StatusNotFound, errs.CodeNotFound, "gallery image route not found")
}

func parseAdminImageReviewAction(path string) (string, string, *errs.Error) {
	trimmed := strings.TrimPrefix(path, "/api/ops/admin/v1/image-reviews/")
	if parts := strings.Split(strings.Trim(trimmed, "/"), "/"); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], nil
	}
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

func (a *API) adminReadinessChecks(ctx context.Context) ([]adminReadinessCheck, *errs.Error) {
	checkedAt := time.Now().UTC()
	checks := make([]adminReadinessCheck, 0, 10)

	enabled := true
	accounts, err := a.modelAdmin.ListModelAccounts(ctx, domainmodeladmin.ModelAccountListRequest{Page: 1, PageSize: 200, Status: domainmodeladmin.ModelAccountStatusEnabled})
	if err != nil {
		return nil, normalizeAppError(err)
	}
	checks = append(checks, readinessCheck("model_accounts", "模型接入账号", statusByPositiveCount(accounts.Total), fmt.Sprintf("%d 个已启用账号", accounts.Total), "provider-models", "去配置", checkedAt))

	providerModels, err := a.modelAdmin.ListModelAccountModels(ctx, domainmodeladmin.ModelAccountModelListRequest{Page: 1, PageSize: 200, Enabled: &enabled})
	if err != nil {
		return nil, normalizeAppError(err)
	}
	checks = append(checks, readinessCheck("provider_models", "真实模型", statusByPositiveCount(providerModels.Total), fmt.Sprintf("%d 个已启用真实模型", providerModels.Total), "provider-models", "去配置", checkedAt))

	routeModels, err := a.modelAdmin.ListRouteModels(ctx, domainmodeladmin.RouteModelListRequest{Page: 1, PageSize: 200, Enabled: &enabled})
	if err != nil {
		return nil, normalizeAppError(err)
	}
	visibleRouteModels := make([]domainmodeladmin.RouteModel, 0, len(routeModels.Items))
	for _, item := range routeModels.Items {
		if item.Visibility == domainmodeladmin.RouteModelVisibilityPublic || item.Visibility == domainmodeladmin.RouteModelVisibilityGroups {
			visibleRouteModels = append(visibleRouteModels, item)
		}
	}
	routeModelStatus := statusByPositiveCount(len(visibleRouteModels))
	checks = append(checks, readinessCheck("route_models", "路由模型", routeModelStatus, fmt.Sprintf("%d 个可见启用路由模型", len(visibleRouteModels)), "routing", "去配置", checkedAt))

	routeCandidateStatus := "fail"
	routeCandidateDetail := "暂无可见启用路由模型"
	if len(visibleRouteModels) > 0 {
		missing := 0
		for _, routeModel := range visibleRouteModels {
			candidates, candidateErr := a.modelAdmin.ListRouteModelCandidates(ctx, routeModel.ID)
			if candidateErr != nil {
				return nil, normalizeAppError(candidateErr)
			}
			hasEnabledCandidate := false
			for _, candidate := range candidates {
				if candidate.Enabled {
					hasEnabledCandidate = true
					break
				}
			}
			if !hasEnabledCandidate {
				missing++
			}
		}
		switch {
		case missing == 0:
			routeCandidateStatus = "pass"
			routeCandidateDetail = fmt.Sprintf("%d 个路由模型均有启用候选", len(visibleRouteModels))
		case missing == len(visibleRouteModels):
			routeCandidateStatus = "fail"
			routeCandidateDetail = fmt.Sprintf("%d 个路由模型缺少启用候选", missing)
		default:
			routeCandidateStatus = "warn"
			routeCandidateDetail = fmt.Sprintf("%d 个路由模型缺少启用候选", missing)
		}
	}
	checks = append(checks, readinessCheck("route_candidates", "候选模型", routeCandidateStatus, routeCandidateDetail, "routing", "去配置", checkedAt))

	prices, err := a.modelAdmin.ListRouteModelPrices(ctx, domainmodeladmin.RouteModelPriceListRequest{Page: 1, PageSize: 200, Enabled: &enabled})
	if err != nil {
		return nil, normalizeAppError(err)
	}
	routePriceStatus := statusByPositiveCount(prices.Total)
	if len(visibleRouteModels) > 0 && routePriceStatus == "pass" {
		priceRouteIDs := map[int64]struct{}{}
		for _, price := range prices.Items {
			priceRouteIDs[price.RouteModelID] = struct{}{}
		}
		missing := 0
		for _, routeModel := range visibleRouteModels {
			if _, ok := priceRouteIDs[routeModel.ID]; !ok {
				missing++
			}
		}
		if missing > 0 {
			routePriceStatus = "warn"
		}
	}
	checks = append(checks, readinessCheck("route_prices", "价格策略", routePriceStatus, fmt.Sprintf("%d 条已启用价格", prices.Total), "pricing", "去配置", checkedAt))

	checks = append(checks, a.paymentReadinessCheck(ctx, checkedAt))
	refundCompensationCheck, appErr := a.refundCompensationReadinessCheck(ctx, checkedAt)
	if appErr != nil {
		return nil, appErr
	}
	checks = append(checks, refundCompensationCheck)
	checks = append(checks, a.signupTrialReadinessCheck(ctx, checkedAt))
	checks = append(checks, a.publicGalleryReadinessCheck(ctx, checkedAt))
	checks = append(checks, a.docsReadinessCheck(ctx, checkedAt))
	return checks, nil
}

func (a *API) adminUserDetailPayload(ctx context.Context, userID int64, detail domainadminuser.Detail) (map[string]any, *errs.Error) {
	orders, err := a.billing.ListOrders(ctx, domainbilling.ListOrdersRequest{UserID: userID, Page: 1, PageSize: 5})
	if err != nil {
		return nil, normalizeAppError(err)
	}
	tasks, err := a.tasks.ListByUser(ctx, userID)
	if err != nil {
		return nil, normalizeAppError(err)
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	if len(tasks) > 5 {
		tasks = tasks[:5]
	}
	apiKeys, err := a.apiKeys.ListByUser(ctx, userID)
	if err != nil {
		return nil, normalizeAppError(err)
	}
	if len(apiKeys) > 10 {
		apiKeys = apiKeys[:10]
	}
	return map[string]any{
		"user":          detail.User,
		"balance":       detail.Balance,
		"recent_ledger": detail.RecentLedger,
		"recent_orders": orders.Items,
		"recent_tasks":  tasks,
		"api_keys":      apiKeyPayloads(apiKeys),
	}, nil
}

func readinessCheck(key, label, status, detail, fixRoute, actionLabel string, checkedAt time.Time) adminReadinessCheck {
	return adminReadinessCheck{
		Key:         key,
		Label:       label,
		Status:      status,
		Detail:      detail,
		Summary:     detail,
		FixRoute:    fixRoute,
		FixAction:   actionLabel,
		ActionRoute: fixRoute,
		ActionLabel: actionLabel,
		Blocking:    status == "fail",
		CheckedAt:   checkedAt,
	}
}

func statusByPositiveCount(count int) string {
	if count > 0 {
		return "pass"
	}
	return "fail"
}

func summarizeReadinessChecks(checks []adminReadinessCheck) (string, map[string]int) {
	summary := map[string]int{"pass": 0, "warn": 0, "fail": 0}
	status := "pass"
	for _, check := range checks {
		switch check.Status {
		case "fail":
			summary["fail"]++
			status = "fail"
		case "warn":
			summary["warn"]++
			if status != "fail" {
				status = "warn"
			}
		default:
			summary["pass"]++
		}
	}
	return status, summary
}

func (a *API) paymentReadinessCheck(ctx context.Context, checkedAt time.Time) adminReadinessCheck {
	if !a.cfg.Cashier.Enabled {
		return readinessCheck("payments", "支付配置", "warn", "收银台未启用，用户无法在线充值", "cashier", "去配置", checkedAt)
	}
	methods := a.cashierVisibleMethods(ctx, false)
	if len(methods) == 0 {
		return readinessCheck("payments", "支付配置", "fail", "暂无可见支付方式", "cashier", "去配置", checkedAt)
	}
	instances := a.cashierProviderInstances(ctx)
	enabledInstanceCount := 0
	for _, instance := range instances {
		if !instance.Enabled {
			continue
		}
		if isProductionAppEnv(a.cfg.App.Env) && instance.ProviderType == "mock" {
			continue
		}
		if instance.ProviderType != "mock" && instance.ConfigStatus != "configured" {
			continue
		}
		enabledInstanceCount++
	}
	if enabledInstanceCount == 0 {
		return readinessCheck("payments", "支付配置", "fail", "暂无可用支付渠道实例", "cashier", "去配置", checkedAt)
	}
	return readinessCheck("payments", "支付配置", "pass", fmt.Sprintf("%d 个可见支付方式，%d 个可用渠道实例", len(methods), enabledInstanceCount), "cashier", "去配置", checkedAt)
}

func (a *API) refundCompensationReadinessCheck(ctx context.Context, checkedAt time.Time) (adminReadinessCheck, *errs.Error) {
	events, err := a.billing.ListWebhookEvents(ctx, 1, 1000)
	if err != nil {
		return adminReadinessCheck{}, normalizeAppError(err)
	}
	summary := summarizeRefundCompensationFailures(events.Items)
	if summary.Count == 0 {
		return readinessCheck("refund_compensation", "退款补偿", "pass", "暂无待补偿退款失败事件", "cashier", "去处理", checkedAt), nil
	}
	detail := fmt.Sprintf("%d 个退款补偿失败事件需处理", summary.Count)
	if summary.OldestFailedAt != nil {
		detail = fmt.Sprintf("%s，最早失败于 %s", detail, summary.OldestFailedAt.Format("2006-01-02 15:04"))
	}
	return readinessCheck("refund_compensation", "退款补偿", "fail", detail, "cashier", "去处理", checkedAt), nil
}

func (a *API) signupTrialReadinessCheck(ctx context.Context, checkedAt time.Time) adminReadinessCheck {
	trial := a.signupTrialConfig(ctx)
	if !trial.Enabled {
		return readinessCheck("signup_trial", "注册送体验额度", "warn", "注册送体验额度未启用", "config", "去配置", checkedAt)
	}
	points, err := decimal.NewFromString(strings.TrimSpace(trial.Points))
	if err != nil || !points.IsPositive() {
		return readinessCheck("signup_trial", "注册送体验额度", "fail", "体验额度积分配置无效", "config", "去配置", checkedAt)
	}
	if trial.ValidDays <= 0 {
		return readinessCheck("signup_trial", "注册送体验额度", "fail", "体验额度有效期配置无效", "config", "去配置", checkedAt)
	}
	return readinessCheck("signup_trial", "注册送体验额度", "pass", fmt.Sprintf("注册赠送 %s 积分，有效期 %d 天", points.StringFixed(5), trial.ValidDays), "config", "去配置", checkedAt)
}

func (a *API) publicGalleryReadinessCheck(ctx context.Context, checkedAt time.Time) adminReadinessCheck {
	if !a.adminConfigBool(ctx, "public_gallery", "gallery_enabled", true) {
		return readinessCheck("public_gallery", "公开广场", "warn", "公开广场入口未启用", "reviews", "去审核", checkedAt)
	}
	pending, err := a.tasks.ListGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 1, Status: domainimagetask.VisibilityPendingReview})
	if err != nil {
		return readinessCheck("public_gallery", "公开广场", "warn", "审核队列读取失败", "reviews", "去审核", checkedAt)
	}
	public, err := a.tasks.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 1})
	if err != nil {
		return readinessCheck("public_gallery", "公开广场", "warn", "公开作品读取失败", "reviews", "去审核", checkedAt)
	}
	status := "pass"
	if public.Total == 0 {
		status = "warn"
	}
	return readinessCheck("public_gallery", "公开广场", status, fmt.Sprintf("%d 个公开作品，%d 个待审核", public.Total, pending.Total), "reviews", "去审核", checkedAt)
}

func (a *API) docsReadinessCheck(ctx context.Context, checkedAt time.Time) adminReadinessCheck {
	if strings.TrimSpace(a.cfg.Docs.Title) == "" || strings.TrimSpace(a.cfg.Docs.BasePath) == "" {
		return readinessCheck("docs", "开发文档", "warn", "开发文档标题或 base path 未配置", "config", "去配置", checkedAt)
	}
	checker := a.docsReady
	if checker == nil {
		checker = defaultDocsReadinessChecker
	}
	result := checker(ctx)
	status := strings.TrimSpace(result.Status)
	if status == "" {
		status = "pass"
	}
	detail := strings.TrimSpace(result.Detail)
	if detail == "" {
		detail = "OpenAPI、示例和错误码文档可解析"
	}
	return readinessCheck("docs", "开发文档", status, detail, "config", "去配置", checkedAt)
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

func decodeModelAccountWriteRequest(w http.ResponseWriter, r *http.Request) (domainmodeladmin.ModelAccountWriteRequest, bool) {
	var req struct {
		Name             string            `json:"name"`
		AdapterType      string            `json:"adapter_type"`
		AuthType         string            `json:"auth_type"`
		BaseURL          string            `json:"base_url"`
		Credentials      map[string]string `json:"credentials"`
		Status           string            `json:"status"`
		Priority         int               `json:"priority"`
		Weight           int               `json:"weight"`
		ConcurrencyLimit int               `json:"concurrency_limit"`
		TimeoutMS        int               `json:"timeout_ms"`
		Extra            map[string]any    `json:"extra"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.ModelAccountWriteRequest{}, false
	}
	return domainmodeladmin.ModelAccountWriteRequest{Name: req.Name, AdapterType: req.AdapterType, AuthType: req.AuthType, BaseURL: req.BaseURL, Credentials: req.Credentials, Status: req.Status, Priority: req.Priority, Weight: req.Weight, ConcurrencyLimit: req.ConcurrencyLimit, TimeoutMS: req.TimeoutMS, Extra: req.Extra}, true
}

func decodeModelAccountModelWriteRequest(w http.ResponseWriter, r *http.Request, accountID int64) (domainmodeladmin.ModelAccountModelWriteRequest, bool) {
	var req struct {
		ModelCode    string         `json:"model_code"`
		DisplayName  string         `json:"display_name"`
		TaskTypes    []string       `json:"task_types"`
		Qualities    []string       `json:"qualities"`
		CostPerImage string         `json:"cost_per_image"`
		Currency     string         `json:"currency"`
		Enabled      bool           `json:"enabled"`
		Extra        map[string]any `json:"extra"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.ModelAccountModelWriteRequest{}, false
	}
	return domainmodeladmin.ModelAccountModelWriteRequest{AccountID: accountID, ModelCode: req.ModelCode, DisplayName: req.DisplayName, TaskTypes: req.TaskTypes, Qualities: req.Qualities, CostPerImage: req.CostPerImage, Currency: req.Currency, Enabled: req.Enabled, Extra: req.Extra}, true
}

func decodeModelAccountTestImageRequest(w http.ResponseWriter, r *http.Request) (domainimagetask.TestModelAccountRequest, bool) {
	var req struct {
		ModelID    int64  `json:"model_id"`
		ModelCode  string `json:"model_code"`
		Prompt     string `json:"prompt"`
		SourceMode string `json:"source_mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainimagetask.TestModelAccountRequest{}, false
	}
	return domainimagetask.TestModelAccountRequest{
		ModelID:    req.ModelID,
		ModelCode:  req.ModelCode,
		Prompt:     req.Prompt,
		SourceMode: req.SourceMode,
	}, true
}

func decodeRouteModelWriteRequest(w http.ResponseWriter, r *http.Request) (domainmodeladmin.RouteModelWriteRequest, bool) {
	var req struct {
		Code        string  `json:"code"`
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Visibility  string  `json:"visibility"`
		Enabled     bool    `json:"enabled"`
		SortOrder   int     `json:"sort_order"`
		GroupIDs    []int64 `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.RouteModelWriteRequest{}, false
	}
	return domainmodeladmin.RouteModelWriteRequest{Code: req.Code, Name: req.Name, Description: req.Description, Visibility: req.Visibility, Enabled: req.Enabled, SortOrder: req.SortOrder, GroupIDs: req.GroupIDs}, true
}

func decodeRouteModelCandidateWriteRequest(w http.ResponseWriter, r *http.Request, routeModelID int64) (domainmodeladmin.RouteModelCandidateWriteRequest, bool) {
	var req struct {
		AccountModelID int64 `json:"account_model_id"`
		Priority       int   `json:"priority"`
		Weight         int   `json:"weight"`
		FallbackOrder  int   `json:"fallback_order"`
		Enabled        bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.RouteModelCandidateWriteRequest{}, false
	}
	return domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: routeModelID, AccountModelID: req.AccountModelID, Priority: req.Priority, Weight: req.Weight, FallbackOrder: req.FallbackOrder, Enabled: req.Enabled}, true
}

func decodeRouteModelPriceWriteRequest(w http.ResponseWriter, r *http.Request) (domainmodeladmin.RouteModelPriceWriteRequest, bool) {
	var req struct {
		RouteModelID        int64  `json:"route_model_id"`
		TaskType            string `json:"task_type"`
		Quality             string `json:"quality"`
		BasePoints          string `json:"base_points"`
		ReferenceMultiplier string `json:"reference_multiplier"`
		Enabled             bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return domainmodeladmin.RouteModelPriceWriteRequest{}, false
	}
	return domainmodeladmin.RouteModelPriceWriteRequest{RouteModelID: req.RouteModelID, TaskType: req.TaskType, Quality: req.Quality, BasePoints: req.BasePoints, ReferenceMultiplier: req.ReferenceMultiplier, Enabled: req.Enabled}, true
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

func pagedPayload[T any](items []T, page, pageSize, total int) map[string]any {
	if pageSize <= 0 {
		pageSize = len(items)
	}
	return map[string]any{
		"items": items,
		"pagination": map[string]any{
			"page":      page,
			"page_size": pageSize,
			"total":     total,
		},
	}
}

func cashierPlanPayload(plan domainbilling.SubscriptionPlan) map[string]any {
	return map[string]any{
		"id":               plan.ID,
		"plan_code":        plan.PlanCode,
		"plan_name":        plan.PlanName,
		"status":           plan.Status,
		"price_cny":        plan.PriceCNY,
		"points":           plan.Points,
		"bonus_points":     plan.BonusPoints,
		"duration_days":    plan.DurationDays,
		"currency":         plan.Currency,
		"sort_order":       plan.SortOrder,
		"description":      plan.Description,
		"created_at":       plan.CreatedAt,
		"updated_at":       plan.UpdatedAt,
		"plan_type":        plan.PlanType,
		"purchase_enabled": plan.PurchaseEnabled,
	}
}

func isPurchasableCashierPlan(plan domainbilling.SubscriptionPlan) bool {
	return plan.Status == "active" && plan.PlanType == "points_package" && plan.PurchaseEnabled
}

func isPaidCashierOrder(status string) bool {
	status = strings.ToLower(strings.TrimSpace(status))
	return status == "paid" || status == "completed"
}

func sameUTCDate(left, right time.Time) bool {
	left = left.UTC()
	right = right.UTC()
	return left.Year() == right.Year() && left.YearDay() == right.YearDay()
}

func isPreflightErrorCode(code string) bool {
	switch strings.TrimSpace(code) {
	case errs.CodeModelRouteNotFound,
		errs.CodeModelRouteNotVisible,
		errs.CodeModelRouteNoCandidate,
		errs.CodeRouteModelPriceMissing,
		errs.CodeInsufficientPoints,
		errs.CodeImageCapabilityMismatch,
		errs.CodeImageReferenceRequired,
		errs.CodeImageReferenceExceeded,
		errs.CodeImageAutoUnsupported:
		return true
	default:
		return false
	}
}

func hasTrialBalance(balance domainbilling.BalanceSummary) bool {
	if isPositiveDecimalString(balance.TrialPoints) {
		return true
	}
	for _, bucket := range balance.Buckets {
		if bucket.Bucket == "trial" && isPositiveDecimalString(bucket.AvailablePoints) {
			return true
		}
	}
	return false
}

func hasTrialExpiringBalance(balance domainbilling.BalanceSummary) bool {
	for _, bucket := range balance.Buckets {
		if bucket.Bucket == "trial" && bucket.ExpireWarning && isPositiveDecimalString(bucket.AvailablePoints) {
			return true
		}
	}
	return false
}

type refundCompensationFailureSummary struct {
	Count          int
	OldestFailedAt *time.Time
}

func summarizeRefundCompensationFailures(events []domainbilling.PaymentWebhookEvent) refundCompensationFailureSummary {
	var summary refundCompensationFailureSummary
	for _, event := range events {
		if event.EventType != "refund.local_finalize_failed" || event.Status != "failed" {
			continue
		}
		summary.Count++
		receivedAt := event.ReceivedAt
		if summary.OldestFailedAt == nil || receivedAt.Before(*summary.OldestFailedAt) {
			summary.OldestFailedAt = &receivedAt
		}
	}
	return summary
}

func refundCompensationTrend(summary refundCompensationFailureSummary) string {
	if summary.Count == 0 {
		return "正常"
	}
	if summary.OldestFailedAt == nil {
		return "需处理"
	}
	return fmt.Sprintf("需处理，最早 %s", summary.OldestFailedAt.Format("2006-01-02 15:04"))
}

func refundCompensationTone(count int) string {
	if count > 0 {
		return "danger"
	}
	return "good"
}

func isPositiveDecimalString(value string) bool {
	parsed, err := decimal.NewFromString(strings.TrimSpace(value))
	return err == nil && parsed.IsPositive()
}

func dashboardMetricTone(good bool, bad bool) string {
	if bad {
		return "bad"
	}
	if good {
		return "good"
	}
	return "neutral"
}

func boolTrend(ok bool, okText, badText string) string {
	if ok {
		return okText
	}
	return badText
}

func boolLabel(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func paginateAny[T any](items []T, page, pageSize int) []T {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = len(items)
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end]
}

func (a *API) cashierCustomAmountConfig(ctx context.Context) cashierCustomAmountConfig {
	normalized, err := a.cashierConfigFacade().CustomAmountConfig(ctx)
	if err != nil {
		return cashierCustomAmountConfig{
			Enabled:      true,
			MinAmountCNY: "1.00000",
			MaxAmountCNY: "999.00000",
			CNYPerPoint:  handlerBillingString(a.cfg.Billing.CNYPerPoint, "0.31250"),
		}
	}
	return normalized
}

func positiveDecimalString(raw, field string) (string, decimal.Decimal, *errs.Error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.IsPositive() {
		return "", decimal.Zero, errs.BadRequest(field + " must be positive")
	}
	return value.StringFixed(5), value, nil
}

func validateCashierCustomAmount(raw string, cfg cashierCustomAmountConfig) *errs.Error {
	if err := cashierservice.ValidateCustomAmount(raw, cfg); err != nil {
		return normalizeAppError(err)
	}
	return nil
}

func configValueItem(category, key string, value any) domainadminconfig.Item {
	return domainadminconfig.Item{
		ConfigCategory: category,
		ConfigKey:      key,
		ConfigValue:    map[string]any{"value": value},
		Scope:          "global",
	}
}

func configBoolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}

func configStringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		return trimmed, trimmed != ""
	case fmt.Stringer:
		trimmed := strings.TrimSpace(typed.String())
		return trimmed, trimmed != ""
	default:
		return fmt.Sprint(value), value != nil
	}
}

func (a *API) signupTrialConfig(ctx context.Context) config.SignupTrialConfig {
	trial := normalizeHandlerSignupTrialConfig(a.cfg.Billing.SignupTrial)
	if a.admin == nil {
		return trial
	}
	tab, err := a.admin.GetTab(ctx, "trial_credits")
	if err != nil {
		return trial
	}
	for _, item := range tab.Items {
		if item.ConfigKey != "signup_trial" {
			continue
		}
		raw, ok := item.ConfigValue["value"]
		if !ok {
			return trial
		}
		return mergeSignupTrialConfig(trial, raw)
	}
	return trial
}

func mergeSignupTrialConfig(base config.SignupTrialConfig, raw any) config.SignupTrialConfig {
	cfg := base
	values, ok := raw.(map[string]any)
	if !ok {
		return normalizeHandlerSignupTrialConfig(cfg)
	}
	if enabled, ok := configBoolValue(values["enabled"]); ok {
		cfg.Enabled = enabled
	}
	if points, ok := configStringValue(values["points"]); ok {
		cfg.Points = points
	}
	if days, ok := configIntValue(values["valid_days"]); ok {
		cfg.ValidDays = days
	}
	if days, ok := configIntValue(values["expiry_reminder_days"]); ok {
		cfg.ExpiryReminderDays = days
	}
	if once, ok := configBoolValue(values["grant_once_per_user"]); ok {
		cfg.GrantOncePerUser = once
	}
	return normalizeHandlerSignupTrialConfig(cfg)
}

func configIntValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizeHandlerSignupTrialConfig(cfg config.SignupTrialConfig) config.SignupTrialConfig {
	if strings.TrimSpace(cfg.Points) == "" {
		cfg.Points = "20.00000"
	}
	if cfg.ValidDays == 0 {
		cfg.ValidDays = 7
	}
	if cfg.ExpiryReminderDays == 0 {
		cfg.ExpiryReminderDays = 2
	}
	cfg.GrantOncePerUser = true
	return cfg
}

func (a *API) cashierVisibleMethods(ctx context.Context, includeDisabled bool) []cashierVisibleMethod {
	methods, err := a.cashierConfigFacade().VisibleMethods(ctx, includeDisabled)
	if err != nil {
		return cashierservice.DefaultVisibleMethods()
	}
	return methods
}

func (a *API) cashierVisibleMethod(ctx context.Context, methodName string) (cashierVisibleMethod, bool) {
	methodName = strings.ToLower(strings.TrimSpace(methodName))
	for _, method := range a.cashierVisibleMethods(ctx, false) {
		if method.Method == methodName {
			return method, true
		}
	}
	return cashierVisibleMethod{}, false
}

func (a *API) cashierPurchasablePlan(ctx context.Context, planCode string) (domainbilling.SubscriptionPlan, *errs.Error) {
	planCode = strings.TrimSpace(planCode)
	if planCode == "" {
		return domainbilling.SubscriptionPlan{}, errs.BadRequest("plan_code is required")
	}
	plans, err := a.billing.ListPlans(ctx)
	if err != nil {
		return domainbilling.SubscriptionPlan{}, normalizeAppError(err)
	}
	for _, plan := range plans {
		if plan.PlanCode == planCode && isPurchasableCashierPlan(plan) {
			return plan, nil
		}
	}
	return domainbilling.SubscriptionPlan{}, errs.New(http.StatusNotFound, errs.CodeNotFound, "subscription plan not found")
}

func (a *API) scheduleCashierProviderInstance(ctx context.Context, method cashierVisibleMethod, amountCNY string) (cashierProviderInstance, *errs.Error) {
	method.Method = strings.ToLower(strings.TrimSpace(method.Method))
	method.SourceProviderType = strings.ToLower(strings.TrimSpace(method.SourceProviderType))
	if method.Method == "" || method.SourceProviderType == "" {
		return cashierProviderInstance{}, errs.New(http.StatusBadRequest, errs.CodePaymentMethodUnavailable, "payment method is unavailable")
	}
	if _, _, amountErr := positiveDecimalString(amountCNY, "amount_cny"); amountErr != nil {
		return cashierProviderInstance{}, amountErr
	}
	instances := a.cashierProviderInstances(ctx)
	if isProductionAppEnv(a.cfg.App.Env) {
		filtered := make([]cashierProviderInstance, 0, len(instances))
		for _, instance := range instances {
			if isProductionAppEnv(a.cfg.App.Env) && instance.ProviderType == "mock" {
				continue
			}
			filtered = append(filtered, instance)
		}
		instances = filtered
	}
	schedulerState := a.cashierSchedulerState(ctx)
	svc := cashierservice.NewServiceWithSchedulerState(cashierservice.SchedulerStateIDs(schedulerState))
	selected, err := svc.ScheduleProviderInstanceWithDailyUsage(ctx, method, instances, amountCNY, a.cashierProviderDailyUsage(ctx))
	if err != nil {
		if errors.Is(err, cashierservice.ErrPaymentMethodUnavailable) {
			return cashierProviderInstance{}, errs.New(http.StatusBadRequest, errs.CodePaymentMethodUnavailable, "payment method is unavailable")
		}
		return cashierProviderInstance{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if strings.EqualFold(method.SchedulerStrategy, "round_robin") {
		merged := cashierservice.MergeSchedulerStateIDs(schedulerState, svc.SchedulerState())
		if err := a.saveCashierSchedulerState(ctx, merged); err != nil {
			return selected, nil
		}
	}
	return selected, nil
}

func (a *API) cashierProviderDailyUsage(ctx context.Context) map[int64]decimal.Decimal {
	usage := map[int64]decimal.Decimal{}
	if a == nil || a.billing == nil {
		return usage
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	page := 1
	const pageSize = 200
	for {
		orders, err := a.billing.ListOrders(ctx, domainbilling.ListOrdersRequest{Page: page, PageSize: pageSize})
		if err != nil {
			return usage
		}
		for _, order := range orders.Items {
			if order.ProviderInstanceID <= 0 || order.CreatedAt.Before(today) {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(order.Status)) {
			case "pending", "paid", "completed", "partially_refunded":
			default:
				continue
			}
			amount, err := decimal.NewFromString(strings.TrimSpace(order.AmountCNY))
			if err != nil || !amount.IsPositive() {
				continue
			}
			usage[order.ProviderInstanceID] = usage[order.ProviderInstanceID].Add(amount)
		}
		if page*orders.PageSize >= orders.Total || len(orders.Items) == 0 {
			break
		}
		page++
	}
	return usage
}

func (a *API) cashierSchedulerState(ctx context.Context) map[string]map[string]any {
	state, err := a.cashierConfigFacade().SchedulerState(ctx)
	if err != nil {
		return map[string]map[string]any{}
	}
	return state
}

func (a *API) saveCashierSchedulerState(ctx context.Context, state map[string]map[string]any) error {
	return a.cashierConfigFacade().SaveSchedulerState(ctx, state, 0)
}

type cashierPaymentDisplayRequest = cashierservice.PaymentDisplayRequest
type cashierPaymentBuildResult = cashierservice.PaymentDisplayResult

func newCashierOrderNo() string {
	now := time.Now().UTC()
	return fmt.Sprintf("PGO-%d-%06d", now.Unix(), now.Nanosecond()%1000000)
}

func (a *API) cashierPaymentDisplay(ctx context.Context, method cashierVisibleMethod, instance cashierProviderInstance, req cashierPaymentDisplayRequest) (cashierPaymentBuildResult, *errs.Error) {
	req.Method = method
	req.Instance = instance
	registry := cashierservice.NewPaymentAdapterRegistryWithBuilders(cashierservice.PaymentProviderBuilders{
		AlipayDirect: cashierservice.NewAlipayPaymentDisplayBuilder(cashierservice.CallbackURLConfig{SiteBaseURL: a.cfg.Cashier.SiteBaseURL}),
		WxPayDirect:  cashierservice.NewWxPayPaymentDisplayBuilder(cashierservice.CallbackURLConfig{SiteBaseURL: a.cfg.Cashier.SiteBaseURL}),
		EasyPay:      cashierservice.NewEasyPayPaymentDisplayBuilder(cashierservice.CallbackURLConfig{SiteBaseURL: a.cfg.Cashier.SiteBaseURL}),
		JeePay:       cashierservice.NewJeePayPaymentDisplayBuilder(cashierservice.CallbackURLConfig{SiteBaseURL: a.cfg.Cashier.SiteBaseURL}),
	})

	result, err := registry.BuildPaymentDisplay(ctx, req)
	if err != nil {
		if errors.Is(err, cashierservice.ErrPaymentProviderNotImplemented) {
			return cashierPaymentBuildResult{}, errs.New(http.StatusNotImplemented, errs.CodePaymentProviderNotImplemented, "payment provider is not implemented")
		}
		return cashierPaymentBuildResult{}, normalizeAppError(err)
	}
	return result, nil
}

func (a *API) cashierCallbackURLs(instance cashierProviderInstance, providerType string, clientReturnURL ...string) (string, string) {
	notifyURL := strings.TrimSpace(mapStringValue(instance.Config, "notify_url", "notifyUrl"))
	returnURL := strings.TrimSpace(mapStringValue(instance.Config, "return_url", "returnUrl"))
	for _, value := range clientReturnURL {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			returnURL = trimmed
			break
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(a.cfg.Cashier.SiteBaseURL), "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if notifyURL == "" {
		notifyURL = baseURL + "/api/open/image/v1/payments/webhooks/" + strings.ToLower(strings.TrimSpace(providerType))
	}
	if returnURL == "" {
		returnURL = baseURL + "/#/checkout"
	}
	return notifyURL, returnURL
}

func trimEasyPayEndpointBase(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		path := strings.TrimRight(parsed.Path, "/")
		lower := strings.ToLower(path)
		for _, endpoint := range []string{"/submit.php", "/mapi.php", "/api.php"} {
			if strings.HasSuffix(lower, endpoint) {
				path = strings.TrimRight(path[:len(path)-len(endpoint)], "/")
				break
			}
		}
		parsed.Path = path
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func trimJeePayEndpointBase(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		path := strings.TrimRight(parsed.Path, "/")
		lower := strings.ToLower(path)
		for _, endpoint := range []string{"/api/pay/unifiedorder", "/api/pay/notify", "/api/pay"} {
			if strings.HasSuffix(lower, endpoint) {
				path = strings.TrimRight(path[:len(path)-len(endpoint)], "/")
				break
			}
		}
		parsed.Path = path
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

func appendQuery(raw string, values url.Values) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if strings.Contains(raw, "?") {
			return raw + "&" + values.Encode()
		}
		return raw + "?" + values.Encode()
	}
	query := parsed.Query()
	for key, items := range values {
		for _, item := range items {
			query.Set(key, item)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func alipayRSA2Sign(values url.Values, privateKeyPEM string) (string, *errs.Error) {
	privateKeyPEM = strings.TrimSpace(privateKeyPEM)
	if privateKeyPEM == "" {
		return "", errs.BadRequest("alipay private key is required")
	}
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", errs.BadRequest("alipay private key is invalid")
	}
	signContent := alipaySignContent(values)
	digest := sha256.Sum256([]byte(signContent))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errs.Internal("failed to sign alipay payment")
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

func alipayRSA2Verify(values url.Values, publicKeyPEM string, signatureBase64 string) bool {
	publicKey, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureBase64))
	if err != nil {
		return false
	}
	signContent := alipaySignContent(values)
	digest := sha256.Sum256([]byte(signContent))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature) == nil
}

func wxPayVerifySignature(publicKeyPEM string, timestamp string, nonce string, body string, signatureBase64 string) bool {
	publicKey, err := parseRSAPublicKey(publicKeyPEM)
	if err != nil {
		return false
	}
	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(signatureBase64))
	if err != nil {
		return false
	}
	message := strings.TrimSpace(timestamp) + "\n" + strings.TrimSpace(nonce) + "\n" + body + "\n"
	digest := sha256.Sum256([]byte(message))
	return rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest[:], signature) == nil
}

func wxPayBuildAuthorization(method string, requestURI string, body string, mchID string, serial string, privateKeyPEM string, timestamp int64, nonce string) (string, *errs.Error) {
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", errs.BadRequest("wxpay merchant private key is invalid")
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	requestURI = strings.TrimSpace(requestURI)
	nonce = strings.TrimSpace(nonce)
	if method == "" || requestURI == "" || strings.TrimSpace(mchID) == "" || strings.TrimSpace(serial) == "" || nonce == "" {
		return "", errs.BadRequest("wxpay authorization fields are required")
	}
	message := fmt.Sprintf("%s\n%s\n%d\n%s\n%s\n", method, requestURI, timestamp, nonce, body)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errs.Internal("failed to sign wxpay request")
	}
	return fmt.Sprintf(`WECHATPAY2-SHA256-RSA2048 mchid="%s",nonce_str="%s",signature="%s",timestamp="%d",serial_no="%s"`,
		strings.TrimSpace(mchID),
		nonce,
		base64.StdEncoding.EncodeToString(signature),
		timestamp,
		strings.TrimSpace(serial),
	), nil
}

func wxPayBuildJSAPIClientToken(appID string, prepayID string, privateKeyPEM string) (string, *errs.Error) {
	privateKey, err := parseRSAPrivateKey(privateKeyPEM)
	if err != nil {
		return "", errs.BadRequest("wxpay merchant private key is invalid")
	}
	appID = strings.TrimSpace(appID)
	prepayID = strings.TrimSpace(prepayID)
	if appID == "" || prepayID == "" {
		return "", errs.BadRequest("wxpay jsapi app_id and prepay_id are required")
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce := uuid.NewString()
	packageValue := "prepay_id=" + prepayID
	message := fmt.Sprintf("%s\n%s\n%s\n%s\n", appID, timestamp, nonce, packageValue)
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", errs.Internal("failed to sign wxpay jsapi token")
	}
	token := map[string]string{
		"appId":     appID,
		"timeStamp": timestamp,
		"nonceStr":  nonce,
		"package":   packageValue,
		"signType":  "RSA",
		"paySign":   base64.StdEncoding.EncodeToString(signature),
	}
	body, err := json.Marshal(token)
	if err != nil {
		return "", errs.Internal("failed to build wxpay jsapi token")
	}
	return string(body), nil
}

type wxPayWebhookEnvelope struct {
	EventType string `json:"event_type"`
	Resource  struct {
		Algorithm      string `json:"algorithm"`
		Ciphertext     string `json:"ciphertext"`
		Nonce          string `json:"nonce"`
		AssociatedData string `json:"associated_data"`
	} `json:"resource"`
}

type wxPayTransactionResource struct {
	AppID         string `json:"appid"`
	MchID         string `json:"mchid"`
	OutTradeNo    string `json:"out_trade_no"`
	TransactionID string `json:"transaction_id"`
	TradeState    string `json:"trade_state"`
	Amount        struct {
		Total int64 `json:"total"`
	} `json:"amount"`
}

func decryptWxPayTransaction(body []byte, apiV3Key string) (wxPayTransactionResource, *errs.Error) {
	var envelope wxPayWebhookEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return wxPayTransactionResource{}, errs.BadRequest("invalid wxpay webhook body")
	}
	if envelope.Resource.Algorithm != "" && !strings.EqualFold(envelope.Resource.Algorithm, "AEAD_AES_256_GCM") {
		return wxPayTransactionResource{}, errs.BadRequest("unsupported wxpay resource algorithm")
	}
	key := []byte(strings.TrimSpace(apiV3Key))
	if len(key) != 32 {
		return wxPayTransactionResource{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(envelope.Resource.Ciphertext))
	if err != nil {
		return wxPayTransactionResource{}, errs.BadRequest("invalid wxpay resource ciphertext")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return wxPayTransactionResource{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return wxPayTransactionResource{}, errs.Internal("failed to initialize wxpay decryptor")
	}
	plaintext, err := gcm.Open(nil, []byte(envelope.Resource.Nonce), ciphertext, []byte(envelope.Resource.AssociatedData))
	if err != nil {
		return wxPayTransactionResource{}, errs.New(http.StatusBadRequest, errs.CodePaymentSignatureInvalid, "payment webhook signature is invalid")
	}
	var transaction wxPayTransactionResource
	if err := json.Unmarshal(plaintext, &transaction); err != nil {
		return wxPayTransactionResource{}, errs.BadRequest("invalid wxpay transaction resource")
	}
	return transaction, nil
}

func wxPayAmountCNYFromFen(totalFen int64) string {
	return decimal.NewFromInt(totalFen).Div(decimal.NewFromInt(100)).Round(5).StringFixed(5)
}

func wxPayAmountFenFromCNY(amountCNY string) (int64, *errs.Error) {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil || amount.LessThanOrEqual(decimal.Zero) {
		return 0, errs.BadRequest("amount_cny is invalid")
	}
	return amount.Mul(decimal.NewFromInt(100)).Round(0).IntPart(), nil
}

func parseRSAPrivateKey(raw string) (*rsa.PrivateKey, error) {
	if !strings.Contains(raw, "-----BEGIN") {
		raw = "-----BEGIN RSA PRIVATE KEY-----\n" + raw + "\n-----END RSA PRIVATE KEY-----"
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("invalid pem private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not rsa")
	}
	return key, nil
}

func parseRSAPublicKey(raw string) (*rsa.PublicKey, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "-----BEGIN") {
		raw = "-----BEGIN PUBLIC KEY-----\n" + raw + "\n-----END PUBLIC KEY-----"
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("invalid pem public key")
	}
	if parsed, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if key, ok := parsed.(*rsa.PublicKey); ok {
			return key, nil
		}
		return nil, fmt.Errorf("public key is not rsa")
	}
	if key, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return key, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("certificate public key is not rsa")
	}
	return key, nil
}

func alipaySignContent(values url.Values) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		value := strings.TrimSpace(values.Get(key))
		if key == "sign" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+values.Get(key))
	}
	return strings.Join(parts, "&")
}

func easyPaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if name == "sign" || name == "sign_type" || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, name := range keys {
		if index > 0 {
			_ = builder.WriteByte('&')
		}
		_, _ = builder.WriteString(name + "=" + params[name])
	}
	_, _ = builder.WriteString(key)
	sum := md5.Sum([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func jeepaySign(params map[string]string, key string) string {
	keys := make([]string, 0, len(params))
	for name, value := range params {
		if strings.EqualFold(name, "sign") || strings.EqualFold(name, "signType") || strings.TrimSpace(value) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, name := range keys {
		if index > 0 {
			_ = builder.WriteByte('&')
		}
		_, _ = builder.WriteString(name + "=" + params[name])
	}
	_, _ = builder.WriteString("&key=" + strings.TrimSpace(key))
	sum := md5.Sum([]byte(builder.String()))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func easyPaySignFromValues(values url.Values, key string) string {
	params := make(map[string]string, len(values))
	for name := range values {
		params[name] = values.Get(name)
	}
	return easyPaySign(params, key)
}

func jeepaySignFromValues(values url.Values, key string) string {
	params := make(map[string]string, len(values))
	for name := range values {
		params[name] = values.Get(name)
	}
	return jeepaySign(params, key)
}

func jeepayAmountFenFromCNY(amountCNY string) string {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountCNY))
	if err != nil {
		return "0"
	}
	return strconv.FormatInt(amount.Mul(decimal.NewFromInt(100)).Round(0).IntPart(), 10)
}

func jeepayAmountCNYFromFen(amountFen string) string {
	amount, err := decimal.NewFromString(strings.TrimSpace(amountFen))
	if err != nil {
		return ""
	}
	return amount.Div(decimal.NewFromInt(100)).Round(5).StringFixed(5)
}

func mapStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		value := strings.TrimSpace(fmt.Sprint(raw))
		if value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func mapJSONOrStringValue(values map[string]any, keys ...string) (string, *errs.Error) {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok || raw == nil {
			continue
		}
		if value, ok := raw.(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value, nil
			}
			continue
		}
		body, err := json.Marshal(raw)
		if err != nil {
			return "", errs.BadRequest(key + " must be a JSON string or object")
		}
		value := strings.TrimSpace(string(body))
		if value != "" && value != "null" {
			return value, nil
		}
	}
	return "", nil
}

func stringListContains(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func decodeCashierProviderInstanceRequest(w http.ResponseWriter, r *http.Request) (cashierProviderInstanceWriteRequest, bool) {
	var req cashierProviderInstanceWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return cashierProviderInstanceWriteRequest{}, false
	}
	return req, true
}

func normalizeStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	normalized := make(map[string]any, len(value))
	for key, raw := range value {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = raw
	}
	return normalized
}

func (a *API) cashierProviderInstances(ctx context.Context) []cashierProviderInstance {
	instances, err := a.cashierConfigFacade().ProviderInstances(ctx)
	if err != nil {
		return []cashierProviderInstance{}
	}
	return instances
}

func cashierProviderInstancePayloads(instances []cashierProviderInstance) []map[string]any {
	items := make([]map[string]any, 0, len(instances))
	for _, item := range instances {
		items = append(items, cashierProviderInstancePayload(item))
	}
	return items
}

func cashierProviderInstancePayload(item cashierProviderInstance) map[string]any {
	return cashierservice.ProviderInstancePayload(item)
}

func int64FromAny(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		return parsed
	default:
		return 0
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

func userGroupCodes(user *domainauth.User) []string {
	if user == nil {
		return []string{"basic"}
	}
	seen := map[string]struct{}{}
	codes := make([]string, 0, len(user.GroupCodes)+1)
	for _, code := range append([]string{user.GroupCode}, user.GroupCodes...) {
		normalized := strings.ToLower(strings.TrimSpace(code))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		codes = append(codes, normalized)
	}
	if len(codes) == 0 {
		return []string{"basic"}
	}
	return codes
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

func parseOptionalInt64Query(r *http.Request, key string) (int64, *errs.Error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errs.BadRequest("invalid " + key)
	}
	return parsed, nil
}

func splitAdminSuffix(path, prefix string) []string {
	suffix := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if suffix == "" {
		return []string{}
	}
	return strings.Split(suffix, "/")
}

func parseInt64Part(parts []string, index int, label string) (int64, *errs.Error) {
	if index >= len(parts) {
		return 0, errs.BadRequest("invalid " + label)
	}
	parsed, err := strconv.ParseInt(parts[index], 10, 64)
	if err != nil || parsed <= 0 {
		return 0, errs.BadRequest("invalid " + label)
	}
	return parsed, nil
}

func cloneMapAny(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
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

func handlerBillingString(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func isProductionAppEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}
