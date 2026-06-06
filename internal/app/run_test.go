package app

import (
	"context"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
)

func TestDefaultAdminSeedRoleDefaultsToAdmin(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_ROLE", "")

	if got := defaultAdminSeedRole(); got != domainadminauth.RoleAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleAdmin)
	}
}

func TestDefaultAdminSeedRoleAllowsExplicitSuperAdmin(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_ROLE", " super_admin ")

	if got := defaultAdminSeedRole(); got != domainadminauth.RoleSuperAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleSuperAdmin)
	}
}

func TestDefaultAdminSeedRoleRejectsUnknownRole(t *testing.T) {
	t.Setenv("PIC_GALLERY_ADMIN_ROLE", "ops_admin")

	if got := defaultAdminSeedRole(); got != domainadminauth.RoleAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleAdmin)
	}
}

func TestRunWiresAPIKeySigningSecretEncryptionKey(t *testing.T) {
	cfg := config.Config{
		App: config.AppConfig{Env: "prod"},
		APIKey: config.APIKeyConfig{
			SigningSecretEncryptionKey: "prod-runtime-api-key-signing-secret-encryption-key",
		},
	}

	svc, err := newRuntimeAPIKeyService(cfg, apikeyservice.NewMemoryStore())
	if err != nil {
		t.Fatalf("newRuntimeAPIKeyService: %v", err)
	}
	created, err := svc.CreateKey(context.Background(), apikeyservice.CreateRequest{
		UserID: 1,
		Name:   "runtime",
		Secret: "sk-runtime-secret",
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	if got, ok := domainapikey.DecryptSigningSecret(created.Key.SigningSecret, cfg.APIKey.SigningSecretEncryptionKey); !ok || got != "sk-runtime-secret" {
		t.Fatalf("expected runtime API key service to encrypt signing secret with cfg key, got %q ok=%v", got, ok)
	}
	if _, ok := domainapikey.DecryptSigningSecret(created.Key.SigningSecret, "local-dev-api-key-signing-secret-encryption-key"); ok {
		t.Fatal("expected runtime API key service not to encrypt signing secret with default local dev key")
	}
}

func TestRunRejectsWeakAPIKeySigningSecretEncryptionKeyInProd(t *testing.T) {
	weakValues := []string{
		"",
		"secret",
		"password",
		"admin",
		"admin-secret",
		"admin-token-secret",
		"short-prod-key",
		"change-me-in-prod",
		"example-api-key-signing-secret-encryption-key",
		"local-dev-api-key-signing-secret-encryption-key",
	}
	for _, value := range weakValues {
		cfg := config.Config{
			App:    config.AppConfig{Env: "prod"},
			APIKey: config.APIKeyConfig{SigningSecretEncryptionKey: value},
		}
		if _, err := newRuntimeAPIKeyService(cfg, apikeyservice.NewMemoryStore()); err == nil {
			t.Fatalf("expected prod runtime API key service to reject weak signing secret encryption key %q", value)
		}
	}
}

func TestRunAllowsDefaultAPIKeySigningSecretEncryptionKeyOutsideProd(t *testing.T) {
	cfg := config.Config{
		App:    config.AppConfig{Env: "local"},
		APIKey: config.APIKeyConfig{SigningSecretEncryptionKey: "local-dev-api-key-signing-secret-encryption-key"},
	}
	if _, err := newRuntimeAPIKeyService(cfg, apikeyservice.NewMemoryStore()); err != nil {
		t.Fatalf("expected local runtime API key service to allow default dev signing secret encryption key: %v", err)
	}
}
