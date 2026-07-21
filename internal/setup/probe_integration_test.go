package setup

import (
	"context"
	cryptorand "crypto/rand"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestProbePostgresIntegration(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if databaseURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run the isolated PostgreSQL probe integration")
	}
	service := NewProbeService()
	result := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: databaseURL})
	if !result.Success || result.Code != ProbeCodeOK || result.Version == "" {
		t.Fatalf("least-privilege PostgreSQL result = %#v", result)
	}

	invalidAuth, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse application PostgreSQL URL: %v", err)
	}
	invalidAuth.User = url.UserPassword(invalidAuth.User.Username(), "definitely-invalid-probe-password")
	result = service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: invalidAuth.String()})
	if result.Code != ProbeCodeAuthenticationFailed {
		t.Fatalf("invalid PostgreSQL auth result = %#v", result)
	}

	t.Run("admin and restricted fixtures", func(t *testing.T) {
		adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_ADMIN_URL"))
		if adminURL == "" {
			t.Skip("set PIC_GALLERY_TEST_POSTGRES_ADMIN_URL to run superuser and restricted-role probe integration")
		}
		adminResult := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: adminURL})
		if adminResult.Code != ProbeCodeUnsafePrivileges || adminResult.Success {
			t.Fatalf("PostgreSQL fixture admin was not rejected: %#v", adminResult)
		}

		applicationURL, noCreateURL, downscopedAdminURL := preparePostgresProbeRoles(t, adminURL)
		result := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: applicationURL})
		if !result.Success || result.Code != ProbeCodeOK || result.Version == "" {
			t.Fatalf("fixture application PostgreSQL result = %#v", result)
		}
		result = service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: noCreateURL})
		if result.Code != ProbeCodeInsufficientPrivileges || result.Success {
			t.Fatalf("no-CREATE PostgreSQL result = %#v", result)
		}
		result = service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: downscopedAdminURL})
		if result.Code != ProbeCodeUnsafePrivileges || result.Success {
			t.Fatalf("down-scoped PostgreSQL fixture admin was not rejected: %#v", result)
		}
	})
}

func preparePostgresProbeRoles(t *testing.T, databaseURL string) (string, string, string) {
	t.Helper()
	admin, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Skipf("isolated PostgreSQL URL cannot prepare restricted fixture: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.PingContext(t.Context()); err != nil {
		t.Skipf("isolated PostgreSQL URL cannot prepare restricted fixture: %v", err)
	}
	var fixtureSuperuser bool
	if err := admin.QueryRowContext(t.Context(), `
		SELECT EXISTS (
			SELECT 1
			FROM pg_catalog.pg_roles
			WHERE rolname IN (session_user, current_user) AND rolsuper
		)`).Scan(&fixtureSuperuser); err != nil || !fixtureSuperuser {
		t.Skipf("PIC_GALLERY_TEST_POSTGRES_ADMIN_URL must identify an isolated server-superuser fixture: superuser=%t err=%v", fixtureSuperuser, err)
	}
	applicationRole, err := randomProbeIdentifier(cryptorand.Reader, "setup_probe_app_")
	if err != nil {
		t.Fatalf("generate application role: %v", err)
	}
	noCreateRole, err := randomProbeIdentifier(cryptorand.Reader, "setup_probe_readonly_")
	if err != nil {
		t.Fatalf("generate no-CREATE role: %v", err)
	}
	schema, err := randomProbeIdentifier(cryptorand.Reader, "setup_probe_schema_")
	if err != nil {
		t.Fatalf("generate application schema: %v", err)
	}
	applicationPassword, err := randomProbeIdentifier(cryptorand.Reader, "password_")
	if err != nil {
		t.Fatalf("generate application password: %v", err)
	}
	noCreatePassword, err := randomProbeIdentifier(cryptorand.Reader, "password_")
	if err != nil {
		t.Fatalf("generate no-CREATE password: %v", err)
	}
	var databaseName string
	if err := admin.QueryRowContext(t.Context(), "SELECT current_database()").Scan(&databaseName); err != nil {
		t.Fatalf("read PostgreSQL fixture database name: %v", err)
	}

	applicationRoleCreated := false
	noCreateRoleCreated := false
	schemaCreated := false
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if schemaCreated {
			if _, cleanupErr := admin.ExecContext(cleanupCtx, "DROP SCHEMA IF EXISTS "+pq.QuoteIdentifier(schema)+" CASCADE"); cleanupErr != nil {
				t.Errorf("clean PostgreSQL probe schema fixture: %v", cleanupErr)
			}
		}
		for role, created := range map[string]bool{applicationRole: applicationRoleCreated, noCreateRole: noCreateRoleCreated} {
			if !created {
				continue
			}
			if _, cleanupErr := admin.ExecContext(cleanupCtx, "DROP OWNED BY "+pq.QuoteIdentifier(role)); cleanupErr != nil {
				t.Errorf("drop PostgreSQL probe role ownership: %v", cleanupErr)
			}
			if _, cleanupErr := admin.ExecContext(cleanupCtx, "DROP ROLE IF EXISTS "+pq.QuoteIdentifier(role)); cleanupErr != nil {
				t.Errorf("clean PostgreSQL probe role fixture: %v", cleanupErr)
			}
		}
	})
	if _, err := admin.ExecContext(t.Context(), "CREATE ROLE "+pq.QuoteIdentifier(applicationRole)+" LOGIN NOSUPERUSER PASSWORD "+pq.QuoteLiteral(applicationPassword)); err != nil {
		t.Skipf("isolated PostgreSQL URL cannot create application role fixture: %v", err)
	}
	applicationRoleCreated = true
	if _, err := admin.ExecContext(t.Context(), "CREATE ROLE "+pq.QuoteIdentifier(noCreateRole)+" LOGIN NOSUPERUSER PASSWORD "+pq.QuoteLiteral(noCreatePassword)); err != nil {
		t.Skipf("isolated PostgreSQL URL cannot create no-CREATE role fixture: %v", err)
	}
	noCreateRoleCreated = true
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Skipf("isolated PostgreSQL URL cannot create application schema fixture: %v", err)
	}
	schemaCreated = true
	statements := []string{
		"REVOKE ALL ON SCHEMA " + pq.QuoteIdentifier(schema) + " FROM PUBLIC",
		"GRANT USAGE, CREATE ON SCHEMA " + pq.QuoteIdentifier(schema) + " TO " + pq.QuoteIdentifier(applicationRole),
		"GRANT USAGE ON SCHEMA " + pq.QuoteIdentifier(schema) + " TO " + pq.QuoteIdentifier(noCreateRole),
		"REVOKE CREATE ON SCHEMA " + pq.QuoteIdentifier(schema) + " FROM " + pq.QuoteIdentifier(noCreateRole),
		"GRANT CONNECT ON DATABASE " + pq.QuoteIdentifier(databaseName) + " TO " + pq.QuoteIdentifier(applicationRole),
		"GRANT CONNECT ON DATABASE " + pq.QuoteIdentifier(databaseName) + " TO " + pq.QuoteIdentifier(noCreateRole),
	}
	for _, statement := range statements {
		if _, err := admin.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("prepare restricted PostgreSQL fixture: %v", err)
		}
	}

	applicationURL, err := postgresProbeRoleURL(databaseURL, applicationRole, applicationPassword, schema)
	if err != nil {
		t.Fatalf("build application PostgreSQL URL: %v", err)
	}
	noCreateURL, err := postgresProbeRoleURL(databaseURL, noCreateRole, noCreatePassword, schema)
	if err != nil {
		t.Fatalf("build no-CREATE PostgreSQL URL: %v", err)
	}
	downscopedAdminURL, err := postgresProbeDownscopedAdminURL(databaseURL, applicationRole, schema)
	if err != nil {
		t.Fatalf("build down-scoped admin PostgreSQL URL: %v", err)
	}
	return applicationURL, noCreateURL, downscopedAdminURL
}

