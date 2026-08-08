package entstore_test

import (
	"context"
	"reflect"
	"testing"

	"entgo.io/ent/dialect"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
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
	model, err := store.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{AccountID: account.ID, ModelCode: "paid/image", DisplayName: "Paid Image", TaskTypes: []string{"text_to_image"}, Quality: []string{"auto"}, MaxImageCount: 1, CostPerImage: "0.12345", Currency: "USD", Enabled: true})
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
			if len(candidate.SupportedAspectRatios) != 0 || candidate.MaxImageCount != 1 || candidate.MaxReferenceImageCount != 0 || candidate.SupportsImageInput {
				t.Fatalf("runtime mapping must not invent aspect ratios or lose safe limits: %#v", candidate)
			}
			if candidate.ConcurrencyLimit != account.ConcurrencyLimit {
				t.Fatalf("model account concurrency must be propagated to runtime scheduling, got %#v", candidate)
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

func TestModelAdminStoreNormalizesOnlyLegacyDefaultSizeBoundsForEditing(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:modeladmin-legacy-size-bounds?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewModelAdminStore(client)
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "legacy-bounds", AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: "https://example.com", Status: "disabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	legacy, err := client.ModelAccountModel.Create().
		SetAccountID(account.ID).
		SetModelCode("legacy-4096").
		SetDisplayName("Legacy 4096").
		SetTaskTypes([]string{"text_to_image"}).
		SetMaxImageCount(1).
		SetMaxWidth(4096).
		SetMaxHeight(4096).
		Save(ctx)
	if err != nil {
		t.Fatalf("create legacy model: %v", err)
	}
	explicitInvalid, err := client.ModelAccountModel.Create().
		SetAccountID(account.ID).
		SetModelCode("explicit-4000").
		SetDisplayName("Explicit 4000").
		SetTaskTypes([]string{"text_to_image"}).
		SetMaxImageCount(1).
		SetMaxWidth(4000).
		SetMaxHeight(4000).
		Save(ctx)
	if err != nil {
		t.Fatalf("create explicit invalid model: %v", err)
	}

	svc := modeladminservice.NewServiceWithStore(store)
	read, err := svc.GetModelAccountModel(ctx, int64(legacy.ID))
	if err != nil {
		t.Fatalf("read legacy model: %v", err)
	}
	if read.MaxWidth != 3840 || read.MaxHeight != 3840 {
		t.Fatalf("legacy defaults must be exposed as editable legal bounds: %#v", read)
	}
	updated, err := svc.UpdateModelAccountModel(ctx, int64(legacy.ID), modelAccountModelWriteRequest(read))
	if err != nil {
		t.Fatalf("update normalized legacy model: %v", err)
	}
	if updated.MaxWidth != 3840 || updated.MaxHeight != 3840 {
		t.Fatalf("updated legacy model has illegal bounds: %#v", updated)
	}
	persisted, err := client.ModelAccountModel.Get(ctx, legacy.ID)
	if err != nil {
		t.Fatalf("reload updated legacy model: %v", err)
	}
	if persisted.MaxWidth != 3840 || persisted.MaxHeight != 3840 {
		t.Fatalf("normalized bounds were not persisted: %#v", persisted)
	}

	invalidRead, err := svc.GetModelAccountModel(ctx, int64(explicitInvalid.ID))
	if err != nil {
		t.Fatalf("read explicit invalid model: %v", err)
	}
	if invalidRead.MaxWidth != 4000 || invalidRead.MaxHeight != 4000 {
		t.Fatalf("non-default invalid bounds must remain visible: %#v", invalidRead)
	}
	if _, err := svc.UpdateModelAccountModel(ctx, int64(explicitInvalid.ID), modelAccountModelWriteRequest(invalidRead)); err == nil {
		t.Fatal("explicit invalid bounds must fail validation instead of being silently clamped")
	}
}

func modelAccountModelWriteRequest(model domainmodeladmin.ModelAccountModel) domainmodeladmin.ModelAccountModelWriteRequest {
	return domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: model.AccountID, ModelCode: model.ModelCode, DisplayName: model.DisplayName,
		TaskTypes: model.TaskTypes, BaseResolution: model.BaseResolution, Quality: model.Quality,
		MaxReferenceImageCount: model.MaxReferenceImageCount, MaxImageCount: model.MaxImageCount,
		SizeModes: model.SizeModes, SupportedRatios: model.SupportedRatios,
		SupportedPixelSizes: model.SupportedPixelSizes, SupportsCustomRatio: model.SupportsCustomRatio,
		OutputFormat: model.OutputFormat, SupportedBackgrounds: model.SupportedBackgrounds,
		OutputCompression: model.OutputCompression, SupportsOutputCompression: model.SupportsOutputCompression,
		SupportsCustomSize: model.SupportsCustomSize, MinWidth: model.MinWidth, MaxWidth: model.MaxWidth,
		MinHeight: model.MinHeight, MaxHeight: model.MaxHeight, Moderation: model.Moderation,
		CostPerImage: model.CostPerImage, Currency: model.Currency, Enabled: model.Enabled, Extra: model.Extra,
	}
}
