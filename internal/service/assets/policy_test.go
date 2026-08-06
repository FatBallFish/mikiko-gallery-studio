package assets

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
)

type mutableAttachmentPolicySource struct {
	tab   domainadminconfig.Tab
	err   error
	calls int
}

func (s *mutableAttachmentPolicySource) GetTab(context.Context, string) (domainadminconfig.Tab, error) {
	s.calls++
	if s.err != nil {
		return domainadminconfig.Tab{}, s.err
	}
	return s.tab, nil
}

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

func TestAttachmentPolicyRejectsImageSizeAboveInMemoryLimit(t *testing.T) {
	err := ValidateAttachmentPolicyItems([]domainadminconfig.Item{{
		ConfigKey:   AttachmentImageMaxMBKey,
		ConfigValue: map[string]any{"value": MaxImageAttachmentSizeMB + 1},
	}})
	if err == nil || !strings.Contains(err.Error(), "between 1 and 100 MB") {
		t.Fatalf("expected explicit image memory limit error, got %v", err)
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

func TestAttachmentPolicyRefreshesVersionWithoutProcessLocalListener(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	source := &mutableAttachmentPolicySource{tab: attachmentPolicyTab(1, 20, []string{"png"})}
	resolver := newAttachmentPolicyResolver(
		config.AttachmentPolicyConfig{ImageMaxMB: 20, ImageAllowedFormats: []string{"png"}},
		source,
		time.Minute,
		func() time.Time { return now },
	)

	first, err := resolver.Resolve(context.Background())
	if err != nil || first.Image.MaxMB != 20 {
		t.Fatalf("initial Resolve = %#v, %v", first, err)
	}
	source.tab = attachmentPolicyTab(2, 32, []string{"jpeg"})

	withinTTL, err := resolver.Resolve(context.Background())
	if err != nil || withinTTL.Image.MaxMB != 20 || source.calls != 1 {
		t.Fatalf("resolver did not retain bounded cache within TTL: policy=%#v calls=%d err=%v", withinTTL, source.calls, err)
	}

	now = now.Add(time.Minute)
	refreshed, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("refresh Resolve: %v", err)
	}
	if refreshed.Image.MaxMB != 32 || !slices.Equal(refreshed.Image.AllowedFormats, []string{"jpeg"}) || source.calls != 2 {
		t.Fatalf("resolver did not refresh remote version: policy=%#v calls=%d", refreshed, source.calls)
	}
}

func TestAttachmentPolicyRefreshFailureKeepsLastKnownGood(t *testing.T) {
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	source := &mutableAttachmentPolicySource{tab: attachmentPolicyTab(1, 24, []string{"webp"})}
	resolver := newAttachmentPolicyResolver(
		config.AttachmentPolicyConfig{ImageMaxMB: 20, ImageAllowedFormats: []string{"png"}},
		source,
		time.Minute,
		func() time.Time { return now },
	)

	first, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("initial Resolve: %v", err)
	}
	source.err = errors.New("database unavailable")
	now = now.Add(time.Minute)
	stale, err := resolver.Resolve(context.Background())
	if err != nil {
		t.Fatalf("refresh should serve last-known-good policy: %v", err)
	}
	if stale.Image.MaxMB != first.Image.MaxMB || !slices.Equal(stale.Image.AllowedFormats, first.Image.AllowedFormats) {
		t.Fatalf("refresh failure discarded last-known-good policy: first=%#v stale=%#v", first, stale)
	}

	source.err = nil
	source.tab = attachmentPolicyTab(2, 28, []string{"gif"})
	now = now.Add(time.Minute)
	recovered, err := resolver.Resolve(context.Background())
	if err != nil || recovered.Image.MaxMB != 28 || !slices.Equal(recovered.Image.AllowedFormats, []string{"gif"}) {
		t.Fatalf("resolver did not recover after source returned: policy=%#v err=%v", recovered, err)
	}
}

func attachmentPolicyTab(version int64, imageMaxMB int, formats []string) domainadminconfig.Tab {
	return domainadminconfig.Tab{
		TabKey:  AttachmentPolicyTabKey,
		Version: version,
		Items: []domainadminconfig.Item{
			{ConfigKey: AttachmentImageMaxMBKey, ConfigValue: map[string]any{"value": imageMaxMB}},
			{ConfigKey: AttachmentImageAllowedFormatsKey, ConfigValue: map[string]any{"value": formats}},
		},
	}
}
