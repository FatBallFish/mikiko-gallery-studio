package assets

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
)

const (
	AttachmentPolicyTabKey                    = "attachment_policy"
	AttachmentImageMaxMBKey                   = "image_max_mb"
	AttachmentVideoMaxMBKey                   = "video_max_mb"
	AttachmentAudioMaxMBKey                   = "audio_max_mb"
	AttachmentDocumentMaxMBKey                = "document_max_mb"
	AttachmentImageAllowedFormatsKey          = "image_allowed_formats"
	AttachmentVideoAllowedFormatsKey          = "video_allowed_formats"
	AttachmentAudioAllowedFormatsKey          = "audio_allowed_formats"
	AttachmentDocumentAllowedFormatsKey       = "document_allowed_formats"
	maxConfiguredAttachmentSizeMB             = 10240
	defaultAttachmentPolicyRefreshTTL         = 5 * time.Second
	MaxImageAttachmentSizeMB                  = config.MaxImageAttachmentSizeMB
	MaxImageAttachmentBytes             int64 = MaxImageAttachmentSizeMB * 1024 * 1024
)

type FilePolicy struct {
	MaxMB            int
	MaxBytes         int64
	AllowedFormats   []string
	AllowedMIMETypes []string
}

type AttachmentPolicy struct {
	Image    FilePolicy
	Video    FilePolicy
	Audio    FilePolicy
	Document FilePolicy
}

type AttachmentPolicySource interface {
	GetTab(ctx context.Context, tabKey string) (domainadminconfig.Tab, error)
}

type attachmentPolicyInvalidationSource interface {
	RegisterInvalidationListener(listener func(tabKey string))
}

type AttachmentPolicyResolver struct {
	defaults config.AttachmentPolicyConfig
	source   AttachmentPolicySource

	mu           sync.RWMutex
	cached       *AttachmentPolicy
	version      int64
	refreshAfter time.Time
	refreshTTL   time.Duration
	now          func() time.Time
}

func NewAttachmentPolicyResolver(defaults config.AttachmentPolicyConfig, source AttachmentPolicySource) *AttachmentPolicyResolver {
	return newAttachmentPolicyResolver(defaults, source, defaultAttachmentPolicyRefreshTTL, time.Now)
}

func newAttachmentPolicyResolver(defaults config.AttachmentPolicyConfig, source AttachmentPolicySource, refreshTTL time.Duration, now func() time.Time) *AttachmentPolicyResolver {
	defaults = config.ApplyAttachmentPolicyDefaults(defaults, defaults.ImageMaxMB)
	if refreshTTL <= 0 {
		refreshTTL = defaultAttachmentPolicyRefreshTTL
	}
	if now == nil {
		now = time.Now
	}
	resolver := &AttachmentPolicyResolver{
		defaults:   cloneAttachmentPolicyConfig(defaults),
		source:     source,
		refreshTTL: refreshTTL,
		now:        now,
	}
	if invalidationSource, ok := source.(attachmentPolicyInvalidationSource); ok {
		invalidationSource.RegisterInvalidationListener(func(tabKey string) {
			if tabKey == AttachmentPolicyTabKey {
				resolver.Invalidate()
			}
		})
	}
	return resolver
}

