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
	AccessToken string `json:"access_token"`
	UserID      int64  `json:"user_id"`
	SignupGrant struct {
		Granted bool                         `json:"granted"`
		Balance domainbilling.BalanceSummary `json:"balance"`
	} `json:"signup_grant"`
}

func emailCodeLogin(t *testing.T, handler http.Handler, email string) emailCodeLoginResponse {
	t.Helper()
	sendReq := httptest.NewRequest(http.MethodPost, "/api/agent/auth/v1/email/send-code", bytes.NewBufferString(`{"email":"`+email+`","scene":"login"}`))
	sendReq.Header.Set("Content-Type", "application/json")
	sendRec := httptest.NewRecorder()
	handler.ServeHTTP(sendRec, sendReq)
	if sendRec.Code != http.StatusAccepted {
		t.Fatalf("send code expected 202, got %d body=%s", sendRec.Code, sendRec.Body.String())
	}

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
	return resp.Data
}
