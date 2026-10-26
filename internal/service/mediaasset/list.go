package mediaasset

import (
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"strings"
)

// NormalizeVisibilityStatus maps image and video lifecycle names to list filters.
func NormalizeVisibilityStatus(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "all":
		return "", nil
	case "private":
		return "private", nil
	case "pending", "pending_review", "reviewing":
		return "pending", nil
	case "public", "approved":
		return "public", nil
	case "rejected":
		return "rejected", nil
	case "offline", "unpublished":
		return "offline", nil
	default:
		return "", errs.BadRequest("invalid visibility_status")
	}
}
