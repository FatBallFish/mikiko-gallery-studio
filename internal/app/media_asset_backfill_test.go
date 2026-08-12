package app

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/migrationcheckpoint"
)

func TestRunMediaAssetBackfillRejectsNonControlRoleBeforeOpeningDatabase(t *testing.T) {
	opened := false
	_, err := runMediaAssetBackfill(t.Context(), "runtime.env", MediaAssetBackfillCommandOptions{}, mediaAssetBackfillDependencies{
		loadRuntime: func(string) (config.Config, error) {
			return config.Config{Runtime: config.RuntimeConfig{DeploymentRole: config.DeploymentRoleWorker}}, nil
		},
		openDatabase: func(context.Context, string) (*repoent.Client, error) {
			opened = true
			return nil, errors.New("must not open")
		},
	})
	if !errors.Is(err, ErrDatabaseMigrationRoleForbidden) {
		t.Fatalf("error = %v, want role forbidden", err)
	}
	if opened {
		t.Fatal("database opened for forbidden role")
	}
}

func TestRunMediaAssetBackfillChecksSchemaBeforeWriting(t *testing.T) {
	client := seedCommandBackfillDatabase(t, 1)
	schemaErr := errors.New("schema mismatch")
	dependencies := commandBackfillDependencies(client)
	dependencies.checkSchemaCompatibility = func(context.Context, *repoent.Client, config.Config) error { return schemaErr }
	_, err := runMediaAssetBackfill(t.Context(), "runtime.env", MediaAssetBackfillCommandOptions{BatchSize: 1}, dependencies)
	if !errors.Is(err, schemaErr) {
		t.Fatalf("error = %v, want schema mismatch", err)
	}
	if count, countErr := client.MediaAsset.Query().Count(t.Context()); countErr != nil || count != 0 {
		t.Fatalf("asset count = %d, %v", count, countErr)
	}
	if count, countErr := client.MigrationCheckpoint.Query().Count(t.Context()); countErr != nil || count != 0 {
		t.Fatalf("checkpoint count = %d, %v", count, countErr)
	}
}

func TestRunMediaAssetBackfillDryRunDoesNotPersistAssetsOrCheckpoint(t *testing.T) {
	client := seedCommandBackfillDatabase(t, 3)
	report, err := runMediaAssetBackfill(t.Context(), "runtime.env", MediaAssetBackfillCommandOptions{
		DryRun: true, BatchSize: 2,
	}, commandBackfillDependencies(client))
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "dry_run" || report.Batches != 2 || report.Processed != 3 || report.WouldCreate != 3 || !report.Completed {
		t.Fatalf("report = %+v", report)
	}
	if count, err := client.MediaAsset.Query().Count(t.Context()); err != nil || count != 0 {
		t.Fatalf("asset count = %d, %v", count, err)
	}
	if exists, err := client.MigrationCheckpoint.Query().Where(migrationcheckpoint.NameEQ("media_asset_backfill_v1")).Exist(t.Context()); err != nil || exists {
		t.Fatalf("checkpoint exists = %t, %v", exists, err)
	}
}

func TestRunMediaAssetBackfillHonorsBatchLimitAndResumesDurably(t *testing.T) {
	client := seedCommandBackfillDatabase(t, 3)
	options := MediaAssetBackfillCommandOptions{BatchSize: 1, MaxBatches: 2}
	report, err := runMediaAssetBackfill(t.Context(), "runtime.env", options, commandBackfillDependencies(client))
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "apply" || report.Batches != 2 || report.Processed != 2 || report.Completed {
		t.Fatalf("limited report = %+v", report)
	}

	report, err = runMediaAssetBackfill(t.Context(), "runtime.env", MediaAssetBackfillCommandOptions{BatchSize: 1, Verify: true, SampleSize: 3}, commandBackfillDependencies(client))
	if err != nil {
		t.Fatal(err)
	}
	if !report.Completed || report.Processed != 3 || report.Verification == nil || !report.Verification.Valid {
		t.Fatalf("resumed report = %+v", report)
	}
}

func commandBackfillDependencies(client *repoent.Client) mediaAssetBackfillDependencies {
	return mediaAssetBackfillDependencies{
		loadRuntime: func(string) (config.Config, error) {
			return config.Config{Runtime: config.RuntimeConfig{DeploymentRole: config.DeploymentRoleControl}, Database: config.DatabaseConfig{URL: "test"}}, nil
		},
		openDatabase:             func(context.Context, string) (*repoent.Client, error) { return client, nil },
		closeDatabase:            func(*repoent.Client) error { return nil },
		checkSchemaCompatibility: func(context.Context, *repoent.Client, config.Config) error { return nil },
	}
}

func seedCommandBackfillDatabase(t *testing.T, count int) *repoent.Client {
	t.Helper()
	client, err := repoent.Open(dialect.SQLite, "file:command-backfill-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(711).SetName("Default").SetNameKey("default").SetIsDefault(true).SetStatus("active").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	task, err := client.ImageTask.Create().SetUserID(711).SetProjectID(project.ID).SetTaskType("text_to_image").SetPrompt("backfill").SetAbstractModel("basic").Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for index := range count {
		_, err := client.ImageResult.Create().SetTaskID(task.ID).SetUserID(711).SetProjectID(project.ID).SetObjectKey(uuid.NewString() + ".png").SetMimeType("image/png").SetSha256(uuid.NewString()).SetFileSizeBytes(int64(index + 1)).Save(t.Context())
		if err != nil {
			t.Fatal(err)
		}
	}
	return client
}

var _ = db.MediaAssetBackfillVerification{}
