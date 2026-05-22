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
