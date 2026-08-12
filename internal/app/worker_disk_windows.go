//go:build windows

package app

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func diskUsage(path string) (int, int64, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, 0, err
	}
	pointer, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return 0, 0, err
	}
	var available, total, free uint64
	err = windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free)
	if err != nil {
		return 0, 0, err
	}
	if total == 0 {
		return 100, int64(available), nil
	}
	used := total - available
	return int((used*100 + total - 1) / total), int64(available), nil
}
