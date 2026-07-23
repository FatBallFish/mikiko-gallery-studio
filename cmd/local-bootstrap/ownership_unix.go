//go:build unix

package main

import (
	"fmt"
	"os"
	"syscall"
)

func restoreLocalFileOwnership(referencePath, targetPath string) error {
	info, err := os.Stat(referencePath)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("read local runtime ownership metadata")
	}
	return os.Chown(targetPath, int(stat.Uid), int(stat.Gid))
}
