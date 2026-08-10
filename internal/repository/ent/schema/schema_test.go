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
		"objectreconcilecheckpoint.go",
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
	if defaultValue, ok := expiryEnabled.Default.(bool); !ok || !defaultValue {
		t.Fatalf("credit_expiry_enabled default = %#v, want true", expiryEnabled.Default)
	}
}

func TestPaymentOrderSchemaSnapshotsCreditPolicy(t *testing.T) {
	fields := schemaFieldDescriptors(PaymentOrder{}.Fields())
	expiryEnabled := requireSchemaField(t, fields, "credit_expiry_enabled")
	if defaultValue, ok := expiryEnabled.Default.(bool); !ok || defaultValue {
		t.Fatalf("payment order credit_expiry_enabled default = %#v, want false", expiryEnabled.Default)
	}
	for _, name := range []string{"credit_valid_days", "credited_at", "credit_expires_at"} {
		descriptor := requireSchemaField(t, fields, name)
		if !descriptor.Optional || !descriptor.Nillable {
			t.Fatalf("payment_orders.%s must be Optional and Nillable", name)
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
	maxImageCount := requireSchemaField(t, accountModelFields, "max_image_count")
	if defaultValue, ok := maxImageCount.Default.(int); !ok || defaultValue != 1 {
		t.Fatalf("max_image_count default = %#v, want 1", maxImageCount.Default)
	}
	if len(maxImageCount.Validators) == 0 {
		t.Fatal("max_image_count must enforce the upstream n range 1..10")
	}

	for schemaName, fields := range map[string]map[string]*field.Descriptor{
		"image_tasks": schemaFieldDescriptors(ImageTask{}.Fields()),
		"task_images": schemaFieldDescriptors(ImageResult{}.Fields()),
	} {
		if _, ok := fields["project_id"]; !ok {
			t.Fatalf("%s should expose project_id", schemaName)
		}
		projectID := fields["project_id"]
		if !projectID.Optional || !projectID.Nillable {
			t.Fatalf("%s.project_id must remain Optional and Nillable during backfill", schemaName)
		}
	}
	assertEdgeUsesField(t, "image_tasks", ImageTask{}.Edges(), "project", "project_id", "Project")
	assertEdgeUsesField(t, "task_images", ImageResult{}.Edges(), "project", "project_id", "Project")
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

func TestV012PromptTemplateSchemaContracts(t *testing.T) {
	referenceFields := schemaFieldDescriptors(ReferenceAsset{}.Fields())
	for _, name := range []string{"name", "name_normalized"} {
		descriptor := requireSchemaField(t, referenceFields, name)
		if !descriptor.Optional || !descriptor.Nillable {
			t.Fatalf("reference_assets.%s must be nullable during rolling migration", name)
		}
	}
	if referenceFields["name"].Size != 64 || referenceFields["name_normalized"].Size != 64 {
		t.Fatalf("reference asset names must be limited to 64 characters")
	}
	if !hasIndexFields(ReferenceAsset{}.Indexes(), []string{"user_id", "name_normalized"}, true) {
		t.Fatal("active reference asset names must be unique per user")
	}

	taskFields := schemaFieldDescriptors(ImageTask{}.Fields())
	promptTemplate := requireSchemaField(t, taskFields, "prompt_template")
	if !promptTemplate.Optional || !promptTemplate.Nillable {
		t.Fatal("image_tasks.prompt_template must be nullable for legacy tasks")
	}
	version := requireSchemaField(t, taskFields, "prompt_template_version")
	if defaultValue, ok := version.Default.(int); !ok || defaultValue != 0 {
		t.Fatalf("prompt_template_version default = %#v, want 0", version.Default)
	}
	requireSchemaField(t, taskFields, "prompt_binding_snapshot")
}

func TestModelLifecycleSchemasRetainTombstonesForHistoricalSafety(t *testing.T) {
	for name, mixins := range map[string][]ent.Mixin{
		"route_model_candidates": RouteModelCandidate{}.Mixin(),
		"route_model_prices":     RouteModelPrice{}.Mixin(),
	} {
		fields := map[string]bool{}
		for _, schemaMixin := range mixins {
			for _, schemaField := range schemaMixin.Fields() {
				fields[schemaField.Descriptor().Name] = true
			}
		}
		for _, fieldName := range []string{"created_at", "updated_at", "deleted_at"} {
			if !fields[fieldName] {
				t.Fatalf("%s must retain %s for safe soft deletion", name, fieldName)
			}
		}
	}
}

func TestProjectSchemaEnforcesActiveDefaultAndNameUniqueness(t *testing.T) {
	fields := schemaFieldDescriptors(Project{}.Fields())
	if defaultValue, ok := requireSchemaField(t, fields, "is_default").Default.(bool); !ok || defaultValue {
		t.Fatalf("project is_default default = %#v, want false", fields["is_default"].Default)
	}
	if defaultValue, ok := requireSchemaField(t, fields, "version").Default.(int64); !ok || defaultValue != 1 {
		t.Fatalf("project version default = %#v, want int64(1)", fields["version"].Default)
	}
	assertPartialUniqueIndex(t, Project{}.Indexes(), []string{"user_id"}, "is_default", "status = 'active'", "deleted_at IS NULL")
	assertPartialUniqueIndex(t, Project{}.Indexes(), []string{"user_id", "name_key"}, "status = 'active'", "deleted_at IS NULL")
}

func TestObjectDeletionJobSchemaDeduplicatesLiveObjectIdentity(t *testing.T) {
	fields := schemaFieldDescriptors(ObjectDeletionJob{}.Fields())
	if defaultValue, ok := requireSchemaField(t, fields, "state").Default.(string); !ok || defaultValue != "pending" {
		t.Fatalf("object deletion state default = %#v, want pending", fields["state"].Default)
	}
	if defaultValue, ok := requireSchemaField(t, fields, "attempt_count").Default.(int); !ok || defaultValue != 0 {
		t.Fatalf("object deletion attempt_count default = %#v, want 0", fields["attempt_count"].Default)
	}
	for _, name := range []string{"next_attempt_at", "last_error_code", "last_error_message", "completed_at"} {
		descriptor := requireSchemaField(t, fields, name)
		if !descriptor.Optional || !descriptor.Nillable {
			t.Fatalf("object_deletion_jobs.%s must be Optional and Nillable", name)
		}
	}
	liveStates := "state IN ('pending', 'running', 'retry')"
	assertPartialUniqueIndex(t, ObjectDeletionJob{}.Indexes(), []string{"storage_config_id", "object_key"}, "storage_config_id IS NOT NULL", liveStates)
	assertPartialUniqueIndex(t, ObjectDeletionJob{}.Indexes(), []string{"storage_driver", "bucket", "object_key"}, "storage_config_id IS NULL", liveStates)
}

func TestGalleryExportJobSchemaSupportsDurableLeasesAndTemporaryArchiveCleanup(t *testing.T) {
	fields := schemaFieldDescriptors(GalleryExportJob{}.Fields())
	for _, name := range []string{
		"user_id", "project_id", "image_ids", "state", "estimated_bytes", "archive_size_bytes",
		"storage_config_id", "storage_driver", "bucket", "object_key", "attempt_count",
		"lease_owner", "lease_expires_at", "next_attempt_at", "lifecycle_deadline_at", "expires_at", "last_error_code",
	} {
		if _, ok := fields[name]; !ok {
			t.Fatalf("gallery_export_jobs is missing %q", name)
		}
	}
	if !hasIndexFields(GalleryExportJob{}.Indexes(), []string{"state", "next_attempt_at"}, false) {
		t.Fatal("gallery export jobs need a claim index")
	}
	if !hasIndexFields(GalleryExportJob{}.Indexes(), []string{"state", "expires_at"}, false) {
		t.Fatal("gallery export jobs need an expiry cleanup index")
	}
	if !hasIndexFields(GalleryExportJob{}.Indexes(), []string{"state", "lifecycle_deadline_at"}, false) {
		t.Fatal("gallery export jobs need a lifecycle deadline index")
	}
	if deadline := fields["lifecycle_deadline_at"]; !deadline.Optional || !deadline.Nillable {
		t.Fatal("gallery export lifecycle deadline must be nullable during migration")
	}
}

func TestObjectReconcileCheckpointSchemaPersistsRestartProgress(t *testing.T) {
	fields := schemaFieldDescriptors(ObjectReconcileCheckpoint{}.Fields())
	for _, name := range []string{"storage_identity", "namespace", "prefix", "cursor", "generation"} {
		requireSchemaField(t, fields, name)
	}
	if defaultValue, ok := fields["cursor"].Default.(string); !ok || defaultValue != "" {
		t.Fatalf("object reconcile cursor default = %#v, want empty", fields["cursor"].Default)
	}
	if defaultValue, ok := fields["generation"].Default.(int64); !ok || defaultValue != 0 {
		t.Fatalf("object reconcile generation default = %#v, want int64(0)", fields["generation"].Default)
	}
	if !hasIndexFields(ObjectReconcileCheckpoint{}.Indexes(), []string{"storage_identity", "prefix"}, true) {
		t.Fatal("object reconcile checkpoints must uniquely index storage identity and owned prefix")
	}
	if !hasIndexFields(ObjectReconcileCheckpoint{}.Indexes(), []string{"generation", "updated_at"}, false) {
		t.Fatal("object reconcile checkpoints must index generation and update time for fair scheduling")
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

func requireSchemaField(t *testing.T, fields map[string]*field.Descriptor, name string) *field.Descriptor {
	t.Helper()
	descriptor, ok := fields[name]
	if !ok {
		t.Fatalf("schema is missing field %s", name)
	}
	return descriptor
}

func assertEdgeUsesField(t *testing.T, schemaName string, edges []ent.Edge, edgeName, fieldName, typeName string) {
	t.Helper()
	for _, schemaEdge := range edges {
		descriptor := schemaEdge.Descriptor()
		if descriptor.Name == edgeName && descriptor.Field == fieldName && descriptor.Type == typeName && descriptor.Unique {
			return
		}
	}
	t.Fatalf("%s must define unique %s edge using %s as a Project foreign key", schemaName, edgeName, fieldName)
}

func assertPartialUniqueIndex(t *testing.T, indexes []ent.Index, fields []string, predicates ...string) {
	t.Helper()
	for _, schemaIndex := range indexes {
		descriptor := schemaIndex.Descriptor()
		if !descriptor.Unique || !reflect.DeepEqual(descriptor.Fields, fields) {
			continue
		}
		for _, annotation := range descriptor.Annotations {
			indexAnnotation, ok := annotation.(*entsql.IndexAnnotation)
			if !ok {
				continue
			}
			matches := true
			for _, predicate := range predicates {
				if !strings.Contains(indexAnnotation.Where, predicate) {
					matches = false
					break
				}
			}
			if matches {
				return
			}
		}
	}
	t.Fatalf("schema must define partial unique index on %v with predicates %v", fields, predicates)
}
