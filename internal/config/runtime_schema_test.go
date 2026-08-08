package config

import (
	"bytes"
	"encoding/base64"
	"slices"
	"strings"
	"testing"
)

func TestApplicationVersionValidationAndRuntimeRoundTrip(t *testing.T) {
	valid := []string{
		"v1.2.3",
		"git-sha",
		"build_2026+linux",
		"sha256:abcdef0123456789",
		strings.Repeat("a", 128),
	}
	for _, value := range valid {
		if err := ValidateApplicationVersion(value); err != nil {
			t.Fatalf("ValidateApplicationVersion(%q): %v", value, err)
		}
	}

	invalid := []string{
		"",
		" v1.2.3",
		"v1.2.3 ",
		"v1 2",
		"v1\t2",
		"v1\rINJECTED=secret",
		"v1\nINJECTED=secret",
		"v1\x00secret",
		"/v1",
		strings.Repeat("a", 129),
	}
	for _, value := range invalid {
		err := ValidateApplicationVersion(value)
		if err == nil {
			t.Fatalf("ValidateApplicationVersion accepted %q", value)
		}
		if (value != "" && strings.Contains(err.Error(), value)) || strings.ContainsAny(err.Error(), "\r\n\x00") {
			t.Fatalf("application version error reflected unsafe input %q: %v", value, err)
		}
	}

	versionField := runtimeSchemaFieldForTest(t, DefaultRuntimeSchema(), "APPLICATION_VERSION")
	if err := versionField.Validate("v2.0.0+git-sha_1"); err != nil {
		t.Fatalf("runtime schema rejected stable application version: %v", err)
	}
	if err := versionField.Validate("v2\nunsafe"); err == nil {
		t.Fatal("runtime schema accepted an unsafe application version")
	}
	rendered, err := RenderRuntimeEnv(DefaultRuntimeSchema(), map[string]string{
		"APPLICATION_VERSION": "v2.0.0+git-sha_1",
	}, nil)
	if err != nil {
		t.Fatalf("render application version: %v", err)
	}
	document, err := ParseRuntimeEnv(rendered)
	if err != nil {
		t.Fatalf("parse rendered application version: %v", err)
	}
	if got := document.Values["APPLICATION_VERSION"]; got != "v2.0.0+git-sha_1" {
		t.Fatalf("application version round trip = %q", got)
	}
}

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
		"DEPLOYMENT_MODE", "DEPLOYMENT_PROFILE", "DEPLOYMENT_TOPOLOGY", "DEPLOYMENT_ROLE", "DEPLOYMENT_MODULES",
		"POSTGRES_MANAGED", "REDIS_MANAGED", "OBJECT_STORAGE_MANAGED",
		"SETUP_COMPLETED", "SETUP_TOKEN",
		"DATABASE_URL", "REDIS_URL", "STORAGE_DRIVER", "STORAGE_S3_ENDPOINT", "STORAGE_S3_SECRET_ACCESS_KEY",
		"AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY", "CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY",
		"PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY", "CLUSTER_ENROLLMENT_SEAL_KEY",
		"PUBLIC_API_URL", "PIC_GALLERY_DOCS_URL", "CORS_ALLOWED_ORIGINS",
		"API_PORT", "GATEWAY_PORT", "MONITORING_PORT", "IMAGE_REGISTRY", "IMAGE_TAG", "RELEASE_VERSION",
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
				Topology: DeploymentTopologySingle, StorageDriver: "s3",
			},
			required: []string{"DEPLOYMENT_TOPOLOGY", "DATABASE_URL", "REDIS_URL", "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "POSTGRES_PASSWORD", "REDIS_PASSWORD", "MINIO_ROOT_PASSWORD", "SETUP_TOKEN"},
		},
		{
			name: "docker core cluster control",
			context: DeploymentContext{
				Mode: DeploymentModeDocker, Profile: DeploymentProfileCore, Role: DeploymentRoleControl,
				Topology: DeploymentTopologyCluster, StorageDriver: "s3",
			},
			required:  []string{"DEPLOYMENT_TOPOLOGY", "DATABASE_URL", "REDIS_URL", "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET", "SETUP_TOKEN", "INSTALLATION_ID", "CLUSTER_NODE_ID"},
			forbidden: []string{"STORAGE_LOCAL_ROOT", "POSTGRES_PASSWORD", "MINIO_ROOT_PASSWORD"},
		},
		{
			name: "native core api replica",
			context: DeploymentContext{
				Mode: DeploymentModeNative, Profile: DeploymentProfileCore, Role: DeploymentRoleAPI,
				Topology: DeploymentTopologyCluster, StorageDriver: "s3", SetupCompleted: true,
			},
			required:  []string{"DATABASE_URL", "REDIS_URL", "STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION", "STORAGE_S3_BUCKET", "AUTH_ACCESS_TOKEN_SECRET", "INSTALLATION_ID", "CLUSTER_NODE_ID"},
			forbidden: []string{"SETUP_TOKEN", "POSTGRES_PASSWORD"},
		},
		{
			name: "native core worker",
			context: DeploymentContext{
				Mode: DeploymentModeNative, Profile: DeploymentProfileCore, Role: DeploymentRoleWorker,
				Topology: DeploymentTopologyCluster, StorageDriver: "s3", SetupCompleted: true,
			},
			required:  []string{"DATABASE_URL", "REDIS_URL", "STORAGE_S3_ACCESS_KEY_ID", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY", "INSTALLATION_ID", "CLUSTER_NODE_ID"},
			forbidden: []string{"SETUP_TOKEN", "PUBLIC_API_URL", "AUTH_ACCESS_TOKEN_SECRET"},
		},
		{
			name: "web node",
			context: DeploymentContext{
				Mode: DeploymentModeNative, Profile: DeploymentProfileCore, Role: DeploymentRoleWeb,
				Topology: DeploymentTopologyCluster, SetupCompleted: true,
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
			fields, err := RequiredRuntimeFields(schema, tt.context)
			if err != nil {
				t.Fatalf("RequiredRuntimeFields rejected valid context: %v", err)
			}
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
		{Mode: DeploymentModeNative, Profile: DeploymentProfileFull, Topology: DeploymentTopologySingle, Role: DeploymentRoleSingle, StorageDriver: "s3"},
		{Mode: DeploymentModeDocker, Profile: DeploymentProfileFull, Topology: DeploymentTopologyCluster, Role: DeploymentRoleControl, StorageDriver: "s3"},
		{Mode: DeploymentModeDocker, Profile: DeploymentProfileCore, Topology: DeploymentTopologyCluster, Role: DeploymentRoleWorker, StorageDriver: "local", SetupCompleted: true},
		{Mode: DeploymentModeDocker, Profile: DeploymentProfileCore, Topology: DeploymentTopologySingle, Role: DeploymentRoleControl, StorageDriver: "local"},
		{Mode: DeploymentModeDocker, Profile: DeploymentProfileCore, Topology: DeploymentTopologyCluster, Role: DeploymentRoleSingle, StorageDriver: "s3"},
		{Mode: "unknown", Profile: DeploymentProfileCore, Topology: DeploymentTopologySingle, Role: DeploymentRoleSingle, StorageDriver: "local"},
	}
	for _, context := range tests {
		if _, err := RequiredRuntimeFields(DefaultRuntimeSchema(), context); err == nil {
			t.Errorf("expected required-field lookup to reject invalid context: %#v", context)
		}
	}
}

func TestRuntimeSchemaRequiresS3EndpointAndRegion(t *testing.T) {
	schema := DefaultRuntimeSchema()
	context := DeploymentContext{
		Mode: DeploymentModeDocker, Profile: DeploymentProfileCore,
		Topology: DeploymentTopologySingle, Role: DeploymentRoleSingle,
		StorageDriver: "s3",
	}
	fields, err := RequiredRuntimeFields(schema, context)
	if err != nil {
		t.Fatalf("RequiredRuntimeFields returned error: %v", err)
	}
	keys := make([]string, 0, len(fields))
	for _, field := range fields {
		keys = append(keys, field.Key)
	}
	for _, key := range []string{"STORAGE_S3_ENDPOINT", "STORAGE_S3_REGION"} {
		if !slices.Contains(keys, key) {
			t.Errorf("S3 deployment must require %s", key)
		}
		field := runtimeSchemaFieldForTest(t, schema, key)
		if strings.Contains(field.DescriptionZH, "可留空") || strings.Contains(strings.ToLower(field.DescriptionEN), "may be empty") {
			t.Errorf("required S3 field %s claims it may be empty", key)
		}
	}
}

func TestRuntimeSchemaSecretValidatorsRejectWhitespaceOnlyValues(t *testing.T) {
	schema := DefaultRuntimeSchema()
	keys := []string{
		"SETUP_TOKEN", "POSTGRES_PASSWORD", "REDIS_PASSWORD", "MINIO_ROOT_PASSWORD",
		"STORAGE_S3_ACCESS_KEY_ID", "STORAGE_S3_SECRET_ACCESS_KEY",
		"AUTH_ACCESS_TOKEN_SECRET", "API_KEY_SIGNING_SECRET_ENCRYPTION_KEY",
		"CASHIER_PROVIDER_CONFIG_ENCRYPTION_KEY", "PIC_GALLERY_SECURE_CONFIG_ENCRYPTION_KEY",
		"PROMPT_OPTIMIZATION_QUOTE_SIGNING_KEY",
	}
	for _, key := range keys {
		field := runtimeSchemaFieldForTest(t, schema, key)
		if err := field.Validate(""); err != nil {
			t.Errorf("%s must allow empty pending value: %v", key, err)
		}
		if err := field.Validate(" \t "); err == nil {
			t.Errorf("%s accepted a whitespace-only provided secret", key)
		}
		if err := field.Validate("generated-secret-value"); err != nil {
			t.Errorf("%s rejected a provided secret: %v", key, err)
		}
	}
}

func TestClusterEnrollmentSealKeyRequiresA32ByteBase64URLValue(t *testing.T) {
	field := runtimeSchemaFieldForTest(t, DefaultRuntimeSchema(), "CLUSTER_ENROLLMENT_SEAL_KEY")
	if err := field.Validate(""); err != nil {
		t.Fatalf("pending empty seal key: %v", err)
	}
	for _, invalid := range []string{"generated-secret-value", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31)), "not+base64url"} {
		if err := field.Validate(invalid); err == nil {
			t.Fatalf("accepted invalid seal key %q", invalid)
		}
	}
	valid := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32))
	if err := field.Validate(valid); err != nil {
		t.Fatalf("rejected valid seal key: %v", err)
	}
}

