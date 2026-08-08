package modelhub

import "testing"

func TestNormalizeCapabilityEnforcesUpstreamMaxImageCountRange(t *testing.T) {
	for _, count := range []int{-1, 0, 11, 64} {
		if _, err := NormalizeCapability(ImageModelCapability{MaxImageCount: count}); err == nil {
			t.Fatalf("NormalizeCapability accepted max_image_count=%d, want range error", count)
		}
	}
	for _, count := range []int{1, 10} {
		capability, err := NormalizeCapability(ImageModelCapability{MaxImageCount: count})
		if err != nil {
			t.Fatalf("NormalizeCapability rejected max_image_count=%d: %v", count, err)
		}
		if capability.MaxImageCount != count {
			t.Fatalf("normalized max_image_count = %d, want %d", capability.MaxImageCount, count)
		}
	}
}
