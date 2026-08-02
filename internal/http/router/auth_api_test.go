package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
)

func TestEmailCodeLoginGrantsSignupTrialOnlyForNewUser(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Billing.SignupTrial = config.SignupTrialConfig{
		Enabled:            true,
		Points:             "15.00000",
		ValidDays:          7,
		ExpiryReminderDays: 2,
		GrantOncePerUser:   true,
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	first := emailCodeLogin(t, handler, "signup-trial@example.com")
	if !first.SignupGrant.Granted || first.SignupGrant.Balance.TrialPoints != "15.00000" {
		t.Fatalf("expected first login to grant trial credits, got %#v", first.SignupGrant)
	}
	if len(first.SignupGrant.Balance.Buckets) != 1 || first.SignupGrant.Balance.Buckets[0].Bucket != "trial" {
		t.Fatalf("expected first login trial bucket, got %#v", first.SignupGrant.Balance.Buckets)
	}

	second := emailCodeLogin(t, handler, "signup-trial@example.com")
	if second.SignupGrant.Granted || second.SignupGrant.Balance.TrialPoints != "15.00000" {
		t.Fatalf("expected second login not to duplicate trial grant, got %#v", second.SignupGrant)
	}
}

func TestAuthPasswordSetupRequiredBeforeFirstSession(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Auth.FixedEmailCode = "123456"
	cfg.Auth.RefreshCookieName = "pg_refresh"
	authSvc := authservice.NewService(cfg.Auth, map[string]string{"basic": "1.00000"})
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil))
	email := "password-setup@example.com"

	sendEmailLoginCode(t, handler, email)
	loginRec := postJSON(t, handler, "/api/agent/auth/v1/login/email-code", `{"email":"`+email+`","code":"123456"}`)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	if cookies := loginRec.Result().Cookies(); len(cookies) != 0 {
		t.Fatalf("passwordless login must not set refresh cookie, got %#v", cookies)
	}
	var loginResponse struct {
		Data struct {
			PasswordSetupRequired bool   `json:"password_setup_required"`
			PasswordSetupToken    string `json:"password_setup_token"`
			AccessToken           string `json:"access_token"`
			UserID                int64  `json:"user_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResponse); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	login := loginResponse.Data
	if !login.PasswordSetupRequired || login.PasswordSetupToken == "" || login.UserID == 0 || login.AccessToken != "" {
		t.Fatalf("expected setup-only login result, got %#v", login)
	}

	profileReq := httptest.NewRequest(http.MethodGet, "/api/agent/user/v1/profile", nil)
	profileReq.Header.Set("Authorization", "Bearer "+login.PasswordSetupToken)
	profileRec := httptest.NewRecorder()
	handler.ServeHTTP(profileRec, profileReq)
	if profileRec.Code != http.StatusUnauthorized {
		t.Fatalf("setup token must not access profile, got %d body=%s", profileRec.Code, profileRec.Body.String())
	}

	setupRec := postJSON(t, handler, "/api/agent/auth/v1/password/setup", `{"password_setup_token":"`+login.PasswordSetupToken+`","new_password":"new-password-123"}`)
	if setupRec.Code != http.StatusOK {
		t.Fatalf("password setup expected 200, got %d body=%s", setupRec.Code, setupRec.Body.String())
	}
	var setupResponse struct {
		Data struct {
			AccessToken string `json:"access_token"`
			UserID      int64  `json:"user_id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(setupRec.Body).Decode(&setupResponse); err != nil {
		t.Fatalf("decode setup response: %v", err)
	}
	if setupResponse.Data.AccessToken == "" || setupResponse.Data.UserID != login.UserID {
		t.Fatalf("password setup must return first normal session, got %#v", setupResponse.Data)
	}
	if cookies := setupRec.Result().Cookies(); len(cookies) != 1 || cookies[0].Name != cfg.Auth.RefreshCookieName || cookies[0].Value == "" {
		t.Fatalf("password setup must set refresh cookie, got %#v", cookies)
	}

	replayRec := postJSON(t, handler, "/api/agent/auth/v1/password/setup", `{"password_setup_token":"`+login.PasswordSetupToken+`","new_password":"another-password-123"}`)
	if replayRec.Code != http.StatusUnauthorized {
		t.Fatalf("setup token replay expected 401, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
}

func TestEmailCodeRegisterExposesEmailDerivedNickname(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Auth.FixedEmailCode = "123456"
	authSvc := authservice.NewService(cfg.Auth, map[string]string{"basic": "1.00000"})
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, nil))

	login := emailCodeLogin(t, handler, "alice.name+tag@example.com")
	profileReq := httptest.NewRequest(http.MethodGet, "/api/agent/user/v1/profile", nil)
	profileReq.Header.Set("Authorization", "Bearer "+login.AccessToken)
	profileRec := httptest.NewRecorder()
	handler.ServeHTTP(profileRec, profileReq)
	if profileRec.Code != http.StatusOK {
		t.Fatalf("profile expected 200, got %d body=%s", profileRec.Code, profileRec.Body.String())
	}
	var response struct {
		Data struct {
			Nickname string `json:"nickname"`
		} `json:"data"`
	}
	if err := json.NewDecoder(profileRec.Body).Decode(&response); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if response.Data.Nickname != "alice.name+tag" {
		t.Fatalf("expected email-derived nickname, got %q", response.Data.Nickname)
	}
}

