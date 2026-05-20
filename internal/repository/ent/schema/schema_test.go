package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCoreSchemaFilesExist(t *testing.T) {
	expected := []string{
		"user.go",
		"usergroup.go",
		"adminuser.go",
		"refreshsession.go",
		"apikey.go",
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

	if _, err := os.Stat(filepath.Join("..", "migrations", "000001_init.sql")); err != nil {
		t.Fatalf("expected initial migration file: %v", err)
	}
}
