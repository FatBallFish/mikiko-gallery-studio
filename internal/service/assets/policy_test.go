package assets

import (
	"context"
	"slices"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
)

func TestAttachmentPolicyDefaultsAndNormalizesImageAliases(t *testing.T) {
	resolver := NewAttachmentPolicyResolver(config.AttachmentPolicyConfig{
		ImageMaxMB:          20,
		ImageAllowedFormats: []string{".PNG", "image/jpeg", "jpg", "WEBP", "image/gif"},
	}, nil)

	policy, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if policy.Image.MaxMB != 20 || policy.Image.MaxBytes != 20*1024*1024 {
		t.Fatalf("unexpected image size policy: %#v", policy.Image)
	}
	if !slices.Equal(policy.Image.AllowedFormats, []string{"png", "jpeg", "webp", "gif"}) {
		t.Fatalf("unexpected normalized image formats: %#v", policy.Image.AllowedFormats)
	}
	if !slices.Equal(policy.Image.AllowedMIMETypes, []string{"image/png", "image/jpeg", "image/webp", "image/gif"}) {
		t.Fatalf("unexpected normalized image MIME types: %#v", policy.Image.AllowedMIMETypes)
	}
}

func TestAttachmentPolicyRejectsInvalidSizeAndSVG(t *testing.T) {
	for _, items := range [][]domainadminconfig.Item{
		{{ConfigKey: AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": 0}}},
		{{ConfigKey: AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": -1}}},
		{{ConfigKey: AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": 1.5}}},
		{{ConfigKey: AttachmentImageAllowedFormatsKey, ConfigValue: map[string]any{"value": []any{"png", "svg"}}}},
		{{ConfigKey: AttachmentImageAllowedFormatsKey, ConfigValue: map[string]any{"value": []any{}}}},
	} {
		if err := ValidateAttachmentPolicyItems(items); err == nil {
			t.Fatalf("expected invalid policy items to be rejected: %#v", items)
		}
	}
}

func TestAttachmentPolicyInvalidatesAfterAdminConfigUpdate(t *testing.T) {
	cfg := config.Config{AttachmentPolicy: config.AttachmentPolicyConfig{
		ImageMaxMB: 20, ImageAllowedFormats: []string{"png", "jpeg", "webp", "gif"},
	}}
	admin := adminconfigservice.NewServiceWithStore(cfg, adminconfigservice.NewMemoryStore())
	resolver := NewAttachmentPolicyResolver(cfg.AttachmentPolicy, admin)

	first, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("first Resolve: %v", err)
	}
	if first.Image.MaxMB != 20 {
		t.Fatalf("unexpected initial policy: %#v", first)
	}

	updated, err := admin.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  AttachmentPolicyTabKey,
		Version: 1,
		Items: []domainadminconfig.Item{
			{ConfigCategory: AttachmentPolicyTabKey, ConfigKey: AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": 32}, Scope: "global"},
			{ConfigCategory: AttachmentPolicyTabKey, ConfigKey: AttachmentImageAllowedFormatsKey, ConfigValue: map[string]any{"value": []any{"jpeg", "webp"}}, Scope: "global"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateTab: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("unexpected updated version: %d", updated.Version)
	}

	second, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("second Resolve: %v", err)
	}
	if second.Image.MaxMB != 32 || !slices.Equal(second.Image.AllowedFormats, []string{"jpeg", "webp"}) {
		t.Fatalf("resolver kept stale cached policy: %#v", second)
	}
}
