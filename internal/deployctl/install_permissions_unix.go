//go:build !windows

package deployctl

import "os"

func secureInstallDirectory(path string) error {
	return os.Chmod(path, 0o700)
}

func secureInstallFile(_ string, file *os.File, mode os.FileMode) error {
	return file.Chmod(mode)
}
