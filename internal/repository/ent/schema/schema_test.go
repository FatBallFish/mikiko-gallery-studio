package schema

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
)

func TestCoreSchemaFilesExist(t *testing.T) {
	expected := []string{
		"installation.go",
		"clusternode.go",
		"clustertoken.go",
		"user.go",
		"usergroup.go",
		"adminuser.go",
		"refreshsession.go",
		"apikey.go",
		"apikeyquotareservation.go",
		"redeemcode.go",
		"pointledger.go",
		"modelprovider.go",
		"modelroute.go",
		"providererrorpolicy.go",
		"configitem.go",
		"objectstorageconfig.go",
		"referenceasset.go",
		"imagetask.go",
		"imageresult.go",
		"project.go",
		"objectdeletionjob.go",
		"auditlog.go",
		"textmodelaccount.go",
		"textmodel.go",
		"promptoptimizationrun.go",
	}

	for _, name := range expected {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("expected schema file %s: %v", name, err)
		}
	}

	migrationPath := filepath.Join("..", "migrations", "000001_init.sql")
	if _, err := os.Stat(migrationPath); err != nil {
		t.Fatalf("expected initial migration file: %v", err)
	}
	contents, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read initial migration file: %v", err)
	}
	migration := string(contents)
	expectedMigrationSnippets := []string{
		"create table if not exists installations",
		"check (singleton_key = 'installation')",
		"create table if not exists cluster_nodes",
		"create table if not exists cluster_tokens",
		"create table if not exists cluster_challenges",
		"token_hash varchar(64) not null",
		"token_proof_public_key varchar(43) not null",
		"consumed_by_node_id varchar(128)",
		"actor_id varchar(128)",
		"target_id varchar(128)",
		"create table if not exists api_key_quota_reservations",
		"create index if not exists apikey_user_id on api_keys (user_id)",
		"create index if not exists apikey_status on api_keys (status)",
		"create index if not exists apikey_group_code on api_keys (group_code)",
		"create index if not exists apikeyquotareservation_api_key_id_status on api_key_quota_reservations (api_key_id, status)",
	}
	for _, snippet := range expectedMigrationSnippets {
		if !strings.Contains(migration, snippet) {
			t.Fatalf("expected initial migration to contain %q", snippet)
		}
	}
}

func TestInstallationSchemaEnforcesSingletonIdentityAndVersions(t *testing.T) {
	fields := schemaFieldDescriptors(Installation{}.Fields())
	for _, name := range []string{
		"singleton_key",
		"installation_id",
		"config_schema_version",
		"database_schema_version",
		"app_version",
		"setup_operation_id",
		"setup_admin_id",
		"setup_config_revision",
		"setup_request_digest",
		"initialized_at",
		"migrated_at",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("installation schema is missing %s", name)
		}
	}
	if fields["singleton_key"].Unique == false {
		t.Fatal("installation singleton_key must be unique")
	}
	if !hasIndexFields(Installation{}.Indexes(), []string{"installation_id"}, true) {
		t.Fatal("installations must uniquely index installation_id")
	}
	if !hasIndexFields(Installation{}.Indexes(), []string{"setup_operation_id"}, true) {
		t.Fatal("installations must uniquely index setup_operation_id")
	}
	if !fields["setup_request_digest"].Sensitive {
		t.Fatal("setup_request_digest must be marked sensitive")
	}
	annotations := Installation{}.Annotations()
	if len(annotations) != 1 {
		t.Fatalf("installation annotations = %d, want 1", len(annotations))
	}
	annotation, ok := annotations[0].(entsql.Annotation)
	if !ok || !strings.Contains(annotation.Check, "singleton_key = 'installation'") {
		t.Fatalf("installation singleton_key must have a database CHECK constraint, got %#v", annotations[0])
	}
}

func TestClusterNodeSchemaCarriesRuntimeCompatibilityAndHealth(t *testing.T) {
	fields := schemaFieldDescriptors(ClusterNode{}.Fields())
	for _, name := range []string{
		"node_id",
		"installation_id",
		"role",
		"app_version",
		"runtime_schema_version",
		"config_revision",
		"health",
		"last_error",
		"last_heartbeat_at",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("cluster node schema is missing %s", name)
		}
	}
	if fields["node_id"].Unique == false {
		t.Fatal("cluster node_id must be unique")
	}
	if !hasIndexFields(ClusterNode{}.Indexes(), []string{"installation_id", "role"}, false) {
		t.Fatal("cluster nodes must index installation_id and role")
	}
}

