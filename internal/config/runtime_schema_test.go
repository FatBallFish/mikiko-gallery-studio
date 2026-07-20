package config

import (
	"slices"
	"strings"
	"testing"
)

func TestRuntimeSchemaMetadataIsCompleteAndSafe(t *testing.T) {
	schema := DefaultRuntimeSchema()
	if schema.Version <= 0 {
		t.Fatalf("schema version must be positive, got %d", schema.Version)
	}
	if len(schema.Fields) == 0 {
		t.Fatal("runtime schema must contain fields")
	}

	seen := make(map[string]struct{}, len(schema.Fields))
	for _, field := range schema.Fields {
		if field.Key == "" || field.Group == "" {
			t.Fatalf("field key and group must be populated: %#v", field)
		}
		if _, exists := seen[field.Key]; exists {
			t.Fatalf("duplicate runtime field key %q", field.Key)
		}
		seen[field.Key] = struct{}{}
		if strings.TrimSpace(field.DescriptionZH) == "" || strings.TrimSpace(field.DescriptionEN) == "" {
			t.Fatalf("field %s must have bilingual descriptions", field.Key)
		}
		if field.RequiredWhen == nil || field.Validate == nil {
			t.Fatalf("field %s must define required and validation rules", field.Key)
		}
		if field.Secret && strings.TrimSpace(field.Example) != "" {
			t.Fatalf("secret field %s must not contain a real example", field.Key)
		}
	}

	for _, key := range []string{
		"RUNTIME_SCHEMA_VERSION",
		"DEPLOYMENT_MODE", "DEPLOYMENT_PROFILE", "DEPLOYMENT_ROLE", "DEPLOYMENT_MODULES",
		"POSTGRES_MANAGED", "REDIS_MANAGED", "OBJECT_STORAGE_MANAGED",
		"SETUP_COMPLETED", "SETUP_TOKEN",
		"DATABASE_URL", "REDIS_URL", "STORAGE_DRIVER", "STORAGE_S3_ENDPOINT", "STORAGE_S3_SECRET_ACCESS_KEY",
		"AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY",
		"PUBLIC_API_URL", "CORS_ALLOWED_ORIGINS",
		"API_PORT", "GATEWAY_PORT", "IMAGE_REGISTRY", "IMAGE_TAG", "RELEASE_VERSION",
		"INSTALLATION_ID", "CLUSTER_NODE_ID", "CONFIG_REVISION", "APPLICATION_VERSION",
	} {
		if _, exists := seen[key]; !exists {
			t.Errorf("runtime schema is missing required coverage for %s", key)
		}
	}

	for key := range seen {
		if strings.Contains(key, "ADMIN_PASSWORD") || key == "PIC_GALLERY_ADMIN_PASSWORD" {
			t.Errorf("administrator plaintext password must not be persisted: %s", key)
		}
	}
	if err := schema.Validate(); err != nil {
		t.Fatalf("default runtime schema must validate: %v", err)
	}
}

func TestRequiredRuntimeFieldsDeploymentMatrix(t *testing.T) {
	schema := DefaultRuntimeSchema()
	tests := []struct {
		name      string
		context   DeploymentContext
		required  []string
		forbidden []string
	}{
		{
			name: "docker full single",
			context: DeploymentContext{
				Mode: DeploymentModeDocker, Profile: DeploymentProfileFull, Role: DeploymentRoleSingle,
				StorageDriver: "s3",
			},
			required: []string{"DATABASE_URL", "REDIS_URL", "STORAGE_S3_ENDPOINT", "POSTGRES_PASSWORD", "REDIS_PASSWORD", "MINIO_ROOT_PASSWORD", "SETUP_TOKEN"},
		},
		{
			name: "docker core control",
			context: DeploymentContext{
				Mode: DeploymentModeDocker, Profile: DeploymentProfileCore, Role: DeploymentRoleControl,
				StorageDriver: "local",
			},
			required:  []string{"DATABASE_URL", "REDIS_URL", "STORAGE_LOCAL_ROOT", "SETUP_TOKEN", "INSTALLATION_ID", "CLUSTER_NODE_ID"},
			forbidden: []string{"POSTGRES_PASSWORD", "MINIO_ROOT_PASSWORD"},
		},
		{
			name: "native core api replica",
			context: DeploymentContext{
				Mode: DeploymentModeNative, Profile: DeploymentProfileCore, Role: DeploymentRoleAPI,
				MultiNode: true, StorageDriver: "s3", SetupCompleted: true,
			},
			required:  []string{"DATABASE_URL", "REDIS_URL", "STORAGE_S3_BUCKET", "AUTH_ACCESS_TOKEN_SECRET", "INSTALLATION_ID", "CLUSTER_NODE_ID"},
			forbidden: []string{"SETUP_TOKEN", "POSTGRES_PASSWORD"},
		},
		{
			name: "native core worker",
			context: DeploymentContext{
				Mode: DeploymentModeNative, Profile: DeploymentProfileCore, Role: DeploymentRoleWorker,
				MultiNode: true, StorageDriver: "s3", SetupCompleted: true,
			},
			required:  []string{"DATABASE_URL", "REDIS_URL", "STORAGE_S3_ACCESS_KEY_ID", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "INSTALLATION_ID", "CLUSTER_NODE_ID"},
			forbidden: []string{"SETUP_TOKEN", "PUBLIC_API_URL", "AUTH_ACCESS_TOKEN_SECRET"},
		},
		{
			name: "web node",
			context: DeploymentContext{
				Mode: DeploymentModeNative, Profile: DeploymentProfileCore, Role: DeploymentRoleWeb,
				MultiNode: true, SetupCompleted: true,
			},
			required:  []string{"PUBLIC_API_URL", "INSTALLATION_ID", "CLUSTER_NODE_ID", "APPLICATION_VERSION"},
			forbidden: []string{"DATABASE_URL", "REDIS_URL", "STORAGE_DRIVER", "AUTH_ACCESS_TOKEN_SECRET", "SETUP_TOKEN"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateDeploymentContext(tt.context); err != nil {
				t.Fatalf("valid deployment context rejected: %v", err)
			}
			fields := RequiredRuntimeFields(schema, tt.context)
			keys := make([]string, 0, len(fields))
			for _, field := range fields {
				keys = append(keys, field.Key)
			}
			for _, key := range tt.required {
				if !slices.Contains(keys, key) {
					t.Errorf("required fields missing %s; got %v", key, keys)
				}
			}
			for _, key := range tt.forbidden {
				if slices.Contains(keys, key) {
					t.Errorf("field %s must not be required; got %v", key, keys)
				}
			}
		})
	}
}

func TestRequiredRuntimeFieldsRejectsInvalidDeploymentContexts(t *testing.T) {
	tests := []DeploymentContext{
		{Mode: DeploymentModeNative, Profile: DeploymentProfileFull, Role: DeploymentRoleSingle, StorageDriver: "s3"},
		{Mode: DeploymentModeDocker, Profile: DeploymentProfileFull, Role: DeploymentRoleControl, MultiNode: true, StorageDriver: "s3"},
		{Mode: DeploymentModeDocker, Profile: DeploymentProfileCore, Role: DeploymentRoleWorker, MultiNode: true, StorageDriver: "local"},
		{Mode: "unknown", Profile: DeploymentProfileCore, Role: DeploymentRoleSingle, StorageDriver: "local"},
	}
	for _, context := range tests {
		if err := ValidateDeploymentContext(context); err == nil {
			t.Errorf("expected invalid context to fail: %#v", context)
		}
	}
}
