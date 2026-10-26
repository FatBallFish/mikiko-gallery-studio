package handlers

import (
	"context"
	"github.com/fatballfish/pic-gallery/internal/config"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenPrincipalBearerScopesAndLegacySignatureRejection(t *testing.T) {
	keys := apikeyservice.NewService(nil)
	key, err := keys.CreateKey(context.Background(), apikeyservice.CreateRequest{UserID: 42, Scopes: []string{"models:read"}})
	if err != nil {
		t.Fatal(err)
	}
	api := &API{apiKeys: keys}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("X-Access-Key", key.Key.AccessKey)
	req.Header.Set("X-Signature", key.Secret)
	if _, err := api.GetOpenPrincipal(req); err == nil || err.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy signature accepted: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+key.Secret)
	principal, appErr := api.GetOpenPrincipal(req, "models:read")
	if appErr != nil || principal.UserID != 42 || principal.APIKeyID == "" {
		t.Fatalf("principal %#v error %v", principal, appErr)
	}
	if _, err := api.GetOpenPrincipal(req, "images:write"); err == nil || err.StatusCode != http.StatusForbidden {
		t.Fatalf("missing scope: %v", err)
	}
	if _, err := api.requireCompatAPIKey(req); err != nil {
		t.Fatal(err)
	}
	req.URL.Path = "/v1/images/generations"
	req.Method = http.MethodPost
	if _, err := api.requireCompatAPIKey(req); err == nil || err.StatusCode != http.StatusForbidden {
		t.Fatalf("compat scope: %v", err)
	}
	if err := keys.SetExpiresAtForTest(context.Background(), key.Key.ID, time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := api.GetOpenPrincipal(req); err == nil {
		t.Fatal("expired key accepted")
	}
}

func TestOpenPrincipalRejectsCurrentDisabledAndMissingUser(t *testing.T) {
	auth := authservice.NewService(config.AuthConfig{AccessTokenTTL: time.Hour, RefreshTokenTTL: time.Hour, Issuer: "test", AccessTokenSecret: "test-secret"}, map[string]string{"basic": "1.00000"})
	if err := auth.SendEmailCode("bearer@example.com", "login"); err != nil {
		t.Fatal(err)
	}
	user, _, err := auth.LoginWithEmailCode("bearer@example.com", "123456")
	if err != nil {
		t.Fatal(err)
	}
	keys := apikeyservice.NewService(nil)
	key, err := keys.CreateKey(context.Background(), apikeyservice.CreateRequest{UserID: user.ID})
	if err != nil {
		t.Fatal(err)
	}
	api := &API{apiKeys: keys, auth: auth}
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+key.Secret)
	if _, err := api.GetOpenPrincipal(req); err != nil {
		t.Fatal(err)
	}
	auth.DisableUserForTest(user.ID)
	if _, err := api.GetOpenPrincipal(req); err == nil || err.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled user: %v", err)
	}
	missing, err := keys.CreateKey(context.Background(), apikeyservice.CreateRequest{UserID: 99999})
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+missing.Secret)
	if _, err := api.GetOpenPrincipal(req); err == nil || err.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing user: %v", err)
	}
}
