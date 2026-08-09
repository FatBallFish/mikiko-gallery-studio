package objectcleanup

import (
	"strings"
	"time"
)

const (
	StatePending = "pending"
	StateRunning = "running"
	StateRetry   = "retry"
	StateDone    = "done"
	StateBlocked = "blocked"
)

type Identity struct {
	StorageConfigID string
	StorageDriver   string
	Bucket          string
	ObjectKey       string
}

// CanonicalIdentity keeps backend metadata available for routing while making
// the configured storage namespace independent from mutable driver/bucket data.
func CanonicalIdentity(value Identity) Identity {
	value.StorageConfigID = strings.TrimSpace(value.StorageConfigID)
	value.StorageDriver = strings.ToLower(strings.TrimSpace(value.StorageDriver))
	value.Bucket = strings.TrimSpace(value.Bucket)
	value.ObjectKey = strings.TrimSpace(value.ObjectKey)
	if value.StorageConfigID != "" {
		value.Bucket = ""
	}
	return value
}

func CanonicalKey(value Identity) string {
	value = CanonicalIdentity(value)
	if value.StorageConfigID != "" {
		return strings.Join([]string{"config", value.StorageConfigID, value.ObjectKey}, "\x00")
	}
	return strings.Join([]string{"legacy", value.StorageDriver, value.Bucket, value.ObjectKey}, "\x00")
}

type Job struct {
	ID               string
	Identity         Identity
	State            string
	AttemptCount     int
	NextAttemptAt    *time.Time
	LastErrorCode    string
	LastErrorMessage string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}
