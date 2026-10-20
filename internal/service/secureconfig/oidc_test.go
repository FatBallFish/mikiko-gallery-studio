package secureconfig

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
)

func newTestService() *Service {
	return NewService(NewMemoryStore(), "test-encryption-key", config.SMTPConfig{}, "test")
}

func enabledOIDCRequest() domainsecureconfig.UpdateOIDCConfigRequest {
	return domainsecureconfig.UpdateOIDCConfigRequest{
		Enabled:           true,
		IssuerURL:         "https://oauth.mikiko.ai/realms/mikiko",
		ClientID:          "mikiko_studio",
		AllowLocalLogin:   true,
		AllowRegistration: true,
		Secrets:           map[string]string{"client_secret": "s3cret-value"},
	}
}

func TestOIDCDefaultsKeepLocalLoginOpen(t *testing.T) {
	svc := newTestService()
	view, err := svc.GetOIDCConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view.Enabled || !view.AllowLocalLogin || !view.AllowRegistration {
		t.Fatalf("defaults must keep oidc off and local login open: %#v", view)
	}
	settings, err := svc.ResolveOIDCSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || !settings.AllowLocalLogin || !settings.AllowRegistration {
		t.Fatalf("resolved settings must match defaults: %#v", settings)
	}
	if _, ok, err := svc.ResolveOIDCConfig(t.Context()); err != nil || ok {
		t.Fatalf("disabled oidc must not resolve runtime config: ok=%v err=%v", ok, err)
	}
}

func TestOIDCUpdateValidatesEnabledConfig(t *testing.T) {
	svc := newTestService()
	cases := []struct {
		name    string
		mutate  func(req *domainsecureconfig.UpdateOIDCConfigRequest)
		wantErr bool
	}{
		{"missing issuer", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.IssuerURL = "" }, true},
		{"missing client id", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.ClientID = "" }, true},
		{"missing secret", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.Secrets = nil }, true},
		{"http issuer on public host", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.IssuerURL = "http://oauth.example.com/realms/mikiko" }, true},
		{"issuer with query", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.IssuerURL = "https://oauth.example.com/realms/mikiko?x=1" }, true},
		{"http localhost issuer", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.IssuerURL = "http://localhost:8080/realms/mikiko" }, false},
		{"masked secret", func(req *domainsecureconfig.UpdateOIDCConfigRequest) { req.Secrets = map[string]string{"client_secret": "********"} }, true},
	}
	for _, tc := range cases {
		req := enabledOIDCRequest()
		tc.mutate(&req)
		_, err := svc.UpdateOIDCConfig(t.Context(), req)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s: err=%v wantErr=%v", tc.name, err, tc.wantErr)
		}
	}
}

func TestOIDCDisablingCannotLockOutLocalLogin(t *testing.T) {
	svc := newTestService()
	saved, err := svc.UpdateOIDCConfig(t.Context(), enabledOIDCRequest())
	if err != nil {
		t.Fatal(err)
	}
	lockout := enabledOIDCRequest()
	lockout.Enabled = false
	lockout.AllowLocalLogin = false
	lockout.Version = saved.Version
	if _, err := svc.UpdateOIDCConfig(t.Context(), lockout); err == nil {
		t.Fatal("disabling oidc while local login is off must be rejected")
	}
}

func TestOIDCRoundTripPreservesSecretAndBumpsVersion(t *testing.T) {
	svc := newTestService()
	first, err := svc.UpdateOIDCConfig(t.Context(), enabledOIDCRequest())
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || !first.SecretStatus.HasSecret {
		t.Fatalf("first save must version 1 and record the secret: %#v", first)
	}
	// Update without secrets keeps the stored secret.
	update := enabledOIDCRequest()
	update.DisplayName = "Mikiko ID"
	update.Version = first.Version
	second, err := svc.UpdateOIDCConfig(t.Context(), update)
	if err != nil {
		t.Fatal(err)
	}
	if second.Version != 2 || !second.SecretStatus.HasSecret || second.DisplayName != "Mikiko ID" {
		t.Fatalf("second save must keep the secret and bump version: %#v", second)
	}
	// Version conflict is detected.
	conflict := enabledOIDCRequest()
	conflict.Version = first.Version
	if _, err := svc.UpdateOIDCConfig(t.Context(), conflict); err == nil {
		t.Fatal("stale version must conflict")
	}
	runtimeCfg, ok, err := svc.ResolveOIDCConfig(t.Context())
	if err != nil || !ok {
		t.Fatalf("enabled oidc must resolve runtime config: ok=%v err=%v", ok, err)
	}
	if runtimeCfg.ClientSecret != "s3cret-value" || runtimeCfg.ClientID != "mikiko_studio" {
		t.Fatalf("runtime config must carry the decrypted secret: %#v", runtimeCfg)
	}
	settings, err := svc.ResolveOIDCSettings(t.Context())
	if err != nil || !settings.Enabled || !settings.AllowLocalLogin || !settings.AllowRegistration {
		t.Fatalf("settings must reflect the saved record: %#v err=%v", settings, err)
	}
}
