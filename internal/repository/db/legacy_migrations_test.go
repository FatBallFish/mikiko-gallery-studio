package db

import (
	"context"
	"testing"
)

func TestPrepareLegacyDataSkipsNonPostgresDatabases(t *testing.T) {
	for _, url := range []string{"file:test.db", "sqlite://test.db", ":memory:"} {
		if err := PrepareLegacyData(context.Background(), url); err != nil {
			t.Fatalf("PrepareLegacyData(%q): %v", url, err)
		}
	}
}

func TestIsPostgresURL(t *testing.T) {
	for _, url := range []string{"postgres://db/app", " postgresql://db/app "} {
		if !isPostgresURL(url) {
			t.Fatalf("expected postgres URL: %q", url)
		}
	}
	if isPostgresURL("file:app.db") {
		t.Fatal("sqlite URL must not be treated as postgres")
	}
}
