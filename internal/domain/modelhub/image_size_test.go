package modelhub

import "testing"

func TestCalculateImageSizeUsesPresetDimensions(t *testing.T) {
	cases := []struct {
		quality string
		ratio   string
		want    string
	}{
		{quality: "1K", ratio: "1:1", want: "1024x1024"},
		{quality: "2k", ratio: "16:9", want: "2560x1440"},
		{quality: "4K", ratio: "1:1", want: "2880x2880"},
		{quality: "4K", ratio: "21:9", want: "3840x1600"},
	}

	for _, tc := range cases {
		got, err := CalculateImageSize(tc.quality, tc.ratio)
		if err != nil {
			t.Fatalf("CalculateImageSize(%q, %q): %v", tc.quality, tc.ratio, err)
		}
		if got != tc.want {
			t.Fatalf("CalculateImageSize(%q, %q) = %q, want %q", tc.quality, tc.ratio, got, tc.want)
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
