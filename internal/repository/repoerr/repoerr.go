package repoerr

import (
	"errors"
	"fmt"
	"strings"
)

var ErrNotFound = errors.New("repository: not found")
var ErrConflict = errors.New("repository: conflict")
var ErrConfigurationInUse = errors.New("repository: configuration in use")
var ErrDefaultModelRequired = errors.New("repository: text model default required")
var ErrTransientContention = errors.New("repository: transient contention")

type configurationInUseError struct {
	dependency string
	count      int
}

func (e *configurationInUseError) Error() string {
	return fmt.Sprintf("%s: %s (%d)", ErrConfigurationInUse, e.dependency, e.count)
}

func (e *configurationInUseError) Unwrap() error { return ErrConfigurationInUse }

func ConfigurationInUse(dependency string, count int) error {
	return &configurationInUseError{dependency: strings.TrimSpace(dependency), count: count}
}

func ConfigurationInUseDetails(err error) (dependency string, count int, ok bool) {
	var target *configurationInUseError
	if !errors.As(err, &target) {
		return "", 0, false
	}
	return target.dependency, target.count, true
}

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
