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
	providerModels, err := store.ListProviderModels(ctx, domainmodeladmin.ProviderModelListRequest{Page: 1, PageSize: 10, ProviderCode: "openrouter"})
	if err != nil {
		t.Fatalf("ListProviderModels default: %v", err)
	}
	if providerModels.Total != 1 || len(providerModels.Items) != 1 || providerModels.Items[0].ProviderCode != "openrouter" {
		t.Fatalf("expected default provider model, got %#v", providerModels)
	}
	explicitModel, err := store.CreateProviderModel(ctx, domainmodeladmin.ProviderModelWriteRequest{
		ProviderCode:           "openrouter",
		ModelCode:              "openrouter/vision",
		CompatMode:             "openrouter_chat_image",
		SupportsImageInput:     true,
		SupportedQualities:     []string{"1k", "2k"},
		MaxImageCount:          4,
		MaxReferenceImageCount: 2,
		TimeoutMS:              45000,
		InputCost:              "0.12",
		OutputCost:             "0.34",
		Currency:               "USD",
		HealthStatus:           "healthy",
		Enabled:                true,
	})
	if err != nil {
		t.Fatalf("CreateProviderModel explicit: %v", err)
	}
	if explicitModel.ID == 0 || explicitModel.ModelCode != "openrouter/vision" {
		t.Fatalf("unexpected explicit model %#v", explicitModel)
	}

	route, err := store.CreateRoute(ctx, domainmodeladmin.RouteWriteRequest{
		GroupCode:       "plus",
		TaskType:        "text_to_image",
		ProviderModelID: explicitModel.ID,
		Priority:        0,
		FallbackOrder:   1,
		WeightPercent:   100,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if route.ProviderModelID != explicitModel.ID || route.ProviderCode != "openrouter" {
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

func TestModelAdminStoreRuntimeSnapshotMapsAccountModelCostToOutputCost(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladminstore-cost?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name:        "OpenAI primary",
		AdapterType: "openai",
		AuthType:    "api_key",
		BaseURL:     "https://api.openai.test",
		Credentials: map[string]string{"api_key": "cipher"},
		Status:      domainmodeladmin.ModelAccountStatusEnabled,
	})
	if err != nil {
		t.Fatalf("CreateModelAccount: %v", err)
	}
	model, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID:    account.ID,
		ModelCode:    "gpt-image-1",
		DisplayName:  "GPT Image",
		TaskTypes:    []string{"text_to_image"},
		Qualities:    []string{"1k"},
		CostPerImage: "0.12500",
		Currency:     "USD",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("CreateModelAccountModel: %v", err)
	}

	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("ModelRoutingConfig: %v", err)
	}
	if len(snapshot.ProviderModels) != 1 {
		t.Fatalf("expected one provider candidate, got %#v", snapshot.ProviderModels)
	}
	candidate := snapshot.ProviderModels[0]
	if candidate.AccountModelID != model.ID {
		t.Fatalf("expected account model candidate %d, got %#v", model.ID, candidate)
	}
	if candidate.OutputCost != "0.12500" || candidate.Currency != "USD" {
		t.Fatalf("expected account model cost to populate runtime output cost, got %#v", candidate)
	}
}
