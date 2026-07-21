package setup

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

const defaultApplyLockTimeout = 30 * time.Second

func newRuntimeApplyLocker(runtimeEnvPath string) applyLocker {
	lockPath, pathErr := normalizeStatePath(runtimeEnvPath + ".setup.lock")
	processLock := processLockForPath(normalizeProcessLockKey(lockPath, runtime.GOOS == "windows"))
	return func(ctx context.Context) (applyUnlock, error) {
		if pathErr != nil {
			return nil, fmt.Errorf("normalize setup apply lock path: %w", pathErr)
		}
		if ctx == nil {
			ctx = context.Background()
		}
		deadline := time.Now().Add(defaultApplyLockTimeout)
		for !processLock.TryLock() {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !time.Now().Before(deadline) {
				return nil, ErrInstallStateLockTimeout
			}
			timer := time.NewTimer(min(10*time.Millisecond, time.Until(deadline)))
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		fileLock, err := acquireStateFileLockUntil(ctx, lockPath, deadline, platformStateAtomicOps())
		if err != nil {
			processLock.Unlock()
			return nil, err
		}
		return func() error {
			fileErr := fileLock.release()
			processLock.Unlock()
			return fileErr
		}, nil
	}
}

func noOpApplyLocker(context.Context) (applyUnlock, error) {
	return func() error { return nil }, nil
}
