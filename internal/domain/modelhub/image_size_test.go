package modelhub

import (
	"fmt"
	"math/rand"
	"testing"
)

func TestCalculateImageSizeUsesPresetDimensions(t *testing.T) {
	cases := []struct {
		baseResolution string
		ratio          string
		want           string
	}{
		{baseResolution: "1K", ratio: "1:1", want: "1024x1024"},
		{baseResolution: "2k", ratio: "16:9", want: "2560x1440"},
		{baseResolution: "4K", ratio: "1:1", want: "2880x2880"},
		{baseResolution: "4K", ratio: "21:9", want: "3840x1600"},
	}

	for _, tc := range cases {
		got, err := CalculateImageSize(tc.baseResolution, tc.ratio)
		if err != nil {
			t.Fatalf("CalculateImageSize(%q, %q): %v", tc.baseResolution, tc.ratio, err)
		}
		if got != tc.want {
			t.Fatalf("CalculateImageSize(%q, %q) = %q, want %q", tc.baseResolution, tc.ratio, got, tc.want)
		}
	}
}

func TestCalculateImageSizeKeepsCustomRatioWithinModelLimits(t *testing.T) {
	got, err := CalculateImageSize("4k", "7:5")
	if err != nil {
		t.Fatalf("CalculateImageSize custom ratio: %v", err)
	}
	width, height, ok := ParseImageSize(got)
	if !ok {
		t.Fatalf("expected parseable size, got %q", got)
	}
	if width%16 != 0 || height%16 != 0 {
		t.Fatalf("expected 16px multiples, got %s", got)
	}
	if width > 3840 || height > 3840 {
		t.Fatalf("expected max edge <= 3840, got %s", got)
	}
	ratio := float64(width) / float64(height)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	if ratio > 3 {
		t.Fatalf("expected ratio <= 3, got %s", got)
	}
	pixels := width * height
	if pixels < 655360 || pixels > 8294400 {
		t.Fatalf("expected pixels within legal range, got %d for %s", pixels, got)
	}
}

func TestCalculateImageSizeRejectsInvalidRatio(t *testing.T) {
	if _, err := CalculateImageSize("4k", "8:1"); err == nil {
		t.Fatal("expected invalid model ratio to fail")
	}
}