func (r *AttachmentPolicyResolver) Resolve(ctx context.Context) (AttachmentPolicy, error) {
	now := r.now()
	r.mu.RLock()
	if r.cached != nil && (r.source == nil || now.Before(r.refreshAfter)) {
		policy := cloneAttachmentPolicy(*r.cached)
		r.mu.RUnlock()
		return policy, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	now = r.now()
	if r.cached != nil && (r.source == nil || now.Before(r.refreshAfter)) {
		return cloneAttachmentPolicy(*r.cached), nil
	}

	resolved := cloneAttachmentPolicyConfig(r.defaults)
	resolvedVersion := r.version
	if r.source != nil {
		tab, err := r.source.GetTab(ctx, AttachmentPolicyTabKey)
		if err != nil {
			r.refreshAfter = now.Add(r.refreshTTL)
			if r.cached != nil {
				return cloneAttachmentPolicy(*r.cached), nil
			}
			return AttachmentPolicy{}, fmt.Errorf("load attachment policy: %w", err)
		}
		if r.cached != nil && tab.Version == r.version {
			r.refreshAfter = now.Add(r.refreshTTL)
			return cloneAttachmentPolicy(*r.cached), nil
		}
		if err := applyAttachmentPolicyItems(&resolved, tab.Items); err != nil {
			r.refreshAfter = now.Add(r.refreshTTL)
			if r.cached != nil {
				return cloneAttachmentPolicy(*r.cached), nil
			}
			return AttachmentPolicy{}, err
		}
		resolvedVersion = tab.Version
	}
	policy, err := buildAttachmentPolicy(resolved)
	if err != nil {
		r.refreshAfter = now.Add(r.refreshTTL)
		if r.cached != nil {
			return cloneAttachmentPolicy(*r.cached), nil
		}
		return AttachmentPolicy{}, err
	}
	r.cached = &policy
	r.version = resolvedVersion
	r.refreshAfter = now.Add(r.refreshTTL)
	return cloneAttachmentPolicy(policy), nil
}

func (r *AttachmentPolicyResolver) Invalidate() {
	r.mu.Lock()
	r.refreshAfter = time.Time{}
	r.mu.Unlock()
}

func ValidateAttachmentPolicyItems(items []domainadminconfig.Item) error {
	configValue := config.AttachmentPolicyConfig{
		ImageMaxMB: 20, VideoMaxMB: 100, AudioMaxMB: 50, DocumentMaxMB: 20,
		ImageAllowedFormats: []string{"png", "jpeg", "webp", "gif"},
		VideoAllowedFormats: []string{"mp4"}, AudioAllowedFormats: []string{"mp3"}, DocumentAllowedFormats: []string{"pdf"},
	}
	if err := applyAttachmentPolicyItems(&configValue, items); err != nil {
		return err
	}
	_, err := buildAttachmentPolicy(configValue)
	return err
}

func applyAttachmentPolicyItems(target *config.AttachmentPolicyConfig, items []domainadminconfig.Item) error {
	for _, item := range items {
		value := item.ConfigValue["value"]
		switch item.ConfigKey {
		case AttachmentImageMaxMBKey:
			parsed, err := attachmentSizeMB(value, MaxImageAttachmentSizeMB)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.ImageMaxMB = parsed
		case AttachmentVideoMaxMBKey:
			parsed, err := attachmentSizeMB(value, maxConfiguredAttachmentSizeMB)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.VideoMaxMB = parsed
		case AttachmentAudioMaxMBKey:
			parsed, err := attachmentSizeMB(value, maxConfiguredAttachmentSizeMB)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.AudioMaxMB = parsed
		case AttachmentDocumentMaxMBKey:
			parsed, err := attachmentSizeMB(value, maxConfiguredAttachmentSizeMB)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.DocumentMaxMB = parsed
		case AttachmentImageAllowedFormatsKey:
			parsed, err := attachmentFormats(value)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.ImageAllowedFormats = parsed
		case AttachmentVideoAllowedFormatsKey:
			parsed, err := attachmentFormats(value)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.VideoAllowedFormats = parsed
		case AttachmentAudioAllowedFormatsKey:
			parsed, err := attachmentFormats(value)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.AudioAllowedFormats = parsed
		case AttachmentDocumentAllowedFormatsKey:
			parsed, err := attachmentFormats(value)
			if err != nil {
				return fmt.Errorf("%s: %w", item.ConfigKey, err)
			}
			target.DocumentAllowedFormats = parsed
		}
	}
	return nil
}

func buildAttachmentPolicy(value config.AttachmentPolicyConfig) (AttachmentPolicy, error) {
	imageFormats, imageMIMEs, err := normalizeImageFormats(value.ImageAllowedFormats)
	if err != nil {
		return AttachmentPolicy{}, err
	}
	videoFormats, err := normalizeReservedFormats(value.VideoAllowedFormats)
	if err != nil {
		return AttachmentPolicy{}, fmt.Errorf("video formats: %w", err)
	}
	audioFormats, err := normalizeReservedFormats(value.AudioAllowedFormats)
	if err != nil {
		return AttachmentPolicy{}, fmt.Errorf("audio formats: %w", err)
	}
	documentFormats, err := normalizeReservedFormats(value.DocumentAllowedFormats)
	if err != nil {
		return AttachmentPolicy{}, fmt.Errorf("document formats: %w", err)
	}
	if value.ImageMaxMB <= 0 || value.ImageMaxMB > MaxImageAttachmentSizeMB {
		return AttachmentPolicy{}, fmt.Errorf("image attachment size must be between 1 and %d MB", MaxImageAttachmentSizeMB)
	}
	for name, size := range map[string]int{"video": value.VideoMaxMB, "audio": value.AudioMaxMB, "document": value.DocumentMaxMB} {
		if size <= 0 || size > maxConfiguredAttachmentSizeMB {
			return AttachmentPolicy{}, fmt.Errorf("%s attachment size must be between 1 and %d MB", name, maxConfiguredAttachmentSizeMB)
		}
	}
	return AttachmentPolicy{
		Image:    filePolicy(value.ImageMaxMB, imageFormats, imageMIMEs),
		Video:    filePolicy(value.VideoMaxMB, videoFormats, nil),
		Audio:    filePolicy(value.AudioMaxMB, audioFormats, nil),
		Document: filePolicy(value.DocumentMaxMB, documentFormats, nil),
	}, nil
}

func normalizeImageFormats(values []string) ([]string, []string, error) {
	formats := make([]string, 0, len(values))
	mimes := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		canonical := strings.ToLower(strings.TrimSpace(value))
		canonical = strings.TrimPrefix(canonical, ".")
		switch canonical {
		case "png", "image/png":
			canonical = "png"
		case "jpg", "jpeg", "image/jpg", "image/jpeg":
			canonical = "jpeg"
		case "webp", "image/webp":
			canonical = "webp"
		case "gif", "image/gif":
			canonical = "gif"
		default:
			return nil, nil, fmt.Errorf("unsupported image format %q", value)
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		formats = append(formats, canonical)
		switch canonical {
		case "png":
			mimes = append(mimes, "image/png")
		case "jpeg":
			mimes = append(mimes, "image/jpeg")
		case "webp":
			mimes = append(mimes, "image/webp")
		case "gif":
			mimes = append(mimes, "image/gif")
		}
	}
	if len(formats) == 0 {
		return nil, nil, fmt.Errorf("at least one image format is required")
	}
	return formats, mimes, nil
}

func normalizeReservedFormats(values []string) ([]string, error) {
	formats := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		canonical := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, ".")))
		if canonical == "" {
			return nil, fmt.Errorf("empty format is not allowed")
		}
		if _, exists := seen[canonical]; exists {
			continue
		}
		seen[canonical] = struct{}{}
		formats = append(formats, canonical)
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("at least one format is required")
	}
	return formats, nil
}

