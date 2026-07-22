package setupui

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestNewModelProjectsSanitizedRuntimeSchema(t *testing.T) {
	secret := "postgres://app:database-secret@db/app?sslmode=disable"
	bootstrap := setupBootstrapForTest()
	bootstrap.PostgresManaged = true
	bootstrap.Values["POSTGRES_MANAGED"] = "true"
	bootstrap.Values["DATABASE_URL"] = secret
	bootstrap.Values["REDIS_URL"] = "redis://:redis-secret@redis:6379/0"
	bootstrap.Values["REDIS_KEY_PREFIX"] = "gallery"
	bootstrap.Values["PUBLIC_API_URL"] = "http://127.0.0.1:8080"

	model, err := NewModel(config.DefaultRuntimeSchema(), bootstrap)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	if model.SchemaVersion != config.CurrentRuntimeSchemaVersion || model.Deployment.Profile != "full" {
		t.Fatalf("unexpected model metadata: %#v", model)
	}
	database := fieldByKey(t, model, "DATABASE_URL")
	if !database.Secret || !database.Managed || !database.ReadOnly || !database.Required {
		t.Fatalf("managed database field metadata = %#v", database)
	}
	if database.Value != "" || database.Example != "" || strings.Contains(database.DescriptionZH+database.DescriptionEN, secret) {
		t.Fatalf("secret database field leaked material: %#v", database)
	}
	redisPrefix := fieldByKey(t, model, "REDIS_KEY_PREFIX")
	if redisPrefix.Value != "gallery" || redisPrefix.DescriptionZH == "" || redisPrefix.DescriptionEN == "" {
		t.Fatalf("non-secret setup field lost schema metadata/value: %#v", redisPrefix)
	}
	for _, field := range model.Fields {
		if field.Owner != string(config.FieldOwnerSetup) {
			t.Fatalf("browser model exposed non-setup field %q owned by %q", field.Key, field.Owner)
		}
		if field.Secret && (field.Value != "" || field.Example != "") {
			t.Fatalf("secret field %q exposed a value or example", field.Key)
		}
	}
	encoded := model.JSON()
	for _, forbidden := range []string{secret, "redis-secret", bootstrap.SetupToken, "AUTH_ACCESS_TOKEN_SECRET"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("sanitized model contains %q: %s", forbidden, encoded)
		}
	}
}

func TestNewModelRejectsInvalidSchemaAndDeploymentContext(t *testing.T) {
	t.Run("schema", func(t *testing.T) {
		if _, err := NewModel(config.RuntimeSchema{}, setupBootstrapForTest()); err == nil {
			t.Fatal("NewModel accepted an invalid runtime schema")
		}
	})

	t.Run("deployment context", func(t *testing.T) {
		bootstrap := setupBootstrapForTest()
		bootstrap.Deployment.Mode = ""
		if _, err := NewModel(config.DefaultRuntimeSchema(), bootstrap); err == nil {
			t.Fatal("NewModel accepted an invalid deployment context")
		}
	})
}

func TestNewModelLeavesCoreSetupFieldsEditable(t *testing.T) {
	bootstrap := setupBootstrapForTest()
	bootstrap.Deployment.Profile = config.DeploymentProfileCore
	bootstrap.PostgresManaged = false
	bootstrap.RedisManaged = false
	bootstrap.ObjectStorageManaged = false
	model, err := NewModel(config.DefaultRuntimeSchema(), bootstrap)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	for _, key := range []string{"DATABASE_URL", "REDIS_URL", "STORAGE_DRIVER"} {
		field := fieldByKey(t, model, key)
		if field.Managed || field.ReadOnly {
			t.Fatalf("core setup field %s unexpectedly read-only: %#v", key, field)
		}
	}
}

