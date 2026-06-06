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
	"fmt"
	"io"
	"math/big"
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
	adminPerms domainadminauth.PermissionResolver
	cfg        config.Config
}

type cashierCustomAmountConfig struct {
	Enabled      bool   `json:"enabled"`
	MinAmountCNY string `json:"min_amount_cny"`
	MaxAmountCNY string `json:"max_amount_cny"`
	CNYPerPoint  string `json:"cny_per_point"`
}

type cashierVisibleMethod struct {
	Method             string `json:"method"`
	Label              string `json:"label"`
	Enabled            bool   `json:"enabled"`
	SourceProviderType string `json:"source_provider_type,omitempty"`
	SchedulerStrategy  string `json:"scheduler_strategy,omitempty"`
	DisplayOrder       int    `json:"display_order"`
	Description        string `json:"description,omitempty"`
}

type cashierProviderInstance struct {
	ID               int64          `json:"id"`
	ProviderType     string         `json:"provider_type"`
	Name             string         `json:"name"`
	Enabled          bool           `json:"enabled"`
	SupportedMethods []string       `json:"supported_methods"`
	SortOrder        int            `json:"sort_order"`
	SchedulerWeight  int            `json:"scheduler_weight"`
	Limits           map[string]any `json:"limits,omitempty"`
	Config           map[string]any `json:"config,omitempty"`
	ConfigStatus     string         `json:"config_status,omitempty"`
	LastError        string         `json:"last_error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

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

type adminCashierOrderSyncResult struct {
	ProviderType       string         `json:"provider_type"`
	ProviderInstanceID int64          `json:"provider_instance_id,omitempty"`
	QueryStatus        string         `json:"query_status"`
	RiskCategory       string         `json:"risk_category,omitempty"`
	ActionHint         string         `json:"action_hint,omitempty"`
	Paid               bool           `json:"paid"`
	Completed          bool           `json:"completed"`
	TradeNo            string         `json:"trade_no,omitempty"`
	AmountCNY          string         `json:"amount_cny,omitempty"`
	Message            string         `json:"message,omitempty"`
	Raw                map[string]any `json:"raw,omitempty"`
	SyncedAt           time.Time      `json:"synced_at"`
}

type adminCashierOrderSyncResponse struct {
	Order domainbilling.PaymentOrder  `json:"order"`
	Sync  adminCashierOrderSyncResult `json:"sync"`
}

func buildAdminCashierOrderSyncResult(instance cashierProviderInstance, order domainbilling.PaymentOrder, queryStatus cashierQueryStatus, tradeNo, amountCNY string, raw map[string]any) adminCashierOrderSyncResult {
	return adminCashierOrderSyncResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		QueryStatus:        queryStatus.Status,
		RiskCategory:       queryStatus.RiskCategory,
		ActionHint:         queryStatus.ActionHint,
		Paid:               queryStatus.Paid,
		TradeNo:            strings.TrimSpace(tradeNo),
		AmountCNY:          strings.TrimSpace(amountCNY),
		Message:            queryStatus.Message,
		Raw:                raw,
		SyncedAt:           time.Now().UTC(),
	}
}

type cashierProviderRefundResult struct {
	ProviderType       string         `json:"provider_type"`
	ProviderInstanceID int64          `json:"provider_instance_id,omitempty"`
	RefundStatus       string         `json:"refund_status"`
	RefundTradeNo      string         `json:"refund_trade_no"`
	ChannelRefundNo    string         `json:"channel_refund_no,omitempty"`
	Message            string         `json:"message,omitempty"`
	Raw                map[string]any `json:"raw,omitempty"`
	RefundedAt         time.Time      `json:"refunded_at"`
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
		adminPerms: domainadminauth.RolePermissionResolver{},
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
		result, err := a.billing.EnsureSignupTrialGrant(ctx, billingservice.SignupTrialGrantRequest{UserID: userID})
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
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	var req struct {
		PurchaseType  string `json:"purchase_type"`
		PlanCode      string `json:"plan_code"`
		AmountCNY     string `json:"amount_cny"`
		VisibleMethod string `json:"visible_method"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	result, createErr := a.createCashierOrder(r.Context(), user.ID, cashierOrderCreateInput{
		PurchaseType:   req.PurchaseType,
		PlanCode:       req.PlanCode,
		AmountCNY:      req.AmountCNY,
		VisibleMethod:  req.VisibleMethod,
		IdempotencyKey: strings.TrimSpace(r.Header.Get("Idempotency-Key")),
	})
	if createErr != nil {
		httpx.WriteError(w, r, createErr)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, result)
}

