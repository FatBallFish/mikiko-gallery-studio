package modelhub

import (
	"context"
	"fmt"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"reflect"
	"testing"
)

func TestNativeVisibleReferenceCapabilities(t *testing.T) {
	for _, genericReferences := range []int{0, 10} {
		t.Run(fmt.Sprint(genericReferences), func(t *testing.T) {
			snapshot := ModelRoutingSnapshot{RouteModels: []RouteModelConfig{{ID: 1, Code: "native", Visibility: "public", Enabled: true}}, Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []ProviderCandidate{{AccountModelID: 2, AdapterType: nativeimage.AdapterQwen, ModelCode: "qwen-image-2.1-pro", MaxReferenceImageCount: genericReferences}}}
			r := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 4, ReferenceImageMaxCount: 10}})
			r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
			items, err := r.ListVisibleRouteModels(context.Background(), nil, nil)
			if err != nil || len(items) != 1 {
				t.Fatalf("discovery: %+v %v", items, err)
			}
			item := items[0]
			if item.MaxReferenceImageCount != 10 || item.CapabilitiesByTaskType["image_edit"].MaxReferenceImageCount != 10 || item.CapabilitiesByTaskType["text_to_image"].MaxReferenceImageCount != 0 || item.MaxOutputImageCount != 4 {
				t.Fatalf("inconsistent native projection: %+v", item)
			}
		})
	}
}

func TestNativeRouteProfileIntersectsFallbackConstraints(t *testing.T) {
	full, _ := nativeimage.ProfileForModel("qwen-image-3.0")
	narrow := full
	narrow.MaxReferenceImages, narrow.MaxOutputImages = 1, 2
	narrow.MinPixels, narrow.MaxPixels, narrow.MaxAspectRatio = 524288, 2097152, 4
	narrow.Seed, narrow.NegativePrompt = false, false
	narrow.InputFormats, narrow.SizeModes = []string{"png"}, []string{"dimensions"}
	narrow.Operations = []nativeimage.Operation{nativeimage.OperationGenerate}
	snapshot := ModelRoutingSnapshot{Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}, {RouteModelID: 1, AccountModelID: 3, Enabled: true}}, ProviderModels: []ProviderCandidate{
		{AccountModelID: 2, AdapterType: full.Adapter, ModelCode: full.Model},
		{AccountModelID: 3, AdapterType: full.Adapter, ModelCode: full.Model, ModelExtra: map[string]any{"native_image": NativeImageConfig{Profile: &narrow}}},
	}}
	for _, reverse := range []bool{false, true} {
		if reverse {
			snapshot.Candidates[0], snapshot.Candidates[1] = snapshot.Candidates[1], snapshot.Candidates[0]
		}
		p := nativeRouteProfile(RouteModelConfig{ID: 1}, snapshot)
		if !reflect.DeepEqual(p, &narrow) {
			t.Fatalf("intersection must retain all narrower constraints: got %+v want %+v", p, narrow)
		}
	}
	snapshot.ProviderModels[1].ModelCode = "qwen-image-2.1-pro"
	snapshot.ProviderModels[1].ModelExtra = nil
	if p := nativeRouteProfile(RouteModelConfig{ID: 1}, snapshot); p != nil {
		t.Fatalf("mixed models advertised a shared native profile: %+v", p)
	}
	r := NewResolver(config.Config{})
	if caps := r.visibleRouteModelLimits(RouteModelConfig{ID: 1}, snapshot, "image_edit"); caps.MaxReferenceImageCount != 0 || len(caps.SizeModes) != 0 {
		t.Fatalf("incompatible native profiles leaked generic capability: %+v", caps)
	}
}

