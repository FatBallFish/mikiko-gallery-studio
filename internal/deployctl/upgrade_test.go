package deployctl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestUpgradeMigratesOnceOnControlPreservesExtensionsAndRollsServicesAfterMigration(t *testing.T) {
	events := make([]string, 0, 3)
	var written []byte
	manifestWrites := 0
	result, err := Upgrade(context.Background(), UpgradeOptions{RuntimeDir: "runtime", ApplicationVersion: "v2", ImageTag: "sha-v2", Migrate: true}, UpgradeDependencies{
		LoadInstallation: func(string) (InstallPlan, config.RuntimeEnvDocument, error) {
			return upgradeTestPlan(config.DeploymentRoleControl), upgradeTestDocument(config.DeploymentRoleControl), nil
		},
		WriteRuntimeEnv: func(_ string, content []byte) error {
			events = append(events, "write")
			written = append([]byte(nil), content...)
			return nil
		},
		WriteManifest: func(_ string, plan InstallPlan) error {
			manifestWrites++
			if plan.ApplicationVersion != "v2" || plan.ImageTag != "sha-v2" {
				t.Fatalf("manifest plan was not upgraded: %#v", plan)
			}
			return nil
		},
		Migrate:         func(context.Context, string) error { events = append(events, "migrate"); return nil },
		ApplyDeployment: func(context.Context, InstallPlan) error { events = append(events, "roll"); return nil },
	})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if got := strings.Join(events, ","); got != "write,migrate,roll" || !result.Migrated || manifestWrites != 1 {
		t.Fatalf("upgrade events/result = %q, %#v", got, result)
	}
	document, err := config.ParseRuntimeEnv(written)
	if err != nil {
		t.Fatal(err)
	}
	if document.Values["APPLICATION_VERSION"] != "v2" || document.Values["IMAGE_TAG"] != "sha-v2" || document.Values["CUSTOM_EXTENSION"] != "preserve-me" {
		t.Fatalf("upgraded runtime values = %#v", document.Values)
	}
	if !strings.Contains(string(written), "# [中文]") || !strings.Contains(string(written), "# [English]") {
		t.Fatal("schema upgrade did not refresh bilingual comments")
	}
}

