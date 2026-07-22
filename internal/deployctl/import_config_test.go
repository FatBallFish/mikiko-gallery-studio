package deployctl

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestImportConfigMapsLegacyVariantsGeneratesSecretsAndLeavesSourceUntouched(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		content string
		want    map[string]string
	}{
		{
			name: "root dotenv", source: ".env",
			content: "DATABASE_URL=postgres://app:legacy-password@db:5432/app?sslmode=disable\nREDIS_URL=redis://:redis-password@redis:6379/0\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./legacy-assets\nAUTH_ACCESS_TOKEN_SECRET=existing-auth-secret\n",
			want:    map[string]string{"DATABASE_URL": "postgres://app:legacy-password@db:5432/app?sslmode=disable", "STORAGE_LOCAL_ROOT": "./legacy-assets", "AUTH_ACCESS_TOKEN_SECRET": "existing-auth-secret"},
		},
		{
			name: "compose production", source: ".env.prod",
			content: "POSTGRES_DB=gallery\nPOSTGRES_USER=gallery_user\nPOSTGRES_PASSWORD=postgres-password\nREDIS_PASSWORD=redis-password\nSTORAGE_DRIVER=s3\nMINIO_ROOT_USER=minio-root\nMINIO_ROOT_PASSWORD=minio-root-password\nPIC_GALLERY_IMAGE_REGISTRY=registry.example.test/gallery\nPIC_GALLERY_IMAGE_TAG=sha-abc\n",
			want:    map[string]string{"POSTGRES_DATABASE": "gallery", "POSTGRES_USER": "gallery_user", "POSTGRES_PASSWORD": "postgres-password", "IMAGE_REGISTRY": "registry.example.test/gallery", "IMAGE_TAG": "sha-abc"},
		},
		{
			name: "packaged backend", source: "backend.env",
			content: "DATABASE_URL=postgres://app:db-password@db:5432/app?sslmode=disable\nREDIS_URL=redis://:redis-password@redis:6379/0\nSTORAGE_DRIVER=s3\nBFSS_ENDPOINT=https://objects.example.test\nBFSS_REGION=cn-test-1\nBFSS_BUCKET=gallery\nBFSS_ACCESS_KEY_ID=legacy-access\nBFSS_SECRET_ACCESS_KEY=legacy-secret\n",
			want:    map[string]string{"STORAGE_S3_ENDPOINT": "https://objects.example.test", "STORAGE_S3_REGION": "cn-test-1", "STORAGE_S3_BUCKET": "gallery", "STORAGE_S3_ACCESS_KEY_ID": "legacy-access", "STORAGE_S3_SECRET_ACCESS_KEY": "legacy-secret"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			runtimeDir := t.TempDir()
			sourcePath := filepath.Join(runtimeDir, testCase.source)
			original := []byte(testCase.content)
			if err := os.WriteFile(sourcePath, original, 0o600); err != nil {
				t.Fatal(err)
			}
			var written []byte
			result, err := ImportConfig(context.Background(), ImportConfigOptions{
				Source: sourcePath, RuntimeDir: runtimeDir, Mode: config.DeploymentModeDocker,
				Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
				Role: config.DeploymentRoleSingle, ApplicationVersion: "v2", StorageDriver: "local",
			}, ImportConfigDependencies{
				Entropy:             bytes.NewReader(bytes.Repeat([]byte{0x2a}, 128)),
				Now:                 func() time.Time { return time.Date(2026, 7, 23, 1, 2, 3, 0, time.UTC) },
				WriteRuntimeEnv:     func(_ string, content []byte) error { written = append([]byte(nil), content...); return nil },
				WriteInstallState:   func(string, setup.InstallState) error { return nil },
				WriteManifest:       func(string, []byte) error { return nil },
				WriteDeploymentFile: func(string, []byte) error { return nil },
			})
			if err != nil {
				t.Fatalf("ImportConfig: %v", err)
			}
			if result.Completed {
				t.Fatal("import inferred completion without middleware/installation/admin probes")
			}
			document, err := config.ParseRuntimeEnv(written)
			if err != nil {
				t.Fatalf("parse imported runtime env: %v", err)
			}
			for key, want := range testCase.want {
				if got := document.Values[key]; got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if testCase.source == ".env.prod" {
				if !strings.Contains(document.Values["DATABASE_URL"], "postgres-password") || !strings.Contains(document.Values["REDIS_URL"], "redis-password") {
					t.Fatalf("managed legacy credentials were not reflected in connection URLs: database=%q redis=%q", document.Values["DATABASE_URL"], document.Values["REDIS_URL"])
				}
				if document.Values["STORAGE_DRIVER"] != "local" {
					t.Fatalf("explicit storage driver was overridden by legacy config: %q", document.Values["STORAGE_DRIVER"])
				}
			}
			for _, key := range []string{"SETUP_TOKEN", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY", "CLUSTER_ENROLLMENT_SEAL_KEY"} {
				if strings.TrimSpace(document.Values[key]) == "" {
					t.Errorf("generated secret %s is empty", key)
				}
			}
			for _, fragment := range []string{"# [中文]", "# [English]", "运行时配置", "Runtime configuration"} {
				if !bytes.Contains(written, []byte(fragment)) {
					t.Errorf("rendered env missing bilingual fragment %q", fragment)
				}
			}
			after, err := os.ReadFile(sourcePath)
			if err != nil || !bytes.Equal(after, original) {
				t.Fatalf("source was modified: %v, %q", err, after)
			}
		})
	}
}

