//go:build !windows

package mgsctl

import (
	"fmt"
	"os"
)

func dockerRuntimeUser() string {
	return fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
}
