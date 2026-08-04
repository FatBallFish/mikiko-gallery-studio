package main

import (
	"context"
	"io"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

func TestRunMigrationCommandReconcilesLegacyBindingOnlyForUpgradeFlag(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantRegular int
		wantUpgrade int
	}{
		{name: "ordinary migration", args: []string{"--env-file", "runtime.env"}, wantRegular: 1},
		{name: "upgrade migration", args: []string{
			"--env-file", "runtime.env", "--reconcile-legacy-setup-binding",
			"--legacy-application-version", "v1.0.0", "--legacy-image-registry", "docker.io/fatballfish",
			"--legacy-image-tag", "v1.0.0", "--legacy-release-version", "",
		}, wantUpgrade: 1},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			regularCalls, upgradeCalls := 0, 0
			regular := func(context.Context, string) (db.MigrationResult, error) {
				regularCalls++
				return db.MigrationResult{}, nil
			}
			upgrade := func(_ context.Context, _ string, identity setup.LegacySetupReleaseIdentity) (db.MigrationResult, error) {
				upgradeCalls++
				if identity.ApplicationVersion != "v1.0.0" || identity.ImageRegistry != "docker.io/fatballfish" || identity.ImageTag != "v1.0.0" {
					t.Fatalf("legacy release identity = %#v", identity)
				}
				return db.MigrationResult{}, nil
			}
			if err := runMigrationCommand(t.Context(), testCase.args, io.Discard, regular, upgrade); err != nil {
				t.Fatal(err)
			}
			if regularCalls != testCase.wantRegular || upgradeCalls != testCase.wantUpgrade {
				t.Fatalf("migration calls regular=%d upgrade=%d", regularCalls, upgradeCalls)
			}
		})
	}
}