func TestImportConfigRollsBackPublishedTargetsAfterPartialFailure(t *testing.T) {
	written := make([]string, 0, 4)
	removed := make([]string, 0, 4)
	_, err := ImportConfig(context.Background(), testImportOptions(), ImportConfigDependencies{
		ReadFile: func(string) ([]byte, error) {
			return []byte("DATABASE_URL=postgres://app:secret@db/app\nREDIS_URL=redis://redis/0\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./data\n"), nil
		},
		Entropy:           bytes.NewReader(bytes.Repeat([]byte{0x21}, 128)),
		PathExists:        func(string) (bool, error) { return false, nil },
		AcquireLock:       func(context.Context, string) (func() error, error) { return func() error { return nil }, nil },
		MakeDirectory:     func(string, os.FileMode) error { return nil },
		WriteInstallState: func(path string, _ setup.InstallState) error { written = append(written, path); return nil },
		WriteManifest:     func(path string, _ []byte) error { written = append(written, path); return nil },
		WriteDeploymentFile: func(path string, _ []byte) error {
			written = append(written, path)
			return os.ErrPermission
		},
		WriteRuntimeEnv: func(path string, _ []byte) error { written = append(written, path); return nil },
		RemovePath:      func(path string) error { removed = append(removed, path); return nil },
	})
	if err == nil {
		t.Fatal("partial import unexpectedly succeeded")
	}
	if len(written) != 3 || len(removed) != 2 || removed[0] != written[1] || removed[1] != written[0] {
		t.Fatalf("partial import writes=%v removals=%v", written, removed)
	}
}

func TestImportConfigMarksCompletedOnlyAfterAllLegacyChecks(t *testing.T) {
	legacy := []byte("DATABASE_URL=postgres://app:secret@db:5432/app?sslmode=disable\nREDIS_URL=redis://redis:6379/0\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./data\n")
	for _, probe := range []LegacyCompletionProbe{
		{MiddlewareReachable: false, InstallationMatches: true, AdministratorExists: true},
		{MiddlewareReachable: true, InstallationMatches: false, AdministratorExists: true},
		{MiddlewareReachable: true, InstallationMatches: true, AdministratorExists: false},
	} {
		result, err := ImportConfig(context.Background(), testImportOptions(), ImportConfigDependencies{
			ReadFile:   func(string) ([]byte, error) { return legacy, nil },
			Entropy:    bytes.NewReader(bytes.Repeat([]byte{0x31}, 128)),
			PathExists: func(string) (bool, error) { return false, nil },
			AcquireLock: func(context.Context, string) (func() error, error) {
				return func() error { return nil }, nil
			},
			MakeDirectory:   func(string, os.FileMode) error { return nil },
			ProbeCompletion: func(context.Context, map[string]string) (LegacyCompletionProbe, error) { return probe, nil },
			WriteRuntimeEnv: func(string, []byte) error { return nil }, WriteInstallState: func(string, setup.InstallState) error { return nil },
			WriteManifest: func(string, []byte) error { return nil }, WriteDeploymentFile: func(string, []byte) error { return nil },
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Completed {
			t.Fatalf("partial probe %#v inferred completion", probe)
		}
	}
}

func TestImportConfigCarriesVerifiedSetupBindingIntoCompletedState(t *testing.T) {
	legacy := []byte("DATABASE_URL=postgres://app:secret@db:5432/app?sslmode=disable\nREDIS_URL=redis://redis:6379/0\nSTORAGE_DRIVER=local\nSTORAGE_LOCAL_ROOT=./data\n")
	var writtenState setup.InstallState
	var writtenEnv []byte
	options := testImportOptions()
	result, err := ImportConfig(context.Background(), options, ImportConfigDependencies{
		ReadFile:      func(string) ([]byte, error) { return legacy, nil },
		Entropy:       bytes.NewReader(bytes.Repeat([]byte{0x61}, 128)),
		PathExists:    func(string) (bool, error) { return false, nil },
		AcquireLock:   func(context.Context, string) (func() error, error) { return func() error { return nil }, nil },
		MakeDirectory: func(string, os.FileMode) error { return nil },
		ProbeCompletion: func(_ context.Context, values map[string]string) (LegacyCompletionProbe, error) {
			return LegacyCompletionProbe{
				MiddlewareReachable: true, InstallationMatches: true, AdministratorExists: true,
				Commit: &setup.CommitJournal{
					OperationID: "legacy-operation", InstallationID: values["INSTALLATION_ID"],
					RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 7,
					RequestDigest: strings.Repeat("a", 64),
				},
			}, nil
		},
		WriteRuntimeEnv:   func(_ string, content []byte) error { writtenEnv = append([]byte(nil), content...); return nil },
		WriteInstallState: func(_ string, state setup.InstallState) error { writtenState = state; return nil },
		WriteManifest:     func(string, []byte) error { return nil }, WriteDeploymentFile: func(string, []byte) error { return nil },
	})
	if err != nil || !result.Completed {
		t.Fatalf("completed import = %#v, %v", result, err)
	}
	if writtenState.Validate() != nil || writtenState.Phase != setup.InstallPhaseCompleted || writtenState.Commit == nil || writtenState.Commit.ConfigRevision != 7 {
		t.Fatalf("completed state = %#v", writtenState)
	}
	document, err := config.ParseRuntimeEnv(writtenEnv)
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["SETUP_COMPLETED"] != "true" || document.Values["SETUP_TOKEN"] != "" || document.Values["CONFIG_REVISION"] != "7" {
		t.Fatalf("completed runtime values = %#v", document.Values)
	}
}

func testImportOptions() ImportConfigOptions {
	return ImportConfigOptions{Source: ".env", RuntimeDir: ".", Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle, StorageDriver: "local", ApplicationVersion: "v2"}
}
