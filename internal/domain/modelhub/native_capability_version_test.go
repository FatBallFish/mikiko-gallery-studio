package modelhub

import (
	"context"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
)

// The model catalogue must expose the same capability fingerprint preflight
// compares against. Clients that only see native_image_profile.version (a
// schema date) deadlock on 409 capability_changed forever.
func TestListVisibleRouteModelsExposesTaskCapabilityVersions(t *testing.T) {
	snapshot := ModelRoutingSnapshot{
		RouteModels:    []RouteModelConfig{{ID: 1, Code: "native", Visibility: "public", Enabled: true}},
		Candidates:     []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}},
		ProviderModels: []ProviderCandidate{{AccountModelID: 2, AdapterType: nativeimage.AdapterSeedream, ModelCode: "doubao-seedream-5-0-flash-260915", HealthStatus: "enabled"}},
	}
	r := NewResolver(config.Config{Billing: config.BillingConfig{CNYPerPoint: ".01"}, GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 5, ReferenceImageMaxCount: 4}})
	r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
	items, err := r.ListVisibleRouteModels(context.Background(), nil, nil)
	if err != nil || len(items) != 1 {
		t.Fatalf("discovery: %+v %v", items, err)
	}
	byTask := items[0].CapabilitiesByTaskType
	for _, taskType := range []string{"text_to_image", "image_edit"} {
		version := byTask[taskType].CapabilityVersion
		if len(version) != 64 {
			t.Fatalf("%s capability_version must be the preflight fingerprint, got %q", taskType, version)
		}
	}
	if byTask["text_to_image"].CapabilityVersion == byTask["image_edit"].CapabilityVersion {
		t.Fatal("task-specific fingerprints must differ")
	}

	// The exposed version must satisfy preflight's expected-version check unchanged.
	req := ResolveRequest{RouteModelCode: "native", TaskType: "image_edit", Operation: nativeimage.OperationEdit, ReferenceImageCount: 2, RequestedOutputImageCount: 1}
	resolved, err := r.Resolve(req)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.CapabilityVersion != byTask["image_edit"].CapabilityVersion {
		t.Fatalf("catalogue version %q != preflight version %q", byTask["image_edit"].CapabilityVersion, resolved.CapabilityVersion)
	}
	req.ExpectedCapabilityVersion = byTask["image_edit"].CapabilityVersion
	if _, err = r.Resolve(req); err != nil {
		t.Fatalf("catalogue version rejected by preflight: %v", err)
	}
	// Native profile version (a schema date) must never satisfy the check.
	req.ExpectedCapabilityVersion = items[0].NativeImageProfile.Version
	if _, err = r.Resolve(req); err == nil {
		t.Fatal("profile schema date accepted as capability version")
	}
}
