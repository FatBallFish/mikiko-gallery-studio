package modeladmin

import (
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"testing"
)

func TestNativeDefaultsWriteValidation(t *testing.T) {
	for _, test := range []struct {
		defaults map[string]any
		invalid  bool
	}{
		{map[string]any{"watermark": false, "qwen": map[string]any{"prompt_extend": true}}, false},
		{map[string]any{"invented": true}, true},
		{map[string]any{"qwen": map[string]any{"enable_thinking": true}}, true},
		{map[string]any{"seed": 2147483648}, true},
		{map[string]any{"seed": 0}, false},
	} {
		extra := map[string]any{"native_image": map[string]any{"unknown_existing": true, "defaults": test.defaults}, "unrelated": true}
		err := validateNativeDefaults(extra, nativeimage.AdapterQwen, "")
		if (err != nil) != test.invalid {
			t.Fatalf("defaults=%+v err=%v", test.defaults, err)
		}
		if extra["unrelated"] != true {
			t.Fatal("extension mutated")
		}
	}
}
