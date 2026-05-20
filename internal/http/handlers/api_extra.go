package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

type adminClaims struct {
	AdminID int64  `json:"admin_id"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	jwt.RegisteredClaims
}

func (a *API) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token"))
		return
	}
	_, claims, err := a.auth.ValidateAccessToken(strings.TrimPrefix(authHeader, "Bearer "))
	if err != nil {
		httpx.WriteError(w, r, err.(*errs.Error))
		return
	}
	if err := a.auth.Logout(claims.SessionID); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: a.cfg.Auth.RefreshCookieName, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Unix(0, 0), MaxAge: -1})
	w.WriteHeader(http.StatusNoContent)
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
	var req struct{ OldPassword, NewPassword string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if err := a.auth.ChangePassword(user.ID, req.OldPassword, req.NewPassword); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "ok"})
}

func (a *API) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct{ Email, Code, NewPassword string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	if err := a.auth.ResetPassword(req.Email, req.Code, req.NewPassword); err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"status": "ok"})
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
	var req struct{ Theme, DefaultLocale *string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	updated, err := a.auth.UpdateProfile(user.ID, nil, nil, req.Theme, req.DefaultLocale, nil)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	writeProfile(w, r, updated)
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
	key := asset.ObjectKey
	updated, err := a.auth.UpdateProfile(user.ID, nil, nil, nil, nil, &key)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	writeProfile(w, r, updated)
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
	summary, err := a.billing.RedeemCode(r.Context(), billingservice.RedeemCodeRequest{
		UserID:         user.ID,
		Code:           req.Code,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	})
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, summary)
}

func (a *API) HandleAPIKeys(w http.ResponseWriter, r *http.Request) {
	user, appErr := a.requireUser(r)
	if appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	switch r.Method {
	case http.MethodGet:
		keys, err := a.apiKeys.ListKeys(r.Context(), user.ID)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": keys})
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
		created, err := a.apiKeys.CreateKey(r.Context(), structToCreateKey(user.ID, req.Name, user.GroupCode, req.TotalQuotaPoints, req.DailyQuotaPoints, req.RPMLimit, req.ExpiresAt))
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusCreated, map[string]any{"api_key": created.Key, "secret": created.Secret})
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleAPIKeyDetail(w http.ResponseWriter, r *http.Request) {
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
		id, parseErr := strconv.ParseInt(strings.TrimSuffix(path, "/reset-secret"), 10, 64)
		if parseErr != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid api key id"))
			return
		}
		result, err := a.apiKeys.ResetSecret(r.Context(), user.ID, id)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"api_key": result.Key, "secret": result.Secret})
		return
	}
	id, parseErr := strconv.ParseInt(strings.Trim(path, "/"), 10, 64)
	if parseErr != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid api key id"))
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name             *string    `json:"name"`
			Status           *string    `json:"status"`
			GroupCode        *string    `json:"group_code"`
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
		expiresAt := &req.ExpiresAt
		updated, err := a.apiKeys.UpdateKey(r.Context(), structToUpdateKey(user.ID, id, req.Name, req.Status, nil, req.TotalQuotaPoints, req.DailyQuotaPoints, req.RPMLimit, expiresAt))
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	case http.MethodDelete:
		if err := a.apiKeys.DeleteKey(r.Context(), user.ID, id); err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
	}
}

func (a *API) HandleOpenBalance(w http.ResponseWriter, r *http.Request) {
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

func (a *API) HandleOpenCapabilities(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireOpenAPIKey(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, a.caps.List())
}

func (a *API) HandleOpenTaskDetail(w http.ResponseWriter, r *http.Request) {
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

func (a *API) HandleOpenReferenceAssetMultipart(w http.ResponseWriter, r *http.Request) {
	identity, appErr := a.requireOpenAPIKey(r)
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
	content, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, r, errs.BadRequest("failed to read upload"))
		return
	}
	apiKeyID := identity.APIKeyID
	asset, err := a.assets.UploadWithMetadata(identity.UserID, header.Filename, header.Header.Get("Content-Type"), content, structToAssetMetadata(&apiKeyID, "openapi"))
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusCreated, asset)
}

func (a *API) HandleOpenReferenceAssetGet(w http.ResponseWriter, r *http.Request) {
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

func (a *API) HandleImageDownload(w http.ResponseWriter, r *http.Request) {
	if _, appErr := a.requireUser(r); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	if r.Method != http.MethodGet {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	imageID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/agent/image/v1/images/"), "/download")
	if strings.TrimSpace(imageID) == "" {
		httpx.WriteError(w, r, errs.BadRequest("image id is required"))
		return
	}
	httpx.WriteError(w, r, errs.New(http.StatusNotFound, errs.CodeNotFound, "image download is not available for this storage object"))
}

func (a *API) HandleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteError(w, r, errs.New(http.StatusMethodNotAllowed, errs.CodeMethodNotAllowed, "method not allowed"))
		return
	}
	var req struct{ Email, Password string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	seedEmail := strings.TrimSpace(os.Getenv("PIC_GALLERY_ADMIN_EMAIL"))
	seedPassword := os.Getenv("PIC_GALLERY_ADMIN_PASSWORD")
	secret, ok := a.adminSigningSecret()
	if seedEmail == "" || seedPassword == "" || !ok {
		httpx.WriteError(w, r, errs.Internal("admin authentication is not configured"))
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Email), seedEmail) || subtle.ConstantTimeCompare([]byte(req.Password), []byte(seedPassword)) != 1 {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid admin credentials"))
		return
	}
	adminID := int64(1)
	if rawAdminID := strings.TrimSpace(os.Getenv("PIC_GALLERY_ADMIN_ID")); rawAdminID != "" {
		parsed, err := strconv.ParseInt(rawAdminID, 10, 64)
		if err != nil || parsed <= 0 {
			httpx.WriteError(w, r, errs.Internal("admin authentication is not configured"))
			return
		}
		adminID = parsed
	}
	now := time.Now()
	ttl := a.cfg.Auth.AccessTokenTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	claims := adminClaims{
		AdminID: adminID,
		Email:   seedEmail,
		Role:    "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(adminID, 10),
			Issuer:    a.adminTokenIssuer(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		httpx.WriteError(w, r, errs.Internal("failed to issue admin token"))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in_seconds": int(ttl.Seconds())})
}

func (a *API) HandleAdminLogout(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) HandleAdminAuditLogs(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": []any{}, "pagination": map[string]any{"page": 1, "page_size": 20, "total": 0}})
}
func (a *API) HandleAdminCallRecords(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.requireAdmin(w, r); !ok {
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"items": []any{}})
}

func (a *API) HandleDocsOpenAPIYAML(w http.ResponseWriter, r *http.Request) {
	serveFile(w, "api/openapi/openapi.yaml", "application/yaml")
}
func (a *API) HandleDocsOpenAPIJSON(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"openapi": "3.1.0", "source": "/docs/openapi.yaml"})
}
func (a *API) HandleDocsExamples(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, docsExamples())
}
func (a *API) HandleDocsErrors(w http.ResponseWriter, r *http.Request) {
	httpx.WriteSuccess(w, r, http.StatusOK, map[string]any{"codes": []string{errs.CodeBadRequest, errs.CodeUnauthorized, errs.CodeRateLimited, errs.CodeInsufficientPoints, errs.CodeImageTaskFailed}})
}

func (a *API) requireAdmin(w http.ResponseWriter, r *http.Request) (int64, bool) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authHeader, "Bearer ") {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "admin token required"))
		return 0, false
	}
	rawToken := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
	secret, ok := a.adminSigningSecret()
	if rawToken == "" || !ok {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "admin token required"))
		return 0, false
	}
	token, err := jwt.ParseWithClaims(rawToken, &adminClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errs.Unauthorized("invalid admin token")
		}
		return []byte(secret), nil
	})
	if err != nil {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid or expired admin token"))
		return 0, false
	}
	claims, ok := token.Claims.(*adminClaims)
	if !ok || !token.Valid || !a.validAdminClaims(claims) {
		httpx.WriteError(w, r, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid or expired admin token"))
		return 0, false
	}
	return claims.AdminID, true
}

func (a *API) adminSigningSecret() (string, bool) {
	envSecret := strings.TrimSpace(os.Getenv("PIC_GALLERY_ADMIN_TOKEN_SECRET"))
	if isProductionEnv(a.cfg.App.Env) {
		if isWeakAdminSigningSecret(envSecret) {
			return "", false
		}
		return envSecret, true
	}
	secret := envSecret
	if secret == "" {
		secret = strings.TrimSpace(a.cfg.Auth.AccessTokenSecret)
	}
	if isWeakAdminSigningSecret(secret) {
		return "", false
	}
	return secret, true
}

func isWeakAdminSigningSecret(secret string) bool {
	value := strings.ToLower(strings.TrimSpace(secret))
	switch value {
	case "", "secret", "password", "admin", "admin-token-secret", "admin-secret":
		return true
	default:
		return strings.HasPrefix(value, "change-me") || strings.HasPrefix(value, "local-dev") || len(value) < 32
	}
}

func (a *API) adminTokenIssuer() string {
	return defaultString(a.cfg.Auth.Issuer, "pic-gallery-admin")
}

func (a *API) validAdminClaims(claims *adminClaims) bool {
	if claims == nil || claims.AdminID <= 0 || claims.Role != "admin" || strings.TrimSpace(claims.Email) == "" {
		return false
	}
	if claims.Issuer != a.adminTokenIssuer() {
		return false
	}
	if claims.Subject != strconv.FormatInt(claims.AdminID, 10) {
		return false
	}
	if rawAdminID := strings.TrimSpace(os.Getenv("PIC_GALLERY_ADMIN_ID")); rawAdminID != "" {
		configuredAdminID, err := strconv.ParseInt(rawAdminID, 10, 64)
		if err != nil || configuredAdminID <= 0 || claims.AdminID != configuredAdminID {
			return false
		}
	}
	if adminEmail := strings.TrimSpace(os.Getenv("PIC_GALLERY_ADMIN_EMAIL")); adminEmail != "" && !strings.EqualFold(claims.Email, adminEmail) {
		return false
	}
	return true
}

func isProductionEnv(env string) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "prod", "production":
		return true
	default:
		return false
	}
}

func serveFile(w http.ResponseWriter, name, contentType string) {
	root, _ := os.Getwd()
	path := filepath.Join(root, name)
	content, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(content)
}

func docsExamples() map[string]any {
	return map[string]any{"items": []map[string]string{{"name": "create task curl", "language": "curl", "code": "curl -X POST /api/open/image/v1/tasks ..."}, {"name": "get balance typescript", "language": "typescript", "code": "await fetch('/api/open/image/v1/balance')"}}}
}

func structToCreateKey(userID int64, name, group string, total, daily *string, rpm *int, expires *time.Time) apikeyservice.CreateRequest {
	return apikeyservice.CreateRequest{UserID: userID, Name: name, GroupCode: group, TotalQuotaPoints: total, DailyQuotaPoints: daily, RPMLimit: rpm, ExpiresAt: expires}
}

func structToUpdateKey(userID, id int64, name, status, group *string, total, daily *string, rpm *int, expires **time.Time) apikeyservice.UpdateRequest {
	return apikeyservice.UpdateRequest{UserID: userID, ID: id, Name: name, Status: status, GroupCode: group, TotalQuotaPoints: total, DailyQuotaPoints: daily, RPMLimit: rpm, ExpiresAt: expires}
}

func structToAssetMetadata(apiKeyID *int64, source string) domainassets.UploadMetadata {
	return domainassets.UploadMetadata{APIKeyID: apiKeyID, UploadSource: source}
}

var _ = domainapikey.StatusActive