func TestCalculateImageSizeWithinCapabilityUsesNearestLegalRatioSize(t *testing.T) {
	tests := []struct {
		name       string
		base       string
		ratio      string
		capability ImageModelCapability
		want       string
		wantError  bool
	}{
		{name: "unbounded preset compatibility", base: "1k", ratio: "16:9", capability: ImageModelCapability{}, want: "1280x720"},
		{name: "tight square", base: "1k", ratio: "1:1", capability: ImageModelCapability{MinWidth: 512, MaxWidth: 900, MinHeight: 512, MaxHeight: 900}, want: "896x896"},
		{name: "non symmetric bounds", base: "1k", ratio: "16:9", capability: ImageModelCapability{MinWidth: 1000, MaxWidth: 1200, MinHeight: 600, MaxHeight: 700}, want: "1200x672"},
		{name: "boundary equality", base: "1k", ratio: "16:9", capability: ImageModelCapability{MinWidth: 1280, MaxWidth: 1280, MinHeight: 720, MaxHeight: 720}, want: "1280x720"},
		{name: "no solution below global area", base: "1k", ratio: "1:1", capability: ImageModelCapability{MinWidth: 512, MaxWidth: 700, MinHeight: 512, MaxHeight: 700}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateImageSizeWithinCapability(tt.base, tt.ratio, tt.capability)
			if tt.wantError {
				if err == nil {
					t.Fatalf("CalculateImageSizeWithinCapability() = %q, want error", got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("CalculateImageSizeWithinCapability() = %q, %v; want %q", got, err, tt.want)
			}
			again, repeatErr := CalculateImageSizeWithinCapability(tt.base, tt.ratio, tt.capability)
			if repeatErr != nil || again != got {
				t.Fatalf("bounded ratio resolution must be deterministic: first=%q second=%q err=%v", got, again, repeatErr)
			}
		})
	}
}

func TestCalculateImageSizeWithinCapabilityPropertyReturnsExactLegalBoundary(t *testing.T) {
	rng := rand.New(rand.NewSource(20260809))
	checked := 0
	for checked < 250 {
		width := (51 + rng.Intn(130)) * imageSizeMultiple
		height := (51 + rng.Intn(130)) * imageSizeMultiple
		if !IsLegalCustomImageSize(width, height) {
			continue
		}
		ratio := fmt.Sprintf("%d:%d", width, height)
		capability := ImageModelCapability{MinWidth: width, MaxWidth: width, MinHeight: height, MaxHeight: height, SupportsCustomRatio: true}
		got, err := CalculateImageSizeWithinCapability("4k", ratio, capability)
		if err != nil || got != fmt.Sprintf("%dx%d", width, height) {
			t.Fatalf("legal exact boundary %dx%d ratio=%s resolved as %q, %v", width, height, ratio, got, err)
		}
		resolvedWidth, resolvedHeight, ok := ParseImageSize(got)
		if !ok || !legalExplicitDimensions(resolvedWidth, resolvedHeight, capability) {
			t.Fatalf("resolved property size is illegal: %q capability=%#v", got, capability)
		}
		actualRatio := float64(resolvedWidth) / float64(resolvedHeight)
		targetRatio := float64(width) / float64(height)
		if ratioError(actualRatio, targetRatio) > imageMaxRatioError {
			t.Fatalf("resolved ratio drifted: got=%q target=%s", got, ratio)
		}
		checked++
	}
}

func BenchmarkCalculateImageSizeWithinCapability(b *testing.B) {
	capability := ImageModelCapability{
		MinWidth: 512, MaxWidth: imageMaxEdge,
		MinHeight: 512, MaxHeight: imageMaxEdge,
		SupportsCustomRatio: true,
	}
	for b.Loop() {
		if _, err := CalculateImageSizeWithinCapability("4k", "137:89", capability); err != nil {
			b.Fatal(err)
		}
	}
}

func TestNormalizeCustomImageSize(t *testing.T) {
	tests := []struct {
		name          string
		width         int
		height        int
		want          string
		wantError     bool
		wantLandscape bool
	}{
		{name: "already legal", width: 1024, height: 1024, want: "1024x1024"},
		{name: "align to multiples", width: 1001, height: 777, want: "1008x784", wantLandscape: true},
		{name: "raise minimum pixels", width: 256, height: 256, want: "816x816"},
		{name: "cap maximum pixels", width: 5000, height: 5000, want: "2880x2880"},
		{name: "clamp landscape ratio", width: 4000, height: 500, want: "2448x816", wantLandscape: true},
		{name: "clamp portrait ratio", width: 500, height: 4000, want: "816x2448"},
		{name: "respect edge and pixel pressure", width: 5000, height: 3000, want: "3712x2224", wantLandscape: true},
		{name: "reject zero", width: 0, height: 1024, wantError: true},
		{name: "reject negative", width: 1024, height: -1, wantError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeCustomImageSize(tc.width, tc.height)
			if tc.wantError {
				if err == nil {
					t.Fatalf("NormalizeCustomImageSize(%d, %d) = %q, want error", tc.width, tc.height, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeCustomImageSize(%d, %d): %v", tc.width, tc.height, err)
			}
			if tc.want != "" && got != tc.want {
				t.Fatalf("NormalizeCustomImageSize(%d, %d) = %q, want %q", tc.width, tc.height, got, tc.want)
			}
			width, height, ok := ParseImageSize(got)
			if !ok || !IsLegalCustomImageSize(width, height) {
				t.Fatalf("normalized size must be legal, got %q", got)
			}
			if tc.wantLandscape && width <= height {
				t.Fatalf("expected landscape result, got %q", got)
			}
			if tc.name == "clamp portrait ratio" && width >= height {
				t.Fatalf("expected portrait result, got %q", got)
			}
			again, err := NormalizeCustomImageSize(width, height)
			if err != nil || again != got {
				t.Fatalf("normalization must be idempotent: %q -> %q (%v)", got, again, err)
			}
		})
	}
}
