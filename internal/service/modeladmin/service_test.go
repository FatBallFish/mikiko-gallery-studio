package modeladmin_test

import (
	"context"
	"testing"

	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/service/modeladmin"
)

func TestServiceValidatesProviderAndRouteCRUD(t *testing.T) {
	ctx := context.Background()
	svc := modeladmin.NewServiceWithStore(nil)

	if _, err := svc.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "OpenAI", ProviderType: "openai", Enabled: true}); err != nil {
		t.Fatalf("CreateProvider: %v", err)
	}
	if _, err := svc.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "openrouter", ProviderType: "openrouter", Enabled: true}); err != nil {
		t.Fatalf("CreateProvider openrouter: %v", err)
	}
	if _, err := svc.CreateProvider(ctx, domainmodeladmin.ProviderWriteRequest{ProviderCode: "", ProviderType: "openai"}); err == nil {
		t.Fatal("expected empty provider code to fail")
	}

	updated, err := svc.UpdateProvider(ctx, "OpenAI", domainmodeladmin.ProviderWriteRequest{ProviderType: "openai", HealthStatus: "healthy", Enabled: false})
	if err != nil {
		t.Fatalf("UpdateProvider: %v", err)
	}
	if updated.ProviderCode != "openai" || updated.Enabled {
		t.Fatalf("unexpected updated provider %#v", updated)
	}

	route, err := svc.CreateRoute(ctx, domainmodeladmin.RouteWriteRequest{
		GroupCode:     "plus",
		TaskType:      "text_to_image",
		ProviderCode:  "openrouter",
		Priority:      1,
		FallbackOrder: 0,
		WeightPercent: 100,
		Enabled:       true,
	})
	if err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	if route.ID == 0 || route.ProviderCode != "openrouter" {
		t.Fatalf("unexpected route %#v", route)
	}
	if _, err := svc.CreateRoute(ctx, domainmodeladmin.RouteWriteRequest{GroupCode: "plus", TaskType: "text_to_image", ProviderCode: "missing", Enabled: true}); err == nil {
		t.Fatal("expected missing provider route to fail")
	}
	if err := svc.DeleteProvider(ctx, "openrouter"); err == nil {
		t.Fatal("expected provider with routes to fail deletion")
	}

	page, err := svc.ListRoutes(ctx, domainmodeladmin.RouteListRequest{Page: 1, PageSize: 10, GroupCode: "plus"})
	if err != nil {
		t.Fatalf("ListRoutes: %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("unexpected route page %#v", page)
	}
}

func TestServiceRejectsRemovedReferenceGenerationConfiguration(t *testing.T) {
	ctx := context.Background()
	svc := modeladmin.NewServiceWithStore(nil)
	account, err := svc.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "image account", AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: "https://images.example.com", Credentials: map[string]string{"api_key": "test-key"},
		Status: "enabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("CreateModelAccount: %v", err)
	}
	if _, err := svc.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "legacy", TaskTypes: []string{"reference_generate"}, Enabled: true,
	}); err == nil {
		t.Fatal("expected removed model task type to be rejected")
	}
	if _, err := svc.CreateRoute(ctx, domainmodeladmin.RouteWriteRequest{
		GroupCode: "plus", TaskType: "reference_to_image", ProviderModelID: 1, Enabled: true,
	}); err == nil {
		t.Fatal("expected removed route task type to be rejected")
	}
	if _, err := svc.CreateRouteModelPrice(ctx, domainmodeladmin.RouteModelPriceWriteRequest{
		RouteModelID: 1, TaskType: "reference_generate", BaseResolution: "1k", BasePoints: "1.00000", Enabled: true,
	}); err == nil {
		t.Fatal("expected removed route price task type to be rejected")
	}
}

func TestMemoryStorePreservesModelAccountGenerationCapabilities(t *testing.T) {
	ctx := context.Background()
	svc := modeladmin.NewServiceWithStore(nil)
	account, err := svc.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: "memory image account", AdapterType: "openai_compatible", AuthType: "api_key",
		BaseURL: "https://images.example.com", Credentials: map[string]string{"api_key": "test-key"},
		Status: "enabled", Priority: 1, Weight: 100, ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("CreateModelAccount: %v", err)
	}
	model, err := svc.CreateModelAccountModel(ctx, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "memory-image", DisplayName: "Memory Image",
		TaskTypes: []string{"image_edit"}, Quality: []string{"high"},
		SupportedRatios: []string{"16:9"}, MaxImageCount: 2, MaxReferenceImageCount: 3,
		CostPerImage: "0.10000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModelAccountModel: %v", err)
	}
	if len(model.SupportedRatios) != 1 || model.SupportedRatios[0] != "16:9" || model.MaxImageCount != 2 || model.MaxReferenceImageCount != 3 {
		t.Fatalf("memory create lost generation capabilities: %#v", model)
	}
	snapshot, err := svc.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("ModelRoutingConfig after create: %v", err)
	}
	if len(snapshot.ProviderModels) != 1 || len(snapshot.ProviderModels[0].SupportedAspectRatios) != 1 || snapshot.ProviderModels[0].SupportedAspectRatios[0] != "16:9" || snapshot.ProviderModels[0].MaxImageCount != 2 || snapshot.ProviderModels[0].MaxReferenceImageCount != 3 || !snapshot.ProviderModels[0].SupportsImageInput {
		t.Fatalf("memory snapshot lost generation capabilities: %#v", snapshot.ProviderModels)
	}

	updated, err := svc.UpdateModelAccountModel(ctx, model.ID, domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "memory-image", DisplayName: "Memory Image",
		TaskTypes: []string{"text_to_image"}, Quality: []string{"auto"},
		SupportedRatios: []string{"1:1"}, MaxImageCount: 1, MaxReferenceImageCount: 0,
		CostPerImage: "0.10000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("UpdateModelAccountModel: %v", err)
	}
	if len(updated.SupportedRatios) != 1 || updated.SupportedRatios[0] != "1:1" || updated.MaxImageCount != 1 || updated.MaxReferenceImageCount != 0 {
		t.Fatalf("memory update lost generation capabilities: %#v", updated)
	}
	snapshot, err = svc.ModelRoutingConfig(ctx)
	if err != nil {
		t.Fatalf("ModelRoutingConfig after update: %v", err)
	}
	if len(snapshot.ProviderModels) != 1 || snapshot.ProviderModels[0].MaxReferenceImageCount != 0 || snapshot.ProviderModels[0].SupportsImageInput {
		t.Fatalf("memory snapshot must preserve explicit no-reference support: %#v", snapshot.ProviderModels)
	}
}