func attachmentSizeMB(value any, maxMB int) (int, error) {
	var parsed int64
	switch typed := value.(type) {
	case int:
		parsed = int64(typed)
	case int8:
		parsed = int64(typed)
	case int16:
		parsed = int64(typed)
	case int32:
		parsed = int64(typed)
	case int64:
		parsed = typed
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, fmt.Errorf("size is out of range")
		}
		parsed = int64(typed)
	case uint32:
		parsed = int64(typed)
	case uint64:
		if typed > math.MaxInt64 {
			return 0, fmt.Errorf("size is out of range")
		}
		parsed = int64(typed)
	case float64:
		if math.Trunc(typed) != typed {
			return 0, fmt.Errorf("size must be a whole number")
		}
		parsed = int64(typed)
	case string:
		value, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("size must be a whole number")
		}
		parsed = value
	default:
		return 0, fmt.Errorf("size must be a whole number")
	}
	if parsed <= 0 || parsed > int64(maxMB) {
		return 0, fmt.Errorf("size must be between 1 and %d MB", maxMB)
	}
	return int(parsed), nil
}

func attachmentFormats(value any) ([]string, error) {
	var values []string
	switch typed := value.(type) {
	case string:
		values = strings.Split(typed, ",")
	case []string:
		values = append(values, typed...)
	case []any:
		for _, raw := range typed {
			text, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("formats must contain strings")
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("formats must be a list or comma-separated string")
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one format is required")
	}
	return values, nil
}

func filePolicy(maxMB int, formats, mimes []string) FilePolicy {
	return FilePolicy{MaxMB: maxMB, MaxBytes: int64(maxMB) * 1024 * 1024, AllowedFormats: append([]string(nil), formats...), AllowedMIMETypes: append([]string(nil), mimes...)}
}

func cloneAttachmentPolicyConfig(value config.AttachmentPolicyConfig) config.AttachmentPolicyConfig {
	value.ImageAllowedFormats = append([]string(nil), value.ImageAllowedFormats...)
	value.VideoAllowedFormats = append([]string(nil), value.VideoAllowedFormats...)
	value.AudioAllowedFormats = append([]string(nil), value.AudioAllowedFormats...)
	value.DocumentAllowedFormats = append([]string(nil), value.DocumentAllowedFormats...)
	return value
}

func cloneAttachmentPolicy(value AttachmentPolicy) AttachmentPolicy {
	value.Image = filePolicy(value.Image.MaxMB, value.Image.AllowedFormats, value.Image.AllowedMIMETypes)
	value.Video = filePolicy(value.Video.MaxMB, value.Video.AllowedFormats, value.Video.AllowedMIMETypes)
	value.Audio = filePolicy(value.Audio.MaxMB, value.Audio.AllowedFormats, value.Audio.AllowedMIMETypes)
	value.Document = filePolicy(value.Document.MaxMB, value.Document.AllowedFormats, value.Document.AllowedMIMETypes)
	return value
}
