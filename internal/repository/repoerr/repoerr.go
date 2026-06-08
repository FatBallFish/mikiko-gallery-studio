package repoerr

import (
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("repository: not found")
var ErrConflict = errors.New("repository: conflict")
var ErrTransientContention = errors.New("repository: transient contention")

func TransientContention(err error) error {
	if err == nil {
		return ErrTransientContention
	}
	return fmt.Errorf("%w: %w", ErrTransientContention, err)
}

func IsTransientContention(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrTransientContention) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "could not serialize access") ||
		strings.Contains(message, "serialization failure") ||
		strings.Contains(message, "deadlock detected") ||
		strings.Contains(message, "sqlite_busy") ||
		strings.Contains(message, "sqlite_locked") ||
		strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked")
}