func TestClusterTokenSchemaPersistsHashesAndAuditDataOnly(t *testing.T) {
	fields := schemaFieldDescriptors(ClusterToken{}.Fields())
	for _, name := range []string{
		"token_id",
		"token_hash",
		"installation_id",
		"role",
		"expires_at",
		"consumed_at",
		"revoked_at",
		"audit_actor",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("cluster token schema is missing %s", name)
		}
	}
	for name := range fields {
		lower := strings.ToLower(name)
		if lower == "token" || strings.Contains(lower, "plaintext") || strings.Contains(lower, "secret") {
			t.Fatalf("cluster token schema must not persist plaintext token material: %s", name)
		}
	}
	if fields["token_id"].Unique == false {
		t.Fatal("cluster token_id must be unique")
	}
	if fields["token_hash"].Unique == false {
		t.Fatal("cluster token_hash must be unique so token material cannot be reused")
	}
	if !fields["token_hash"].Sensitive {
		t.Fatal("cluster token_hash must be marked sensitive")
	}
}

func schemaFieldDescriptors(fields []ent.Field) map[string]*field.Descriptor {
	descriptors := make(map[string]*field.Descriptor, len(fields))
	for _, schemaField := range fields {
		descriptor := schemaField.Descriptor()
		descriptors[descriptor.Name] = descriptor
	}
	return descriptors
}

func TestSubscriptionPlanSchemaCarriesPurchaseTypeContract(t *testing.T) {
	fields := SubscriptionPlan{}.Fields()
	hasPlanType := false
	hasPurchaseEnabled := false
	for _, field := range fields {
		descriptor := field.Descriptor()
		switch descriptor.Name {
		case "plan_type":
			hasPlanType = true
			if descriptor.Default == nil {
				t.Fatalf("plan_type should default to points_package")
			}
		case "purchase_enabled":
			hasPurchaseEnabled = true
			if descriptor.Default == nil {
				t.Fatalf("purchase_enabled should default to true")
			}
		}
	}
	if !hasPlanType || !hasPurchaseEnabled {
		t.Fatalf("subscription_plans should expose plan_type and purchase_enabled fields, got plan_type=%v purchase_enabled=%v", hasPlanType, hasPurchaseEnabled)
	}
}

func TestSubscriptionPlanSchemaCarriesCreditExpiryPolicy(t *testing.T) {
	fields := schemaFieldDescriptors(SubscriptionPlan{}.Fields())
	expiryEnabled, ok := fields["credit_expiry_enabled"]
	if !ok {
		t.Fatal("subscription_plans should expose credit_expiry_enabled")
	}
	if expiryEnabled.Default == nil {
		t.Fatal("credit_expiry_enabled should default to true for existing and new plans")
	}
}

func TestPaymentOrderSchemaSnapshotsCreditPolicy(t *testing.T) {
	fields := schemaFieldDescriptors(PaymentOrder{}.Fields())
	for _, name := range []string{
		"credit_expiry_enabled",
		"credit_valid_days",
		"credited_at",
		"credit_expires_at",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("payment_orders should expose %s", name)
		}
	}
}

func TestImageSchemasCarryProjectAndCapabilityContracts(t *testing.T) {
	accountModelFields := schemaFieldDescriptors(ModelAccountModel{}.Fields())
	for _, name := range []string{
		"supports_custom_ratio",
		"min_width",
		"max_width",
		"min_height",
		"max_height",
		"supported_backgrounds",
	} {
		if _, ok := accountModelFields[name]; !ok {
			t.Fatalf("model_account_models should expose %s", name)
		}
	}

	for schemaName, fields := range map[string]map[string]*field.Descriptor{
		"image_tasks": schemaFieldDescriptors(ImageTask{}.Fields()),
		"task_images": schemaFieldDescriptors(ImageResult{}.Fields()),
	} {
		if _, ok := fields["project_id"]; !ok {
			t.Fatalf("%s should expose project_id", schemaName)
		}
	}
	if _, ok := schemaFieldDescriptors(ImageTask{}.Fields())["background"]; !ok {
		t.Fatal("image_tasks should expose background")
	}

	referenceFields := schemaFieldDescriptors(ReferenceAsset{}.Fields())
	for _, name := range []string{"source_image_result_id", "owns_object"} {
		if _, ok := referenceFields[name]; !ok {
			t.Fatalf("reference_assets should expose %s", name)
		}
	}
}