func postgresProbeRoleURL(databaseURL, role, password, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	parsed.User = url.UserPassword(role, password)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func postgresProbeDownscopedAdminURL(databaseURL, role, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	query.Set("options", "-c role="+role)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func TestProbeRedisIntegration(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_REDIS_URL"))
	if redisURL == "" {
		t.Skip("set PIC_GALLERY_TEST_REDIS_URL to run the isolated Redis probe integration")
	}
	service := NewProbeService()
	result := service.ProbeRedis(t.Context(), RedisProbeRequest{RedisURL: redisURL, KeyPrefix: "setup-integration"})
	if !result.Success || result.Code != ProbeCodeOK {
		t.Fatalf("Redis integration result = %#v", result)
	}
	t.Run("wrong password", func(t *testing.T) {
		parsed, err := url.Parse(redisURL)
		if err != nil {
			t.Fatalf("parse Redis fixture URL: %v", err)
		}
		password, hasPassword := parsed.User.Password()
		if parsed.User == nil || !hasPassword || password == "" {
			t.Skip("PIC_GALLERY_TEST_REDIS_URL has no password; skipping only wrong-password authentication subcase")
		}
		parsed.User = url.UserPassword(parsed.User.Username(), "definitely-invalid-probe-password")
		result := service.ProbeRedis(t.Context(), RedisProbeRequest{RedisURL: parsed.String(), KeyPrefix: "setup-integration"})
		if result.Code != ProbeCodeAuthenticationFailed || result.Success {
			t.Fatalf("invalid Redis auth result = %#v", result)
		}
	})
}

func TestProbeS3Integration(t *testing.T) {
	endpoint := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_S3_ENDPOINT"))
	accessKey := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_S3_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_S3_SECRET_ACCESS_KEY"))
	bucket := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_S3_BUCKET"))
	if endpoint == "" || accessKey == "" || secretKey == "" || bucket == "" {
		t.Skip("set PIC_GALLERY_TEST_S3_ENDPOINT, PIC_GALLERY_TEST_S3_ACCESS_KEY_ID, PIC_GALLERY_TEST_S3_SECRET_ACCESS_KEY, and PIC_GALLERY_TEST_S3_BUCKET to run the isolated S3 probe integration")
	}
	region := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_S3_REGION"))
	if region == "" {
		region = "us-east-1"
	}
	result := NewProbeService().ProbeStorage(context.Background(), StorageProbeRequest{Config: config.StorageConfig{
		Driver: "s3",
		S3: config.StorageS3Config{
			Endpoint: endpoint, Region: region, Bucket: bucket,
			AccessKeyID: accessKey, SecretAccessKey: secretKey, ForcePathStyle: true, Prefix: "setup-integration",
		},
	}})
	if !result.Success || result.Code != ProbeCodeOK {
		t.Fatalf("S3 integration result = %#v", result)
	}
}