func TestPageServesCompleteOperationalSetupConsole(t *testing.T) {
	model, err := NewModel(config.DefaultRuntimeSchema(), setupBootstrapForTest())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	page, err := NewPage(model)
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	recorder := httptest.NewRecorder()
	page.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	body := recorder.Body.String()
	for _, required := range []string{
		"<!doctype html>", "lang=\"zh-CN\"", "部署初始化", "Setup console",
		"deployctl setup token show", "deployctl setup token reset",
		"data-step=\"database\"", "data-step=\"redis\"", "data-step=\"storage\"",
		"id=\"admin-email\"", "id=\"admin-password\"", "id=\"apply-setup\"",
		"role=\"status\"", "aria-live=\"polite\"", "<progress", "prefers-reduced-motion",
		"/api/setup/v1/session", "/api/setup/v1/probes/database", "/api/setup/v1/probes/redis",
		"/api/setup/v1/probes/storage", "/api/setup/v1/apply", "/api/setup/v1/progress/",
		"/api/system/v1/bootstrap-status", "history.back()",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("setup page missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"localStorage", "sessionStorage", "URLSearchParams", "location.search",
		"fonts.googleapis.com", "<script src=", "<link rel=\"stylesheet\"", "unsafe-eval", "unsafe-inline",
		"user navigation", "开始创作", "账户余额",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("setup page contains forbidden content %q", forbidden)
		}
	}
	if strings.Contains(body, "token=") {
		t.Fatal("setup page may not place the token in a URL")
	}

	style, script, modelJSON := inlinePageContent(t, body)
	styleHash := contentHash(style)
	scriptHash := contentHash(script)
	modelHash := contentHash(modelJSON)
	wantCSP := "default-src 'none'; base-uri 'none'; connect-src 'self'; form-action 'self'; frame-ancestors 'none'; img-src 'none'; font-src 'none'; style-src '" + styleHash + "'; script-src '" + scriptHash + "' '" + modelHash + "'"
	if got := recorder.Header().Get("Content-Security-Policy"); got != wantCSP {
		t.Fatalf("CSP = %q, want %q", got, wantCSP)
	}
	for name, want := range map[string]string{
		"Cache-Control":          "no-store",
		"Content-Type":           "text/html; charset=utf-8",
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	} {
		if got := recorder.Header().Get(name); got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestPageEscapesBootstrapMetadataAndRejectsNonGET(t *testing.T) {
	bootstrap := setupBootstrapForTest()
	bootstrap.Values["REDIS_KEY_PREFIX"] = `</script><img src=x onerror=alert(1)>`
	model, err := NewModel(config.DefaultRuntimeSchema(), bootstrap)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}
	page, err := NewPage(model)
	if err != nil {
		t.Fatalf("NewPage: %v", err)
	}
	recorder := httptest.NewRecorder()
	page.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/setup", nil))
	if strings.Contains(recorder.Body.String(), "</script><img") {
		t.Fatal("page did not escape model data before embedding it")
	}

	recorder = httptest.NewRecorder()
	page.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/setup", nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST /setup = %d Allow=%q, want 405 Allow=GET", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func setupBootstrapForTest() config.BootstrapConfig {
	values := map[string]string{
		"DEPLOYMENT_MODE":             "docker",
		"DEPLOYMENT_PROFILE":          "full",
		"DEPLOYMENT_TOPOLOGY":         "single",
		"DEPLOYMENT_ROLE":             "single",
		"POSTGRES_MANAGED":            "true",
		"REDIS_MANAGED":               "true",
		"OBJECT_STORAGE_MANAGED":      "true",
		"STORAGE_DRIVER":              "s3",
		"REDIS_KEY_PREFIX":            "app",
		"STORAGE_S3_REGION":           "us-east-1",
		"STORAGE_S3_BUCKET":           "app-assets",
		"STORAGE_S3_FORCE_PATH_STYLE": "true",
	}
	return config.BootstrapConfig{
		SchemaVersion: config.CurrentRuntimeSchemaVersion,
		Deployment: config.DeploymentContext{
			Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileFull,
			Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
			StorageDriver: "s3",
		},
		PostgresManaged: true, RedisManaged: true, ObjectStorageManaged: true,
		SetupToken: "setup-token-must-not-be-embedded", Values: values,
	}
}

func fieldByKey(t *testing.T, model Model, key string) Field {
	t.Helper()
	for _, field := range model.Fields {
		if field.Key == key {
			return field
		}
	}
	t.Fatalf("model missing field %q", key)
	return Field{}
}

func inlinePageContent(t *testing.T, document string) (string, string, string) {
	t.Helper()
	between := func(start, end string) string {
		startIndex := strings.Index(document, start)
		if startIndex < 0 {
			t.Fatalf("document missing %q", start)
		}
		startIndex += len(start)
		endIndex := strings.Index(document[startIndex:], end)
		if endIndex < 0 {
			t.Fatalf("document missing closing %q", end)
		}
		return document[startIndex : startIndex+endIndex]
	}
	return between("<style>", "</style>"), between("<script>", "</script>"), between("<script id=\"setup-model\" type=\"application/json\">", "</script>")
}

func contentHash(content string) string {
	digest := sha256.Sum256([]byte(content))
	return "sha256-" + base64.StdEncoding.EncodeToString(digest[:])
}
