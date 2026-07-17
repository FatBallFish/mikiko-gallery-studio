package entstore_test

import (
	"context"
	"reflect"
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
		ProviderCode:            "openrouter",
		ModelCode:               "openrouter/vision",
		CompatMode:              "openrouter_chat_image",
		SupportsImageInput:      true,
		SupportedBaseResolution: []string{"1k", "2k"},
		MaxImageCount:           4,
		MaxReferenceImageCount:  2,
		TimeoutMS:               45000,
		InputCost:               "0.12",
		OutputCost:              "0.34",
		Currency:                "USD",
		HealthStatus:            "healthy",
		Enabled:                 true,
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

func TestModelAdminStoreMapsAccountModelCostToRuntimeOutputCost(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-account-cost?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{Name: "paid", AdapterType: "openrouter", AuthType: "api_key", BaseURL: "https://example.com", Status: "enabled", Priority: 1, Weight: 100, ConcurrencyLimit: 1, TimeoutMS: 30000})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	model, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{AccountID: account.ID, ModelCode: "paid/image", DisplayName: "Paid Image", TaskTypes: []string{"text_to_image"}, Quality: []string{"auto"}, CostPerImage: "0.12345", Currency: "USD", Enabled: true})
	if err != nil {
		t.Fatalf("create account model: %v", err)
	}
	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("runtime snapshot: %v", err)
	}
	for _, candidate := range snapshot.ProviderModels {
		if candidate.AccountModelID == model.ID {
			if candidate.OutputCost != "0.12345" {
				t.Fatalf("account model cost must be runtime output cost, got %#v", candidate)
			}
			if !reflect.DeepEqual(candidate.SupportedAspectRatios, []string{"1:1"}) || candidate.MaxImageCount != 1 || candidate.MaxReferenceImageCount != 0 || candidate.SupportsImageInput {
				t.Fatalf("account model safe capability defaults were not preserved: %#v", candidate)
			}
			return
		}
	}
	t.Fatalf("runtime snapshot missing account model %d: %#v", model.ID, snapshot.ProviderModels)
}

func TestModelAdminStoreMapsAccountModelGenerationLimitsToRuntimeSnapshot(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-account-capabilities?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "image-account", AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: "https://example.com", Status: "enabled", Priority: 1, Weight: 100,
		ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	model, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-image-2", DisplayName: "GPT Image 2",
		TaskTypes: []string{"text_to_image"}, Quality: []string{"auto", "high"},
		SupportedRatios: []string{"1:1", "16:9"}, MaxImageCount: 2, MaxReferenceImageCount: 3,
		CostPerImage: "0.04000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create account model: %v", err)
	}

	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("runtime snapshot: %v", err)
	}
	for _, candidate := range snapshot.ProviderModels {
		if candidate.AccountModelID != model.ID {
			continue
		}
		wantRatios := []string{"1:1", "16:9"}
		if !reflect.DeepEqual(candidate.SupportedAspectRatios, wantRatios) {
			t.Fatalf("runtime candidate ratios = %#v, want %#v", candidate.SupportedAspectRatios, wantRatios)
		}
		if candidate.MaxImageCount != 2 || candidate.MaxReferenceImageCount != 3 || !candidate.SupportsImageInput {
			t.Fatalf("runtime candidate limits = output:%d reference:%d supports_input:%t, want output:2 reference:3 supports_input:true", candidate.MaxImageCount, candidate.MaxReferenceImageCount, candidate.SupportsImageInput)
		}
		return
	}
	t.Fatalf("runtime snapshot missing account model %d: %#v", model.ID, snapshot.ProviderModels)
}
