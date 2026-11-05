package openplatform_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/imagepricing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/service/assets"
	"github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/capabilities"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

type staticPreflightRouting struct{ snapshot modelhub.ModelRoutingSnapshot }

func (r staticPreflightRouting) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return r.snapshot, nil
}

// Reproduces the customer deadlock: an MCP preflight echoing the only version
// the catalogue used to expose (native_image_profile.version, a schema date)
// fails with capability_changed forever, and a sparse native request without
// size_mode failed candidate resolution because generic ratio defaults
// injected aspect_ratio=1:1 which native candidates reject.
func TestNativePreflightAcceptsCatalogueVersionAndSparseSizeMode(t *testing.T) {
	const model = "doubao-seedream-5-0-flash-260915"
	price, ok := nativeimage.PricingForModel(model)
	if !ok {
		t.Fatal("missing seedream pricing")
	}
	inputPrice := "0.0"
	outputTiers := make([]imagepricing.Tier, 0, len(price.OutputTiers))
	for _, tier := range price.OutputTiers {
		outputTiers = append(outputTiers, imagepricing.Tier{Name: tier.Name, MaxPixels: int64(tier.MaxPixels), Price: "0.2"})
	}
	routePrice := &imagepricing.Config{SchemaVersion: 1, Rules: []imagepricing.Rule{
		{Operation: "generate", Mode: "per_image", Currency: "CNY", Version: "fixture-v1", Input: imagepricing.Input{FreeImages: price.FreeInputImages, PerImage: &inputPrice}, OutputTiers: outputTiers},
		{Operation: "edit", Mode: "per_image", Currency: "CNY", Version: "fixture-v1", Input: imagepricing.Input{FreeImages: price.FreeInputImages, PerImage: &inputPrice}, OutputTiers: outputTiers},
	}}
	if err := imagepricing.Validate(routePrice); err != nil {
		t.Fatal(err)
	}
	routing := staticPreflightRouting{modelhub.ModelRoutingSnapshot{Version: "fixture-native-v1",
		RouteModels:    []modelhub.RouteModelConfig{{ID: 1, Code: "native", Name: "Fake native", Visibility: "public", Enabled: true, ImagePricing: routePrice}},
		ProviderModels: []modelhub.ProviderCandidate{{AccountModelID: 11, Provider: nativeimage.AdapterSeedream, AdapterType: nativeimage.AdapterSeedream, AuthType: "api_key", ModelCode: model, HealthStatus: "enabled", MaxImageCount: 6, ModelExtra: map[string]any{"native_image": modelhub.NativeImageConfig{Model: model, Pricing: &price}}}},
		Candidates:     []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 11, Priority: 1, Weight: 100, Enabled: true}}}}

	cfg := config.Config{}
	cfg.Billing = config.BillingConfig{CNYPerPoint: "0.01", PointsScale: 5, TaskMultipliers: map[string]string{"text_to_image": "1.00000", "image_edit": "1.00000"}}
	cfg.GenerationLimits = config.GenerationLimitsConfig{MaxImageCount: 6, ReferenceImageMaxCount: 4}

	bills := billing.NewService(cfg.Billing)
	bills.SetModelRoutingSource(routing)
	if _, err := bills.AdminAdjust(t.Context(), domainbilling.AdjustRequest{UserID: 7, ChangePoints: "1000.00000", Reason: "fixture", IdempotencyKey: "credit"}); err != nil {
		t.Fatal(err)
	}

	referenceID := uuid.NewString()
	probe := &unifiedReferenceProbe{owner: 7, asset: domainassets.ReferenceAsset{ID: referenceID, Name: "subject", Status: "ready", MimeType: "image/png", ObjectKey: "fixture.png"}}
	refs := assets.NewServiceWithStoreAndBackend(config.StorageConfig{}, cfg.GenerationLimits, probe, nil)
	images := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), refs, bills)
	images.SetModelRoutingSource(routing)
	imageModels := capabilities.NewService(cfg)
	imageModels.SetModelRoutingSource(routing)
	svc := openplatform.New(openplatform.Dependencies{Auth: preflightUsers{}, Images: images, ImageModels: imageModels, References: refs, Billing: bills, QuoteKey: []byte("01234567890123456789012345678901")})

	catalogue, err := imageModels.ListForGroups(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var catalogueVersion string
	for _, group := range catalogue.ModelGroups {
		if group.Code == "native" {
			if group.CapabilitiesByTaskType["image_edit"].CapabilityVersion == "" {
				t.Fatal("catalogue must expose image_edit capability_version")
			}
			catalogueVersion = group.CapabilitiesByTaskType["image_edit"].CapabilityVersion
		}
	}
	if catalogueVersion == "" {
		t.Fatal("native route missing from catalogue")
	}

	preflight := func(capabilityVersion string) error {
		input := map[string]any{"project_id": "fixture", "route_model_code": "native", "task_type": "image_edit", "operation": "edit", "prompt": "edit this", "reference_asset_ids": []string{referenceID}}
		if capabilityVersion != "" {
			input["capability_version"] = capabilityVersion
		}
		raw, _ := json.Marshal(input)
		_, err := svc.Execute(t.Context(), openplatform.Principal{UserID: 7, Scopes: []string{"images:write"}}, "preflight_image_task", raw)
		return err
	}

	if err := preflight(""); err != nil {
		t.Fatalf("sparse native preflight without size_mode must succeed: %v", err)
	}
	if err := preflight(catalogueVersion); err != nil {
		t.Fatalf("catalogue capability_version must satisfy preflight: %v", err)
	}
	err = preflight("2026-10-25")
	var app *errs.Error
	if !errors.As(err, &app) || app.Code != "capability_changed" {
		t.Fatalf("profile schema date must fail with capability_changed, got %v", err)
	}
	_ = repoerr.ErrNotFound
}
