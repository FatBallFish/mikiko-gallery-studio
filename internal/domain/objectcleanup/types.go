package objectcleanup

import "time"

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
