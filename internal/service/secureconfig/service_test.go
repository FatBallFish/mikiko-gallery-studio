package secureconfig

import (
	"context"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainsecureconfig "github.com/fatballfish/pic-gallery/internal/domain/secureconfig"
)

func TestSMTPConfigStoresSecretsWriteOnlyAndPreservesOnUpdate(t *testing.T) {
	ctx := context.Background()
	svc := NewService(NewMemoryStore(), "secure-config-test-key", emptySMTPConfig(), "test")

	created, err := svc.UpdateSMTPConfig(ctx, domainsecureconfig.UpdateSMTPConfigRequest{
		Enabled:  true,
		Host:     "smtp.example.com",
		Port:     587,
		Username: "mailer@example.com",
		From:     "Pic Gallery <noreply@example.com>",
		StartTLS: true,
		Secrets:  map[string]string{"password": "smtp-password"},
	})
	if err != nil {
		t.Fatalf("UpdateSMTPConfig create: %v", err)
	}
	if !created.SecretStatus.HasSecret || created.SecretStatus.Fingerprint == "" || len(created.SecretStatus.SecretFields) != 1 || created.SecretStatus.SecretFields[0] != "password" {
		t.Fatalf("expected secret status without plaintext, got %#v", created.SecretStatus)
	}

	updated, err := svc.UpdateSMTPConfig(ctx, domainsecureconfig.UpdateSMTPConfigRequest{
		Version:  created.Version,
		Enabled:  true,
		Host:     "smtp2.example.com",
		Port:     465,
		Username: "mailer@example.com",
		From:     "Pic Gallery <noreply@example.com>",
	})
	if err != nil {
		t.Fatalf("UpdateSMTPConfig preserve password: %v", err)
	}
	if !updated.SecretStatus.HasSecret || updated.SecretStatus.Fingerprint != created.SecretStatus.Fingerprint {
		t.Fatalf("expected password to be preserved when secrets omitted, created=%#v updated=%#v", created.SecretStatus, updated.SecretStatus)
	}

	resolved, ok, err := svc.ResolveSMTPConfig(ctx)
	if err != nil {
		t.Fatalf("ResolveSMTPConfig: %v", err)
	}
	if !ok || resolved.Host != "smtp2.example.com" || resolved.Password != "smtp-password" {
		t.Fatalf("expected resolved SMTP config to include preserved password for runtime use, got ok=%v cfg=%#v", ok, resolved)
	}
	view, err := svc.GetSMTPConfig(ctx)
	if err != nil {
		t.Fatalf("GetSMTPConfig: %v", err)
	}
	if view.Host != "smtp2.example.com" || view.SecretStatus.Fingerprint == "" {
		t.Fatalf("expected public view with secret status, got %#v", view)
	}
}

func emptySMTPConfig() config.SMTPConfig {
	return config.SMTPConfig{}
}
