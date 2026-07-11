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
	capability, err := NormalizeCapability(ImageModelCapability{SupportsOutputCompression: true})
	if err != nil {
		t.Fatalf("NormalizeCapability: %v", err)
	}
	if !capability.SupportsOutputCompression {
		t.Fatal("normalization must preserve compression support")
	}
}
