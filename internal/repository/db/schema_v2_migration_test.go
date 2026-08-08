package db_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/fatballfish/pic-gallery/internal/config"
	repositorydb "github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
)

func TestSchemaV2MigratesLegacyRefreshSessions(t *testing.T) {
	if repositorydb.CurrentDatabaseSchemaVersion < 2 {
		t.Fatalf("database schema version = %d, want at least 2 for refresh session token_version", repositorydb.CurrentDatabaseSchemaVersion)
	}

	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL schema v2 migration integration")
	}

	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schemaName := fmt.Sprintf("migration_v2_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schemaName)); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + pq.QuoteIdentifier(schemaName) + ` CASCADE`)
	})
	databaseURL := postgresURLWithSearchPath(t, adminURL, schemaName)

	ctx := context.Background()
	request := repositorydb.MigrationRequest{
		InstallationID: "schema-v2-installation",
		AppVersion:     "v0.0.5",
		ConfigVersion:  config.CurrentRuntimeSchemaVersion,
	}
	if _, err := repositorydb.Migrate(ctx, databaseURL, request); err != nil {
		t.Fatalf("create current fixture schema: %v", err)
	}

	client, err := repositorydb.Open(databaseURL)
	if err != nil {
		t.Fatalf("open fixture schema: %v", err)
	}
	legacyUser, err := client.User.Create().
		SetEmail("legacy-passwordless@example.com").
		SetNickname("legacy-passwordless").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		_ = client.Close()
		t.Fatalf("create passwordless user: %v", err)
	}

	refreshToken := "legacy-refresh-token"
	refreshHash := hashRefreshToken(refreshToken)
	legacySessionID := uuid.New()
	legacyFamilyID := uuid.New()
	if err := entstore.NewAuthStore(client).SaveRefreshSession(ctx, entstore.RefreshSessionRecord{
		ID:               legacySessionID.String(),
		FamilyID:         legacyFamilyID.String(),
		UserID:           int64(legacyUser.ID),
		TokenVersion:     legacyUser.TokenVersion,
		RefreshTokenHash: refreshHash,
		Status:           "active",
		ExpiresAt:        time.Now().Add(time.Hour).Unix(),
	}); err != nil {
		_ = client.Close()
		t.Fatalf("create refresh session fixture: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close fixture schema client: %v", err)
	}

	if _, err := admin.Exec(`ALTER TABLE ` + pq.QuoteIdentifier(schemaName) + `.refresh_sessions DROP COLUMN token_version`); err != nil {
		t.Fatalf("remove post-v0.0.5 refresh-session column: %v", err)
	}
	if _, err := admin.Exec(`UPDATE ` + pq.QuoteIdentifier(schemaName) + `.installations SET database_schema_version = 1, app_version = 'v0.0.5'`); err != nil {
		t.Fatalf("mark fixture as database schema v1: %v", err)
	}

	request.AppVersion = "v0.0.6"
	result, err := repositorydb.Migrate(ctx, databaseURL, request)
	if err != nil {
		t.Fatalf("migrate schema v1 to v2: %v", err)
	}
	if result.Previous == nil || result.Previous.DatabaseSchemaVersion != 1 || result.Current.DatabaseSchemaVersion != 2 || !result.Changed {
		t.Fatalf("unexpected schema v2 migration result: %#v", result)
	}

	var columnDefault string
	if err := admin.QueryRow(`
		SELECT column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'refresh_sessions' AND column_name = 'token_version'
	`, schemaName).Scan(&columnDefault); err != nil {
		t.Fatalf("query migrated token_version column: %v", err)
	}
	if !strings.Contains(columnDefault, "0") {
		t.Fatalf("token_version default = %q, want zero", columnDefault)
	}

	client, err = repositorydb.Open(databaseURL)
	if err != nil {
		t.Fatalf("open migrated schema: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	authStore := entstore.NewAuthStore(client)
	legacySession, err := authStore.GetRefreshSessionByHash(ctx, refreshHash)
	if err != nil {
		t.Fatalf("read migrated refresh session: %v", err)
	}
	if legacySession.TokenVersion != 0 || legacySession.Status != "active" {
		t.Fatalf("migrated refresh session = %#v, want token version 0 and active", legacySession)
	}

	auth := authservice.NewServiceWithStore(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "schema-v2-test",
		AccessTokenSecret: "schema-v2-secret",
	}, map[string]string{"basic": "1.00000"}, authStore)
	if _, _, err := auth.Refresh(refreshToken); err == nil {
		t.Fatal("passwordless legacy refresh token was accepted after migration")
	}
	revoked, err := authStore.GetRefreshSessionByHash(ctx, refreshHash)
	if err != nil {
		t.Fatalf("read revoked migrated refresh session: %v", err)
	}
	if revoked.Status != "revoked" {
		t.Fatalf("passwordless legacy refresh session status = %q, want revoked", revoked.Status)
	}
}

func postgresURLWithSearchPath(t *testing.T, rawURL, schemaName string) string {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func hashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
