package db

import (
	"context"
	"reflect"
	"testing"

	"entgo.io/ent/dialect"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/configitem"
	_ "github.com/mattn/go-sqlite3"
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

func TestBackfillLegacyModelAccountCapabilitiesRunsOnce(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:legacy-model-capabilities?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	legacyRatios := []string{"1:1", "16:9", "9:16", "4:3", "3:4"}
	legacy, err := client.ModelAccountModel.Create().
		SetAccountID(1).
		SetModelCode("legacy-default").
		SetSupportedRatios(legacyRatios).
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(4).
		Save(ctx)
	if err != nil {
		t.Fatalf("create legacy model: %v", err)
	}
	legacyEmptyRatios, err := client.ModelAccountModel.Create().
		SetAccountID(1).
		SetModelCode("legacy-empty-ratios").
		SetSupportedRatios([]string{}).
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(4).
		Save(ctx)
	if err != nil {
		t.Fatalf("create legacy model with empty ratios: %v", err)
	}
	explicit, err := client.ModelAccountModel.Create().
		SetAccountID(1).
		SetModelCode("explicit-capability").
		SetSupportedRatios([]string{"1:1", "16:9"}).
		SetMaxImageCount(1).
		SetMaxReferenceImageCount(4).
		Save(ctx)
	if err != nil {
		t.Fatalf("create explicit model: %v", err)
	}

	updated, err := BackfillLegacyModelAccountCapabilities(ctx, client)
	if err != nil {
		t.Fatalf("BackfillLegacyModelAccountCapabilities: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected both legacy row shapes to be backfilled, got %d", updated)
	}
	legacy, err = client.ModelAccountModel.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload legacy model: %v", err)
	}
	if !reflect.DeepEqual(legacy.SupportedRatios, []string{"1:1"}) || legacy.MaxReferenceImageCount != 0 {
		t.Fatalf("legacy optimistic defaults were not replaced: %#v", legacy)
	}
	legacyEmptyRatios, err = client.ModelAccountModel.Get(ctx, legacyEmptyRatios.ID)
	if err != nil {
		t.Fatalf("reload legacy model with empty ratios: %v", err)
	}
	if !reflect.DeepEqual(legacyEmptyRatios.SupportedRatios, []string{"1:1"}) || legacyEmptyRatios.MaxReferenceImageCount != 0 {
		t.Fatalf("legacy empty ratios and optimistic reference limit were not replaced: %#v", legacyEmptyRatios)
	}
	explicit, err = client.ModelAccountModel.Get(ctx, explicit.ID)
	if err != nil {
		t.Fatalf("reload explicit model: %v", err)
	}
	if !reflect.DeepEqual(explicit.SupportedRatios, []string{"1:1", "16:9"}) || explicit.MaxReferenceImageCount != 4 {
		t.Fatalf("explicit capabilities must not be changed: %#v", explicit)
	}
	markerCount, err := client.ConfigItem.Query().Where(
		configitem.ConfigCategoryEQ("system_migration"),
		configitem.ConfigKeyEQ("model_account_capability_defaults_v1"),
		configitem.ScopeEQ("global"),
	).Count(ctx)
	if err != nil || markerCount != 1 {
		t.Fatalf("expected one migration marker, count=%d err=%v", markerCount, err)
	}

	if _, err := legacy.Update().SetSupportedRatios(legacyRatios).SetMaxReferenceImageCount(4).Save(ctx); err != nil {
		t.Fatalf("restore explicit post-migration capabilities: %v", err)
	}
	updated, err = BackfillLegacyModelAccountCapabilities(ctx, client)
	if err != nil {
		t.Fatalf("second BackfillLegacyModelAccountCapabilities: %v", err)
	}
	if updated != 0 {
		t.Fatalf("migration must run once, second update count=%d", updated)
	}
	legacy, err = client.ModelAccountModel.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload post-migration model: %v", err)
	}
	if !reflect.DeepEqual(legacy.SupportedRatios, legacyRatios) || legacy.MaxReferenceImageCount != 4 {
		t.Fatalf("post-migration explicit capabilities must survive restart: %#v", legacy)
	}
}
