package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
)

func TestRuntimeSchemaCheckDoesNotCreateOrMigrateTables(t *testing.T) {
	dsn := "file:app-compatibility?mode=memory&cache=shared&_fk=1"
	client, err := repoent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	cfg := config.Config{Runtime: config.RuntimeConfig{
		InstallationID:      "installation-test",
		ApplicationVersion:  "v1",
		ConfigSchemaVersion: config.CurrentRuntimeSchemaVersion,
	}}

	err = checkRuntimeSchemaCompatibility(context.Background(), client, cfg)
	var compatibilityErr *db.CompatibilityError
	if !errors.As(err, &compatibilityErr) || compatibilityErr.Kind != db.CompatibilityMissing {
		t.Fatalf("compatibility check error = %T %v, want typed missing error", err, err)
	}
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	defer database.Close()
	var count int
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&count); err != nil {
		t.Fatalf("inspect sqlite schema: %v", err)
	}
	if count != 0 {
		t.Fatalf("compatibility check created %d application tables", count)
	}
}

func TestOrdinaryAPIAndWorkerStartupContainNoMigrationCalls(t *testing.T) {
	for _, name := range []string{"run.go", "worker.go"} {
		contents, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		source := string(contents)
		for _, forbidden := range []string{"PrepareLegacyData(", ".Schema.Create(", "BackfillLegacyModelAccountCapabilities("} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("ordinary startup %s still contains database mutation %q", name, forbidden)
			}
		}
	}
}

func TestDefaultAdminSeedRoleDefaultsToAdmin(t *testing.T) {
	if got := defaultAdminSeedRole(""); got != domainadminauth.RoleAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleAdmin)
	}
}

func TestDefaultAdminSeedRoleAllowsExplicitSuperAdmin(t *testing.T) {
	if got := defaultAdminSeedRole(" super_admin "); got != domainadminauth.RoleSuperAdmin {
		t.Fatalf("defaultAdminSeedRole() = %q, want %q", got, domainadminauth.RoleSuperAdmin)
	}
}

func TestDefaultAdminSeedRoleRejectsUnknownRole(t *testing.T) {
	if got := defaultAdminSeedRole("ops_admin"); got != domainadminauth.RoleAdmin {
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

func TestRunRejectsWeakSecureConfigEncryptionKeyInProd(t *testing.T) {
	weakValues := []string{
		"",
		"secret",
		"short-prod-key",
		"example-secure-config-encryption-key",
		"local-dev-secure-config-encryption-key",
	}
	for _, value := range weakValues {
		cfg := config.Config{
			App:      config.AppConfig{Env: "prod"},
			Security: config.SecurityConfig{SecureConfigEncryptionKey: value},
		}
		if err := validateSecureConfigEncryptionKey(cfg); err == nil {
			t.Fatalf("expected prod runtime to reject weak secure config encryption key %q", value)
		}
	}
}

func TestRunAllowsDefaultSecureConfigEncryptionKeyOutsideProd(t *testing.T) {
	cfg := config.Config{
		App:      config.AppConfig{Env: "local"},
		Security: config.SecurityConfig{SecureConfigEncryptionKey: "local-dev-secure-config-encryption-key"},
	}
	if err := validateSecureConfigEncryptionKey(cfg); err != nil {
		t.Fatalf("expected local runtime to allow default secure config encryption key: %v", err)
	}
}

func TestRunRejectsWeakPromptOptimizationQuoteSigningKeyInProd(t *testing.T) {
	weakValues := []string{"", "secret", "short-prod-key", "example-prompt-quote-key", "local-dev-prompt-optimization-quote-signing-key"}
	for _, value := range weakValues {
		cfg := config.Config{
			App:      config.AppConfig{Env: "prod"},
			Security: config.SecurityConfig{PromptOptimizationQuoteSigningKey: value},
		}
		if err := validatePromptOptimizationQuoteSigningKey(cfg); err == nil {
			t.Fatalf("expected prod runtime to reject weak prompt optimization quote signing key %q", value)
		}
	}
}

func TestRunAllowsDefaultPromptOptimizationQuoteSigningKeyOutsideProd(t *testing.T) {
	cfg := config.Config{
		App:      config.AppConfig{Env: "local"},
		Security: config.SecurityConfig{PromptOptimizationQuoteSigningKey: "local-dev-prompt-optimization-quote-signing-key"},
	}
	if err := validatePromptOptimizationQuoteSigningKey(cfg); err != nil {
		t.Fatalf("expected local runtime to allow default prompt optimization quote signing key: %v", err)
	}
}