func TestUpgradeRefusesMigrationOutsideSingleOrControlAndDoesNotWrite(t *testing.T) {
	writes := 0
	_, err := Upgrade(context.Background(), UpgradeOptions{RuntimeDir: "runtime", ApplicationVersion: "v2", Migrate: true}, UpgradeDependencies{
		LoadInstallation: func(string) (InstallPlan, config.RuntimeEnvDocument, error) {
			return upgradeTestPlan(config.DeploymentRoleWorker), upgradeTestDocument(config.DeploymentRoleWorker), nil
		},
		WriteRuntimeEnv: func(string, []byte) error { writes++; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "cannot execute migrations") || writes != 0 {
		t.Fatalf("non-control migration = err %v, writes %d", err, writes)
	}
}

func TestUpgradeRestoresRuntimeConfigWhenMigrationOrUnmigratedRollFails(t *testing.T) {
	for _, failure := range []string{"migration", "roll_without_migration"} {
		t.Run(failure, func(t *testing.T) {
			writes := make([]string, 0, 2)
			appliedVersions := make([]string, 0, 2)
			migrate := failure == "migration"
			_, err := Upgrade(context.Background(), UpgradeOptions{RuntimeDir: "runtime", ApplicationVersion: "v2", Migrate: migrate}, UpgradeDependencies{
				LoadInstallation: func(string) (InstallPlan, config.RuntimeEnvDocument, error) {
					return upgradeTestPlan(config.DeploymentRoleControl), upgradeTestDocument(config.DeploymentRoleControl), nil
				},
				WriteRuntimeEnv: func(_ string, content []byte) error { writes = append(writes, string(content)); return nil },
				Migrate: func(context.Context, string) error {
					if failure == "migration" {
						return errors.New("migration failed")
					}
					return nil
				},
				ApplyDeployment: func(_ context.Context, plan InstallPlan) error {
					appliedVersions = append(appliedVersions, plan.ApplicationVersion)
					if len(appliedVersions) == 1 {
						return errors.New("roll failed")
					}
					return nil
				},
			})
			if err == nil || len(writes) != 2 {
				t.Fatalf("failure %s = err %v, writes %d", failure, err, len(writes))
			}
			restored, parseErr := config.ParseRuntimeEnv([]byte(writes[1]))
			if parseErr != nil || restored.Values["APPLICATION_VERSION"] != "v1" {
				t.Fatalf("runtime config was not restored: %v, %#v", parseErr, restored.Values)
			}
			if failure == "roll_without_migration" && strings.Join(appliedVersions, ",") != "v2,v1" {
				t.Fatalf("roll recovery plans = %q, want v2,v1", strings.Join(appliedVersions, ","))
			}
		})
	}
}

func TestUpgradeRollbackUsesBoundedContextAfterRollContextIsCanceled(t *testing.T) {
	upgradeContext, cancelUpgrade := context.WithCancel(context.Background())
	appliedVersions := make([]string, 0, 2)
	_, err := Upgrade(upgradeContext, UpgradeOptions{RuntimeDir: "runtime", ApplicationVersion: "v2"}, UpgradeDependencies{
		LoadInstallation: func(string) (InstallPlan, config.RuntimeEnvDocument, error) {
			return upgradeTestPlan(config.DeploymentRoleControl), upgradeTestDocument(config.DeploymentRoleControl), nil
		},
		WriteRuntimeEnv: func(string, []byte) error { return nil },
		WriteManifest:   func(string, InstallPlan) error { return nil },
		ApplyDeployment: func(ctx context.Context, plan InstallPlan) error {
			appliedVersions = append(appliedVersions, plan.ApplicationVersion)
			if len(appliedVersions) == 1 {
				cancelUpgrade()
				return context.Canceled
			}
			if ctx.Err() != nil {
				return fmt.Errorf("rollback context is already canceled: %w", ctx.Err())
			}
			if _, ok := ctx.Deadline(); !ok {
				return errors.New("rollback context has no deadline")
			}
			return nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("Upgrade error = %v, want original cancellation", err)
	}
	if strings.Contains(err.Error(), "restore previous deployment") {
		t.Fatalf("Upgrade rollback failed after cancellation: %v", err)
	}
	if got := strings.Join(appliedVersions, ","); got != "v2,v1" {
		t.Fatalf("applied versions = %q, want v2,v1", got)
	}
}

func TestUpgradeRetainsTargetPlanForRetryAfterSuccessfulMigrationAndRollFailure(t *testing.T) {
	writes := make([]string, 0, 2)
	manifestVersions := make([]string, 0, 2)
	migrations := 0
	_, err := Upgrade(context.Background(), UpgradeOptions{RuntimeDir: "runtime", ApplicationVersion: "v2", Migrate: true}, UpgradeDependencies{
		LoadInstallation: func(string) (InstallPlan, config.RuntimeEnvDocument, error) {
			return upgradeTestPlan(config.DeploymentRoleControl), upgradeTestDocument(config.DeploymentRoleControl), nil
		},
		WriteRuntimeEnv: func(_ string, content []byte) error {
			writes = append(writes, string(content))
			return nil
		},
		WriteManifest: func(_ string, plan InstallPlan) error {
			manifestVersions = append(manifestVersions, plan.ApplicationVersion)
			return nil
		},
		Migrate:         func(context.Context, string) error { migrations++; return nil },
		ApplyDeployment: func(context.Context, InstallPlan) error { return errors.New("roll failed") },
	})
	if err == nil || !strings.Contains(err.Error(), "rerun upgrade") {
		t.Fatalf("roll failure error=%v, want explicit forward recovery guidance", err)
	}
	if migrations != 1 || len(writes) != 1 || len(manifestVersions) != 1 || manifestVersions[0] != "v2" {
		t.Fatalf("forward recovery mutated target state: migrations=%d writes=%d manifests=%v", migrations, len(writes), manifestVersions)
	}
	document, parseErr := config.ParseRuntimeEnv([]byte(writes[0]))
	if parseErr != nil || document.Values["APPLICATION_VERSION"] != "v2" {
		t.Fatalf("target runtime was not retained: %v, %#v", parseErr, document.Values)
	}
}

func upgradeTestPlan(role config.DeploymentRole) InstallPlan {
	components := []Component{ComponentWorker}
	if role == config.DeploymentRoleControl {
		components = []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway}
	}
	return InstallPlan{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologyCluster, Role: role, Components: components, RuntimeDir: "runtime", StorageDriver: "s3", ApplicationVersion: "v1", ImageTag: "v1", APIPort: "8080", GatewayPort: "80", UserWebPort: "5173", AdminWebPort: "5174", DocsWebPort: "5175", RequiresEnrollment: role == config.DeploymentRoleWorker}
}

func upgradeTestDocument(role config.DeploymentRole) config.RuntimeEnvDocument {
	values := map[string]string{
		"RUNTIME_SCHEMA_VERSION": "1", "DEPLOYMENT_MODE": "docker", "DEPLOYMENT_PROFILE": "core", "DEPLOYMENT_TOPOLOGY": "cluster", "DEPLOYMENT_ROLE": string(role),
		"DEPLOYMENT_MODULES": "worker", "POSTGRES_MANAGED": "false", "REDIS_MANAGED": "false", "OBJECT_STORAGE_MANAGED": "false", "SETUP_COMPLETED": "true", "SETUP_TOKEN_VERSION": "1",
		"DATABASE_URL": "postgres://app:secret@db/app", "REDIS_URL": "redis://redis/0", "REDIS_KEY_PREFIX": "app", "STORAGE_DRIVER": "s3", "STORAGE_S3_ENDPOINT": "https://s3.example.test", "STORAGE_S3_REGION": "test-1", "STORAGE_S3_BUCKET": "app", "STORAGE_S3_ACCESS_KEY_ID": "access", "STORAGE_S3_SECRET_ACCESS_KEY": "secret",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY": "secure", "INSTALLATION_ID": "019d0000-0000-7000-8000-000000000111", "CLUSTER_NODE_ID": "019d0000-0000-7000-8000-000000000112", "CONFIG_REVISION": "1", "APPLICATION_VERSION": "v1", "IMAGE_TAG": "v1", "CUSTOM_EXTENSION": "preserve-me",
	}
	return config.RuntimeEnvDocument{Values: values, Extensions: []config.EnvEntry{{Key: "CUSTOM_EXTENSION", Value: "preserve-me"}}}
}

func TestUninstallPreservesRuntimeByDefaultAndRequiresExactPhraseForDestruction(t *testing.T) {
	const installationID = "019d0000-0000-7000-8000-000000000123"
	plan := InstallPlan{RuntimeDir: "runtime"}
	stops, destroys, removals := 0, 0, 0
	deps := UninstallDependencies{
		LoadInstallation:           func(string) (InstallPlan, string, error) { return plan, installationID, nil },
		ValidateRuntimeDirectory:   func(InstallPlan) error { return nil },
		StopDeployment:             func(context.Context, InstallPlan) error { stops++; return nil },
		DestroyPersistentResources: func(context.Context, InstallPlan) error { destroys++; return nil },
		RemoveRuntimeDirectory:     func(string) error { removals++; return nil },
	}
	if err := Uninstall(context.Background(), UninstallOptions{RuntimeDir: "runtime"}, deps); err != nil {
		t.Fatal(err)
	}
	if stops != 1 || destroys != 0 || removals != 0 {
		t.Fatalf("ordinary uninstall side effects = stops %d, destroys %d, removals %d", stops, destroys, removals)
	}
	for _, confirmation := range []string{"", "yes", "DELETE DATA", strings.ToLower(DestructiveUninstallConfirmation(installationID))} {
		if err := Uninstall(context.Background(), UninstallOptions{RuntimeDir: "runtime", DeleteData: true, Confirmation: confirmation}, deps); err == nil {
			t.Errorf("destructive uninstall accepted confirmation %q", confirmation)
		}
	}
	if destroys != 0 || removals != 0 {
		t.Fatalf("invalid confirmations mutated data: destroys %d, removals %d", destroys, removals)
	}
	if err := Uninstall(context.Background(), UninstallOptions{RuntimeDir: "runtime", DeleteData: true, Confirmation: DestructiveUninstallConfirmation(installationID)}, deps); err != nil {
		t.Fatal(err)
	}
	if stops != 2 || destroys != 1 || removals != 1 {
		t.Fatalf("destructive uninstall side effects = stops %d, destroys %d, removals %d", stops, destroys, removals)
	}
}

func TestDestructiveUninstallValidatesManagedRuntimeBeforeDestroyingData(t *testing.T) {
	const installationID = "019d0000-0000-7000-8000-000000000123"
	plan := InstallPlan{RuntimeDir: "runtime"}
	validated, stops, destroys, removals := 0, 0, 0, 0
	err := Uninstall(context.Background(), UninstallOptions{
		RuntimeDir: "runtime", DeleteData: true, Confirmation: DestructiveUninstallConfirmation(installationID),
	}, UninstallDependencies{
		LoadInstallation: func(string) (InstallPlan, string, error) { return plan, installationID, nil },
		ValidateRuntimeDirectory: func(InstallPlan) error {
			validated++
			return errors.New("runtime contains unmanaged file")
		},
		StopDeployment:             func(context.Context, InstallPlan) error { stops++; return nil },
		DestroyPersistentResources: func(context.Context, InstallPlan) error { destroys++; return nil },
		RemoveRuntimeDirectory:     func(string) error { removals++; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "unmanaged") || validated != 1 || stops != 0 || destroys != 0 || removals != 0 {
		t.Fatalf("unsafe uninstall = err %v validated=%d stops=%d destroys=%d removals=%d", err, validated, stops, destroys, removals)
	}
}

func TestValidateManagedRuntimeDirectoryRejectsUnmanagedFiles(t *testing.T) {
	runtimeDir := t.TempDir()
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore,
		Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
		RuntimeDir: runtimeDir, StorageDriver: "local", ApplicationVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "operator-notes.txt"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateManagedRuntimeDirectory(plan); err == nil || !strings.Contains(err.Error(), "operator-notes.txt") {
		t.Fatalf("validateManagedRuntimeDirectory error=%v, want unmanaged path", err)
	}
}

func TestValidateManagedNativeRuntimeRejectsUnmanagedReleaseAndServiceFiles(t *testing.T) {
	for _, testCase := range []struct {
		relativePath string
		errorPath    string
	}{
		{relativePath: filepath.Join("web", "operator-notes.txt"), errorPath: "native release target web"},
		{relativePath: filepath.Join("services", "custom.service"), errorPath: filepath.Join("services", "custom.service")},
	} {
		t.Run(testCase.relativePath, func(t *testing.T) {
			runtimeDir := t.TempDir()
			plan := writeManagedNativeRuntimeForUninstallTest(t, runtimeDir, NativePlatformLinux)
			if err := validateManagedRuntimeDirectoryForPlatform(plan, NativePlatformLinux); err != nil {
				t.Fatalf("managed native runtime was rejected: %v", err)
			}
			unmanagedPath := filepath.Join(runtimeDir, testCase.relativePath)
			if err := os.MkdirAll(filepath.Dir(unmanagedPath), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(unmanagedPath, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := validateManagedRuntimeDirectoryForPlatform(plan, NativePlatformLinux); err == nil || !strings.Contains(err.Error(), testCase.errorPath) {
				t.Fatalf("validateManagedRuntimeDirectory error=%v, want unmanaged path %s", err, testCase.relativePath)
			}
		})
	}
}

func writeManagedNativeRuntimeForUninstallTest(t *testing.T, runtimeDir string, platform NativePlatform) InstallPlan {
	t.Helper()
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore,
		Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
		RuntimeDir: runtimeDir, StorageDriver: "s3", ApplicationVersion: "v1", ReleaseVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	document := upgradeTestDocument(config.DeploymentRoleSingle)
	document.Values["DEPLOYMENT_MODE"] = string(config.DeploymentModeNative)
	document.Values["DEPLOYMENT_PROFILE"] = string(config.DeploymentProfileCore)
	document.Values["DEPLOYMENT_TOPOLOGY"] = string(config.DeploymentTopologySingle)
	document.Values["DEPLOYMENT_ROLE"] = string(config.DeploymentRoleSingle)
	document.Values["RELEASE_VERSION"] = "v1"
	runtimeEnv, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), document.Values, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(runtimeDir, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "config", "runtime.env"), runtimeEnv, 0o600); err != nil {
		t.Fatal(err)
	}
	releaseFile := filepath.Join(runtimeDir, "web", "index.html")
	if err := os.MkdirAll(filepath.Dir(releaseFile), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("managed")
	if err := os.WriteFile(releaseFile, content, 0o600); err != nil {
		t.Fatal(err)
	}
	releaseInfo, err := os.Stat(releaseFile)
	if err != nil {
		t.Fatal(err)
	}
	releaseHasher := sha256.New()
	_, _ = fmt.Fprintf(releaseHasher, "native-release-file-v1 mode=%04o\n", releaseInfo.Mode().Perm())
	_, _ = releaseHasher.Write(content)
	archiveDigest := sha256.Sum256([]byte("archive"))
	manifest := nativeReleaseJournal{
		SchemaVersion: nativeReleaseJournalSchema,
		ArchiveSHA256: hex.EncodeToString(archiveDigest[:]),
		Files:         map[string]string{"web/index.html": hex.EncodeToString(releaseHasher.Sum(nil))},
	}
	if err := writeNativeReleaseJournal(filepath.Join(runtimeDir, ".native-release.manifest.json"), manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, ".native-release.sha256"), []byte(manifest.ArchiveSHA256+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	serviceFiles, err := BuildNativeServiceFiles(plan, document.Values["INSTALLATION_ID"], platform)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range serviceFiles {
		path := filepath.Join(runtimeDir, file.RelativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, file.Content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return plan
}
