package deployctl

import (
	"context"
	"errors"
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

func TestUpgradeRestoresRuntimeConfigWhenMigrationOrRollFails(t *testing.T) {
	for _, failure := range []string{"migration", "roll"} {
		t.Run(failure, func(t *testing.T) {
			writes := make([]string, 0, 2)
			_, err := Upgrade(context.Background(), UpgradeOptions{RuntimeDir: "runtime", ApplicationVersion: "v2", Migrate: true}, UpgradeDependencies{
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
				ApplyDeployment: func(context.Context, InstallPlan) error {
					if failure == "roll" {
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
		})
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
