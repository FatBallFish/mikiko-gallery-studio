package entstore_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	secureconfigservice "github.com/fatballfish/pic-gallery/internal/service/secureconfig"
	_ "github.com/mattn/go-sqlite3"
)

func TestSecureConfigStorePersistsSMTPSecretEncrypted(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:secure-config-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewSecureConfigStore(client)
	svc := secureconfigservice.NewService(store, "secure-config-store-test-key", config.SMTPConfig{}, "test")
	if _, err := svc.UpdateSMTPConfig(ctx, secureconfig.UpdateSMTPConfigRequest{
		Enabled:  true,
		Host:     "smtp.example.com",
		Port:     587,
		Username: "mailer@example.com",
		From:     "Pic Gallery <noreply@example.com>",
		Secrets:  map[string]string{"password": "smtp-secret-password"},
	}); err != nil {
		t.Fatalf("UpdateSMTPConfig: %v", err)
	}

	rows, err := client.SecureConfig.Query().All(ctx)
	if err != nil {
		t.Fatalf("query secure configs: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one secure config row, got %d", len(rows))
	}
	rawSecret, err := json.Marshal(rows[0].SecretEncrypted)
	if err != nil {
		t.Fatalf("marshal secret envelope: %v", err)
	}
	if strings.Contains(string(rawSecret), "smtp-secret-password") {
		t.Fatalf("expected encrypted secret envelope, got %s", rawSecret)
	}
	if rows[0].SecretFingerprint == "" || len(rows[0].SecretFields) != 1 || rows[0].SecretFields[0] != "password" {
		t.Fatalf("expected secret metadata, got fingerprint=%q fields=%#v", rows[0].SecretFingerprint, rows[0].SecretFields)
	}
}
