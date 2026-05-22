package entstore_test

import (
	"context"
	"testing"

	"entgo.io/ent/dialect"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	_ "github.com/mattn/go-sqlite3"
)

func TestModelAdminStorePersistsProvidersRoutesAndRuntimeSnapshot(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladminstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	openAI, err := store.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "openai", ProviderType: "openai", HealthStatus: "healthy", Enabled: false})
	if err != nil {
		t.Fatalf("CreateProvider openai: %v", err)
	}
	openRouter, err := store.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "openrouter", ProviderType: "openrouter", HealthStatus: "healthy", Enabled: true})
	if err != nil {
		t.Fatalf("CreateProvider openrouter: %v", err)
	}
	if openAI.ID == 0 || openRouter.ID == 0 {
		t.Fatalf("expected provider ids, got %#v %#v", openAI, openRouter)
	}

	route, err := store.CreateRoute(ctx, domainmodeladmin.RouteWriteRequest{
		GroupCode:     "plus",
		TaskType:      "text_to_image",
		ProviderCode:  "openrouter",
		Priority:      0,
		FallbackOrder: 1,
		WeightPercent: 100,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if route.ProviderModelID != openRouter.ID || route.ProviderCode != "openrouter" {
		t.Fatalf("route should store provider id compatibility, got %#v", route)
	}

	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("ModelRoutingConfig: %v", err)
	}
	if len(snapshot.Providers) != 2 || len(snapshot.Routes) != 1 {
		t.Fatalf("unexpected snapshot %#v", snapshot)
	}
	if snapshot.Routes[0].ProviderCode != "openrouter" {
		t.Fatalf("expected provider code in runtime route, got %#v", snapshot.Routes[0])
	}
	if err := store.DeleteProvider(ctx, "openrouter"); err == nil {
		t.Fatal("expected provider with routes to fail deletion")
	}

	if err := store.DeleteRoute(ctx, route.ID); err != nil {
		t.Fatalf("DeleteRoute: %v", err)
	}
	if err := store.DeleteProvider(ctx, "openrouter"); err != nil {
		t.Fatalf("DeleteProvider: %v", err)
	}
}
