package adminconfig

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
)

func validateAttachmentPolicyItems(items []domainadminconfig.Item) error {
	for _, item := range items {
		value := item.ConfigValue["value"]
		switch item.ConfigKey {
		case "image_max_mb":
			if err := validateAttachmentSize(value, config.MaxImageAttachmentSizeMB); err != nil {
				return fmt.Errorf("%s %w", item.ConfigKey, err)
			}
		case "video_max_mb", "audio_max_mb", "document_max_mb":
			if err := validateAttachmentSize(value, 10240); err != nil {
				return fmt.Errorf("%s %w", item.ConfigKey, err)
			}
		case "image_allowed_formats":
			formats, err := configFormatValues(value)
			if err != nil {
				return fmt.Errorf("%s %w", item.ConfigKey, err)
			}
			for _, format := range formats {
				normalized := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(format, ".")))
				switch normalized {
				case "png", "image/png", "jpg", "jpeg", "image/jpg", "image/jpeg", "webp", "image/webp", "gif", "image/gif":
				default:
					return fmt.Errorf("image_allowed_formats contains unsupported format %q", format)
				}
			}
		case "video_allowed_formats", "audio_allowed_formats", "document_allowed_formats":
			if _, err := configFormatValues(value); err != nil {
				return fmt.Errorf("%s %w", item.ConfigKey, err)
			}
		}
	}
	return nil
}

func validateAttachmentSize(value any, maxMB int64) error {
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
			return fmt.Errorf("must be between 1 and %d MB", maxMB)
		}
		parsed = int64(typed)
	case uint32:
		parsed = int64(typed)
	case uint64:
		if typed > math.MaxInt64 {
			return fmt.Errorf("must be between 1 and %d MB", maxMB)
		}
		parsed = int64(typed)
	case float64:
		if math.Trunc(typed) != typed {
			return fmt.Errorf("must be a whole number")
		}
		parsed = int64(typed)
	case string:
		value, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return fmt.Errorf("must be a whole number")
		}
		parsed = value
	default:
		return fmt.Errorf("must be a whole number")
	}
	if parsed <= 0 || parsed > maxMB {
		return fmt.Errorf("must be between 1 and %d MB", maxMB)
	}
	return nil
}

func configFormatValues(value any) ([]string, error) {
	var formats []string
	switch typed := value.(type) {
	case string:
		formats = strings.Split(typed, ",")
	case []string:
		formats = append(formats, typed...)
	case []any:
		for _, raw := range typed {
			format, ok := raw.(string)
			if !ok {
				return nil, fmt.Errorf("must contain strings")
			}
			formats = append(formats, format)
		}
	default:
		return nil, fmt.Errorf("must be a list or comma-separated string")
	}
	if len(formats) == 0 {
		return nil, fmt.Errorf("must contain at least one format")
	}
	for _, format := range formats {
		if strings.TrimSpace(format) == "" {
			return nil, fmt.Errorf("must not contain empty formats")
		}
	}
	return formats, nil
}
