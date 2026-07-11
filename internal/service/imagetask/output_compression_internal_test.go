package imagetask

import (
	"testing"

	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

func TestApplyOutputCompressionCapability(t *testing.T) {
	tests := []struct {
		name      string
		format    string
		quality   int
		supported bool
		want      int
	}{
		{name: "supported webp", format: "webp", quality: 72, supported: true, want: 72},
		{name: "unsupported webp", format: "webp", quality: 100, supported: false, want: 0},
		{name: "png never compresses", format: "png", quality: 72, supported: true, want: 0},
		{name: "jpeg without value", format: "jpeg", quality: 0, supported: true, want: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := provider.ImageRequest{OutputFormat: test.format, OutputCompression: test.quality}
			applyOutputCompressionCapability(&req, modelhub.ProviderCandidate{SupportsOutputCompression: test.supported})
			if req.OutputCompression != test.want {
				t.Fatalf("OutputCompression = %d, want %d", req.OutputCompression, test.want)
			}
		})
	}
}
