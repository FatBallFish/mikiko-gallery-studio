package adminconfig

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
)

func validatePaymentItems(items []domainadminconfig.Item) error {
	for _, item := range items {
		if item.ConfigKey != "order_timeout_seconds" {
			continue
		}
		value, err := configWholeNumber(item.ConfigValue["value"])
		if err != nil {
			return fmt.Errorf("order_timeout_seconds %w", err)
		}
		if value < 60 || value > 86400 {
			return fmt.Errorf("order_timeout_seconds must be between 60 and 86400 seconds")
		}
	}
	return nil
}

func configWholeNumber(value any) (int64, error) {
	switch typed := value.(type) {
	case int:
		return int64(typed), nil
	case int32:
		return int64(typed), nil
	case int64:
		return typed, nil
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, fmt.Errorf("must be a whole number")
		}
		return int64(typed), nil
	case uint32:
		return int64(typed), nil
	case uint64:
		if typed > math.MaxInt64 {
			return 0, fmt.Errorf("must be a whole number")
		}
		return int64(typed), nil
	case float64:
		if math.Trunc(typed) != typed || typed > math.MaxInt64 || typed < math.MinInt64 {
			return 0, fmt.Errorf("must be a whole number")
		}
		return int64(typed), nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("must be a whole number")
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("must be a whole number")
	}
}