func TestPaymentProviderInstanceSchemaCarriesCashierContract(t *testing.T) {
	fields := PaymentProviderInstance{}.Fields()
	required := map[string]bool{
		"provider_type":           false,
		"name":                    false,
		"config_encrypted":        false,
		"credentials_fingerprint": false,
		"supported_methods":       false,
		"enabled":                 false,
		"sort_order":              false,
		"scheduler_weight":        false,
		"limits":                  false,
		"refund_enabled":          false,
		"health_status":           false,
		"last_error":              false,
		"last_used_at":            false,
		"metadata":                false,
	}
	for _, field := range fields {
		name := field.Descriptor().Name
		if _, ok := required[name]; ok {
			required[name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Fatalf("payment_provider_instances should expose %s", name)
		}
	}
}

func TestWalletBucketSchemaCarriesReservationIndexes(t *testing.T) {
	if !hasIndexFields(WalletGrant{}.Indexes(), []string{"user_id", "status", "grant_type", "expires_at"}, false) {
		t.Fatal("wallet_grants should index user_id,status,grant_type,expires_at for bucket expiry scans")
	}
	if !hasIndexFields(WalletReservationAllocation{}.Indexes(), []string{"wallet_grant_id", "task_id", "reservation_cycle"}, true) {
		t.Fatal("wallet_reservation_allocations should uniquely index wallet_grant_id,task_id,reservation_cycle")
	}
}

func TestImageArtifactRecoveryAndStorageConfigSchema(t *testing.T) {
	requiredTaskFields := map[string]bool{
		"provider_request_id":        false,
		"upstream_succeeded_at":      false,
		"artifact_recovery_status":   false,
		"artifact_recovery_payload":  false,
		"artifact_attempt_count":     false,
		"artifact_next_retry_at":     false,
		"artifact_last_diagnostic":   false,
		"artifact_storage_config_id": false,
		"artifact_storage_version":   false,
	}
	for _, schemaField := range (ImageTask{}).Fields() {
		name := schemaField.Descriptor().Name
		if _, ok := requiredTaskFields[name]; ok {
			requiredTaskFields[name] = true
		}
	}
	for name, found := range requiredTaskFields {
		if !found {
			t.Fatalf("image_tasks should expose %s", name)
		}
	}

	for name, fields := range map[string][]ent.Field{
		"task_images":      (ImageResult{}).Fields(),
		"reference_assets": (ReferenceAsset{}).Fields(),
	} {
		found := false
		for _, schemaField := range fields {
			if schemaField.Descriptor().Name == "storage_config_id" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s should expose storage_config_id", name)
		}
	}

	if _, err := os.Stat("objectstorageconfig.go"); err != nil {
		t.Fatalf("expected object storage config schema: %v", err)
	}
	migrationBytes, err := os.ReadFile(filepath.Join("..", "migrations", "000001_init.sql"))
	if err != nil {
		t.Fatalf("read initial migration: %v", err)
	}
	migration := string(migrationBytes)
	for _, snippet := range []string{
		"create table if not exists object_storage_configs",
		"artifact_recovery_status",
		"artifact_recovery_payload",
		"artifact_next_retry_at",
		"storage_config_id uuid",
	} {
		if !strings.Contains(migration, snippet) {
			t.Fatalf("expected initial migration to contain %q", snippet)
		}
	}
}

func hasIndexFields(indexes []ent.Index, fields []string, unique bool) bool {
	for _, idx := range indexes {
		descriptor := idx.Descriptor()
		if descriptor.Unique == unique && reflect.DeepEqual(descriptor.Fields, fields) {
			return true
		}
	}
	return false
}
