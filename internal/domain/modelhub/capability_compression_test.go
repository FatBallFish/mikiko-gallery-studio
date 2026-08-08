package modelhub

import "testing"

func TestCandidateSupportsCustomCompressionOnlyWhenDeclared(t *testing.T) {
	request := ResolveRequest{
		TaskType:                  "text_to_image",
		SizeMode:                  SizeModeRatio,
		AspectRatio:               "1:1",
		BaseResolution:            "1k",
		Quality:                   "auto",
		OutputFormat:              "webp",
		OutputCompression:         75,
		Moderation:                "auto",
		RequestedOutputImageCount: 1,
	}
	candidate := ProviderCandidate{
		HealthStatus:            "enabled",
		SupportedTaskTypes:      []string{"text_to_image"},
		SupportedBaseResolution: []string{"1k"},
		Quality:                 []string{"auto"},
		SizeModes:               []string{SizeModeRatio},
		SupportedAspectRatios:   []string{"1:1"},
		OutputFormat:            []string{"webp"},
		Moderation:              []string{"auto"},
		MaxImageCount:           1,
	}

	if CandidateSupportsRequest(candidate, request, "1k") {
		t.Fatal("candidate without compression support must reject custom compression")
	}
	candidate.SupportsOutputCompression = true
	if !CandidateSupportsRequest(candidate, request, "1k") {
		t.Fatal("candidate with compression support should accept custom WebP compression")
	}

	candidate.SupportsOutputCompression = false
	request.OutputCompression = 100
	if !CandidateSupportsRequest(candidate, request, "1k") {
		t.Fatal("compatibility value 100 should remain routable for unsupported candidates")
	}
}

func TestNormalizeCapabilityPreservesCompressionSupport(t *testing.T) {
	capability, err := NormalizeCapability(ImageModelCapability{MaxImageCount: 1, SupportsOutputCompression: true})
	if err != nil {
		t.Fatalf("NormalizeCapability: %v", err)
	}
	if !capability.SupportsOutputCompression {
		t.Fatal("normalization must preserve compression support")
	}
}

func TestCandidateSupportsCustomPixelSizeOnlyWhenDeclared(t *testing.T) {
	request := ResolveRequest{
		TaskType: "text_to_image", SizeMode: SizeModePixel, RequestedSize: "1008x1008",
		Quality: "auto", OutputFormat: "png", Moderation: "auto", RequestedOutputImageCount: 1,
	}
	candidate := ProviderCandidate{
		HealthStatus: "enabled", SupportedTaskTypes: []string{"text_to_image"},
		SizeModes: []string{SizeModePixel}, SupportedPixelSizes: []string{"1024x1024"},
		Quality: []string{"auto"}, OutputFormat: []string{"png"}, Moderation: []string{"auto"}, MaxImageCount: 1,
	}
	if CandidateSupportsRequest(candidate, request, "1k") {
		t.Fatal("preset-only candidate must reject an arbitrary pixel size")
	}
	candidate.SupportsCustomSize = true
	if !CandidateSupportsRequest(candidate, request, "1k") {
		t.Fatal("custom-size candidate should accept a legal normalized pixel size")
	}

	capability, err := NormalizeCapability(ImageModelCapability{
		MaxImageCount: 1, SizeModes: []string{SizeModePixel}, SupportsCustomSize: true,
		MinWidth: 512, MaxWidth: 3840, MinHeight: 512, MaxHeight: 3840,
	})
	if err != nil {
		t.Fatalf("NormalizeCapability custom size: %v", err)
	}
	if !capability.SupportsCustomSize {
		t.Fatal("normalization must preserve custom-size support")
	}
}