func TestEmailCodeLoginUsesAdminSignupTrialConfig(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Billing.SignupTrial = config.SignupTrialConfig{
		Enabled:            true,
		Points:             "5.00000",
		ValidDays:          3,
		ExpiryReminderDays: 1,
		GrantOncePerUser:   true,
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	billingSvc := billingservice.NewService(cfg.Billing)
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "trial_credits",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_trial",
			ConfigKey:      "signup_trial",
			ConfigValue: map[string]any{"value": map[string]any{
				"enabled":              true,
				"points":               "18.00000",
				"valid_days":           9,
				"expiry_reminder_days": 4,
				"grant_once_per_user":  true,
			}},
			Scope: "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab trial_credits: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, adminCfgSvc, billingSvc))

	first := emailCodeLogin(t, handler, "signup-trial-config@example.com")
	if !first.SignupGrant.Granted || first.SignupGrant.Balance.TrialPoints != "18.00000" {
		t.Fatalf("expected admin-configured trial credits, got %#v", first.SignupGrant)
	}
	if len(first.SignupGrant.Balance.Buckets) != 1 || first.SignupGrant.Balance.Buckets[0].Bucket != "trial" || first.SignupGrant.Balance.Buckets[0].AvailablePoints != "18.00000" {
		t.Fatalf("expected trial bucket from admin config, got %#v", first.SignupGrant.Balance.Buckets)
	}
}

func TestEmailCodeLoginUsesAdminSignupTrialExpiryReminderDays(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Billing.SignupTrial = config.SignupTrialConfig{
		Enabled:            true,
		Points:             "18.00000",
		ValidDays:          9,
		ExpiryReminderDays: 1,
		GrantOncePerUser:   true,
	}
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	billingSvc := billingservice.NewService(cfg.Billing)
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "trial_credits",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_trial",
			ConfigKey:      "signup_trial",
			ConfigValue: map[string]any{"value": map[string]any{
				"enabled":              true,
				"points":               "18.00000",
				"valid_days":           5,
				"expiry_reminder_days": 6,
				"grant_once_per_user":  true,
			}},
			Scope: "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab trial_credits: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, adminCfgSvc, billingSvc))

	first := emailCodeLogin(t, handler, "signup-trial-reminder@example.com")
	if !first.SignupGrant.Granted || len(first.SignupGrant.Balance.Buckets) != 1 {
		t.Fatalf("expected signup grant with trial bucket, got %#v", first.SignupGrant)
	}
	if !first.SignupGrant.Balance.Buckets[0].ExpireWarning {
		t.Fatalf("expected admin-configured expiry_reminder_days to mark trial bucket expiring, got %#v", first.SignupGrant.Balance.Buckets[0])
	}
}

type emailCodeLoginResponse struct {
	AccessToken           string `json:"access_token"`
	UserID                int64  `json:"user_id"`
	PasswordSetupRequired bool   `json:"password_setup_required"`
	PasswordSetupToken    string `json:"password_setup_token"`
	SignupGrant           struct {
		Granted bool                         `json:"granted"`
		Balance domainbilling.BalanceSummary `json:"balance"`
	} `json:"signup_grant"`
}

func emailCodeLogin(t *testing.T, handler http.Handler, email string) emailCodeLoginResponse {
	t.Helper()
	sendEmailLoginCode(t, handler, email)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/agent/auth/v1/login/email-code", bytes.NewBufferString(`{"email":"`+email+`","code":"123456"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var resp struct {
		Data emailCodeLoginResponse `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Data.PasswordSetupRequired {
		setupRec := postJSON(t, handler, "/api/agent/auth/v1/password/setup", `{"password_setup_token":"`+resp.Data.PasswordSetupToken+`","new_password":"test-password-123"}`)
		if setupRec.Code != http.StatusOK {
			t.Fatalf("password setup expected 200, got %d body=%s", setupRec.Code, setupRec.Body.String())
		}
		var setupResponse struct {
			Data struct {
				AccessToken string `json:"access_token"`
			} `json:"data"`
		}
		if err := json.NewDecoder(setupRec.Body).Decode(&setupResponse); err != nil {
			t.Fatalf("decode password setup response: %v", err)
		}
		resp.Data.AccessToken = setupResponse.Data.AccessToken
	}
	return resp.Data
}

func sendEmailLoginCode(t *testing.T, handler http.Handler, email string) {
	t.Helper()
	sendRec := postJSON(t, handler, "/api/agent/auth/v1/email/send-code", `{"email":"`+email+`","scene":"login"}`)
	if sendRec.Code != http.StatusAccepted {
		t.Fatalf("send code expected 202, got %d body=%s", sendRec.Code, sendRec.Body.String())
	}
}

func postJSON(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func loginAuthUserWithPasswordSetup(t *testing.T, authSvc *authservice.Service, email, code string) (domainauth.User, domainauth.Session, error) {
	t.Helper()
	login, err := authSvc.LoginWithEmailCodeResult(email, code)
	if err != nil {
		return domainauth.User{}, domainauth.Session{}, err
	}
	if !login.PasswordSetupRequired {
		return login.User, login.Session, nil
	}
	return authSvc.CompletePasswordSetup(login.PasswordSetupToken, "test-password-123")
}
