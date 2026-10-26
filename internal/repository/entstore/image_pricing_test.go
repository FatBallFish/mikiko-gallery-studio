package entstore_test

import (
	"reflect"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
)

func TestRouteImagePricingStorageProjectionAndClear(t *testing.T) {
	store := entstore.NewModelAdminStore(openModelAdminTestClient(t, "route-image-pricing"))
	ctx := t.Context()
	zero := "0"
	config := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{{Operation: "generate", Mode: "per_image", Currency: "points", Version: "v1", Input: imagepricing.Input{PerImage: &zero}, OutputTiers: []imagepricing.Tier{{Name: "legacy", BaseResolution: "1K", Price: "1.23456789"}}}}}
	req := domainmodeladmin.RouteModelWriteRequest{Code: "priced", Name: "Priced", Visibility: "public", MediaType: "image", Enabled: true, ImagePricing: config}
	route, err := store.CreateRouteModel(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetRouteModel(ctx, route.ID)
	if err != nil || !reflect.DeepEqual(detail.ImagePricing, config) {
		t.Fatalf("detail %+v %v", detail, err)
	}
	page, err := store.ListRouteModels(ctx, domainmodeladmin.RouteModelListRequest{})
	if err != nil || !reflect.DeepEqual(page.Items[0].ImagePricing, config) {
		t.Fatal("list lost pricing", err)
	}
	snapshot, err := store.ModelRoutingConfig(ctx)
	if err != nil || !reflect.DeepEqual(snapshot.RouteModels[0].ImagePricing, config) {
		t.Fatal("projection lost pricing", err)
	}
	req.ImagePricing = nil
	if _, err := store.UpdateRouteModel(ctx, route.ID, req); err != nil {
		t.Fatal(err)
	}
	preserved, err := store.GetRouteModel(ctx, route.ID)
	if err != nil || !reflect.DeepEqual(preserved.ImagePricing, config) {
		t.Fatal("omitted pricing erased", err)
	}
	req.ImagePricingPresent = true
	if _, err := store.UpdateRouteModel(ctx, route.ID, req); err != nil {
		t.Fatal(err)
	}
	detail, err = store.GetRouteModel(ctx, route.ID)
	if err != nil || detail.ImagePricing != nil {
		t.Fatalf("clear failed %+v %v", detail.ImagePricing, err)
	}
}
