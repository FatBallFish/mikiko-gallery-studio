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
		t.Fatalf("PostgreSQL integration result = %#v", result)
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL fixture URL: %v", err)
	}
	username := "probe-invalid"
	if parsed.User != nil && parsed.User.Username() != "" {
		username = parsed.User.Username()
	}
	parsed.User = url.UserPassword(username, "definitely-invalid-probe-password")
	result = service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: parsed.String()})
	if result.Code != ProbeCodeAuthenticationFailed {
		t.Fatalf("invalid PostgreSQL auth result = %#v", result)
	}

	t.Run("insufficient schema privileges", func(t *testing.T) {
		testPostgresInsufficientPrivileges(t, service, databaseURL)
	})
}

func testPostgresInsufficientPrivileges(t *testing.T, service *ProbeService, databaseURL string) {
	t.Helper()
	admin, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Skipf("isolated PostgreSQL URL cannot prepare restricted fixture: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.PingContext(t.Context()); err != nil {
		t.Skipf("isolated PostgreSQL URL cannot prepare restricted fixture: %v", err)
	}
	role, err := randomProbeIdentifier(cryptorand.Reader, "setup_probe_role_")
	if err != nil {
		t.Fatalf("generate restricted role: %v", err)
	}
	schema, err := randomProbeIdentifier(cryptorand.Reader, "setup_probe_schema_")
	if err != nil {
		t.Fatalf("generate restricted schema: %v", err)
	}
	password, err := randomProbeIdentifier(cryptorand.Reader, "password_")
	if err != nil {
		t.Fatalf("generate restricted password: %v", err)
	}

	if _, err := admin.ExecContext(t.Context(), "CREATE ROLE "+pq.QuoteIdentifier(role)+" LOGIN PASSWORD "+pq.QuoteLiteral(password)); err != nil {
		t.Skipf("isolated PostgreSQL URL lacks CREATEROLE for restricted fixture: %v", err)
	}
	roleCreated := true
	schemaCreated := false
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if schemaCreated {
			if _, cleanupErr := admin.ExecContext(cleanupCtx, "DROP SCHEMA IF EXISTS "+pq.QuoteIdentifier(schema)+" CASCADE"); cleanupErr != nil {
				t.Errorf("clean restricted PostgreSQL schema fixture: %v", cleanupErr)
			}
		}
		if roleCreated {
			if _, cleanupErr := admin.ExecContext(cleanupCtx, "DROP ROLE IF EXISTS "+pq.QuoteIdentifier(role)); cleanupErr != nil {
				t.Errorf("clean restricted PostgreSQL role fixture: %v", cleanupErr)
			}
		}
	})
	if _, err := admin.ExecContext(t.Context(), "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Skipf("isolated PostgreSQL URL cannot create restricted schema fixture: %v", err)
	}
	schemaCreated = true
	statements := []string{
		"REVOKE ALL ON SCHEMA " + pq.QuoteIdentifier(schema) + " FROM PUBLIC",
		"GRANT USAGE ON SCHEMA " + pq.QuoteIdentifier(schema) + " TO " + pq.QuoteIdentifier(role),
		"REVOKE CREATE ON SCHEMA " + pq.QuoteIdentifier(schema) + " FROM " + pq.QuoteIdentifier(role),
	}
	for _, statement := range statements {
		if _, err := admin.ExecContext(t.Context(), statement); err != nil {
			t.Fatalf("prepare restricted PostgreSQL fixture: %v", err)
		}
	}

	restricted, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("parse restricted PostgreSQL URL: %v", err)
	}
	restricted.User = url.UserPassword(role, password)
	query := restricted.Query()
	query.Set("search_path", schema)
	restricted.RawQuery = query.Encode()
	result := service.ProbePostgres(t.Context(), PostgresProbeRequest{DatabaseURL: restricted.String()})
	if result.Code != ProbeCodeInsufficientPrivileges {
		t.Fatalf("restricted PostgreSQL result = %#v", result)
	}
}

func TestProbeRedisIntegration(t *testing.T) {
	redisURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_REDIS_URL"))
	if redisURL == "" {
		t.Skip("set PIC_GALLERY_TEST_REDIS_URL to run the isolated Redis probe integration")
	}
	result := NewProbeService().ProbeRedis(t.Context(), RedisProbeRequest{RedisURL: redisURL, KeyPrefix: "setup-integration"})
	if !result.Success || result.Code != ProbeCodeOK {
		t.Fatalf("Redis integration result = %#v", result)
	}
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
