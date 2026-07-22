package deployctl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorReportsConfigurationPermissionsIdentityMiddlewareAndSchemaWithoutSecrets(t *testing.T) {
	runtimeDir := t.TempDir()
	configDir := filepath.Join(runtimeDir, "config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	const secret = "doctor-must-not-print-this-password"
	env := "RUNTIME_SCHEMA_VERSION=999\nDEPLOYMENT_MODE=docker\nDEPLOYMENT_PROFILE=core\nDEPLOYMENT_TOPOLOGY=single\nDEPLOYMENT_ROLE=single\nDEPLOYMENT_MODULES=api,worker\nPOSTGRES_MANAGED=false\nREDIS_MANAGED=false\nOBJECT_STORAGE_MANAGED=false\nSETUP_COMPLETED=true\nSETUP_TOKEN_VERSION=1\nDATABASE_URL=postgres://app:" + secret + "@db:5432/app\nREDIS_URL=redis://:" + secret + "@redis:6379/0\nREDIS_KEY_PREFIX=app\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./data\nSTORAGE_SHARED_VOLUME=true\nINSTALLATION_ID=runtime-id\nAPPLICATION_VERSION=v1\nAPI_PORT=8080\nIMAGE_TAG=v1\n"
	envPath := filepath.Join(configDir, "runtime.env")
	if err := os.WriteFile(envPath, []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"schema_version":1,"installation_id":"manifest-id","created_at":"2026-07-23T00:00:00Z","plan":{}}`)
	if err := os.WriteFile(filepath.Join(runtimeDir, "deployment.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	report := Doctor(context.Background(), runtimeDir, DoctorDependencies{
		ProbeMiddleware: func(context.Context, map[string]string) error { return errors.New("database rejected " + secret) },
		CheckSchema:     func(context.Context, map[string]string) error { return errors.New("schema drift near " + secret) },
	})
	rendered := report.String()
	for _, code := range []string{"CONFIG_REQUIRED_FIELD", "CONFIG_PERMISSIONS", "INSTALLATION_MISMATCH", "MIDDLEWARE", "SCHEMA_DRIFT"} {
		if !strings.Contains(rendered, code) {
			t.Errorf("doctor report missing %s: %s", code, rendered)
		}
	}
	if strings.Contains(rendered, secret) || strings.Contains(rendered, "postgres://") || strings.Contains(rendered, "redis://") {
		t.Fatalf("doctor leaked a secret: %s", rendered)
	}
}