func TestNativeRouteLayerProfileIntersection(t *testing.T) {
	full, _ := nativeimage.ProfileForModel("doubao-seedream-5-0-pro-260628")
	narrow := full
	narrow.MaxLayers = 4
	narrow.LayerSizeModes = []string{"1K"}
	narrow.OutputFormats = []string{"png"}
	narrow.OptimizePromptModes = []string{"standard"}
	snapshot := ModelRoutingSnapshot{Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}, {RouteModelID: 1, AccountModelID: 3, Enabled: true}}, ProviderModels: []ProviderCandidate{
		{AccountModelID: 2, AdapterType: full.Adapter, ModelCode: full.Model},
		{AccountModelID: 3, AdapterType: full.Adapter, ModelCode: full.Model, ModelExtra: map[string]any{"native_image": NativeImageConfig{Profile: &narrow}}},
	}}
	if p := nativeRouteProfile(RouteModelConfig{ID: 1}, snapshot); !reflect.DeepEqual(p, &narrow) {
		t.Fatalf("layer constraints were not intersected: %+v", p)
	}
	narrow.LayerDecomposition = false
	narrow.Operations = []nativeimage.Operation{nativeimage.OperationGenerate, nativeimage.OperationEdit}
	if p := nativeRouteProfile(RouteModelConfig{ID: 1}, snapshot); p == nil || p.LayerDecomposition || containsNativeOperation(p.Operations, nativeimage.OperationLayerDecomposition) {
		t.Fatalf("fallback without layers advertised decomposition: %+v", p)
	}
}

func TestNativeDefaultsInheritanceAndVersion(t *testing.T) {
	c := ProviderCandidate{AccountModelID: 2, AdapterType: nativeimage.AdapterQwen, ModelCode: "qwen-image-3.0", AccountExtra: map[string]any{"native_image": map[string]any{"unknown_extension": true, "defaults": map[string]any{"watermark": true, "qwen": map[string]any{"prompt_extend": true, "enable_thinking": true}}}}, ModelExtra: map[string]any{"native_image": map[string]any{"defaults": map[string]any{"qwen": map[string]any{"enable_thinking": false}}}}}
	cfg, err := NativeConfig(c)
	if err != nil || !*cfg.Defaults.Watermark || !*cfg.Defaults.Qwen.PromptExtend || *cfg.Defaults.Qwen.EnableThinking || cfg.Defaults.Qwen.PromptExtendMode != "direct" {
		t.Fatalf("inheritance: %+v %v", cfg.Defaults, err)
	}
	snapshot := ModelRoutingSnapshot{RouteModels: []RouteModelConfig{{ID: 1, Code: "native", Visibility: "public", Enabled: true}}, Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []ProviderCandidate{c}}
	r := NewResolver(config.Config{GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 6, ReferenceImageMaxCount: 10}})
	r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
	req := ResolveRequest{RouteModelCode: "native", TaskType: "text_to_image", RequestedOutputImageCount: 1}
	first, err := r.Resolve(req)
	if err != nil || first.NativeOptions == nil || !*first.NativeOptions.Qwen.PromptExtend {
		t.Fatalf("effective: %+v %v", first, err)
	}
	c.ModelExtra = map[string]any{"native_image": map[string]any{"defaults": map[string]any{"qwen": map[string]any{"prompt_extend": false, "enable_thinking": false}}}}
	snapshot.ProviderModels[0] = c
	r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
	req.ExpectedCapabilityVersion = first.CapabilityVersion
	if _, err := r.Resolve(req); err == nil {
		t.Fatal("defaults changed without invalidating version")
	}
	req.ExpectedCapabilityVersion = ""
	req.PreserveNativeOptions = true
	no := false
	for _, original := range []*nativeimage.Options{nil, {Qwen: &nativeimage.QwenOptions{}}, {Watermark: &no, Qwen: &nativeimage.QwenOptions{PromptExtend: &no}}, first.NativeOptions} {
		req.NativeOptions = original
		resolved, err := r.Resolve(req)
		if err != nil || !reflect.DeepEqual(resolved.NativeOptions, original) {
			t.Fatalf("persisted options changed after config update: original=%+v resolved=%+v error=%v", original, resolved.NativeOptions, err)
		}
	}
}

