package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
