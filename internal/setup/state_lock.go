package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type stateFileLock struct {
	file *os.File
}

func acquireStateFileLock(path string, timeout time.Duration, operations stateAtomicOps) (*stateFileLock, error) {
	if timeout <= 0 {
		return nil, fmt.Errorf("state file lock timeout must be positive")
	}
	if err := operations.validate(); err != nil {
		return nil, err
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create state lock directory %q: %w", directory, err)
	}
	if err := operations.secureDirectory(directory); err != nil {
		return nil, fmt.Errorf("secure state lock directory %q: %w", directory, err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock file %q: %w", path, err)
	}
	if err := operations.secureFile(path, file); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure state lock file %q: %w", path, err)
	}

	deadline := time.Now().Add(timeout)
	for {
		locked, err := tryLockStateFile(file)
		if err != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock state file %q: %w", path, err)
		}
		if locked {
			return &stateFileLock{file: file}, nil
		}
		if !time.Now().Before(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("%w: %q", ErrInstallStateLockTimeout, path)
		}
		time.Sleep(min(10*time.Millisecond, time.Until(deadline)))
	}
}

func (lock *stateFileLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unlockStateFile(lock.file)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
}
