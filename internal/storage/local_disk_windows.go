//go:build windows

package storage

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func localAvailableDiskBytes(path string) (int64, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return 0, err
	}
	pointer, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return 0, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(pointer, &available, &total, &free); err != nil {
		return 0, err
	}
	return int64(available), nil
}
