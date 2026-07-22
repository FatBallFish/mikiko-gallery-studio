package deployctl

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const installLockTimeout = 5 * time.Second

func acquireInstallLock(ctx context.Context, path string) (func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := secureInstallDirectory(filepath.Dir(path)); err != nil {
		return nil, err
	}
	file, err := openInstallLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := secureInstallFile(path, file, 0o600); err != nil {
		_ = file.Close()
		return nil, err
	}
	deadline := time.Now().Add(installLockTimeout)
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		locked, err := tryLockInstallFile(file)
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		if locked {
			return func() error {
				return errors.Join(unlockInstallFile(file), file.Close())
			}, nil
		}
		if !time.Now().Before(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("another deployctl install is still running")
		}
		timer := time.NewTimer(min(25*time.Millisecond, time.Until(deadline)))
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