type cashierOrderCreateInput struct {
	PurchaseType   string
	PlanCode       string
	AmountCNY      string
	VisibleMethod  string
	IdempotencyKey string
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
			OrderNo:   orderNo,
			AmountCNY: strings.TrimSpace(req.AmountCNY),
			Subject:   "自定义充值",
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
		OrderNo:   orderNo,
		AmountCNY: plan.PriceCNY,
		Subject:   plan.PlanName,
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
	if strings.EqualFold(providerCode, "easypay") {
		result, appErr := a.handleEasyPayWebhook(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	if strings.EqualFold(providerCode, "alipay_direct") || strings.EqualFold(providerCode, "alipay") {
		result, appErr := a.handleAlipayWebhook(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	if strings.EqualFold(providerCode, "wxpay_direct") || strings.EqualFold(providerCode, "wxpay") || strings.EqualFold(providerCode, "wechatpay") {
		result, appErr := a.handleWxPayWebhook(r)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
		return
	}
	if strings.EqualFold(providerCode, "jeepay") || strings.HasPrefix(strings.ToLower(providerCode), "jeepay_") {
		result, appErr := a.handleJeePayWebhook(r, providerCode)
		if appErr != nil {
			httpx.WriteError(w, r, appErr)
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, result)
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
	return fmt.Sprintf("%s:%s:%s:%d", task.ID, task.Status, task.UpdatedAt.Format(time.RFC3339Nano), len(task.Results))
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
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "login required to view public image detail"))
		return
	}
	_, appErr := a.requireUserWithQueryToken(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	image, err := a.tasks.GetPublicImage(r.Context(), imageID)
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
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"today_order_count":          todayOrderCount,
		"today_completed_count":      todayCompletedCount,
		"today_amount_cny":           todayAmount.StringFixed(5),
		"success_rate":               successRate,
		"pending_count":              pendingCount,
		"failed_webhook_count":       failedWebhookCount,
		"enabled_methods":            enabledMethods,
		"enabled_provider_instances": len(enabledMethods),
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
	if r.Method != http.MethodPut {
		writeMethodNotAllowed(w, r)
		return
	}
	planID, parseErr := parseAdminCashierPlanID(r.URL.Path)
	if parseErr != nil {
		httpx.WriteError(w, r, parseErr)
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
		normalized, configErr := normalizeCashierCustomAmountConfig(req)
		if configErr != nil {
			httpx.WriteError(w, r, configErr)
			return
		}
		current, err := a.admin.GetTab(r.Context(), "payments")
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_, err = a.admin.UpdateTab(r.Context(), domainadminconfig.UpdateTabRequest{
			TabKey:  "payments",
			Version: current.Version,
			Items: []domainadminconfig.Item{
				configValueItem("payments", "custom_amount_enabled", normalized.Enabled),
				configValueItem("payments", "custom_amount_min_cny", normalized.MinAmountCNY),
				configValueItem("payments", "custom_amount_max_cny", normalized.MaxAmountCNY),
				configValueItem("payments", "custom_amount_cny_per_point", normalized.CNYPerPoint),
			},
			UpdatedBy: admin.AdminID,
		})
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.custom_amount_config.update", "cashier", "custom_amount_config", map[string]any{"enabled": normalized.Enabled}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, a.cashierCustomAmountConfig(r.Context()))
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
		normalized, configErr := normalizeCashierVisibleMethods(req.Items)
		if configErr != nil {
			httpx.WriteError(w, r, configErr)
			return
		}
		current, err := a.admin.GetTab(r.Context(), "payments")
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		_, err = a.admin.UpdateTab(r.Context(), domainadminconfig.UpdateTabRequest{
			TabKey:    "payments",
			Version:   current.Version,
			Items:     []domainadminconfig.Item{configValueItem("payments", "visible_methods", cashierVisibleMethodsConfigValue(normalized))},
			UpdatedBy: admin.AdminID,
		})
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
		items := cashierProviderInstancePayloads(a.cashierProviderInstances(r.Context()))
		httpx.WriteSuccess(w, r, http.StatusOK, pagedPayload(paginateAny(items, page, pageSize), page, pageSize, len(items)))
	case http.MethodPost:
		req, ok := decodeCashierProviderInstanceRequest(w, r)
		if !ok {
			return
		}
		normalized, normalizeErr := normalizeCashierProviderInstance(req, 0)
		if normalizeErr != nil {
			httpx.WriteError(w, r, normalizeErr)
			return
		}
		current := a.cashierProviderInstances(r.Context())
		normalized.ID = nextCashierProviderInstanceID(current)
		now := time.Now().UTC()
		normalized.CreatedAt = now
		normalized.UpdatedAt = now
		current = append(current, normalized)
		if err := a.saveCashierProviderInstances(r.Context(), current, admin.AdminID); err != nil {
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
	instances := a.cashierProviderInstances(r.Context())
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
		normalized, normalizeErr := normalizeCashierProviderInstance(req, instanceID)
		if normalizeErr != nil {
			httpx.WriteError(w, r, normalizeErr)
			return
		}
		normalized.CreatedAt = instances[index].CreatedAt
		if normalized.CreatedAt.IsZero() {
			normalized.CreatedAt = time.Now().UTC()
		}
		normalized.UpdatedAt = time.Now().UTC()
		instances[index] = normalized
		if err := a.saveCashierProviderInstances(r.Context(), instances, admin.AdminID); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "cashier.provider.update", "payment_provider_instance", fmt.Sprintf("%d", normalized.ID), map[string]any{"provider_type": normalized.ProviderType, "enabled": normalized.Enabled}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, cashierProviderInstancePayload(normalized))
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
	result, err := a.billing.ListOrders(r.Context(), domainbilling.ListOrdersRequest{Page: page, PageSize: pageSize})
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
	if order.Status == "refunded" || (order.Status != "completed" && order.Status != "partially_refunded") {
		return nil, nil
	}
	providerType := cashierOrderProviderType(order, cashierProviderInstance{})
	if providerType == "" || providerType == "mock" || strings.HasPrefix(providerType, "manual") {
		return nil, nil
	}
	instance, ok := a.cashierProviderInstanceForOrder(ctx, order)
	if !ok {
		return nil, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	providerType = cashierOrderProviderType(order, instance)
	switch providerType {
	case "alipay_direct":
		result, refundErr := a.refundAlipayCashierOrder(ctx, order, instance, refundTradeNo, refundAmountCNY, reason)
		return &result, refundErr
	case "wxpay_direct":
		result, refundErr := a.refundWxPayCashierOrder(ctx, order, instance, refundTradeNo, refundAmountCNY, reason)
		return &result, refundErr
	case "easypay_alipay", "easypay_wxpay":
		result, refundErr := a.refundEasyPayCashierOrder(ctx, order, instance, refundTradeNo, refundAmountCNY, reason)
		return &result, refundErr
	case "jeepay_alipay", "jeepay_wxpay":
		result, refundErr := a.refundJeePayCashierOrder(ctx, order, instance, refundTradeNo, refundAmountCNY, reason)
		return &result, refundErr
	default:
		return nil, nil
	}
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
		return defaultMockCashierProviderInstance(), true
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

func (a *API) queryCashierOrderStatus(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(order.ProviderType))
	}
	if strings.TrimSpace(mapStringValue(instance.Config, "query_status", "sync_status", "payment_status", "trade_status")) != "" || providerType == "mock" {
		return a.queryCashierOrderStatusFromConfig(order, instance)
	}
	switch providerType {
	case "alipay_direct":
		return a.queryAlipayCashierOrderStatus(ctx, order, instance)
	case "wxpay_direct":
		return a.queryWxPayCashierOrderStatus(ctx, order, instance)
	case "easypay_alipay", "easypay_wxpay":
		return a.queryEasyPayCashierOrderStatus(ctx, order, instance)
	case "jeepay_alipay", "jeepay_wxpay":
		return a.queryJeePayCashierOrderStatus(ctx, order, instance)
	default:
		return a.queryCashierOrderStatusFromConfig(order, instance)
	}
}

func (a *API) queryCashierOrderStatusFromConfig(order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	if providerType == "" {
		providerType = strings.ToLower(strings.TrimSpace(order.ProviderType))
	}
	now := time.Now().UTC()
	status := strings.ToLower(strings.TrimSpace(mapStringValue(instance.Config, "query_status", "sync_status", "payment_status", "trade_status")))
	if status == "" && order.Status == "completed" {
		status = "paid"
	}
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(mapStringValue(instance.Config, "query_trade_no", "sync_trade_no", "trade_no", "pay_order_id", "transaction_id"))
	if tradeNo == "" {
		tradeNo = order.TradeNo
	}
	amountCNY := strings.TrimSpace(mapStringValue(instance.Config, "query_amount_cny", "sync_amount_cny", "amount_cny", "money", "total_amount"))
	if amountCNY == "" {
		amountCNY = order.AmountCNY
	}
	raw := map[string]any{
		"source":        "provider_instance_config",
		"provider_type": providerType,
		"order_no":      order.OrderNo,
		"status":        status,
	}
	if tradeNo != "" {
		raw["trade_no"] = tradeNo
	}
	if amountCNY != "" {
		raw["amount_cny"] = amountCNY
	}
	queryStatus := normalizeCashierQueryStatus(status)
	result := buildAdminCashierOrderSyncResult(instance, order, queryStatus, tradeNo, amountCNY, raw)
	result.SyncedAt = now
	return result, nil
}

func (a *API) queryAlipayCashierOrderStatus(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	if appID == "" {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no": strings.TrimSpace(order.OrderNo),
	})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.query")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, mapStringValue(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return adminCashierOrderSyncResult{}, signErr
	}
	values.Set("sign", sign)
	endpoint := appendQuery(gatewayURL, values)
	body, appErr := getJSONForCashierQuery(ctx, endpoint, nil)
	if appErr != nil {
		return adminCashierOrderSyncResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	data := raw
	if nested, ok := raw["alipay_trade_query_response"].(map[string]any); ok {
		data = nested
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(data, "trade_status", "status")))
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(firstCashierRawString(data, "trade_no"))
	amountCNY := strings.TrimSpace(firstCashierRawString(data, "total_amount", "receipt_amount", "buyer_pay_amount"))
	queryStatus := normalizeCashierQueryStatus(status)
	raw["source"] = "alipay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return buildAdminCashierOrderSyncResult(instance, order, queryStatus, tradeNo, amountCNY, raw), nil
}

func (a *API) queryWxPayCashierOrderStatus(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	mchID := strings.TrimSpace(mapStringValue(instance.Config, "mch_id", "mchId", "merchant_id", "merchantId"))
	serial := strings.TrimSpace(mapStringValue(instance.Config, "merchant_certificate_serial", "merchantCertificateSerial", "merchant_serial_no", "serial_no"))
	privateKeyRaw := strings.TrimSpace(mapStringValue(instance.Config, "merchant_private_key", "private_key", "privateKey"))
	if appID == "" || mchID == "" || serial == "" || privateKeyRaw == "" {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	path := "/v3/pay/transactions/out-trade-no/" + url.PathEscape(strings.TrimSpace(order.OrderNo))
	values := url.Values{}
	values.Set("mchid", mchID)
	requestURI := path + "?" + values.Encode()
	endpoint := strings.TrimRight(gatewayURL, "/") + requestURI
	auth, signErr := wxPayBuildAuthorization(http.MethodGet, requestURI, "", mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return adminCashierOrderSyncResult{}, signErr
	}
	body, appErr := getJSONForCashierQuery(ctx, endpoint, map[string]string{"Authorization": auth})
	if appErr != nil {
		return adminCashierOrderSyncResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(raw, "trade_state", "status")))
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(firstCashierRawString(raw, "transaction_id", "trade_no"))
	amountCNY := ""
	if amountRaw, ok := raw["amount"].(map[string]any); ok {
		totalFen := firstCashierRawString(amountRaw, "total", "payer_total")
		if totalFen != "" {
			if amountFen, err := strconv.ParseInt(strings.TrimSpace(totalFen), 10, 64); err == nil {
				amountCNY = wxPayAmountCNYFromFen(amountFen)
			}
		}
	}
	queryStatus := normalizeCashierQueryStatus(status)
	raw["source"] = "wxpay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return buildAdminCashierOrderSyncResult(instance, order, queryStatus, tradeNo, amountCNY, raw), nil
}

func (a *API) queryEasyPayCashierOrderStatus(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	baseURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	baseURL = trimEasyPayEndpointBase(baseURL)
	pid := strings.TrimSpace(mapStringValue(instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	endpoint := strings.TrimSpace(mapStringValue(instance.Config, "query_url", "queryUrl"))
	if endpoint == "" {
		queryPath := strings.TrimSpace(mapStringValue(instance.Config, "query_path", "queryPath"))
		if queryPath == "" {
			queryPath = "/api.php"
		}
		if !strings.HasPrefix(queryPath, "/") {
			queryPath = "/" + queryPath
		}
		endpoint = strings.TrimRight(baseURL, "/") + queryPath
	}
	values := url.Values{}
	values.Set("act", "order")
	values.Set("pid", pid)
	values.Set("key", key)
	values.Set("out_trade_no", strings.TrimSpace(order.OrderNo))
	body, appErr := postFormForCashierQuery(ctx, endpoint, values)
	if appErr != nil {
		return adminCashierOrderSyncResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	status := strings.ToLower(strings.TrimSpace(cashierRawString(raw["status"])))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(cashierRawString(raw["trade_status"])))
	}
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(cashierRawString(raw["trade_no"]))
	if tradeNo == "" {
		tradeNo = strings.TrimSpace(cashierRawString(raw["api_trade_no"]))
	}
	amountCNY := strings.TrimSpace(cashierRawString(raw["money"]))
	if amountCNY == "" {
		amountCNY = strings.TrimSpace(cashierRawString(raw["amount_cny"]))
	}
	queryStatus := normalizeCashierQueryStatus(status)
	raw["source"] = "easypay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return buildAdminCashierOrderSyncResult(instance, order, queryStatus, tradeNo, amountCNY, raw), nil
}

func (a *API) queryJeePayCashierOrderStatus(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance) (adminCashierOrderSyncResult, *errs.Error) {
	baseURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	baseURL = trimJeePayEndpointBase(baseURL)
	mchNo := strings.TrimSpace(mapStringValue(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if mchNo == "" || appID == "" || key == "" {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	endpoint := strings.TrimSpace(mapStringValue(instance.Config, "query_url", "queryUrl"))
	if endpoint == "" {
		queryPath := strings.TrimSpace(mapStringValue(instance.Config, "query_path", "queryPath"))
		if queryPath == "" {
			queryPath = "/api/pay/query"
		}
		if !strings.HasPrefix(queryPath, "/") {
			queryPath = "/" + queryPath
		}
		endpoint = strings.TrimRight(baseURL, "/") + queryPath
	}
	params := map[string]string{
		"mchNo":      mchNo,
		"appId":      appID,
		"mchOrderNo": strings.TrimSpace(order.OrderNo),
		"signType":   "MD5",
	}
	sign := jeepaySign(params, key)
	params["sign"] = sign
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	body, appErr := postFormForCashierQuery(ctx, endpoint, values)
	if appErr != nil {
		return adminCashierOrderSyncResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return adminCashierOrderSyncResult{}, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		data = nested
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(data, "state", "status", "trade_state", "tradeStatus")))
	if status == "" {
		status = "pending"
	}
	tradeNo := strings.TrimSpace(firstCashierRawString(data, "payOrderId", "channelOrderNo", "trade_no", "tradeNo"))
	amountCNY := strings.TrimSpace(firstCashierRawString(data, "amount_cny", "amountCNY", "money", "total_amount", "totalAmount"))
	if amountCNY == "" {
		amountCNY = jeepayAmountCNYFromFen(firstCashierRawString(data, "amount"))
	}
	queryStatus := normalizeCashierQueryStatus(status)
	raw["source"] = "jeepay_query_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return buildAdminCashierOrderSyncResult(instance, order, queryStatus, tradeNo, amountCNY, raw), nil
}

func (a *API) refundAlipayCashierOrder(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance, refundTradeNo string, refundAmountCNY string, reason string) (cashierProviderRefundResult, *errs.Error) {
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	if appID == "" {
		return cashierProviderRefundResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no":   strings.TrimSpace(order.OrderNo),
		"refund_amount":  defaultString(strings.TrimSpace(refundAmountCNY), strings.TrimSpace(order.AmountCNY)),
		"refund_reason":  defaultString(strings.TrimSpace(reason), "cashier order refund"),
		"out_request_no": strings.TrimSpace(refundTradeNo),
	})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.refund")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("biz_content", string(bizContent))
	sign, signErr := alipayRSA2Sign(values, mapStringValue(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return cashierProviderRefundResult{}, signErr
	}
	values.Set("sign", sign)
	body, appErr := getJSONForCashierQuery(ctx, appendQuery(gatewayURL, values), nil)
	if appErr != nil {
		return cashierProviderRefundResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	data := raw
	if nested, ok := raw["alipay_trade_refund_response"].(map[string]any); ok {
		data = nested
	}
	code := strings.TrimSpace(firstCashierRawString(data, "code"))
	if code != "" && code != "10000" {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	if subCode := strings.TrimSpace(firstCashierRawString(data, "sub_code")); subCode != "" {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(data, "fund_change", "status")))
	if status == "" {
		status = "accepted"
	}
	channelRefundNo := strings.TrimSpace(firstCashierRawString(data, "trade_no", "out_trade_no"))
	raw["source"] = "alipay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return cashierProviderRefundResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(refundTradeNo),
		ChannelRefundNo:    channelRefundNo,
		Message:            strings.TrimSpace(firstCashierRawString(data, "msg", "sub_msg")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func (a *API) refundWxPayCashierOrder(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance, refundTradeNo string, refundAmountCNY string, reason string) (cashierProviderRefundResult, *errs.Error) {
	mchID := strings.TrimSpace(mapStringValue(instance.Config, "mch_id", "mchId", "merchant_id", "merchantId"))
	serial := strings.TrimSpace(mapStringValue(instance.Config, "merchant_certificate_serial", "merchantCertificateSerial", "merchant_serial_no", "serial_no"))
	privateKeyRaw := strings.TrimSpace(mapStringValue(instance.Config, "merchant_private_key", "private_key", "privateKey"))
	if mchID == "" || serial == "" || privateKeyRaw == "" {
		return cashierProviderRefundResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	totalFen, amountErr := wxPayAmountFenFromCNY(order.AmountCNY)
	if amountErr != nil {
		return cashierProviderRefundResult{}, amountErr
	}
	refundFen, refundAmountErr := wxPayAmountFenFromCNY(defaultString(strings.TrimSpace(refundAmountCNY), strings.TrimSpace(order.AmountCNY)))
	if refundAmountErr != nil {
		return cashierProviderRefundResult{}, refundAmountErr
	}
	payload := map[string]any{
		"out_trade_no":  strings.TrimSpace(order.OrderNo),
		"out_refund_no": strings.TrimSpace(refundTradeNo),
		"reason":        defaultString(strings.TrimSpace(reason), "cashier order refund"),
		"amount": map[string]any{
			"refund":   refundFen,
			"total":    totalFen,
			"currency": "CNY",
		},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return cashierProviderRefundResult{}, errs.Internal("failed to build wxpay refund request")
	}
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	requestURI := "/v3/refund/domestic/refunds"
	auth, signErr := wxPayBuildAuthorization(http.MethodPost, requestURI, string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return cashierProviderRefundResult{}, signErr
	}
	respBody, appErr := postJSONForCashierProvider(ctx, strings.TrimRight(gatewayURL, "/")+requestURI, body, map[string]string{"Authorization": auth})
	if appErr != nil {
		return cashierProviderRefundResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(respBody, &raw); err != nil {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(raw, "status", "refund_status")))
	if status == "abnormal" || status == "closed" {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "wxpay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return cashierProviderRefundResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(refundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstCashierRawString(raw, "refund_id", "channel_refund_no")),
		Message:            strings.TrimSpace(firstCashierRawString(raw, "message")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func (a *API) refundEasyPayCashierOrder(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance, refundTradeNo string, refundAmountCNY string, reason string) (cashierProviderRefundResult, *errs.Error) {
	baseURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return cashierProviderRefundResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	baseURL = trimEasyPayEndpointBase(baseURL)
	pid := strings.TrimSpace(mapStringValue(instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" {
		return cashierProviderRefundResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	endpoint := strings.TrimSpace(mapStringValue(instance.Config, "refund_url", "refundUrl"))
	if endpoint == "" {
		endpoint = strings.TrimRight(baseURL, "/") + "/api.php"
	}
	values := url.Values{}
	values.Set("act", "refund")
	values.Set("pid", pid)
	values.Set("key", key)
	values.Set("money", defaultString(strings.TrimSpace(refundAmountCNY), strings.TrimSpace(order.AmountCNY)))
	values.Set("out_trade_no", strings.TrimSpace(order.OrderNo))
	raw, appErr := postEasyPayRefundForm(ctx, endpoint, values)
	if appErr != nil && strings.TrimSpace(order.TradeNo) != "" && easypayRefundShouldRetryByTradeNo(raw) {
		values.Del("out_trade_no")
		values.Set("trade_no", strings.TrimSpace(order.TradeNo))
		raw, appErr = postEasyPayRefundForm(ctx, endpoint, values)
	}
	if appErr != nil {
		return cashierProviderRefundResult{}, appErr
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(raw, "status", "trade_status")))
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "easypay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	raw["refund_trade_no"] = strings.TrimSpace(refundTradeNo)
	return cashierProviderRefundResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(refundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstCashierRawString(raw, "refund_no", "trade_no", "api_trade_no")),
		Message:            strings.TrimSpace(firstCashierRawString(raw, "msg", "message")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func (a *API) refundJeePayCashierOrder(ctx context.Context, order domainbilling.PaymentOrder, instance cashierProviderInstance, refundTradeNo string, refundAmountCNY string, reason string) (cashierProviderRefundResult, *errs.Error) {
	baseURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return cashierProviderRefundResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	baseURL = trimJeePayEndpointBase(baseURL)
	mchNo := strings.TrimSpace(mapStringValue(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if mchNo == "" || appID == "" || key == "" {
		return cashierProviderRefundResult{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	endpoint := strings.TrimSpace(mapStringValue(instance.Config, "refund_url", "refundUrl"))
	if endpoint == "" {
		refundPath := strings.TrimSpace(mapStringValue(instance.Config, "refund_path", "refundPath"))
		if refundPath == "" {
			refundPath = "/api/refund/refundOrder"
		}
		if !strings.HasPrefix(refundPath, "/") {
			refundPath = "/" + refundPath
		}
		endpoint = strings.TrimRight(baseURL, "/") + refundPath
	}
	params := map[string]string{
		"mchNo":        mchNo,
		"appId":        appID,
		"mchOrderNo":   strings.TrimSpace(order.OrderNo),
		"mchRefundNo":  strings.TrimSpace(refundTradeNo),
		"refundAmount": jeepayAmountFenFromCNY(defaultString(strings.TrimSpace(refundAmountCNY), strings.TrimSpace(order.AmountCNY))),
		"currency":     "cny",
		"refundReason": defaultString(strings.TrimSpace(reason), "cashier order refund"),
		"reqTime":      time.Now().UTC().Format("20060102150405"),
		"version":      "1.0",
		"signType":     "MD5",
	}
	if tradeNo := strings.TrimSpace(order.TradeNo); tradeNo != "" {
		params["payOrderId"] = tradeNo
	}
	if clientIP := strings.TrimSpace(mapStringValue(instance.Config, "client_ip", "clientIp", "payer_client_ip", "payerClientIP")); clientIP != "" {
		params["clientIp"] = clientIP
	}
	if notifyURL := strings.TrimSpace(mapStringValue(instance.Config, "refund_notify_url", "refundNotifyUrl")); notifyURL != "" {
		params["notifyUrl"] = notifyURL
	}
	sign := jeepaySign(params, key)
	params["sign"] = sign
	values := url.Values{}
	for name, value := range params {
		values.Set(name, value)
	}
	body, appErr := postFormForCashierQuery(ctx, endpoint, values)
	if appErr != nil {
		return cashierProviderRefundResult{}, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	code := strings.TrimSpace(firstCashierRawString(raw, "code"))
	if code != "" && code != "0" {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		data = nested
	}
	status := strings.ToLower(strings.TrimSpace(firstCashierRawString(data, "state", "status", "refundState", "refund_status")))
	if status == "3" || status == "failed" || status == "fail" || status == "closed" {
		return cashierProviderRefundResult{}, cashierRefundProviderUnavailable()
	}
	if status == "" {
		status = "accepted"
	}
	raw["source"] = "jeepay_refund_api"
	raw["provider_type"] = strings.ToLower(strings.TrimSpace(instance.ProviderType))
	raw["order_no"] = order.OrderNo
	return cashierProviderRefundResult{
		ProviderType:       strings.ToLower(strings.TrimSpace(instance.ProviderType)),
		ProviderInstanceID: instance.ID,
		RefundStatus:       status,
		RefundTradeNo:      strings.TrimSpace(refundTradeNo),
		ChannelRefundNo:    strings.TrimSpace(firstCashierRawString(data, "refundOrderId", "channelOrderNo", "refund_no", "refundNo")),
		Message:            strings.TrimSpace(firstCashierRawString(raw, "msg", "message")),
		Raw:                raw,
		RefundedAt:         time.Now().UTC(),
	}, nil
}

func postEasyPayRefundForm(ctx context.Context, endpoint string, values url.Values) (map[string]any, *errs.Error) {
	body, appErr := postFormForCashierQuery(ctx, endpoint, values)
	if appErr != nil {
		return nil, appErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, cashierRefundProviderUnavailable()
	}
	code := strings.TrimSpace(firstCashierRawString(raw, "code"))
	if code != "1" {
		return raw, cashierRefundProviderUnavailable()
	}
	return raw, nil
}

func easypayRefundShouldRetryByTradeNo(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(firstCashierRawString(raw, "msg", "message", "error")))
	return strings.Contains(message, "not found") ||
		strings.Contains(message, "no order") ||
		strings.Contains(message, "不存在") ||
		strings.Contains(message, "未找到")
}

func postFormForCashierQuery(ctx context.Context, endpoint string, values url.Values) ([]byte, *errs.Error) {
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if reqErr != nil {
		return nil, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return body, nil
}

func postJSONForCashierProvider(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, *errs.Error) {
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if reqErr != nil {
		return nil, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return respBody, nil
}

func getJSONForCashierQuery(ctx context.Context, endpoint string, headers map[string]string) ([]byte, *errs.Error) {
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if reqErr != nil {
		return nil, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	httpReq.Header.Set("Accept", "application/json")
	for key, value := range headers {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			httpReq.Header.Set(key, value)
		}
	}
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return body, nil
}

func cashierRefundProviderUnavailable() *errs.Error {
	return errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
}

func firstCashierRawString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(cashierRawString(raw[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func cashierRawString(raw any) string {
	switch value := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return strings.TrimSpace(value.String())
	case float64:
		return decimal.NewFromFloat(value).String()
	case float32:
		return decimal.NewFromFloat32(value).String()
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case uint:
		return strconv.FormatUint(uint64(value), 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	case uint32:
		return strconv.FormatUint(uint64(value), 10)
	case bool:
		if value {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

type cashierQueryStatus struct {
	Status       string
	RiskCategory string
	ActionHint   string
	Paid         bool
	Message      string
}

func normalizeCashierQueryStatus(status string) cashierQueryStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "paid", "success", "succeeded", "completed", "complete", "trade_success", "trade_finished", "1", "2":
		return cashierQueryStatus{Status: "paid", RiskCategory: "paid", ActionHint: "渠道已确认支付，可核对本地到账状态。", Paid: true, Message: "渠道订单已支付"}
	case "pending", "processing", "process", "wait", "waiting", "created", "new", "0", "wait_buyer_pay", "userpaying", "notpay":
		return cashierQueryStatus{Status: "pending", RiskCategory: "pending", ActionHint: "渠道仍未确认支付，稍后可再次查单。", Message: "渠道订单未支付或仍在处理中"}
	case "closed", "close", "canceled", "cancelled", "cancel", "expired", "trade_closed", "revoked", "3":
		return cashierQueryStatus{Status: "closed", RiskCategory: "closed", ActionHint: "渠道订单已关闭，建议取消当前订单并让用户重新创建订单。", Message: "渠道订单已关闭"}
	case "limited", "limit", "quota_limited", "amount_limited", "frequency_limited", "rate_limited", "exceed_limit", "over_limit":
		return cashierQueryStatus{Status: "failed", RiskCategory: "channel_limited", ActionHint: "渠道订单触发限额限制，建议切换备用渠道、降低单笔金额或调整渠道实例限额后再重试。", Message: "渠道订单触发限额限制"}
	case "sign_error", "signature_error", "invalid_sign", "verify_failed", "signature_invalid", "bad_signature", "sign_invalid":
		return cashierQueryStatus{Status: "failed", RiskCategory: "signature_error", ActionHint: "渠道验签或签名配置异常，请检查商户密钥、证书、公钥、回调地址和签名算法配置。", Message: "渠道验签或签名配置异常"}
	case "amount_mismatch", "money_mismatch", "total_amount_mismatch", "fee_mismatch", "price_mismatch":
		return cashierQueryStatus{Status: "failed", RiskCategory: "amount_mismatch", ActionHint: "渠道订单金额与本地订单不一致，请暂停到账并核对订单金额、汇率、渠道费率和回调原文。", Message: "渠道订单金额与本地订单不一致"}
	case "merchant_disabled", "mch_disabled", "account_disabled", "merchant_abnormal", "account_abnormal", "merchant_closed", "account_closed":
		return cashierQueryStatus{Status: "failed", RiskCategory: "account_abnormal", ActionHint: "渠道商户账号状态异常，建议切换备用账号并登录渠道后台确认商户状态和产品权限。", Message: "渠道商户账号状态异常"}
	case "timeout", "timed_out", "query_timeout", "network_timeout", "gateway_timeout", "request_timeout":
		return cashierQueryStatus{Status: "failed", RiskCategory: "channel_timeout", ActionHint: "渠道查单超时或网络异常，建议稍后重试；连续失败时检查网关地址、网络出口和渠道可用性。", Message: "渠道查单超时或网络异常"}
	case "failed", "failure", "fail", "error", "payerror", "pay_error", "trade_failed", "4":
		return cashierQueryStatus{Status: "failed", RiskCategory: "channel_error", ActionHint: "渠道返回异常状态，请结合原始响应、商户后台和回调事件继续排查。", Message: "渠道订单支付失败"}
	case "risk", "risk_control", "fraud", "intercepted", "security", "blocked":
		return cashierQueryStatus{Status: "failed", RiskCategory: "risk_control", ActionHint: "渠道侧风控或安全策略拦截，建议让用户更换支付渠道或重新创建订单后再支付。", Message: "渠道订单被风控拦截"}
	case "refunded", "refund", "partially_refunded", "partial_refund", "trade_refund":
		return cashierQueryStatus{Status: "refunded", RiskCategory: "refunded", ActionHint: "渠道显示已退款，请核对本地退款流水和用户充值余额是否一致。", Message: "渠道订单已退款"}
	default:
		return cashierQueryStatus{Status: "pending", RiskCategory: "pending", ActionHint: "渠道仍未确认支付，稍后可再次查单。", Message: "渠道订单未支付或仍在处理中"}
	}
}

func cashierQueryStatusIsPaid(status string) bool {
	return normalizeCashierQueryStatus(status).Paid
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

func (a *API) HandleDocsExamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	items := []map[string]any{
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
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": items,
	})
}

func (a *API) HandleDocsErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	items := []map[string]any{
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
		{"code": errs.CodePaymentProviderUnavailable, "http_status": http.StatusConflict, "message": "No enabled payment provider instance can process this order.", "retryable": true},
		{"code": errs.CodePaymentProviderNotImplemented, "http_status": http.StatusNotImplemented, "message": "The selected payment provider adapter is not implemented.", "retryable": false},
		{"code": errs.CodePaymentTooManyPending, "http_status": http.StatusConflict, "message": "The user has too many pending payment orders.", "retryable": false},
		{"code": errs.CodePaymentSignatureInvalid, "http_status": http.StatusForbidden, "message": "Payment webhook signature verification failed.", "retryable": false},
		{"code": errs.CodePaymentAmountMismatch, "http_status": http.StatusConflict, "message": "Payment webhook amount does not match the order amount.", "retryable": false},
		{"code": errs.CodeUpstreamUnavailable, "http_status": http.StatusServiceUnavailable, "message": "Upstream model provider is temporarily unavailable.", "retryable": true},
	}
	codes := make([]string, 0, len(items))
	for _, item := range items {
		codes = append(codes, fmt.Sprint(item["code"]))
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{
		"items": items,
		"codes": codes,
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

	providerModels, err := a.modelAdmin.ListProviderModels(ctx, domainmodeladmin.ProviderModelListRequest{Page: 1, PageSize: 200, Enabled: &enabled})
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
	checks = append(checks, a.signupTrialReadinessCheck(checkedAt))
	checks = append(checks, a.publicGalleryReadinessCheck(ctx, checkedAt))
	checks = append(checks, a.docsReadinessCheck(checkedAt))
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

func (a *API) signupTrialReadinessCheck(checkedAt time.Time) adminReadinessCheck {
	trial := a.cfg.Billing.SignupTrial
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

func (a *API) docsReadinessCheck(checkedAt time.Time) adminReadinessCheck {
	if strings.TrimSpace(a.cfg.Docs.Title) == "" || strings.TrimSpace(a.cfg.Docs.BasePath) == "" {
		return readinessCheck("docs", "开发文档", "warn", "开发文档标题或 base path 未配置", "config", "去配置", checkedAt)
	}
	return readinessCheck("docs", "开发文档", "pass", "OpenAPI 与示例文档路由已注册", "config", "去配置", checkedAt)
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
	cfg := cashierCustomAmountConfig{
		Enabled:      true,
		MinAmountCNY: "1.00000",
		MaxAmountCNY: "999.00000",
		CNYPerPoint:  handlerBillingString(a.cfg.Billing.CNYPerPoint, "0.31250"),
	}
	tab, err := a.admin.GetTab(ctx, "payments")
	if err != nil {
		return cfg
	}
	for _, item := range tab.Items {
		value, ok := item.ConfigValue["value"]
		if !ok {
			continue
		}
		switch item.ConfigKey {
		case "custom_amount_enabled":
			if parsed, ok := configBoolValue(value); ok {
				cfg.Enabled = parsed
			}
		case "custom_amount_min_cny":
			if parsed, ok := configStringValue(value); ok {
				cfg.MinAmountCNY = parsed
			}
		case "custom_amount_max_cny":
			if parsed, ok := configStringValue(value); ok {
				cfg.MaxAmountCNY = parsed
			}
		case "custom_amount_cny_per_point":
			if parsed, ok := configStringValue(value); ok {
				cfg.CNYPerPoint = parsed
			}
		}
	}
	normalized, err := normalizeCashierCustomAmountConfig(cfg)
	if err != nil {
		return cfg
	}
	return normalized
}

func normalizeCashierCustomAmountConfig(cfg cashierCustomAmountConfig) (cashierCustomAmountConfig, *errs.Error) {
	minAmount, minAmountValue, err := positiveDecimalString(cfg.MinAmountCNY, "min_amount_cny")
	if err != nil {
		return cashierCustomAmountConfig{}, err
	}
	maxAmount, maxAmountValue, err := positiveDecimalString(cfg.MaxAmountCNY, "max_amount_cny")
	if err != nil {
		return cashierCustomAmountConfig{}, err
	}
	cnyPerPoint, _, err := positiveDecimalString(cfg.CNYPerPoint, "cny_per_point")
	if err != nil {
		return cashierCustomAmountConfig{}, err
	}
	if minAmountValue.GreaterThan(maxAmountValue) {
		return cashierCustomAmountConfig{}, errs.BadRequest("min_amount_cny must be less than or equal to max_amount_cny")
	}
	return cashierCustomAmountConfig{
		Enabled:      cfg.Enabled,
		MinAmountCNY: minAmount,
		MaxAmountCNY: maxAmount,
		CNYPerPoint:  cnyPerPoint,
	}, nil
}

func positiveDecimalString(raw, field string) (string, decimal.Decimal, *errs.Error) {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if err != nil || !value.IsPositive() {
		return "", decimal.Zero, errs.BadRequest(field + " must be positive")
	}
	return value.StringFixed(5), value, nil
}

func validateCashierCustomAmount(raw string, cfg cashierCustomAmountConfig) *errs.Error {
	_, amount, err := positiveDecimalString(raw, "amount_cny")
	if err != nil {
		return err
	}
	_, minAmount, err := positiveDecimalString(cfg.MinAmountCNY, "min_amount_cny")
	if err != nil {
		return err
	}
	_, maxAmount, err := positiveDecimalString(cfg.MaxAmountCNY, "max_amount_cny")
	if err != nil {
		return err
	}
	if amount.LessThan(minAmount) {
		return errs.BadRequest("amount_cny must be greater than or equal to min_amount_cny")
	}
	if amount.GreaterThan(maxAmount) {
		return errs.BadRequest("amount_cny must be less than or equal to max_amount_cny")
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

func (a *API) cashierVisibleMethods(ctx context.Context, includeDisabled bool) []cashierVisibleMethod {
	methods := defaultCashierVisibleMethods()
	if tab, err := a.admin.GetTab(ctx, "payments"); err == nil {
		for _, item := range tab.Items {
			if item.ConfigKey != "visible_methods" {
				continue
			}
			if parsed, parseErr := parseCashierVisibleMethodsConfig(item.ConfigValue["value"]); parseErr == nil {
				methods = parsed
			}
			break
		}
	}
	filtered := make([]cashierVisibleMethod, 0, len(methods))
	for _, method := range methods {
		if !includeDisabled && !method.Enabled {
			continue
		}
		if !includeDisabled && isProductionAppEnv(a.cfg.App.Env) && method.SourceProviderType == "mock" {
			continue
		}
		filtered = append(filtered, method)
	}
	return filtered
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
	_, amount, amountErr := positiveDecimalString(amountCNY, "amount_cny")
	if amountErr != nil {
		return cashierProviderInstance{}, amountErr
	}
	candidates := make([]cashierProviderInstance, 0)
	for _, instance := range a.cashierProviderInstances(ctx) {
		if !instance.Enabled {
			continue
		}
		if isProductionAppEnv(a.cfg.App.Env) && instance.ProviderType == "mock" {
			continue
		}
		if instance.ProviderType != method.SourceProviderType {
			continue
		}
		if !stringListContains(instance.SupportedMethods, method.Method) {
			continue
		}
		if !cashierProviderInstanceAmountAllowed(instance, amount) {
			continue
		}
		if instance.ProviderType != "mock" && instance.ConfigStatus != "configured" {
			continue
		}
		candidates = append(candidates, instance)
	}
	if len(candidates) == 0 {
		return cashierProviderInstance{}, errs.New(http.StatusConflict, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if method.SchedulerStrategy == "random" && len(candidates) > 1 {
		return randomCashierProviderInstance(candidates), nil
	}
	if method.SchedulerStrategy == "round_robin" && len(candidates) > 1 {
		selected := a.nextRoundRobinCashierProviderInstance(ctx, method, candidates)
		return selected, nil
	}
	return candidates[0], nil
}

func randomCashierProviderInstance(candidates []cashierProviderInstance) cashierProviderInstance {
	return randomCashierProviderInstanceWithReader(rand.Reader, candidates)
}

func randomCashierProviderInstanceWithReader(reader io.Reader, candidates []cashierProviderInstance) cashierProviderInstance {
	if len(candidates) == 0 {
		return cashierProviderInstance{}
	}
	index, err := rand.Int(reader, big.NewInt(int64(len(candidates))))
	if err != nil {
		return candidates[0]
	}
	return candidates[int(index.Int64())]
}

func (a *API) nextRoundRobinCashierProviderInstance(ctx context.Context, method cashierVisibleMethod, candidates []cashierProviderInstance) cashierProviderInstance {
	state := a.cashierSchedulerState(ctx)
	key := cashierSchedulerStateKey(method)
	lastID := int64FromAny(state[key]["last_instance_id"])
	nextIndex := 0
	if lastID > 0 {
		for index, candidate := range candidates {
			if candidate.ID == lastID {
				nextIndex = (index + 1) % len(candidates)
				break
			}
		}
	}
	selected := candidates[nextIndex]
	state[key] = map[string]any{"last_instance_id": selected.ID}
	if err := a.saveCashierSchedulerState(ctx, state); err != nil {
		return candidates[0]
	}
	return selected
}

func cashierSchedulerStateKey(method cashierVisibleMethod) string {
	return strings.ToLower(strings.TrimSpace(method.Method)) + ":" + strings.ToLower(strings.TrimSpace(method.SourceProviderType))
}

func (a *API) cashierSchedulerState(ctx context.Context) map[string]map[string]any {
	state := map[string]map[string]any{}
	if tab, err := a.admin.GetTab(ctx, "payments"); err == nil {
		for _, item := range tab.Items {
			if item.ConfigKey != "scheduler_state" {
				continue
			}
			if parsed := parseCashierSchedulerState(item.ConfigValue["value"]); len(parsed) > 0 {
				state = parsed
			}
			break
		}
	}
	return state
}

func parseCashierSchedulerState(raw any) map[string]map[string]any {
	state := map[string]map[string]any{}
	if raw == nil {
		return state
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return state
	}
	if err := json.Unmarshal(encoded, &state); err == nil {
		return state
	}
	var loose map[string]any
	if err := json.Unmarshal(encoded, &loose); err != nil {
		return state
	}
	for key, value := range loose {
		nested, ok := value.(map[string]any)
		if !ok {
			continue
		}
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		state[trimmedKey] = nested
	}
	return state
}

func (a *API) saveCashierSchedulerState(ctx context.Context, state map[string]map[string]any) error {
	current, err := a.admin.GetTab(ctx, "payments")
	if err != nil {
		return err
	}
	value := make(map[string]any, len(state))
	for key, item := range state {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		value[trimmedKey] = normalizeMap(item)
	}
	_, err = a.admin.UpdateTab(ctx, domainadminconfig.UpdateTabRequest{
		TabKey:    "payments",
		Version:   current.Version,
		Items:     []domainadminconfig.Item{configValueItem("payments", "scheduler_state", value)},
		UpdatedBy: 0,
	})
	return err
}

func cashierProviderInstanceAmountAllowed(instance cashierProviderInstance, amount decimal.Decimal) bool {
	if raw, ok := instance.Limits["min_amount_cny"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		_, minAmount, err := positiveDecimalString(fmt.Sprint(raw), "min_amount_cny")
		if err != nil || amount.LessThan(minAmount) {
			return false
		}
	}
	if raw, ok := instance.Limits["max_amount_cny"]; ok && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		_, maxAmount, err := positiveDecimalString(fmt.Sprint(raw), "max_amount_cny")
		if err != nil || amount.GreaterThan(maxAmount) {
			return false
		}
	}
	return true
}

type cashierPaymentDisplayRequest struct {
	OrderNo   string
	AmountCNY string
	Subject   string
}

type cashierPaymentBuildResult struct {
	Display     map[string]any
	PaymentURL  string
	QRCode      string
	ClientToken string
}

func newCashierOrderNo() string {
	now := time.Now().UTC()
	return fmt.Sprintf("PGO-%d-%06d", now.Unix(), now.Nanosecond()%1000000)
}

func (a *API) cashierPaymentDisplay(ctx context.Context, method cashierVisibleMethod, instance cashierProviderInstance, req cashierPaymentDisplayRequest) (cashierPaymentBuildResult, *errs.Error) {
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	display := map[string]any{
		"type":                 "redirect",
		"visible_method":       strings.ToLower(strings.TrimSpace(method.Method)),
		"provider_type":        providerType,
		"provider_instance_id": instance.ID,
		"order_no":             strings.TrimSpace(req.OrderNo),
		"amount_cny":           strings.TrimSpace(req.AmountCNY),
	}
	result := cashierPaymentBuildResult{Display: display}
	switch providerType {
	case "mock":
		display["type"] = "mock"
	case "easypay_alipay", "easypay_wxpay":
		paymentType := "alipay"
		if providerType == "easypay_wxpay" {
			paymentType = "wxpay"
		}
		prepayMode := strings.ToLower(strings.TrimSpace(mapStringValue(instance.Config, "payment_mode", "prepay_mode", "trade_type")))
		if prepayMode == "api" || prepayMode == "qrcode" || prepayMode == "qr_code" {
			paymentURL, qrCode, sign, buildErr := a.buildEasyPayAPIPayment(ctx, instance, req, paymentType)
			if buildErr != nil {
				return cashierPaymentBuildResult{}, buildErr
			}
			result.PaymentURL = paymentURL
			result.QRCode = qrCode
			display["type"] = "redirect"
			if qrCode != "" {
				display["type"] = "qr_code"
				display["qr_code"] = qrCode
			}
			if paymentURL != "" {
				display["payment_url"] = paymentURL
			}
			display["prepay_mode"] = "api"
			display["sign"] = sign
			display["sign_type"] = "MD5"
			break
		}
		paymentURL, sign, buildErr := a.buildEasyPayPaymentURL(instance, req, paymentType)
		if buildErr != nil {
			return cashierPaymentBuildResult{}, buildErr
		}
		result.PaymentURL = paymentURL
		display["type"] = "redirect"
		display["payment_url"] = paymentURL
		display["sign"] = sign
		display["sign_type"] = "MD5"
	case "alipay_direct":
		paymentURL, signed, buildErr := a.buildAlipayPaymentURL(instance, req)
		if buildErr != nil {
			return cashierPaymentBuildResult{}, buildErr
		}
		result.PaymentURL = paymentURL
		display["type"] = "redirect"
		display["payment_url"] = paymentURL
		display["signed"] = signed
		display["sign_type"] = "RSA2"
	case "jeepay_alipay", "jeepay_wxpay":
		prepayMode := strings.ToLower(strings.TrimSpace(mapStringValue(instance.Config, "payment_mode", "prepay_mode", "trade_type")))
		if prepayMode == "api" || prepayMode == "qrcode" || prepayMode == "qr_code" {
			paymentURL, qrCode, sign, wayCode, channelTradeNo, buildErr := a.buildJeePayAPIPayment(ctx, instance, req)
			if buildErr != nil {
				return cashierPaymentBuildResult{}, buildErr
			}
			result.PaymentURL = paymentURL
			result.QRCode = qrCode
			display["type"] = "redirect"
			if qrCode != "" {
				display["type"] = "qr_code"
				display["qr_code"] = qrCode
			}
			if paymentURL != "" {
				display["payment_url"] = paymentURL
			}
			display["prepay_mode"] = "api"
			display["sign"] = sign
			display["sign_type"] = "MD5"
			display["way_code"] = wayCode
			if channelTradeNo != "" {
				display["channel_trade_no"] = channelTradeNo
			}
			break
		}
		paymentURL, sign, wayCode, buildErr := a.buildJeePayPaymentURL(instance, req)
		if buildErr != nil {
			return cashierPaymentBuildResult{}, buildErr
		}
		result.PaymentURL = paymentURL
		display["type"] = "redirect"
		display["payment_url"] = paymentURL
		display["sign"] = sign
		display["sign_type"] = "MD5"
		display["way_code"] = wayCode
	case "wxpay_direct":
		result.QRCode = strings.TrimSpace(mapStringValue(instance.Config, "qr_code", "qrcode", "code_url"))
		result.PaymentURL = strings.TrimSpace(mapStringValue(instance.Config, "payment_url", "pay_url", "h5_url"))
		result.ClientToken = strings.TrimSpace(mapStringValue(instance.Config, "client_token"))
		prepayMode := strings.ToLower(strings.TrimSpace(mapStringValue(instance.Config, "payment_mode", "prepay_mode", "trade_type")))
		if result.ClientToken == "" && prepayMode == "jsapi" {
			clientToken, buildErr := a.buildWxPayJSAPIClientToken(ctx, instance, req)
			if buildErr != nil {
				return cashierPaymentBuildResult{}, buildErr
			}
			result.ClientToken = clientToken
			display["type"] = "jsapi"
			display["prepay_mode"] = "jsapi"
		}
		if result.PaymentURL == "" && prepayMode == "h5" {
			paymentURL, buildErr := a.buildWxPayH5PaymentURL(ctx, instance, req)
			if buildErr != nil {
				return cashierPaymentBuildResult{}, buildErr
			}
			result.PaymentURL = paymentURL
			display["prepay_mode"] = "h5"
		}
		if result.QRCode == "" && result.PaymentURL == "" && result.ClientToken == "" {
			codeURL, buildErr := a.buildWxPayNativeCodeURL(ctx, instance, req)
			if buildErr != nil {
				return cashierPaymentBuildResult{}, buildErr
			}
			result.QRCode = codeURL
			display["prepay_mode"] = "native"
		}
		if result.QRCode == "" && result.PaymentURL == "" && result.ClientToken == "" {
			return cashierPaymentBuildResult{}, errs.New(http.StatusNotImplemented, errs.CodePaymentProviderNotImplemented, "payment provider is not implemented")
		}
		if result.QRCode != "" {
			display["type"] = "qr_code"
			display["qr_code"] = result.QRCode
		}
		if result.PaymentURL != "" {
			display["payment_url"] = result.PaymentURL
		}
		if result.ClientToken != "" {
			display["client_token"] = result.ClientToken
		}
	default:
		return cashierPaymentBuildResult{}, errs.New(http.StatusNotImplemented, errs.CodePaymentProviderNotImplemented, "payment provider is not implemented")
	}
	return result, nil
}

func (a *API) buildWxPayJSAPIClientToken(ctx context.Context, instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, *errs.Error) {
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	mchID := strings.TrimSpace(mapStringValue(instance.Config, "mch_id", "mchId", "merchant_id", "merchantId"))
	serial := strings.TrimSpace(mapStringValue(instance.Config, "merchant_certificate_serial", "merchantCertificateSerial", "merchant_serial_no", "serial_no"))
	privateKeyRaw := strings.TrimSpace(mapStringValue(instance.Config, "merchant_private_key", "private_key", "privateKey"))
	openID := strings.TrimSpace(mapStringValue(instance.Config, "openid", "open_id", "payer_openid", "payerOpenID"))
	if appID == "" || mchID == "" || serial == "" || privateKeyRaw == "" {
		return "", errs.BadRequest("wxpay app_id, mch_id, merchant_certificate_serial, and merchant_private_key are required")
	}
	if openID == "" {
		return "", errs.BadRequest("wxpay jsapi openid is required")
	}
	totalFen, amountErr := wxPayAmountFenFromCNY(req.AmountCNY)
	if amountErr != nil {
		return "", amountErr
	}
	notifyURL, _ := a.cashierCallbackURLs(instance, "wxpay_direct")
	payload := map[string]any{
		"appid":        appID,
		"mchid":        mchID,
		"description":  defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"amount": map[string]any{
			"total":    totalFen,
			"currency": "CNY",
		},
		"payer": map[string]any{
			"openid": openID,
		},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", errs.Internal("failed to build wxpay jsapi request")
	}
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	endpoint := strings.TrimRight(gatewayURL, "/") + "/v3/pay/transactions/jsapi"
	httpReq, buildErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if buildErr != nil {
		return "", errs.BadRequest("invalid wxpay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	auth, signErr := wxPayBuildAuthorization(http.MethodPost, httpReq.URL.RequestURI(), string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return "", signErr
	}
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	var parsed struct {
		PrepayID string `json:"prepay_id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.PrepayID) == "" {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	token, tokenErr := wxPayBuildJSAPIClientToken(appID, strings.TrimSpace(parsed.PrepayID), privateKeyRaw)
	if tokenErr != nil {
		return "", tokenErr
	}
	return token, nil
}

func (a *API) buildWxPayH5PaymentURL(ctx context.Context, instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, *errs.Error) {
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	mchID := strings.TrimSpace(mapStringValue(instance.Config, "mch_id", "mchId", "merchant_id", "merchantId"))
	serial := strings.TrimSpace(mapStringValue(instance.Config, "merchant_certificate_serial", "merchantCertificateSerial", "merchant_serial_no", "serial_no"))
	privateKeyRaw := strings.TrimSpace(mapStringValue(instance.Config, "merchant_private_key", "private_key", "privateKey"))
	if appID == "" || mchID == "" || serial == "" || privateKeyRaw == "" {
		return "", errs.BadRequest("wxpay app_id, mch_id, merchant_certificate_serial, and merchant_private_key are required")
	}
	totalFen, amountErr := wxPayAmountFenFromCNY(req.AmountCNY)
	if amountErr != nil {
		return "", amountErr
	}
	notifyURL, _ := a.cashierCallbackURLs(instance, "wxpay_direct")
	payload := map[string]any{
		"appid":        appID,
		"mchid":        mchID,
		"description":  defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"amount": map[string]any{
			"total":    totalFen,
			"currency": "CNY",
		},
		"scene_info": map[string]any{
			"payer_client_ip": defaultString(strings.TrimSpace(mapStringValue(instance.Config, "client_ip", "payer_client_ip", "payerClientIP")), "127.0.0.1"),
			"h5_info": map[string]any{
				"type": defaultString(strings.TrimSpace(mapStringValue(instance.Config, "h5_type", "h5InfoType")), "Wap"),
			},
		},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", errs.Internal("failed to build wxpay h5 request")
	}
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	endpoint := strings.TrimRight(gatewayURL, "/") + "/v3/pay/transactions/h5"
	httpReq, buildErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if buildErr != nil {
		return "", errs.BadRequest("invalid wxpay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	auth, signErr := wxPayBuildAuthorization(http.MethodPost, httpReq.URL.RequestURI(), string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return "", signErr
	}
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	var parsed struct {
		H5URL string `json:"h5_url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.H5URL) == "" {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return strings.TrimSpace(parsed.H5URL), nil
}

func (a *API) buildAlipayPaymentURL(instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, bool, *errs.Error) {
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "gatewayUrl"))
	if gatewayURL == "" {
		gatewayURL = "https://openapi.alipaydev.com/gateway.do"
	}
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	if appID == "" {
		return "", false, errs.BadRequest("alipay app_id is required")
	}
	notifyURL, returnURL := a.cashierCallbackURLs(instance, "alipay_direct")
	bizContent, _ := json.Marshal(map[string]string{
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"total_amount": strings.TrimSpace(req.AmountCNY),
		"subject":      defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"product_code": "FAST_INSTANT_TRADE_PAY",
	})
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("method", "alipay.trade.page.pay")
	values.Set("charset", "utf-8")
	values.Set("sign_type", "RSA2")
	values.Set("timestamp", time.Now().UTC().Format("2006-01-02 15:04:05"))
	values.Set("version", "1.0")
	values.Set("notify_url", notifyURL)
	values.Set("return_url", returnURL)
	values.Set("biz_content", string(bizContent))
	values.Set("out_trade_no", strings.TrimSpace(req.OrderNo))
	values.Set("total_amount", strings.TrimSpace(req.AmountCNY))
	sign, signErr := alipayRSA2Sign(values, mapStringValue(instance.Config, "app_private_key", "private_key", "privateKey"))
	if signErr != nil {
		return "", false, signErr
	}
	values.Set("sign", sign)
	return appendQuery(gatewayURL, values), true, nil
}

func (a *API) buildEasyPayPaymentURL(instance cashierProviderInstance, req cashierPaymentDisplayRequest, paymentType string) (string, string, *errs.Error) {
	baseURL, params, sign, _, buildErr := a.buildEasyPayPaymentParams(instance, req, paymentType)
	if buildErr != nil {
		return "", "", buildErr
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return strings.TrimRight(baseURL, "/") + "/submit.php?" + values.Encode(), sign, nil
}

func (a *API) buildJeePayPaymentURL(instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, string, string, *errs.Error) {
	baseURL, params, sign, wayCode, buildErr := a.buildJeePayPaymentParams(instance, req)
	if buildErr != nil {
		return "", "", "", buildErr
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	return strings.TrimRight(baseURL, "/") + "/api/pay/unifiedOrder?" + values.Encode(), sign, wayCode, nil
}

func (a *API) buildJeePayAPIPayment(ctx context.Context, instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, string, string, string, string, *errs.Error) {
	baseURL, params, sign, wayCode, buildErr := a.buildJeePayPaymentParams(instance, req)
	if buildErr != nil {
		return "", "", "", "", "", buildErr
	}
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/api/pay/unifiedOrder"
	body, postErr := postFormForCashierQuery(ctx, endpoint, values)
	if postErr != nil {
		return "", "", "", "", "", postErr
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	code := strings.TrimSpace(firstCashierRawString(raw, "code"))
	if code != "0" && code != "1" && !strings.EqualFold(code, "success") {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	data := raw
	if nested, ok := raw["data"].(map[string]any); ok {
		data = nested
	}
	paymentURL := strings.TrimSpace(firstCashierRawString(data, "payUrl", "pay_url", "payurl", "payURL", "cashierUrl", "cashier_url"))
	qrCode := strings.TrimSpace(firstCashierRawString(data, "codeUrl", "code_url", "qrCode", "qr_code", "qrcode", "payData", "pay_data"))
	channelTradeNo := strings.TrimSpace(firstCashierRawString(data, "payOrderId", "pay_order_id", "trade_no", "tradeNo", "channelOrderNo"))
	if paymentURL == "" && qrCode == "" {
		return "", "", "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return paymentURL, qrCode, sign, wayCode, channelTradeNo, nil
}

func (a *API) buildJeePayPaymentParams(instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, map[string]string, string, string, *errs.Error) {
	baseURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return "", nil, "", "", errs.BadRequest("jeepay gateway_url is required")
	}
	baseURL = trimJeePayEndpointBase(baseURL)
	mchNo := strings.TrimSpace(mapStringValue(instance.Config, "mch_no", "mchNo", "merchant_id", "merchantId"))
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "api_key", "apiKey", "merchant_key", "merchantKey"))
	if mchNo == "" || appID == "" || key == "" {
		return "", nil, "", "", errs.BadRequest("jeepay mch_no, app_id, and key are required")
	}
	providerType := strings.ToLower(strings.TrimSpace(instance.ProviderType))
	wayCode := strings.TrimSpace(mapStringValue(instance.Config, "way_code", "wayCode"))
	if wayCode == "" {
		if providerType == "jeepay_wxpay" {
			wayCode = "WX_NATIVE"
		} else {
			wayCode = "ALI_PC"
		}
	}
	notifyURL, returnURL := a.cashierCallbackURLs(instance, providerType)
	params := map[string]string{
		"mchNo":      mchNo,
		"appId":      appID,
		"wayCode":    wayCode,
		"mchOrderNo": strings.TrimSpace(req.OrderNo),
		"amount":     jeepayAmountFenFromCNY(req.AmountCNY),
		"currency":   "cny",
		"subject":    defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"body":       defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"notifyUrl":  notifyURL,
		"returnUrl":  returnURL,
		"clientIp":   defaultString(strings.TrimSpace(mapStringValue(instance.Config, "client_ip", "clientIp", "payer_client_ip", "payerClientIP")), "127.0.0.1"),
		"signType":   "MD5",
	}
	channelExtra, channelExtraErr := mapJSONOrStringValue(instance.Config, "channel_extra", "channelExtra", "channel_extra_json", "channelExtraJSON")
	if channelExtraErr != nil {
		return "", nil, "", "", channelExtraErr
	}
	if channelExtra != "" {
		params["channelExtra"] = channelExtra
	}
	sign := jeepaySign(params, key)
	params["sign"] = sign
	return baseURL, params, sign, wayCode, nil
}

func (a *API) buildEasyPayAPIPayment(ctx context.Context, instance cashierProviderInstance, req cashierPaymentDisplayRequest, paymentType string) (string, string, string, *errs.Error) {
	baseURL, params, sign, key, buildErr := a.buildEasyPayPaymentParams(instance, req, paymentType)
	if buildErr != nil {
		return "", "", "", buildErr
	}
	params["clientip"] = defaultString(strings.TrimSpace(mapStringValue(instance.Config, "client_ip", "clientip", "payer_client_ip", "payerClientIP")), "127.0.0.1")
	if device := strings.TrimSpace(mapStringValue(instance.Config, "device")); device != "" {
		params["device"] = device
	}
	sign = easyPaySign(params, key)
	params["sign"] = sign
	params["sign_type"] = "MD5"
	values := url.Values{}
	for key, value := range params {
		values.Set(key, value)
	}
	endpoint := strings.TrimRight(baseURL, "/") + "/mapi.php"
	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if reqErr != nil {
		return "", "", "", errs.BadRequest("invalid easypay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	var parsed struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		TradeNo string `json:"trade_no"`
		PayURL  string `json:"payurl"`
		PayURL2 string `json:"payurl2"`
		QRCode  string `json:"qrcode"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.Code != 1 {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	paymentURL := strings.TrimSpace(parsed.PayURL)
	if paymentURL == "" {
		paymentURL = strings.TrimSpace(parsed.PayURL2)
	}
	qrCode := strings.TrimSpace(parsed.QRCode)
	if paymentURL == "" && qrCode == "" {
		return "", "", "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return paymentURL, qrCode, sign, nil
}

func (a *API) buildEasyPayPaymentParams(instance cashierProviderInstance, req cashierPaymentDisplayRequest, paymentType string) (string, map[string]string, string, string, *errs.Error) {
	baseURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if baseURL == "" {
		return "", nil, "", "", errs.BadRequest("easypay gateway_url is required")
	}
	baseURL = trimEasyPayEndpointBase(baseURL)
	pid := strings.TrimSpace(mapStringValue(instance.Config, "pid", "merchant_id", "merchantId"))
	key := strings.TrimSpace(mapStringValue(instance.Config, "key", "pkey", "merchant_key", "merchantKey"))
	if pid == "" || key == "" {
		return "", nil, "", "", errs.BadRequest("easypay pid and key are required")
	}
	notifyURL, returnURL := a.cashierCallbackURLs(instance, strings.ToLower(strings.TrimSpace(instance.ProviderType)))
	params := map[string]string{
		"pid":          pid,
		"type":         paymentType,
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"return_url":   returnURL,
		"name":         defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"money":        strings.TrimSpace(req.AmountCNY),
	}
	if cid := strings.TrimSpace(mapStringValue(instance.Config, "cid")); cid != "" {
		params["cid"] = cid
	}
	sign := easyPaySign(params, key)
	params["sign"] = sign
	params["sign_type"] = "MD5"
	return baseURL, params, sign, key, nil
}

func (a *API) buildWxPayNativeCodeURL(ctx context.Context, instance cashierProviderInstance, req cashierPaymentDisplayRequest) (string, *errs.Error) {
	appID := strings.TrimSpace(mapStringValue(instance.Config, "app_id", "appId"))
	mchID := strings.TrimSpace(mapStringValue(instance.Config, "mch_id", "mchId", "merchant_id", "merchantId"))
	serial := strings.TrimSpace(mapStringValue(instance.Config, "merchant_certificate_serial", "merchantCertificateSerial", "merchant_serial_no", "serial_no"))
	privateKeyRaw := strings.TrimSpace(mapStringValue(instance.Config, "merchant_private_key", "private_key", "privateKey"))
	if appID == "" || mchID == "" || serial == "" || privateKeyRaw == "" {
		return "", errs.BadRequest("wxpay app_id, mch_id, merchant_certificate_serial, and merchant_private_key are required")
	}
	totalFen, amountErr := wxPayAmountFenFromCNY(req.AmountCNY)
	if amountErr != nil {
		return "", amountErr
	}
	notifyURL, _ := a.cashierCallbackURLs(instance, "wxpay_direct")
	payload := map[string]any{
		"appid":        appID,
		"mchid":        mchID,
		"description":  defaultString(strings.TrimSpace(req.Subject), "Pic Gallery 充值"),
		"out_trade_no": strings.TrimSpace(req.OrderNo),
		"notify_url":   notifyURL,
		"amount": map[string]any{
			"total":    totalFen,
			"currency": "CNY",
		},
	}
	body, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return "", errs.Internal("failed to build wxpay native request")
	}
	gatewayURL := strings.TrimSpace(mapStringValue(instance.Config, "gateway_url", "api_base", "apiBase"))
	if gatewayURL == "" {
		gatewayURL = "https://api.mch.weixin.qq.com"
	}
	endpoint := strings.TrimRight(gatewayURL, "/") + "/v3/pay/transactions/native"
	httpReq, buildErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if buildErr != nil {
		return "", errs.BadRequest("invalid wxpay gateway_url")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	auth, signErr := wxPayBuildAuthorization(http.MethodPost, httpReq.URL.RequestURI(), string(body), mchID, serial, privateKeyRaw, time.Now().Unix(), uuid.NewString())
	if signErr != nil {
		return "", signErr
	}
	httpReq.Header.Set("Authorization", auth)
	httpReq.Header.Set("Accept", "application/json")
	resp, doErr := http.DefaultClient.Do(httpReq)
	if doErr != nil {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	defer resp.Body.Close()
	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	var parsed struct {
		CodeURL string `json:"code_url"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || strings.TrimSpace(parsed.CodeURL) == "" {
		return "", errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "payment provider instance is unavailable")
	}
	return strings.TrimSpace(parsed.CodeURL), nil
}

func (a *API) cashierCallbackURLs(instance cashierProviderInstance, providerType string) (string, string) {
	notifyURL := strings.TrimSpace(mapStringValue(instance.Config, "notify_url", "notifyUrl"))
	returnURL := strings.TrimSpace(mapStringValue(instance.Config, "return_url", "returnUrl"))
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

func defaultCashierVisibleMethods() []cashierVisibleMethod {
	return []cashierVisibleMethod{
		{
			Method:             "mock",
			Label:              "Mock 支付",
			Enabled:            true,
			SourceProviderType: "mock",
			SchedulerStrategy:  "round_robin",
			DisplayOrder:       10,
			Description:        "测试环境模拟支付链路",
		},
	}
}

func parseCashierVisibleMethodsConfig(raw any) ([]cashierVisibleMethod, *errs.Error) {
	if raw == nil {
		return defaultCashierVisibleMethods(), nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errs.BadRequest("visible_methods must be an array")
	}
	var items []cashierVisibleMethod
	if err := json.Unmarshal(encoded, &items); err != nil {
		return nil, errs.BadRequest("visible_methods must be an array")
	}
	return normalizeCashierVisibleMethods(items)
}

func normalizeCashierVisibleMethods(items []cashierVisibleMethod) ([]cashierVisibleMethod, *errs.Error) {
	normalized := make([]cashierVisibleMethod, 0, len(items))
	seen := map[string]struct{}{}
	for index, item := range items {
		item.Method = strings.ToLower(strings.TrimSpace(item.Method))
		if item.Method == "" {
			return nil, errs.BadRequest("visible method is required")
		}
		if _, ok := seen[item.Method]; ok {
			return nil, errs.BadRequest("visible method is duplicated")
		}
		seen[item.Method] = struct{}{}
		item.Label = strings.TrimSpace(item.Label)
		if item.Label == "" {
			item.Label = item.Method
		}
		item.SourceProviderType = strings.ToLower(strings.TrimSpace(item.SourceProviderType))
		if item.SourceProviderType == "" {
			item.SourceProviderType = item.Method
		}
		item.SchedulerStrategy = strings.ToLower(strings.TrimSpace(item.SchedulerStrategy))
		if item.SchedulerStrategy == "" {
			item.SchedulerStrategy = "round_robin"
		}
		if item.SchedulerStrategy != "round_robin" && item.SchedulerStrategy != "random" {
			return nil, errs.BadRequest("scheduler_strategy must be round_robin or random")
		}
		if item.DisplayOrder <= 0 {
			item.DisplayOrder = (index + 1) * 10
		}
		if !cashierVisibleMethodProviderAllowed(item.Method, item.SourceProviderType) {
			return nil, errs.BadRequest("source_provider_type is not allowed for method " + item.Method)
		}
		normalized = append(normalized, item)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].DisplayOrder != normalized[j].DisplayOrder {
			return normalized[i].DisplayOrder < normalized[j].DisplayOrder
		}
		return normalized[i].Method < normalized[j].Method
	})
	return normalized, nil
}

func cashierVisibleMethodProviderAllowed(method, provider string) bool {
	switch method {
	case "mock":
		return provider == "mock"
	case "alipay":
		return provider == "alipay_direct" || provider == "easypay_alipay" || provider == "mock" || provider == "jeepay_alipay"
	case "wxpay":
		return provider == "wxpay_direct" || provider == "easypay_wxpay" || provider == "mock" || provider == "jeepay_wxpay"
	default:
		return false
	}
}

func cashierVisibleMethodsConfigValue(items []cashierVisibleMethod) []map[string]any {
	values := make([]map[string]any, 0, len(items))
	for _, item := range items {
		values = append(values, map[string]any{
			"method":               item.Method,
			"label":                item.Label,
			"enabled":              item.Enabled,
			"source_provider_type": item.SourceProviderType,
			"scheduler_strategy":   item.SchedulerStrategy,
			"display_order":        item.DisplayOrder,
			"description":          item.Description,
		})
	}
	return values
}

func decodeCashierProviderInstanceRequest(w http.ResponseWriter, r *http.Request) (cashierProviderInstance, bool) {
	var req cashierProviderInstance
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return cashierProviderInstance{}, false
	}
	return req, true
}

func normalizeCashierProviderInstance(req cashierProviderInstance, instanceID int64) (cashierProviderInstance, *errs.Error) {
	req.ID = instanceID
	req.ProviderType = strings.ToLower(strings.TrimSpace(req.ProviderType))
	if !cashierProviderTypeAllowed(req.ProviderType) {
		return cashierProviderInstance{}, errs.BadRequest("provider_type is not supported")
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return cashierProviderInstance{}, errs.BadRequest("name is required")
	}
	req.SupportedMethods = normalizeStringList(req.SupportedMethods)
	if len(req.SupportedMethods) == 0 {
		req.SupportedMethods = defaultMethodsForCashierProviderType(req.ProviderType)
	}
	for _, method := range req.SupportedMethods {
		if !cashierProviderSupportsMethod(req.ProviderType, method) {
			return cashierProviderInstance{}, errs.BadRequest("supported_methods is not allowed for provider_type")
		}
	}
	if req.SchedulerWeight <= 0 {
		req.SchedulerWeight = 100
	}
	if req.SortOrder < 0 {
		req.SortOrder = 0
	}
	req.Limits = normalizeMap(req.Limits)
	if limitErr := validateCashierProviderLimits(req.Limits); limitErr != nil {
		return cashierProviderInstance{}, limitErr
	}
	req.Config = normalizeMap(req.Config)
	req.ConfigStatus = "missing"
	if len(req.Config) > 0 || req.ProviderType == "mock" {
		req.ConfigStatus = "configured"
	}
	req.LastError = strings.TrimSpace(req.LastError)
	return req, nil
}

func cashierProviderTypeAllowed(providerType string) bool {
	switch providerType {
	case "mock", "alipay_direct", "wxpay_direct", "easypay_alipay", "easypay_wxpay", "jeepay_alipay", "jeepay_wxpay":
		return true
	default:
		return false
	}
}

func cashierProviderSupportsMethod(providerType, method string) bool {
	method = strings.ToLower(strings.TrimSpace(method))
	switch providerType {
	case "mock":
		return method == "mock" || method == "alipay" || method == "wxpay"
	case "alipay_direct", "easypay_alipay", "jeepay_alipay":
		return method == "alipay"
	case "wxpay_direct", "easypay_wxpay", "jeepay_wxpay":
		return method == "wxpay"
	default:
		return false
	}
}

func defaultMethodsForCashierProviderType(providerType string) []string {
	switch providerType {
	case "wxpay_direct", "easypay_wxpay", "jeepay_wxpay":
		return []string{"wxpay"}
	case "mock":
		return []string{"mock"}
	default:
		return []string{"alipay"}
	}
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

func validateCashierProviderLimits(limits map[string]any) *errs.Error {
	for _, key := range []string{"min_amount_cny", "max_amount_cny", "daily_amount_limit_cny"} {
		raw, ok := limits[key]
		if !ok || raw == nil || strings.TrimSpace(fmt.Sprint(raw)) == "" {
			continue
		}
		formatted, _, err := positiveDecimalString(fmt.Sprint(raw), key)
		if err != nil {
			return err
		}
		limits[key] = formatted
	}
	if minRaw, minOK := limits["min_amount_cny"]; minOK {
		if maxRaw, maxOK := limits["max_amount_cny"]; maxOK {
			_, minValue, minErr := positiveDecimalString(fmt.Sprint(minRaw), "min_amount_cny")
			_, maxValue, maxErr := positiveDecimalString(fmt.Sprint(maxRaw), "max_amount_cny")
			if minErr != nil {
				return minErr
			}
			if maxErr != nil {
				return maxErr
			}
			if minValue.GreaterThan(maxValue) {
				return errs.BadRequest("min_amount_cny must be less than or equal to max_amount_cny")
			}
		}
	}
	return nil
}

func (a *API) cashierProviderInstances(ctx context.Context) []cashierProviderInstance {
	instances := []cashierProviderInstance{}
	if tab, err := a.admin.GetTab(ctx, "payments"); err == nil {
		for _, item := range tab.Items {
			if item.ConfigKey != "provider_instances" {
				continue
			}
			if parsed, parseErr := parseCashierProviderInstancesConfig(item.ConfigValue["value"]); parseErr == nil {
				instances = parsed
			}
			break
		}
	}
	if len(instances) == 0 && !isProductionAppEnv(a.cfg.App.Env) {
		instances = append(instances, defaultMockCashierProviderInstance())
	}
	sort.SliceStable(instances, func(i, j int) bool {
		if instances[i].SortOrder != instances[j].SortOrder {
			return instances[i].SortOrder < instances[j].SortOrder
		}
		return instances[i].ID < instances[j].ID
	})
	return instances
}

func parseCashierProviderInstancesConfig(raw any) ([]cashierProviderInstance, *errs.Error) {
	if raw == nil {
		return []cashierProviderInstance{}, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, errs.BadRequest("provider_instances must be an array")
	}
	var instances []cashierProviderInstance
	if err := json.Unmarshal(encoded, &instances); err != nil {
		return nil, errs.BadRequest("provider_instances must be an array")
	}
	normalized := make([]cashierProviderInstance, 0, len(instances))
	seen := map[int64]struct{}{}
	now := time.Now().UTC()
	for _, item := range instances {
		item.ID = int64FromAny(item.ID)
		if item.ID <= 0 {
			continue
		}
		if _, ok := seen[item.ID]; ok {
			continue
		}
		seen[item.ID] = struct{}{}
		parsed, normalizeErr := normalizeCashierProviderInstance(item, item.ID)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		parsed.CreatedAt = item.CreatedAt
		if parsed.CreatedAt.IsZero() {
			parsed.CreatedAt = now
		}
		parsed.UpdatedAt = item.UpdatedAt
		if parsed.UpdatedAt.IsZero() {
			parsed.UpdatedAt = parsed.CreatedAt
		}
		normalized = append(normalized, parsed)
	}
	return normalized, nil
}

func (a *API) saveCashierProviderInstances(ctx context.Context, instances []cashierProviderInstance, adminID int64) error {
	current, err := a.admin.GetTab(ctx, "payments")
	if err != nil {
		return err
	}
	values := make([]map[string]any, 0, len(instances))
	for _, item := range instances {
		values = append(values, cashierProviderInstanceConfigValue(item))
	}
	_, err = a.admin.UpdateTab(ctx, domainadminconfig.UpdateTabRequest{
		TabKey:    "payments",
		Version:   current.Version,
		Items:     []domainadminconfig.Item{configValueItem("payments", "provider_instances", values)},
		UpdatedBy: adminID,
	})
	return err
}

func nextCashierProviderInstanceID(instances []cashierProviderInstance) int64 {
	var maxID int64
	for _, item := range instances {
		if item.ID > maxID {
			maxID = item.ID
		}
	}
	return maxID + 1
}

func defaultMockCashierProviderInstance() cashierProviderInstance {
	now := time.Now().UTC()
	return cashierProviderInstance{
		ID:               1,
		ProviderType:     "mock",
		Name:             "Mock Payment",
		Enabled:          true,
		SupportedMethods: []string{"mock"},
		SortOrder:        10,
		SchedulerWeight:  1,
		Limits: map[string]any{
			"min_amount_cny": "1.00000",
			"max_amount_cny": "999.00000",
		},
		ConfigStatus: "configured",
		Config:       map[string]any{"mock": true},
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func cashierProviderInstanceConfigValue(item cashierProviderInstance) map[string]any {
	return map[string]any{
		"id":                item.ID,
		"provider_type":     item.ProviderType,
		"name":              item.Name,
		"enabled":           item.Enabled,
		"supported_methods": item.SupportedMethods,
		"sort_order":        item.SortOrder,
		"scheduler_weight":  item.SchedulerWeight,
		"limits":            normalizeMap(item.Limits),
		"config":            normalizeMap(item.Config),
		"config_status":     item.ConfigStatus,
		"last_error":        item.LastError,
		"created_at":        item.CreatedAt,
		"updated_at":        item.UpdatedAt,
	}
}

func cashierProviderInstancePayloads(instances []cashierProviderInstance) []map[string]any {
	items := make([]map[string]any, 0, len(instances))
	for _, item := range instances {
		items = append(items, cashierProviderInstancePayload(item))
	}
	return items
}

func cashierProviderInstancePayload(item cashierProviderInstance) map[string]any {
	return map[string]any{
		"id":                 item.ID,
		"provider_type":      item.ProviderType,
		"name":               item.Name,
		"enabled":            item.Enabled,
		"supported_methods":  item.SupportedMethods,
		"sort_order":         item.SortOrder,
		"scheduler_weight":   item.SchedulerWeight,
		"limits":             normalizeMap(item.Limits),
		"config":             redactCashierProviderConfig(item.Config),
		"config_status":      item.ConfigStatus,
		"credentials_status": cashierCredentialsStatus(item.Config, item.UpdatedAt),
		"last_error":         item.LastError,
		"created_at":         item.CreatedAt,
		"updated_at":         item.UpdatedAt,
	}
}

func redactCashierProviderConfig(config map[string]any) map[string]any {
	redacted := map[string]any{}
	for key, value := range normalizeMap(config) {
		if cashierConfigKeyIsSecret(key) {
			continue
		}
		redacted[key] = value
	}
	return redacted
}

func cashierCredentialsStatus(config map[string]any, updatedAt time.Time) map[string]any {
	secretMaterial := cashierSecretMaterial(config)
	if strings.TrimSpace(secretMaterial) == "" {
		return map[string]any{"has_secret": false}
	}
	sum := sha256.Sum256([]byte(secretMaterial))
	status := map[string]any{
		"has_secret":  true,
		"fingerprint": fmt.Sprintf("sha256:%x", sum[:8]),
	}
	if !updatedAt.IsZero() {
		status["updated_at"] = updatedAt
	}
	return status
}

func cashierSecretMaterial(config map[string]any) string {
	parts := make([]string, 0)
	for key, value := range normalizeMap(config) {
		if cashierConfigKeyIsSecret(key) {
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func cashierConfigKeyIsSecret(key string) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "key", "pkey", "api_v3_key", "apiv3_key", "mch_key", "merchant_key":
		return true
	default:
		return strings.Contains(key, "secret") || strings.Contains(key, "private_key") || strings.Contains(key, "token") || strings.Contains(key, "mch_key") || strings.Contains(key, "api_key")
	}
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