func TestNativeRouteDiscoveryAndVersionWithoutLegacyPrices(t *testing.T) {
	snapshot := ModelRoutingSnapshot{RouteModels: []RouteModelConfig{{ID: 1, Code: "native", Visibility: "public", Enabled: true}}, Candidates: []RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 2, Enabled: true}}, ProviderModels: []ProviderCandidate{{AccountModelID: 2, AdapterType: nativeimage.AdapterQwen, ModelCode: "qwen-image-3.0"}}}
	r := NewResolver(config.Config{Billing: config.BillingConfig{CNYPerPoint: ".01"}, GenerationLimits: config.GenerationLimitsConfig{MaxImageCount: 6, ReferenceImageMaxCount: 10}})
	r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
	items, e := r.ListVisibleRouteModels(context.Background(), nil, nil)
	if e != nil || len(items) != 1 || items[0].NativeImageProfile == nil || len(items[0].TaskTypes) != 2 {
		t.Fatalf("discovery %+v %v", items, e)
	}
	req := ResolveRequest{RouteModelCode: "native", TaskType: "text_to_image", Operation: nativeimage.OperationGenerate, RequestedSize: "2049x513", RequestedOutputImageCount: 1}
	first, e := r.Resolve(req)
	if e != nil {
		t.Fatal(e)
	}
	snapshot.ProviderModels[0].ModelCode = "qwen-image-2.1-pro"
	r.SetModelRoutingSource(staticRoutingSource{snapshot: snapshot})
	req.ExpectedCapabilityVersion = first.CapabilityVersion
	if _, e = r.Resolve(req); e == nil {
		t.Fatal("stale native capability version accepted")
	}
}
func TestNativeCandidateDimensionsAndFallback(t *testing.T) {
	c := ProviderCandidate{AdapterType: nativeimage.AdapterQwen, ModelCode: "qwen-image-3.0"}
	req := ResolveRequest{Operation: nativeimage.OperationGenerate, RequestedSize: "2049x513", RequestedOutputImageCount: 1}
	if !CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("native valid non-grid 4:1 size rejected")
	}
	req.NativeOptions = &nativeimage.Options{NegativePrompt: "blur"}
	if !CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("negative prompt rejected for 3.0")
	}
	c.ModelCode = "qwen-image-2.1-pro"
	if CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("fallback must not drop negative prompt")
	}
	c.AdapterType = "openai"
	if CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("generic fallback must not drop options")
	}
}
func TestNativeProfileCannotExpandOfficialConstraints(t *testing.T) {
	p, _ := nativeimage.ProfileForModel("qwen-image-3.0")
	c := ProviderCandidate{AdapterType: nativeimage.AdapterQwen, ModelCode: p.Model, ModelExtra: map[string]any{"native_image": NativeImageConfig{Profile: &p}}}
	p.MaxReferenceImages = 4
	if _, e := NativeConfig(c); e == nil {
		t.Fatal("expanded reference profile accepted")
	}
	p.MaxReferenceImages = 2
	if _, e := NativeConfig(c); e != nil {
		t.Fatal(e)
	}
	p.Operations = append(p.Operations, nativeimage.OperationLayerDecomposition)
	if _, e := NativeConfig(c); e == nil {
		t.Fatal("Qwen layers accepted")
	}
}
func TestNativeEndpointMappingAndAnnotations(t *testing.T) {
	c := ProviderCandidate{AdapterType: nativeimage.AdapterSeedream, ModelCode: "ep-private", ModelExtra: map[string]any{"native_image": map[string]any{"model": "doubao-seedream-5-0-pro-260628"}}}
	cfg, e := NativeConfig(c)
	if e != nil || cfg.Profile.Model != "doubao-seedream-5-0-pro-260628" {
		t.Fatalf("mapping: %+v %v", cfg, e)
	}
	req := ResolveRequest{Operation: nativeimage.OperationEdit, ReferenceImageCount: 1, RequestedSize: "2K", NativeOptions: &nativeimage.Options{Annotations: []nativeimage.Annotation{{ReferenceIndex: 2, Point: &[2]int{10, 20}}}}}
	if CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("bad reference annotation accepted")
	}
	req.NativeOptions.Annotations[0].ReferenceIndex = 1
	if !CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("valid annotation rejected")
	}
	req.MaskPresent = true
	if CandidateSupportsRequest(c, req, "2k") {
		t.Fatal("unsupported mask accepted")
	}
}
