package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	oidcservice "github.com/fatballfish/pic-gallery/internal/service/oidc"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/fatballfish/pic-gallery/pkg/httpx"
)

func (a *API) SetOIDCClient(client *oidcservice.Client) {
	a.oidc = client
}

// requestOrigin reconstructs the public origin the browser used to reach the
// API (nginx preserves Host and sets X-Forwarded-Proto).
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func oidcRedirectURI(r *http.Request) string {
	return requestOrigin(r) + "/auth/callback"
}

func requestIsHTTPS(r *http.Request) bool {
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") || r.TLS != nil
}

// sanitizeReturnTo keeps the post-login redirect on the requesting origin.
func sanitizeReturnTo(r *http.Request, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "/") && !strings.HasPrefix(raw, "//") {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return ""
	}
	if !strings.EqualFold(parsed.Host, r.Host) {
		return ""
	}
	return raw
}

func oidcNotEnabled() *errs.Error {
	return errs.New(404, "OIDC_NOT_ENABLED", "oidc login is not enabled")
}

func (a *API) HandleOIDCAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w, r)
		return
	}
	if a.oidc == nil || a.secureCfg == nil {
		httpx.WriteError(w, r, oidcNotEnabled())
		return
	}
	provider, ok, err := a.secureCfg.ResolveOIDCConfig(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if !ok {
		httpx.WriteError(w, r, oidcNotEnabled())
		return
	}
	redirectURI := oidcRedirectURI(r)
	returnTo := sanitizeReturnTo(r, r.URL.Query().Get("return_to"))
	authorizeURL, tx, err := a.oidc.BeginLogin(r.Context(), oidcservice.ProviderConfig{
		IssuerURL:    provider.IssuerURL,
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
	}, redirectURI, returnTo)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oidcservice.TransactionCookieName,
		Value:    oidcservice.SealTransaction(tx, oidcservice.TransactionKey(a.cfg.Auth.AccessTokenSecret)),
		Path:     "/api/agent/auth/v1",
		MaxAge:   int(oidcservice.LoginStateTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
	})
	http.Redirect(w, r, authorizeURL, http.StatusFound)
}

func (a *API) HandleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if a.oidc == nil || a.secureCfg == nil {
		httpx.WriteError(w, r, oidcNotEnabled())
		return
	}
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
		return
	}
	cookie, err := r.Cookie(oidcservice.TransactionCookieName)
	if err != nil {
		httpx.WriteError(w, r, errs.New(400, "OIDC_STATE_INVALID", "oidc login transaction cookie is missing"))
		return
	}
	tx, ok := oidcservice.OpenTransaction(cookie.Value, oidcservice.TransactionKey(a.cfg.Auth.AccessTokenSecret))
	if !ok || tx.State == "" || tx.State != strings.TrimSpace(req.State) {
		httpx.WriteError(w, r, errs.New(400, "OIDC_STATE_INVALID", "oidc login transaction is invalid or expired"))
		return
	}
	// The transaction is single-use: burn the cookie before doing anything else.
	http.SetCookie(w, &http.Cookie{
		Name:     oidcservice.TransactionCookieName,
		Value:    "",
		Path:     "/api/agent/auth/v1",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(r),
	})
	provider, ok, err := a.secureCfg.ResolveOIDCConfig(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if !ok {
		httpx.WriteError(w, r, oidcNotEnabled())
		return
	}
	identity, err := a.oidc.CompleteLogin(r.Context(), oidcservice.ProviderConfig{
		IssuerURL:    provider.IssuerURL,
		ClientID:     provider.ClientID,
		ClientSecret: provider.ClientSecret,
	}, oidcRedirectURI(r), strings.TrimSpace(req.Code), tx)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	login, err := a.auth.LoginWithOIDC(authservice.OIDCLoginInput{
		Subject:       identity.Subject,
		Email:         identity.Email,
		EmailVerified: identity.EmailVerified,
		DisplayName:   identity.Name,
		Username:      identity.PreferredUsername,
	})
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
	if auditErr := a.recordAudit(r, "user", fmt.Sprintf("%d", login.User.ID), "auth.oidc_login", "user", fmt.Sprintf("%d", login.User.ID), map[string]any{"subject": identity.Subject}); auditErr != nil {
		httpx.WriteError(w, r, normalizeAppError(auditErr))
		return
	}
	payload := map[string]any{
		"access_token":       login.Session.AccessToken,
		"expires_in_seconds": int(time.Until(login.Session.AccessTokenExpiresAt).Seconds()),
		"user_id":            login.User.ID,
		"return_to":          tx.ReturnTo,
	}
	if signupGrant != nil {
		payload["signup_grant"] = signupGrant
	}
	httpx.WriteSuccess(w, r, http.StatusOK, payload)
}

func (a *API) HandleAdminSecurityOIDC(w http.ResponseWriter, r *http.Request) {
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
		cfg, err := a.secureCfg.GetOIDCConfig(r.Context())
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
		var req domainsecureconfig.UpdateOIDCConfigRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteError(w, r, errs.BadRequest("invalid json body"))
			return
		}
		req.UpdatedBy = admin.AdminID
		updated, err := a.secureCfg.UpdateOIDCConfig(r.Context(), req)
		if err != nil {
			httpx.WriteError(w, r, normalizeAppError(err))
			return
		}
		if auditErr := a.recordAudit(r, "admin", fmt.Sprintf("%d", admin.AdminID), "security.oidc.update", "secure_config", "oidc/default", map[string]any{"enabled": updated.Enabled, "allow_local_login": updated.AllowLocalLogin, "allow_registration": updated.AllowRegistration, "secret_fields": updated.SecretStatus.SecretFields, "fingerprint": updated.SecretStatus.Fingerprint}); auditErr != nil {
			httpx.WriteError(w, r, normalizeAppError(auditErr))
			return
		}
		httpx.WriteSuccess(w, r, http.StatusOK, updated)
	default:
		writeMethodNotAllowed(w, r)
	}
}

func (a *API) HandleAdminSecurityOIDCTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w, r)
		return
	}
	if a.oidc == nil {
		httpx.WriteError(w, r, errs.Internal("oidc client is not available"))
		return
	}
	if _, appErr := a.requireAdminPermission(r, domainadminauth.PermissionManageDangerousConfig); appErr != nil {
		httpx.WriteError(w, r, appErr)
		return
	}
	provider, ok, err := a.secureCfg.ResolveOIDCConfig(r.Context())
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	if !ok {
		httpx.WriteError(w, r, errs.BadRequest("oidc is not enabled or not fully configured"))
		return
	}
	info, err := a.oidc.Discover(r.Context(), provider.IssuerURL)
	if err != nil {
		httpx.WriteError(w, r, normalizeAppError(err))
		return
	}
	httpx.WriteSuccess(w, r, http.StatusOK, domainsecureconfig.OIDCTestResult{
		Issuer:                info.Issuer,
		AuthorizationEndpoint: info.AuthorizationEndpoint,
		TokenEndpoint:         info.TokenEndpoint,
		UserInfoEndpoint:      info.UserInfoEndpoint,
	})
}