func TestRuntimeSchemaPersistsSetupTokenVersionForPendingAuthorities(t *testing.T) {
	schema := DefaultRuntimeSchema()
	versionField := runtimeSchemaFieldForTest(t, schema, "SETUP_TOKEN_VERSION")
	if versionField.Secret {
		t.Fatal("SETUP_TOKEN_VERSION must not be classified as secret")
	}
	if versionField.DefaultValue != "1" {
		t.Fatalf("SETUP_TOKEN_VERSION default = %q, want 1", versionField.DefaultValue)
	}
	if strings.TrimSpace(versionField.DescriptionZH) == "" || strings.TrimSpace(versionField.DescriptionEN) == "" {
		t.Fatal("SETUP_TOKEN_VERSION requires bilingual operational documentation")
	}
	for _, value := range []string{"", "0", "-1", "1.5", "18446744073709551616"} {
		if err := versionField.Validate(value); err == nil {
			t.Errorf("SETUP_TOKEN_VERSION validator accepted %q", value)
		}
	}
	for _, value := range []string{"1", "2", "18446744073709551615"} {
		if err := versionField.Validate(value); err != nil {
			t.Errorf("SETUP_TOKEN_VERSION validator rejected %q: %v", value, err)
		}
	}

	for _, role := range []DeploymentRole{DeploymentRoleSingle, DeploymentRoleControl} {
		context := DeploymentContext{
			Mode: DeploymentModeDocker, Profile: DeploymentProfileCore,
			Topology: DeploymentTopologySingle, Role: role,
			StorageDriver: "local", SetupCompleted: false,
		}
		if role == DeploymentRoleControl {
			context.Topology = DeploymentTopologyCluster
			context.StorageDriver = "s3"
		}
		fields, err := RequiredRuntimeFields(schema, context)
		if err != nil {
			t.Fatalf("RequiredRuntimeFields(%s): %v", role, err)
		}
		keys := make([]string, 0, len(fields))
		for _, field := range fields {
			keys = append(keys, field.Key)
		}
		if !slices.Contains(keys, "SETUP_TOKEN_VERSION") {
			t.Errorf("pending %s must require SETUP_TOKEN_VERSION", role)
		}
	}

	completed, err := RequiredRuntimeFields(schema, DeploymentContext{
		Mode: DeploymentModeDocker, Profile: DeploymentProfileCore,
		Topology: DeploymentTopologySingle, Role: DeploymentRoleSingle,
		StorageDriver: "local", SetupCompleted: true,
	})
	if err != nil {
		t.Fatalf("completed required fields: %v", err)
	}
	completedKeys := make([]string, 0, len(completed))
	for _, field := range completed {
		completedKeys = append(completedKeys, field.Key)
	}
	if slices.Contains(completedKeys, "SETUP_TOKEN") || !slices.Contains(completedKeys, "SETUP_TOKEN_VERSION") {
		t.Fatalf("completed authority must drop token but retain its version: %v", completedKeys)
	}

	joined, err := RequiredRuntimeFields(schema, DeploymentContext{
		Mode: DeploymentModeDocker, Profile: DeploymentProfileCore,
		Topology: DeploymentTopologyCluster, Role: DeploymentRoleWorker,
		StorageDriver: "s3", SetupCompleted: true,
	})
	if err != nil {
		t.Fatalf("joined required fields: %v", err)
	}
	joinedKeys := make([]string, 0, len(joined))
	for _, field := range joined {
		joinedKeys = append(joinedKeys, field.Key)
	}
	if slices.Contains(joinedKeys, "SETUP_TOKEN") || slices.Contains(joinedKeys, "SETUP_TOKEN_VERSION") {
		t.Fatalf("joined node must not receive setup credential material: %v", joinedKeys)
	}
}

