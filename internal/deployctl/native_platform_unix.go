//go:build !windows

package deployctl

import (
	"fmt"
	"os"
	"runtime"
)

func currentNativePlatform() NativePlatform {
	return NativePlatform(runtime.GOOS)
}

func checkNativePrivileges(platform NativePlatform) error {
	if platform != NativePlatformLinux {
		return fmt.Errorf("production native installation supports Linux and Windows only")
	}
	if os.Geteuid() != 0 {
		return fmt.Errorf("Linux native service installation requires root; rerun deployctl with sudo")
	}
	return nil
}

func replaceNativeFile(source, destination string) error {
	return os.Rename(source, destination)
}
