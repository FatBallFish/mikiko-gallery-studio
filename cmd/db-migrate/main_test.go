package main

import (
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
)

func TestFormatMigrationResultEscapesApplicationVersionControls(t *testing.T) {
	output := formatMigrationResult(db.MigrationResult{Current: db.SchemaVersion{
		InstallationID:        "installation-test",
		DatabaseSchemaVersion: 1,
		ConfigVersion:         1,
		AppVersion:            "v1\nINJECTED=secret\r",
	}})
	if strings.ContainsAny(output, "\r\n") {
		t.Fatalf("migration output contains raw line controls: %q", output)
	}
	if !strings.Contains(output, `app="v1\nINJECTED=secret\r"`) {
		t.Fatalf("migration output did not safely quote app version: %q", output)
	}
}
