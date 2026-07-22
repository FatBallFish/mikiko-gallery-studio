//go:build !windows

package deployctl

import "os"

func secureInstallDirectory(path string) error {
	return os.Chmod(path, 0o700)
}

func secureInstallFile(_ string, file *os.File) error {
	return file.Chmod(0o600)
}
