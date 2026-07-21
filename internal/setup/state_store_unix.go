//go:build !windows

package setup

import (
	"fmt"
	"os"
)

func secureStateDirectory(path string) error {
	if err := os.Chmod(path, 0o700); err != nil {
		return fmt.Errorf("set mode 0700: %w", err)
	}
	return nil
}

func secureStateFile(_ string, file *os.File) error {
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("set mode 0600: %w", err)
	}
	return nil
}

func replaceStateFile(source, destination string) error {
	if err := os.Rename(source, destination); err != nil {
		return fmt.Errorf("rename temporary file: %w", err)
	}
	return nil
}

func syncStateDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("fsync directory: %w", err)
	}
	return nil
}
