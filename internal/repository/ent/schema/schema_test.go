package schema

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"entgo.io/ent"
)

func TestCoreSchemaFilesExist(t *testing.T) {
	expected := []string{
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
		"referenceasset.go",
		"imagetask.go",
		"imageresult.go",
		"auditlog.go",
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

func hasIndexFields(indexes []ent.Index, fields []string, unique bool) bool {
	for _, idx := range indexes {
		descriptor := idx.Descriptor()
		if descriptor.Unique == unique && reflect.DeepEqual(descriptor.Fields, fields) {
			return true
		}
	}
	return false
}