func TestRuntimeSchemaDurationValidationUsesGoDurationSyntax(t *testing.T) {
	field := runtimeSchemaFieldForTest(t, DefaultRuntimeSchema(), "DATABASE_CONN_MAX_LIFETIME")
	for _, value := range []string{"1h30m", "1.5s", "250ms"} {
		if err := field.Validate(value); err != nil {
			t.Errorf("valid Go duration %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"1 hour", "forever", "1"} {
		if err := field.Validate(value); err == nil {
			t.Errorf("invalid Go duration %q accepted", value)
		}
	}
}

func TestRuntimeSchemaConnectionValidationRedactsCredentials(t *testing.T) {
	schema := DefaultRuntimeSchema()
	for _, key := range []string{"DATABASE_URL", "REDIS_URL"} {
		field := runtimeSchemaFieldForTest(t, schema, key)
		raw := "postgres://sensitive-user:top-secret@db.example/%zz"
		if key == "REDIS_URL" {
			raw = "redis://sensitive-user:top-secret@cache.example/%zz"
		}
		err := field.Validate(raw)
		if err == nil {
			t.Fatalf("%s validator accepted malformed percent escape", key)
		}
		if got := err.Error(); got != "connection URL is invalid" {
			t.Errorf("%s returned unstable or non-redacted error %q", key, got)
		}
		for _, sensitive := range []string{raw, "sensitive-user", "top-secret"} {
			if strings.Contains(err.Error(), sensitive) {
				t.Errorf("%s validation error exposed sensitive input %q: %v", key, sensitive, err)
			}
		}
	}
}

func runtimeSchemaFieldForTest(t *testing.T, schema RuntimeSchema, key string) RuntimeField {
	t.Helper()
	for _, field := range schema.Fields {
		if field.Key == key {
			return field
		}
	}
	t.Fatalf("runtime schema field %s not found", key)
	return RuntimeField{}
}
