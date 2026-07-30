//go:build windows

package mgsctl

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func currentNativePlatform() NativePlatform { return NativePlatformWindows }

func checkNativePrivileges(platform NativePlatform) error {
	if platform != NativePlatformWindows {
		return fmt.Errorf("production native installation supports Linux and Windows only")
	}
	if !windows.GetCurrentProcessToken().IsElevated() {
		return fmt.Errorf("Windows native service installation requires an elevated Administrator terminal")
	}
	return nil
}

func replaceNativeFile(source, destination string) error {
	sourcePointer, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return fmt.Errorf("encode native metadata source path: %w", err)
	}
	destinationPointer, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return fmt.Errorf("encode native metadata destination path: %w", err)
	}
	flags := uint32(windows.MOVEFILE_REPLACE_EXISTING | windows.MOVEFILE_WRITE_THROUGH)
	if err := windows.MoveFileEx(sourcePointer, destinationPointer, flags); err != nil {
		return fmt.Errorf("replace native metadata file: %w", err)
	}
	return nil
}
